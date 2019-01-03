package main

import (
	"flag"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/rs/xid"
)

const (
	websocketSubprotocolMagicV1 = "vchamber_v1"
	errInvalidRoomID            = "Error: Invalid Room ID"
)

const (
	wsReadBufferSize     = 1024
	wsWriteBufferSize    = 1024
	roomMessageQueueSize = 256
	clientSendQueueSize  = 32
	clientRecvQueueSize  = 32
)

const (
	heartbeatTimeOut = 30 * time.Second
	// heartbeatPeriod  = heartbeatTimeOut * 9 / 10
	broadcastPeriod = 5 * time.Second
	writeWait       = 10 * time.Second
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
	recvQueue chan *Message          // deserialise early in parallel in separate goroutines
	enqClient chan *ClientConn
	deqClient chan *ClientConn
	server    *Server
}

type clientState struct {
	s     clientInternalState
	mutex sync.RWMutex
}

type clientInternalState int

const (
	clientAwaitAuth clientInternalState = iota
	clientInRoomEstablished
	clientInRoomAwaitReAuth
)

// ClientConn encapsulates an established client websocket connection
type ClientConn struct {
	ID        string
	conn      *websocket.Conn
	recvQueue chan *Message
	sendQueue chan []byte
	closing   chan bool
	state     clientState
	room      *Room
}

var bindaddr = flag.String("addr", ":8080", "HTTP Service bind address")

var wsUpgrader = websocket.Upgrader{
	ReadBufferSize:  wsReadBufferSize,
	WriteBufferSize: wsWriteBufferSize,
	Subprotocols: []string{
		websocketSubprotocolMagicV1,
	},
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
			}
		}
	}
}

// RunManager manages room r
func (r *Room) RunManager() {
	updateTicker := time.NewTicker(broadcastPeriod)
	defer func() {
		updateTicker.Stop()
		r.server.deqRoom <- r
	}()
	for {
		select {
		case m := <-r.recvQueue:
			log.Printf("received message %v", m)
			msg, _ := m.Serialise()
			// rebroadcast the message
			for id, c := range r.clients {
				select {
				case c.sendQueue <- msg:
				default:
					delete(r.clients, id)
					close(c.sendQueue)
					close(c.recvQueue)
					close(c.closing)
				}
			}
		case c := <-r.enqClient:
			if nil != c {
				r.clients[c.ID] = c
			}
		case c := <-r.deqClient:
			if nil != c {
				if _c, ok := r.clients[c.ID]; ok && (_c == c) {
					log.Println("closing client", c.conn.RemoteAddr())
					log.Printf("cid: %s", c.ID)
					delete(r.clients, c.ID)
					close(c.sendQueue)
					close(c.recvQueue)
					close(c.closing)
				}
			}
		case <-updateTicker.C:
			m, _ := (&Message{"", "update message: " + time.Now().String()}).Serialise()
			for _, c := range r.clients {
				c.sendQueue <- m
			}
		}

	}
}

// NewRoom creates a room with given id and server with no clients
func NewRoom(id string, server *Server) *Room {
	return &Room{
		ID:        id,
		clients:   make(map[string]*ClientConn),
		recvQueue: make(chan *Message, roomMessageQueueSize),
		enqClient: make(chan *ClientConn),
		deqClient: make(chan *ClientConn),
		server:    server,
	}
}

// NewClientConn creates a client websocket connection wrapper
func NewClientConn(id string, room *Room, conn *websocket.Conn) *ClientConn {
	return &ClientConn{
		ID:        id,
		conn:      conn,
		recvQueue: make(chan *Message, clientRecvQueueSize),
		sendQueue: make(chan []byte, clientSendQueueSize),
		closing:   make(chan bool),
		state:     clientState{clientAwaitAuth, sync.RWMutex{}},
		room:      room,
	}
}

// the goroutine that runs this function reads from c.conn
func (c *ClientConn) handleWSClientRecv() {
	defer func() {
		c.closing <- true
	}()
	// uncomment to remove client after irresponsive for heartbeatTimeOut
	// c.conn.SetReadDeadline(time.Now().Add(heartbeatTimeOut))
	for {
		_, m, err := c.conn.ReadMessage()
		if nil != err {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("Error unexpected closure: %v", err)
			}
			return
		}
		// uncomment to remove client after irresponsive for heartbeatTimeOut
		// c.conn.SetReadDeadline(time.Now().Add(heartbeatTimeOut))
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
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			err := c.conn.WriteMessage(websocket.TextMessage, msg)
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

			// put it straight onto room's recvqueue
			m.Sender = c.ID
			c.room.recvQueue <- m
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
		http.Error(w, errInvalidRoomID, http.StatusBadRequest)
		return
	}

	conn, err := wsUpgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println(err)
		return
	}

	// TODO: check conn.Subprotocol to make sure the correct protocol is negotiated

	cid := xid.New().String()
	client := NewClientConn(cid, room, conn)

	go client.handleVChamberClient()
	go client.handleWSClientSend()
	go client.handleWSClientRecv()

	room.enqClient <- client
	log.Printf("client %s from %s joined room %s", cid, conn.RemoteAddr(), roomid)
}

func main() {

	flag.Parse()

	server := NewServer()
	go server.Run()
	server.enqRoom <- NewRoom("testroom", server)

	http.HandleFunc("/", http.NotFound)
	http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		handleWSClient(server, w, r)
	})

	// TODO: use TLS
	err := http.ListenAndServe(*bindaddr, nil)
	if err != nil {
		log.Fatal("ListenAndServe: ", err)
	}
}
