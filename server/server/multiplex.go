package server

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/rs/xid"
)

const (
	ConnMultiplexorSendQueueSize = 256
)

type MultiplexedMessageType int

const (
	MultiplexedMessageTypePayload MultiplexedMessageType = iota
	MultiplexedMessageTypeConnected
	MultiplexedMessageTypeDisconnect
)

type ConnectedMessage struct {
	ClientID string `json:"cid"`
}

type MultiplexedMessage struct {
	Type     MultiplexedMessageType `json:"type"`
	ClientID string                 `json:"id"`
	Message  interface{}            `json:"msg"`
}

type ReceivedMultiplexedMessage struct {
	Type     MultiplexedMessageType `json:"type"`
	ClientID string                 `json:"id"`
	Message  json.RawMessage        `json:"msg"`
}

func (m *MultiplexedMessage) UnmarshalJSON(data []byte) error {
	var recv ReceivedMultiplexedMessage
	if err := json.Unmarshal(data, &recv); err != nil {
		return err
	}
	m.Type = recv.Type
	m.ClientID = recv.ClientID
	switch m.Type {
	case MultiplexedMessageTypePayload:
		var p Message
		if err := json.Unmarshal(recv.Message, &p); err != nil {
			return err
		}
		m.Message = &p
	case MultiplexedMessageTypeDisconnect:
	case MultiplexedMessageTypeConnected:
		break
	default:
		return errors.New("Unknown MultiplexedMessage type")
	}
	return nil
}

type workType int

const (
	workTypeRecv workType = iota
	workTypeSend
)

type work struct {
	t   workType
	mux *ConnMultiplexor
	p   interface{}
}

func processWork(w *work) {
	switch w.t {
	case workTypeRecv:
		wk := w.p.([]byte)
		var m MultiplexedMessage
		if err := json.Unmarshal(wk, &m); err != nil {
			// invalid message, drop it
			log.Printf("received invalid message %s", string(wk))
			return
		}

		_c, ok := w.mux.clients.Load(m.ClientID)
		if !ok {
			log.Printf("from unknown client id %s", m.ClientID)
			return
		}
		c := _c.(*MultiplexedClientConn)

		switch m.Type {
		case MultiplexedMessageTypePayload:
			msg := m.Message.(*Message)
			msg.Sender = m.ClientID
			switch msg.Type {
			case MessageTypePing:
				// if it is ping we pong it back
				p := msg.Payload.(*PingMessage)
				pong := Message{
					ReceivedAt: msg.ReceivedAt,
					Type:       MessageTypePong,
					Payload: &PongMessage{
						Timestamp: p.Timestamp,
					},
				}
				mmsg := MultiplexedMessage{
					Type:     MultiplexedMessageTypePayload,
					ClientID: m.ClientID,
					Message:  &pong,
				}
				b, err := json.Marshal(mmsg)
				if err != nil {
					log.Printf("error serialising message %v", mmsg)
					return
				}
				w.mux.sendQueue <- b
			case MessageTypeStateUpdate:
				// pass it to room

				if c.GetState() == clientStateMaster {
					c.room.recvQueue <- msg
				} else {
					// otherwise we silently drop it
					log.Println("non master attempted to change room state")
				}
			default:
				// drop it
			}
		case MultiplexedMessageTypeConnected:
			cType := ""
			if c.GetState() == clientStateMaster {
				cType = "master"
			} else if c.GetState() == clientStateGuest {
				cType = "guest"
			}
			c.SendMessage(&Message{
				Type: MessageTypeHello,
				Payload: &HelloMessage{
					ClientType: cType,
				}})
			c.room.enqClient <- c
			log.Printf("%s client %s from %s joined room %s", cType, c.GetID(), c.GetRemoteAddr(), c.room.ID)

		case MultiplexedMessageTypeDisconnect:
			c.room.deqClient <- c
			// reply with a closure
		}
		return

	case workTypeSend:
		wk := w.p.(*MultiplexedMessage)
		if wk.Type == MultiplexedMessageTypePayload {
			msg := wk.Message.(*Message)
			if msg.Type == MessageTypePong {
				// compute the service time
				p := (msg.Payload.(*PongMessage))
				p.SvcTime = time.Since(msg.ReceivedAt).Seconds()
			}
		}

		b, err := json.Marshal(wk)
		if err != nil {
			log.Println("fail to marshal message %v", wk)
			return
		}
		w.mux.sendQueue <- b
	}
}

func handleWSClientConnect(mux *ConnMultiplexor, w http.ResponseWriter, r *http.Request) {

	// parse query string and check if roomid is valid
	q := r.URL.Query()
	roomid := q.Get("rid")
	token := q.Get("token")
	remoteAddr := q.Get("remote")

	room, cState, err := checkValidClient(mux.server, roomid, token)

	if err != nil {
		log.Printf("client from %v fails to connect: %v", r.RemoteAddr, err)
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}

	cid := xid.New().String()
	client := NewMultiplxedClientConn(cid, room, mux, cState, remoteAddr)

	// add the client to mux.clients
	mux.clients.Store(cid, client)

	// respond with connected message,
	msg := ConnectedMessage{cid}
	RespondWithJSON(&msg, http.StatusOK, w)
}

type ConnMultiplexor struct {
	conn      *websocket.Conn
	clients   *sync.Map // map[string]*MultiplexedClientConn
	sendQueue chan []byte
	server    *Server
	name      string
}

func NewConnMultiplexor(s *Server, conn *websocket.Conn, name string) *ConnMultiplexor {
	return &ConnMultiplexor{
		conn:      conn,
		clients:   &sync.Map{},
		sendQueue: make(chan []byte, ConnMultiplexorSendQueueSize),
		server:    s,
		name:      name,
	}
}

type MultiplexedClientConn struct {
	ID         string
	parent     *ConnMultiplexor
	remoteAddr string
	state      clientState
	room       *Room
}

func (c *MultiplexedClientConn) GetID() string         { return c.ID }
func (c *MultiplexedClientConn) GetRemoteAddr() string { return c.remoteAddr }
func (c *MultiplexedClientConn) GetState() clientState { return c.state }
func (c *MultiplexedClientConn) SendMessage(m *Message) {
	c.parent.server.sendQueue <- &work{
		t:   workTypeSend,
		mux: c.parent,
		p: &MultiplexedMessage{
			Type:     MultiplexedMessageTypePayload,
			ClientID: c.GetID(),
			Message:  m,
		},
	}
}
func (c *MultiplexedClientConn) Finalise() {
	c.parent.server.sendQueue <- &work{
		t:   workTypeSend,
		mux: c.parent,
		p: &MultiplexedMessage{
			Type:     MultiplexedMessageTypeDisconnect,
			ClientID: c.GetID(),
		},
	}
	c.parent.clients.Delete(c.GetID())
}

func NewMultiplxedClientConn(cid string, room *Room, mux *ConnMultiplexor, cState clientState, remoteAddr string) *MultiplexedClientConn {
	return &MultiplexedClientConn{
		ID:         cid,
		parent:     mux,
		remoteAddr: remoteAddr,
		state:      cState,
		room:       room,
	}
}

func (mux *ConnMultiplexor) HandleRecv() {
	defer func() {
		mux.conn.Close()
		mux.clients.Range(func(k interface{}, v interface{}) bool {
			c := v.(*MultiplexedClientConn)
			c.room.deqClient <- c
			return true
		})
		mux.server.mutex.Lock()
		delete(mux.server.muxes, mux.name)
		mux.server.mutex.Unlock()
	}()
	for {
		_, m, err := mux.conn.ReadMessage()
		if nil != err {
			log.Printf("Reverse proxy %v disconnected", mux.conn.RemoteAddr)
			return
		}
		mux.server.recvQueue <- &work{
			t:   workTypeRecv,
			mux: mux,
			p:   m,
		}
	}
}

func (mux *ConnMultiplexor) HandleSend() {
	defer func() {
		mux.conn.Close()
	}()
	for {
		b, ok := <-mux.sendQueue
		if !ok {
			mux.conn.WriteMessage(websocket.CloseMessage, []byte{})
			return
		}
		mux.conn.SetWriteDeadline(time.Now().Add(WriteWait))
		err := mux.conn.WriteMessage(websocket.TextMessage, b)
		if err != nil {
			return
		}
	}
}

func RunWorkers(n int, workQueue chan *work) {
	for i := 0; i < n; i++ {
		go func() {
			for {
				w, ok := <-workQueue
				if !ok {
					return
				}
				processWork(w)
			}
		}()
	}
}

func handleWSRevProxy(s *Server, w http.ResponseWriter, r *http.Request) {
	conn, err := wsUpgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println(err)
		return
	}

	proxyid := xid.New().String()
	conn.WriteMessage(websocket.TextMessage, []byte(proxyid))

	mux := NewConnMultiplexor(s, conn, proxyid)

	s.mutex.Lock()
	s.muxes[proxyid] = mux
	s.mutex.Unlock()

	go mux.HandleRecv()
	go mux.HandleSend()
	log.Printf("RevProxy %v, ID %s joined server", r.RemoteAddr, proxyid)
}

// GetVChamberWSHandleFunc returns a handle function for the server
func GetVChamberWSRevProxyHandleFunc(server *Server) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		handleWSRevProxy(server, w, r)
	}
}
