package main

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"github.com/rs/xid"
)

const (
	roomCreationTimeOut = 5 * time.Second
)

type roomCreatedMsg struct {
	OK        bool   `json:"ok"`
	RoomID    string `json:"roomID"`
	MasterKey string `json:"masterToken"`
	GuestKey  string `json:"guestToken"`
}

func respondWithJSON(m interface{}, statusCode int, w http.ResponseWriter) {

	payload, _ := json.Marshal(m)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	w.Write(payload)
}

func respondWithError(reason string, statusCode int, w http.ResponseWriter) {
	respondWithJSON(map[string]interface{}{
		"ok":     false,
		"reason": reason,
	}, statusCode, w)
}

func createRoom(s *Server, w http.ResponseWriter, r *http.Request) {
	rid := xid.New().String()
	room, mk, gk, err := NewRoomWithRandomKeys(rid, s)
	if err != nil {
		respondWithError("An internal error occurred.",
			http.StatusInternalServerError, w)
		return
	}
	select {
	case s.enqRoom <- room:
		rsp := roomCreatedMsg{
			true,
			rid,
			mk,
			gk,
		}
		respondWithJSON(rsp, http.StatusOK, w)
	case <-time.After(roomCreationTimeOut):
		respondWithError(
			"Room creation timed out.",
			http.StatusRequestTimeout,
			w,
		)
	}
}

func destroyRoom(s *Server, w http.ResponseWriter, r *http.Request) {
	respondWithError("DestroyRoom not implemented", http.StatusNotImplemented, w)
}

// NewVChamberRestMux makes the RESTful API servemux of server
func NewVChamberRestMux(server *Server) http.Handler {
	restMux := mux.NewRouter().StrictSlash(true)
	restMux.HandleFunc("/", http.NotFound)
	restMux.HandleFunc("/room", func(w http.ResponseWriter, r *http.Request) {
		createRoom(server, w, r)
	}).Methods("GET", "POST")
	restMux.HandleFunc("/room/{rid}", func(w http.ResponseWriter, r *http.Request) {
		createRoom(server, w, r)
	}).Methods("DELETE")
	return restMux
}
