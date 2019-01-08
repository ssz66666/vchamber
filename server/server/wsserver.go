package server

import (
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/rs/xid"
)

const (
	WebsocketSubprotocolMagicV1 = "vchamber_v1"
	ErrInvalidRoomID            = "Error: Invalid Room ID"
	ErrInvalidToken             = "Error: Invalid token"
)

const (
	wsReadBufferSize     = 1024
	wsWriteBufferSize    = 1024
	roomMessageQueueSize = 256
	clientSendQueueSize  = 32
	clientRecvQueueSize  = 32
	keyLength            = 32
	doCheckSubprotocol   = true
)

const (
	heartbeatTimeout = 30 * time.Second
	// heartbeatPeriod  = heartbeatTimeOut * 9 / 10
	broadcastPeriod          = 5 * time.Second
	writeWait                = 10 * time.Second
	defaultMasterlessTimeout = 5 * time.Minute
	updateCooldown           = 1 * time.Second
)

// Server encapsulates server-level global data
type Server struct {
	rooms   map[string]*Room // a map of rooms
	enqRoom chan *Room
	deqRoom chan *Room
	mutex   sync.RWMutex // guard rooms for look up
}

// Room encapsulates room-level global data and manages users in a room
type Room struct {
	ID        string
	clients   map[string]*ClientConn // a map with id:client kv pairs
	masters   map[string]*ClientConn
	recvQueue chan *Message // deserialise early in parallel in separate goroutines
	enqClient chan *ClientConn
	deqClient chan *ClientConn
	masterKey string
	guestKey  string
	state     *PlaybackState
	server    *Server
}

type clientState int

const (
	clientStateUnauthorised clientState = iota
	clientStateGuest
	clientStateMaster
)

// ClientConn encapsulates an established client websocket connection
type ClientConn struct {
	ID        string
	conn      *websocket.Conn
	recvQueue chan *Message
	sendQueue chan *Message
	closing   chan bool
	state     clientState
	room      *Room
}

var wsUpgrader = websocket.Upgrader{
	ReadBufferSize:  wsReadBufferSize,
	WriteBufferSize: wsWriteBufferSize,
	Subprotocols: []string{
		WebsocketSubprotocolMagicV1,
	},
	CheckOrigin: func(r *http.Request) bool {
		return true
	}, //disable origin check
}

// NewServer creates a new server struct
func NewServer() *Server {
	return &Server{
		make(map[string]*Room),
		make(chan *Room),
		make(chan *Room),
		sync.RWMutex{},
	}
}

func (s *Server) AddRoom(r *Room) {
	s.enqRoom <- r
}

func (s *Server) RemoveRoom(r *Room) {
	s.deqRoom <- r
}

// Run manages server s
func (s *Server) Run() {
	for {
		select {
		case r := <-s.enqRoom:
			if nil != r {
				s.mutex.Lock()
				s.rooms[r.ID] = r
				s.mutex.Unlock()
				go r.RunManager()
				log.Printf("room %s registered", r.ID)
			}
		case r := <-s.deqRoom:
			if nil != r {
				s.mutex.Lock()
				if _r, ok := s.rooms[r.ID]; ok && _r == r {
					delete(s.rooms, r.ID)
					close(r.recvQueue)
					close(r.enqClient)
					close(r.deqClient)
				}
				s.mutex.Unlock()
				log.Printf("room %s deregistered", r.ID)
			}
		}
	}
}

func (r *Room) checkPosition() {
	st := r.state
	newPos := st.position
	if st.status == PlaybackStatusPlaying {
		newPos += time.Since(st.lastUpdated).Seconds() * st.speed
	}
	if newPos >= st.duration {
		st.position = st.duration
		st.status = PlaybackStatusStopped
		st.lastUpdated = time.Now()
	}
}

func (r *Room) UpdateState(p *PlaybackStateUpdateMessage) {
	r.state.source = p.State.Source
	r.state.status = p.State.Status
	r.state.speed = p.State.Speed
	r.state.duration = p.State.Duration

	newPos := p.State.Position + (p.RTT/2.0)*p.State.Speed
	r.state.position = newPos
	r.state.lastUpdated = time.Now()
}

func (r *Room) GetCurrentStateMessage() *Message {
	r.checkPosition()
	st := r.state
	newPos := st.position
	if st.status == PlaybackStatusPlaying {
		newPos += time.Since(st.lastUpdated).Seconds() * st.speed
	}
	return &Message{
		Type: MessageTypeStateBroadcast,
		Payload: &PlaybackStateMessage{
			Source:   st.source,
			Status:   st.status,
			Position: newPos,
			Speed:    st.speed,
			Duration: st.duration,
		},
	}
}

// BroadcastState broadcasts a room's state to all clients in the room, NOT thread-safe
func (r *Room) BroadcastState() {
	m := r.GetCurrentStateMessage()
	for _, c := range r.clients {
		c.sendQueue <- m
	}
}

func (r *Room) SendState(cid string) {
	m := r.GetCurrentStateMessage()
	if c, ok := r.clients[cid]; ok {
		c.sendQueue <- m
	}
}

func (r *Room) AddClient(c *ClientConn) {
	if nil != c {
		r.clients[c.ID] = c
		if c.state == clientStateMaster {
			r.masters[c.ID] = c
		}
	}
}

// RemoveClient removes a client from room r, NOT thread-safe
func (r *Room) RemoveClient(c *ClientConn) {
	if nil != c {
		if _c, ok := r.clients[c.ID]; ok && (_c == c) {
			log.Println("removing client", c.conn.RemoteAddr(), "cid:", c.ID)
			delete(r.clients, c.ID)
			delete(r.masters, c.ID)
			close(c.sendQueue)
			close(c.recvQueue)
			close(c.closing)
		}
	}
}

// RunManager manages room r
func (r *Room) RunManager() {
	// TODO: start a timer to shut down itself after being master-less for a while

	shutdownTimer := time.NewTimer(defaultMasterlessTimeout)
	updateTicker := time.NewTicker(broadcastPeriod)
	defer func() {
		updateTicker.Stop()
		shutdownTimer.Stop()
		for _, c := range r.clients {
			r.RemoveClient(c)
		}
		r.server.deqRoom <- r
	}()
	for {
		select {
		case m := <-r.recvQueue:
			switch m.Type {
			case MessageTypeStateUpdate:
				// TODO: we need to somehow handle conflicting state updates
				// TODO: when we have duration we can then make the video stop as it ends
				p := m.Payload.(*PlaybackStateUpdateMessage)
				if time.Since(m.ReceivedAt) > updateCooldown {
					log.Printf("received state update from %s, new state %v", m.Sender, p.State)
					r.UpdateState(p)
					r.BroadcastState()
				}
				log.Printf("ignored state update from %s, proposed new state %v", m.Sender, p.State)
			}

		case c := <-r.enqClient:
			r.AddClient(c)
			r.SendState(c.ID)
			if c.state == clientStateMaster && len(r.masters) == 1 {
				if !shutdownTimer.Stop() {
					<-shutdownTimer.C
				}
			}
		case c := <-r.deqClient:
			r.RemoveClient(c)
			if c.state == clientStateMaster && len(r.masters) == 0 {
				if !shutdownTimer.Stop() {
					<-shutdownTimer.C
				}
				shutdownTimer.Reset(defaultMasterlessTimeout)
			}
		case <-updateTicker.C:
			r.BroadcastState()
		case <-shutdownTimer.C:
			return
		}

	}
}

// NewRoom creates a room with given id and server with no clients
func NewRoom(id string, server *Server, mKey string, gKey string) *Room {
	return &Room{
		ID:        id,
		clients:   make(map[string]*ClientConn),
		masters:   make(map[string]*ClientConn),
		recvQueue: make(chan *Message, roomMessageQueueSize),
		enqClient: make(chan *ClientConn),
		deqClient: make(chan *ClientConn),
		masterKey: mKey,
		guestKey:  gKey,
		state: &PlaybackState{
			source:      "",
			status:      PlaybackStatusStopped,
			position:    0.0,
			speed:       1.0,
			lastUpdated: time.Now(),
		},
		server: server,
	}
}

// NewRoomWithRandomKeys is /script>a helper function to create a new room with random keys
func NewRoomWithRandomKeys(id string, server *Server) (*Room, string, string, error) {
	mKey, e1 := GenerateKey(keyLength)
	gKey, e2 := GenerateKey(keyLength)
	if e1 != nil {
		return nil, "", "", e1
	}
	if e2 != nil {
		return nil, "", "", e2
	}
	return NewRoom(id, server, mKey, gKey), mKey, gKey, nil
}

// CheckMasterKey verifies key with the room's master key
func (r *Room) CheckMasterKey(key string) bool {
	return key == r.masterKey
}

// CheckGuestKey verifies key with the room's guest key
func (r *Room) CheckGuestKey(key string) bool {
	return key == r.guestKey
}

// NewClientConn creates a client websocket connection wrapper
func NewClientConn(id string, room *Room, conn *websocket.Conn, state clientState) *ClientConn {
	return &ClientConn{
		ID:        id,
		conn:      conn,
		recvQueue: make(chan *Message, clientRecvQueueSize),
		sendQueue: make(chan *Message, clientSendQueueSize),
		closing:   make(chan bool),
		state:     state,
		room:      room,
	}
}

// the goroutine that runs this function reads from c.conn
func (c *ClientConn) handleWSClientRecv() {
	defer func() {
		c.closing <- true
	}()
	// uncomment to remove client after irresponsive for heartbeatTimeOut
	// c.conn.SetReadDeadline(time.Now().Add(heartbeatTimeout))
	for {
		_, m, err := c.conn.ReadMessage()
		if nil != err {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("Error unexpected closure: %v", err)
			}
			return
		}
		// uncomment to remove client after irresponsive for heartbeatTimeOut
		// c.conn.SetReadDeadline(time.Now().Add(heartbeatTimeout))
		var msg Message
		err = Deserialise(m, &msg)
		if nil != err {
			log.Println("Invalid message:", string(m))
			continue
		}
		c.recvQueue <- &msg
	}
}

// the goroutine that runs this function writes to c.conn
func (c *ClientConn) handleWSClientSend() {
	defer func() {
		c.conn.Close()
	}()
	for {
		select {
		case msg, ok := <-c.sendQueue:
			if !ok {
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			if msg.Type == MessageTypePong {
				// compute the service time
				var p *PongMessage
				p = (msg.Payload.(*PongMessage))
				p.SvcTime = time.Since(msg.ReceivedAt).Seconds()
			}
			b, _ := msg.Serialise()
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			err := c.conn.WriteMessage(websocket.TextMessage, b)
			if err != nil {
				return
			}
		}
	}
}

// the goroutine that runs this function controls other mutable states in c
func (c *ClientConn) handleVChamberClient() {
	defer func() {
		c.room.deqClient <- c
	}()
	for {
		select {
		case m := <-c.recvQueue:

			// TODO: handle client specific part of the protocol
			// e.g. authentication
			m.Sender = c.ID

			switch m.Type {
			case MessageTypePing:
				var p *PingMessage
				p = m.Payload.(*PingMessage)

				var pong = Message{
					ReceivedAt: m.ReceivedAt,
					Type:       MessageTypePong,
					Payload: &PongMessage{
						Timestamp: p.Timestamp,
					},
				}
				c.sendQueue <- &pong

			case MessageTypeStateUpdate:
				if c.state == clientStateMaster {
					c.room.recvQueue <- m
				} else {
					// otherwise we silently drop it
					log.Println("non master attempted to change room state")
				}

			default:
				// silently drop the message
			}
		case <-c.closing:
			return
		}
	}
}

func handleWSClient(s *Server, w http.ResponseWriter, r *http.Request) {

	// parse query string and check if roomid is valid
	q := r.URL.Query()
	roomid := q.Get("rid")
	var room *Room

	if "" != roomid {
		s.mutex.RLock()
		rm, ok := s.rooms[roomid]
		if ok {
			room = rm
		}
		s.mutex.RUnlock()
	}

	if nil == room {
		log.Println("client", r.RemoteAddr, "Requested invalid room ID", roomid)
		http.Error(w, ErrInvalidRoomID, http.StatusBadRequest)
		return
	}

	// token check
	token := q.Get("token")
	cState := clientStateUnauthorised
	if room.CheckMasterKey(token) {
		cState = clientStateMaster
	} else if room.CheckGuestKey(token) {
		cState = clientStateGuest
	}

	if cState == clientStateUnauthorised {
		log.Println("client", r.RemoteAddr, "supplied invalid token", token)
		http.Error(w, ErrInvalidToken, http.StatusUnauthorized)
		return
	}

	conn, err := wsUpgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println(err)
		return
	}

	if doCheckSubprotocol && conn.Subprotocol() != WebsocketSubprotocolMagicV1 {
		conn.WriteMessage(websocket.CloseMessage, []byte("unsupported subprotocol version"))
		conn.Close()
		return
	}

	cid := xid.New().String()
	client := NewClientConn(cid, room, conn, cState)

	go client.handleVChamberClient()
	go client.handleWSClientSend()
	go client.handleWSClientRecv()

	cType := ""
	if cState == clientStateMaster {
		cType = "master"
	} else if cState == clientStateGuest {
		cType = "guest"
	}
	// send Hello message
	client.sendQueue <- &Message{
		Type: MessageTypeHello,
		Payload: &HelloMessage{
			ClientType: cType,
		}}
	room.enqClient <- client
	log.Printf("%s client %s from %s joined room %s", cType, cid, conn.RemoteAddr(), roomid)
}

// NewVChamberWSMux makes the websocket servemux of server
func NewVChamberWSMux(server *Server) http.Handler {
	wsMux := http.NewServeMux()

	wsMux.HandleFunc("/", http.NotFound)
	wsMux.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		handleWSClient(server, w, r)
	})
	return wsMux
}