package server

import (
	"encoding/json"
	"log"
	"math"
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
	HeartbeatTimeout = 30 * time.Second
	// heartbeatPeriod  = heartbeatTimeOut * 9 / 10
	BroadcastPeriod          = 5 * time.Second
	WriteWait                = 10 * time.Second
	DefaultMasterlessTimeout = 5 * time.Minute
	UpdateCooldown           = 1 * time.Second
)

// Server encapsulates server-level global data
type Server struct {
	rooms        map[string]*Room // a map of rooms
	enqRoom      chan *Room
	deqRoom      chan *Room
	muxes        map[string]*ConnMultiplexor
	sendQueue    chan *work
	recvQueue    chan *work
	closing      chan bool
	closingGuard sync.Once
	mutex        sync.RWMutex // guard rooms for look up
}

// Room encapsulates room-level global data and manages users in a room
type Room struct {
	ID        string
	clients   map[string]VCClientConn // a map with id:client kv pairs
	masters   map[string]VCClientConn
	recvQueue chan *Message // deserialise early in parallel in separate goroutines
	enqClient chan VCClientConn
	deqClient chan VCClientConn
	closing   chan bool
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

type VCClientConn interface {
	GetID() string
	SendMessage(*Message)
	GetState() clientState
	GetRemoteAddr() string
	Finalise() // run by room manager goroutine
}

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

func (c *ClientConn) GetID() string          { return c.ID }
func (c *ClientConn) GetRemoteAddr() string  { return c.conn.RemoteAddr().String() }
func (c *ClientConn) SendMessage(m *Message) { c.sendQueue <- m }
func (c *ClientConn) GetState() clientState  { return c.state }
func (c *ClientConn) Finalise() {
	close(c.closing)
	close(c.sendQueue)
}

var wsUpgrader = GetWSUpgrader()

// GetWSUpgrader return the websocket upgrader for use with vchamber
func GetWSUpgrader() *websocket.Upgrader {
	return &websocket.Upgrader{
		ReadBufferSize:  wsReadBufferSize,
		WriteBufferSize: wsWriteBufferSize,
		Subprotocols: []string{
			WebsocketSubprotocolMagicV1,
		},
		CheckOrigin: func(r *http.Request) bool {
			return true
		}, //disable origin check
	}
}

// NewServer creates a new server struct
func NewServer() *Server {
	return &Server{
		make(map[string]*Room),
		make(chan *Room),
		make(chan *Room),
		make(map[string]*ConnMultiplexor),
		make(chan *work),
		make(chan *work),
		make(chan bool),
		sync.Once{},
		sync.RWMutex{},
	}
}

func (s *Server) AddRoom(r *Room) {
	s.enqRoom <- r
}

func (s *Server) RemoveRoom(r *Room) {
	s.deqRoom <- r
}

func (s *Server) joinRoom(r *Room) {
	if nil != r {
		s.rooms[r.ID] = r
		go r.RunManager()
		log.Printf("room %s registered", r.ID)
	}
}

func (s *Server) killRoom(r *Room) {
	if nil != r {
		if _r, ok := s.rooms[r.ID]; ok && _r == r {
			delete(s.rooms, r.ID)
			close(r.closing)
			close(r.recvQueue)
			close(r.enqClient)
			close(r.deqClient)
		}
		log.Printf("room %s deregistered", r.ID)
	}
}

// Run manages server s
func (s *Server) Run() {
	// ncpu := runtime.NumCPU()
	ncpu := 128
	log.Printf("spawning %d worker goroutines for json serialisation\n", ncpu)
	RunWorkers(ncpu, s.sendQueue)
	RunWorkers(ncpu, s.recvQueue)
	defer func() {
		s.mutex.Lock()
		// kill all rooms
		for _, r := range s.rooms {
			s.killRoom(r)
		}
		s.mutex.Unlock()
		close(s.sendQueue)
		close(s.recvQueue)
	}()
	for {
		select {
		case r := <-s.enqRoom:
			s.mutex.Lock()
			s.joinRoom(r)
			s.mutex.Unlock()
		case r := <-s.deqRoom:
			s.mutex.Lock()
			s.killRoom(r)
			s.mutex.Unlock()
		case <-s.closing:
			return
		}
	}
}

func (r *Room) checkPosition() {
	st := r.state
	newPos := st.position
	if st.status == PlaybackStatusPlaying {
		newPos += time.Since(st.lastUpdated).Seconds() * st.speed
	}
	if newPos > st.duration {
		st.position = st.duration
		st.status = PlaybackStatusStopped
		st.lastUpdated = time.Now()
	}
}

func (r *Room) UpdateState(p *PlaybackStateUpdateMessage, d time.Duration) {
	r.state.source = p.State.Source
	r.state.status = p.State.Status
	r.state.speed = p.State.Speed
	r.state.duration = p.State.Duration

	newPos := p.State.Position
	if r.state.status == PlaybackStatusPlaying {
		newPos += (math.Max(p.RTT/2.0, 0.0) + d.Seconds()) * p.State.Speed
	}
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
		m.Sender = c.GetID()
		c.SendMessage(m)
	}
}

func (r *Room) SendState(cid string) {
	m := r.GetCurrentStateMessage()
	if c, ok := r.clients[cid]; ok {
		m.Sender = c.GetID()
		c.SendMessage(m)
	}
}

func (r *Room) joinClient(c VCClientConn) {
	if nil != c {
		r.clients[c.GetID()] = c
		if c.GetState() == clientStateMaster {
			r.masters[c.GetID()] = c
		}
	}
}

// killClient removes a client from room r, NOT thread-safe
func (r *Room) killClient(c VCClientConn) {
	if nil != c {
		if _c, ok := r.clients[c.GetID()]; ok && (_c == c) {
			log.Println("removing client", c.GetRemoteAddr(), "cid:", c.GetID())
			delete(r.clients, c.GetID())
			delete(r.masters, c.GetID())
			c.Finalise()
		}
	}
}

// RunManager manages room r
func (r *Room) RunManager() {

	shutdownTimer := time.NewTimer(DefaultMasterlessTimeout)
	updateTicker := time.NewTicker(BroadcastPeriod)
	var bufferedUpdate *Message
	updateCooldownTimer := time.NewTimer(UpdateCooldown)
	updateCooldownTimer.Stop()
	defer func() {
		updateTicker.Stop()
		shutdownTimer.Stop()
		updateCooldownTimer.Stop()
		for _, c := range r.clients {
			r.killClient(c)
		}
		r.server.deqRoom <- r
	}()
	for {
		select {
		case <-updateCooldownTimer.C:
			if bufferedUpdate != nil {
				m := bufferedUpdate
				r.UpdateState(m.Payload.(*PlaybackStateUpdateMessage), time.Since(m.ReceivedAt))
				r.BroadcastState()
			}
			bufferedUpdate = nil
		case m := <-r.recvQueue:
			switch m.Type {
			case MessageTypeStateUpdate:
				// TODO: we need to somehow handle conflicting state updates
				// TODO: when we have duration we can then make the video stop as it ends
				p := m.Payload.(*PlaybackStateUpdateMessage)
				if time.Since(r.state.lastUpdated) > UpdateCooldown {
					// log.Printf("received state update from %s, new state %v", m.Sender, p.State)
					r.UpdateState(p, time.Since(m.ReceivedAt))
					r.BroadcastState()
				} else {
					// buffer the update
					// timer has stopped
					if bufferedUpdate == nil {
						//start the timer
						updateCooldownTimer.Reset(9 * UpdateCooldown / 10)
					}
					bufferedUpdate = m
					log.Printf("buffered state update from %s, proposed new state %v", m.Sender, p.State)
				}
			}

		case c := <-r.enqClient:
			r.joinClient(c)
			r.SendState(c.GetID())
			if c.GetState() == clientStateMaster && len(r.masters) == 1 {
				if !shutdownTimer.Stop() {
					<-shutdownTimer.C
				}
			}
		case c := <-r.deqClient:
			r.killClient(c)
			if c.GetState() == clientStateMaster && len(r.masters) == 0 {
				shutdownTimer.Reset(DefaultMasterlessTimeout)
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
		clients:   make(map[string]VCClientConn),
		masters:   make(map[string]VCClientConn),
		recvQueue: make(chan *Message, roomMessageQueueSize),
		enqClient: make(chan VCClientConn),
		deqClient: make(chan VCClientConn),
		closing:   make(chan bool),
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
func (c *ClientConn) HandleWSClientRecv() {
	defer func() {
		close(c.recvQueue)
		c.room.deqClient <- c
	}()
	// uncomment to remove client after irresponsive for heartbeatTimeOut
	// c.conn.SetReadDeadline(time.Now().Add(heartbeatTimeout))
	for {
		select {
		case <-c.closing:
			return
		default:
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
			err = json.Unmarshal(m, &msg)
			if nil != err {
				log.Println("Invalid message:", string(m))
				continue
			}
			c.recvQueue <- &msg
		}
	}
}

// the goroutine that runs this function writes to c.conn
func (c *ClientConn) HandleWSClientSend() {
	defer func() {
		c.conn.Close()
		c.room.deqClient <- c
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
			b, _ := json.Marshal(msg)
			c.conn.SetWriteDeadline(time.Now().Add(WriteWait))
			err := c.conn.WriteMessage(websocket.TextMessage, b)
			if err != nil {
				return
			}
		case <-c.closing:
			return
		}
	}
}

// the goroutine that runs this function controls other mutable states in c
func (c *ClientConn) HandleVChamberClient() {
	defer func() {
		c.room.deqClient <- c
	}()
	for {
		select {
		case m, ok := <-c.recvQueue:
			if !ok {
				return
			}

			// TODO: handle client specific part of the protocol
			// e.g. authentication
			m.Sender = c.GetID()

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
				c.SendMessage(&pong)

			case MessageTypeStateUpdate:
				if c.GetState() == clientStateMaster {
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

type ErrClientConnect int

const (
	ErrClientConnectBadRoomID ErrClientConnect = iota
	ErrClientConnectBadToken
)

func (e ErrClientConnect) Error() string {
	switch e {
	case ErrClientConnectBadRoomID:
		return ErrInvalidRoomID
	case ErrClientConnectBadToken:
		return ErrInvalidToken
	default:
		return "Unknown connect error"
	}
}

func checkValidClient(s *Server, roomid string, token string) (*Room, clientState, error) {
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
		return nil, clientStateUnauthorised, ErrClientConnectBadRoomID
	}

	// token check
	cState := clientStateUnauthorised
	if room.CheckMasterKey(token) {
		cState = clientStateMaster
	} else if room.CheckGuestKey(token) {
		cState = clientStateGuest
	}

	if cState == clientStateUnauthorised {
		return nil, clientStateUnauthorised, ErrClientConnectBadToken
	}

	return room, cState, nil
}

func handleWSClient(s *Server, w http.ResponseWriter, r *http.Request) {

	// parse query string and check if roomid is valid
	q := r.URL.Query()
	roomid := q.Get("rid")
	token := q.Get("token")

	room, cState, err := checkValidClient(s, roomid, token)

	if err != nil {
		log.Printf("client from %v fails to connect: %v", r.RemoteAddr, err)
		http.Error(w, err.Error(), http.StatusUnauthorized)
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

	go client.HandleVChamberClient()
	go client.HandleWSClientSend()
	go client.HandleWSClientRecv()

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

// GetVChamberWSHandleFunc returns a handle function for the server
func GetVChamberWSHandleFunc(server *Server) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		handleWSClient(server, w, r)
	}
}
