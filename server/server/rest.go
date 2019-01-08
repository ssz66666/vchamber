package server

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

type ServerInfoMsg struct {
	OK    bool `json:"ok"`
	NRoom int  `json:"nroom"`
}

type RoomCreatedMsg struct {
	OK        bool   `json:"ok"`
	RoomID    string `json:"roomID"`
	MasterKey string `json:"masterToken"`
	GuestKey  string `json:"guestToken"`
}

func RespondWithJSON(m interface{}, statusCode int, w http.ResponseWriter) {

	payload, _ := json.Marshal(m)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	w.Write(payload)
}

func RespondWithError(reason string, statusCode int, w http.ResponseWriter) {
	RespondWithJSON(map[string]interface{}{
		"ok":     false,
		"reason": reason,
	}, statusCode, w)
}

func getNRoom(s *Server, w http.ResponseWriter, r *http.Request) {
	s.mutex.RLock()
	nr := len(s.rooms)
	s.mutex.RUnlock()
	RespondWithJSON(&ServerInfoMsg{
		true,
		nr,
	}, http.StatusOK, w)
}

func createRoom(s *Server, w http.ResponseWriter, r *http.Request) {
	rid := xid.New().String()
	room, mk, gk, err := NewRoomWithRandomKeys(rid, s)
	if err != nil {
		RespondWithError("An internal error occurred.",
			http.StatusInternalServerError, w)
		return
	}
	t := time.After(roomCreationTimeOut)
	select {
	case s.enqRoom <- room:
		rsp := RoomCreatedMsg{
			true,
			rid,
			mk,
			gk,
		}
		RespondWithJSON(rsp, http.StatusOK, w)
	case <-t:
		RespondWithError(
			"Room creation timed out.",
			http.StatusRequestTimeout,
			w,
		)
	}
}

func destroyRoom(s *Server, w http.ResponseWriter, r *http.Request) {
	RespondWithError("DestroyRoom not implemented", http.StatusNotImplemented, w)
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
