package schedule

import (
	"encoding/json"
	"io/ioutil"
	"log"
	"net/http"
	"sync"
	"time"

	vsv "github.com/UoB-Cloud-Computing-2018-KLS/vchamber/server"
	"github.com/gorilla/websocket"
)

type ConnectedClient struct {
	ID        string
	Conn      *websocket.Conn
	RecvQueue chan []byte
}

func NewConnectedClient(cid string, conn *websocket.Conn) *ConnectedClient {
	return &ConnectedClient{
		ID:        cid,
		Conn:      conn,
		RecvQueue: make(chan []byte, 32),
	}
}

type BackendServer struct {
	AssignedProxyID string
	Conn            *websocket.Conn
	Clients         *sync.Map // string -> *ConnectedClient
	SendQueue       chan []byte
}

// LoadBalancedReverseProxy is a reverse proxy that serves as an entry point
// for multiple backend servers
type LoadBalancedReverseProxy struct {
	reg      ReadOnlyStorage
	conns    *sync.Map // map[host(string)]*BackendServer
	dialer   *websocket.Dialer
	upgrader *websocket.Upgrader
}

// NewLoadBalancedReverseProxy creates a new reverse proxy with the specific in-memory
// database (room registry) source
func NewLoadBalancedReverseProxy(roomReg ReadOnlyStorage) *LoadBalancedReverseProxy {
	return &LoadBalancedReverseProxy{
		reg:      roomReg,
		conns:    &sync.Map{},
		dialer:   websocket.DefaultDialer,
		upgrader: vsv.GetWSUpgrader(),
	}
}

func (r *BackendServer) SendConnectedMessage(cid string) {
	b, _ := json.Marshal(&vsv.MultiplexedMessage{
		Type:     vsv.MultiplexedMessageTypeConnected,
		ClientID: cid,
		Message:  []byte{},
	})
	r.SendQueue <- b
}

func (r *BackendServer) SendCloseMessage(cid string) {
	b, _ := json.Marshal(&vsv.MultiplexedMessage{
		Type:     vsv.MultiplexedMessageTypeDisconnect,
		ClientID: cid,
		Message:  []byte{},
	})
	r.SendQueue <- b
}

func (r *LoadBalancedReverseProxy) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	q := req.URL.Query()
	rid := q.Get("rid")
	target := ""
	if rid != "" {
		target, _ = r.reg.Get(rid)
	}
	if target == "" {
		log.Println("unknown roomid")
		http.Error(rw, "Unknown room ID", http.StatusBadRequest)
		return
	}
	_conn, ok := r.conns.Load(target)
	if !ok {
		uw := *BackendWSScheme
		uw.Host = target
		uw.Path = "/rev"

		beconn, _, err := r.dialer.Dial(uw.String(), nil)
		if err != nil {
			log.Printf("Failed to connect to backend server %v", &uw)
			http.Error(rw, http.StatusText(http.StatusServiceUnavailable), http.StatusServiceUnavailable)
			return
		}
		_, b, err := beconn.ReadMessage()
		if err != nil {
			log.Printf("Failed to obtain id from backend server %v", &uw)
			http.Error(rw, http.StatusText(http.StatusServiceUnavailable), http.StatusServiceUnavailable)
			return
		}
		proxyid := string(b)

		var loaded bool
		_conn, loaded = r.conns.LoadOrStore(target, &BackendServer{
			AssignedProxyID: proxyid,
			Conn:            beconn,
			Clients:         &sync.Map{},
			SendQueue:       make(chan []byte, 256),
		})
		if loaded {
			beconn.Close()
			// close the useless connection
		} else {
			cnn := _conn.(*BackendServer)
			sq := cnn.SendQueue
			go func() {
				defer beconn.Close()
				for m := range sq {
					beconn.SetWriteDeadline(time.Now().Add(vsv.WriteWait))
					// log.Printf("sending message %v", string(m))
					if err := beconn.WriteMessage(websocket.TextMessage, m); err != nil {
						return
					}
				}
			}()

			go func() {
				defer func() {
					beconn.Close()
					cnn.Clients.Range(func(k interface{}, v interface{}) bool {
						cc := v.(*ConnectedClient)
						close(cc.RecvQueue)
						return true
					})
					log.Println("revproxy: killed all connected clients")
					r.conns.Delete(target)
					close(cnn.SendQueue)
				}()
				for {
					_, b, err := beconn.ReadMessage()
					if err != nil {
						log.Println("connection to backend dropped")
						return
					}

					var msg vsv.ReceivedMultiplexedMessage
					if err := json.Unmarshal(b, &msg); err != nil {
						// drop the message
						continue
					}

					_cc, ok := cnn.Clients.Load(msg.ClientID)
					if !ok {
						log.Printf("dead client %s", msg.ClientID)
						if msg.Type != vsv.MultiplexedMessageTypeDisconnect {
							// we should send a disconnect back
							if _, ok := r.conns.Load(target); ok {
								cnn.SendCloseMessage(msg.ClientID)
							}
						}
					} else {
						cc := _cc.(*ConnectedClient)
						if msg.Type == vsv.MultiplexedMessageTypeDisconnect {

						}

						cc.RecvQueue <- msg.Message
					}
				}
			}()

			log.Printf("established connection to backend %v", &uw)
		}
	}

	conn := _conn.(*BackendServer)

	u := *BackendRESTScheme
	u.Host = target
	u.Fragment = req.URL.Fragment
	u.Path = "/join"
	q.Set("remote", req.RemoteAddr)
	q.Set("proxyid", conn.AssignedProxyID)
	u.RawQuery = q.Encode()

	rsp, err := http.Get(u.String())
	if err != nil {
		log.Printf("authentication failed due to incorrect room id or token, error %s", err.Error())
		http.Error(rw, err.Error(), http.StatusUnauthorized)
		return
	}
	b, err := ioutil.ReadAll(rsp.Body)
	if err != nil {
		log.Println("failed to read server connection response")
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		return
	}

	var m vsv.ConnectedMessage
	if err := json.Unmarshal(b, &m); err != nil {
		log.Printf("Illformed connected message %s", string(b))
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		return
	}

	log.Printf("connecting with client id %s", m.ClientID)
	cid := m.ClientID

	// upgrade the http connection
	cconn, err := r.upgrader.Upgrade(rw, req, nil)

	if err != nil {
		log.Println(err)
		conn.SendCloseMessage(cid)
		return
	}

	cclient := NewConnectedClient(cid, cconn)

	conn.Clients.Store(cid, cclient)

	// sender
	go func() {
		defer func() {
			if _, ok := r.conns.Load(target); ok {
				conn.SendCloseMessage(cid)
			}
			cclient.Conn.Close()
		}()
		for {
			m, ok := <-cclient.RecvQueue
			if !ok {
				cclient.Conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			cclient.Conn.SetWriteDeadline(time.Now().Add(vsv.WriteWait))
			err := cclient.Conn.WriteMessage(websocket.TextMessage, m)
			if err != nil {
				return
			}
		}
	}()

	// receiver
	go func() {
		defer func() {
			cclient.Conn.Close()
			conn.Clients.Delete(cid)
		}()
		for {
			_, b, err := cclient.Conn.ReadMessage()
			if err != nil {
				return
			}

			buf, _ := json.Marshal(&vsv.ReceivedMultiplexedMessage{
				Type:     vsv.MultiplexedMessageTypePayload,
				ClientID: cclient.ID,
				Message:  b,
			})
			log.Printf("forwarding message %s to server", string(buf))
			conn.SendQueue <- buf
		}
	}()

	// inform the backend we are connected
	conn.SendConnectedMessage(cid)
}

// func (r *LoadBalancedReverseProxy) ProxyBackend() func(*http.Request) *url.URL {
// 	return func(req *http.Request) *url.URL {
// 		q := req.URL.Query()
// 		rid := q.Get("rid")
// 		target := ""
// 		if rid != "" {
// 			target, _ = r.reg.Get(rid)
// 		}
// 		if target == "" {
// 			return nil
// 		}
// 		u := *BackendWSScheme
// 		u.Host = target
// 		u.Fragment = req.URL.Fragment
// 		u.Path = req.URL.Path
// 		u.RawQuery = req.URL.RawQuery
// 		return &u
// 	}

// }

// GetProxy returns a websocket reverse proxy object with registry-backed backend
// func (r *LoadBalancedReverseProxy) GetProxy() *websocketproxy.WebsocketProxy {
// 	return &websocketproxy.WebsocketProxy{
// 		Backend:  r.ProxyBackend(),
// 		Upgrader: vsv.GetWSUpgrader(),
// 	}
// }
