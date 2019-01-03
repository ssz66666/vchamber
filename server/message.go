package main

import (
	"encoding/json"
)

// Message defines the vchamber message format
type Message struct {
	// TODO: change to fit the synchronisation protocol
	// just a string for now

	Sender  string `json:"-"`
	Payload string
}

// Serialise a Message to its wire format as []byte
func (m *Message) Serialise() ([]byte, error) {
	return json.Marshal(m)
}

// Deserialise a Message stored in data in its wire format back to a struct
// and store it to the value pointed to by m
func Deserialise(data []byte, m *Message) error {
	// TODO: should store it in an interface{} first and check if it is actually valid
	return json.Unmarshal(data, m)
}
