package server

import (
	"encoding/json"
	"time"
)

// Message defines the vchamber message format
type Message struct {
	Sender     string      `json:"-"`
	ReceivedAt time.Time   `json:"-"`
	Type       MessageType `json:"type"`
	Payload    interface{} `json:"payload"`
}
type receivedMessage struct {
	Type    MessageType     `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

type HelloMessage struct {
	ClientType string `json:"authority"`
	// State      *PlaybackStateMessage `json:"state"`
}

type PingMessage struct {
	Timestamp float64 `json:"sendtime"`
}

type PongMessage struct {
	Timestamp float64 `json:"sendtime"`
	SvcTime   float64 `json:"servicetime"`
}

type PlaybackStateMessage struct {
	Source   string         `json:"src"`
	Status   PlaybackStatus `json:"status"`
	Position float64        `json:"position"`
	Speed    float64        `json:"speed"`
	Duration float64        `json:"duration"`
}

type PlaybackStateUpdateMessage struct {
	State *PlaybackStateMessage `json:"state"`
	RTT   float64               `json:"rtt"`
}

type ReservedMessage json.RawMessage

// MessageType is type of message
type MessageType int

// MessageType instances
const (
	MessageTypeHello MessageType = iota
	MessageTypePing
	MessageTypePong
	MessageTypeStateBroadcast
	MessageTypeStateUpdate
	MessageTypeReserved MessageType = 99
)

// Serialise a Message to its wire format as []byte
func (m *Message) Serialise() ([]byte, error) {
	return json.Marshal(m)
}

// Deserialise a Message stored in data in its wire format back to a struct
// and store it to the value pointed to by m
func Deserialise(data []byte, m *Message) error {
	// TODO: should store it in an interface{} first and check if it is actually valid
	var rm receivedMessage

	err := json.Unmarshal(data, &rm)
	if err != nil {
		return err
	}

	m.ReceivedAt = time.Now()
	m.Type = rm.Type

	switch m.Type {
	case MessageTypeHello:
		var p HelloMessage
		err = json.Unmarshal(rm.Payload, &p)
		m.Payload = &p
	case MessageTypePing:
		var p PingMessage
		err = json.Unmarshal(rm.Payload, &p)
		m.Payload = &p
	case MessageTypePong:
		var p PongMessage
		err = json.Unmarshal(rm.Payload, &p)
		m.Payload = &p
	case MessageTypeStateBroadcast:
		var p PlaybackStateMessage
		err = json.Unmarshal(rm.Payload, &p)
		m.Payload = &p
	case MessageTypeStateUpdate:
		var p PlaybackStateUpdateMessage
		err = json.Unmarshal(rm.Payload, &p)
		m.Payload = &p
	case MessageTypeReserved:
		m.Payload = rm.Payload
	}
	if err != nil {
		return err
	}
	return nil
}
