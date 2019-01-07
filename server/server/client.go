package server

import (
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/gorilla/websocket"
)

// Client is a headless VChamber client, it cannot function as a master
type Client struct {
	conn    *websocket.Conn
	state   *PlaybackState
	latency time.Duration
	stop    chan bool
	stopped chan bool
}

func (c *Client) ClientHandleRecv() {
	defer func() {
		c.conn.Close()
	}()
	for {
		select {
		// not implemented
		case <-c.stop:
			return
		}
	}
}

func (c *Client) ClientSendHeartbeat() {
	var ticker = time.NewTicker(1 * time.Second)
	defer func() {
		close(c.stopped)
		c.conn.Close()
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
		case <-c.stop:
			c.conn.WriteMessage(websocket.CloseMessage, []byte{})
			return
		}
	}
}

func (c *Client) SendMessage(msg *Message) error {
	b, _ := msg.Serialise()
	c.conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	return c.conn.WriteMessage(websocket.TextMessage, b)
}

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
	err = Deserialise(b, &hello)
	if err != nil && hello.Type != MessageTypeHello {
		conn.WriteMessage(websocket.CloseMessage, []byte{})
		conn.Close()
		return nil, err
	}

	var state PlaybackState
	state.lastUpdated = time.Now()

	return &Client{
		conn:    conn,
		state:   &state,
		latency: time.Duration(0),
		stop:    make(chan bool),
		stopped: make(chan bool),
	}, nil
}
