package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/gorilla/websocket"
)

const (
	HeartbeatPeriod = 1 * time.Second
)

// Client is a headless vChamber client, it cannot function as a master
type Client struct {
	Conn    *websocket.Conn
	State   *PlaybackState
	Latency time.Duration
	Stop    chan bool
	Stopped chan bool
}

// ClientHandleRecv is the read loop for vChamber client
func (c *Client) ClientHandleRecv() {
	defer func() {
		c.Conn.Close()
	}()
	for {
		select {
		// not implemented
		case <-c.Stop:
			return
		}
	}
}

// ClientSendHeartbeat periodically pings the server
func (c *Client) ClientSendHeartbeat() {
	var ticker = time.NewTicker(HeartbeatPeriod)
	defer func() {
		close(c.Stopped)
		c.Conn.Close()
	}()
	for {
		select {
		case <-ticker.C:
			var ping PingMessage
			ping.Timestamp = float64(time.Now().UnixNano()) / 1000000000.0
			if err := c.SendMessage(&Message{
				Type:    MessageTypePing,
				Payload: &ping,
			}); err != nil {
				return
			}
		case <-c.Stop:
			c.Conn.WriteMessage(websocket.CloseMessage, []byte{})
			return
		}
	}
}

// SendMessage is a helper function to send a message from c
func (c *Client) SendMessage(msg *Message) error {
	b, _ := json.Marshal(msg)
	c.Conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	return c.Conn.WriteMessage(websocket.TextMessage, b)
}

// Connect initiates a new websocket connection to a vChamber server with given params
func Connect(dialer *websocket.Dialer, addr string, rid string, token string) (*Client, error) {
	if dialer == nil {
		dialer = &websocket.Dialer{
			Proxy:            http.ProxyFromEnvironment,
			HandshakeTimeout: 45 * time.Second,
			Subprotocols:     []string{WebsocketSubprotocolMagicV1},
		}
	}
	u, err := url.Parse(addr)
	if err != nil {
		return nil, err
	}
	q := u.Query()
	q.Set("rid", rid)
	q.Set("token", token)
	u.RawQuery = q.Encode()
	fmt.Println(u.String())
	conn, _, err := dialer.Dial(u.String(), nil)
	if err != nil {
		return nil, err
	}

	_, b, err := conn.ReadMessage()
	if err != nil {
		conn.Close()
		return nil, err
	}

	var hello Message
	err = json.Unmarshal(b, &hello)
	if err != nil && hello.Type != MessageTypeHello {
		conn.WriteMessage(websocket.CloseMessage, []byte{})
		conn.Close()
		return nil, err
	}

	var state PlaybackState
	state.lastUpdated = time.Now()

	return &Client{
		Conn:    conn,
		State:   &state,
		Latency: time.Duration(0),
		Stop:    make(chan bool),
		Stopped: make(chan bool),
	}, nil
}
