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
	OK    bool     `json:"ok"`
	NRoom int      `json:"nroom"`
	Rooms []string `json:"rooms"`
}

type RoomCreatedMsg struct {
	OK        bool   `json:"ok"`
	RoomID    string `json:"roomID"`
	MasterKey string `json:"masterToken"`
	GuestKey  string `json:"guestToken"`
}

type RoomInfo struct {
	RoomID    string `json:"roomID"`
	MasterKey string `json:"masterToken"`
	GuestKey  string `json:"guestToken"`
}

type AllRoomInfoMsg struct {
	OK    bool        `json:"ok"`
	Rooms []*RoomInfo `json:"rooms"`
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

func getServerInfo(s *Server, w http.ResponseWriter, r *http.Request) {
	var rms []string
	s.mutex.RLock()
	nr := len(s.rooms)
	for rm := range s.rooms {
		rms = append(rms, rm)
	}
	s.mutex.RUnlock()
	RespondWithJSON(&ServerInfoMsg{
		true,
		nr,
		rms,
	}, http.StatusOK, w)
}

func destroyServer(s *Server, w http.ResponseWriter, r *http.Request) {
	s.closingGuard.Do(func() { close(s.closing) })
	RespondWithJSON(map[string]bool{
		"ok": true,
	}, http.StatusOK, w)
}

func getAllRoomInfo(s *Server, w http.ResponseWriter, r *http.Request) {
	var rms []*RoomInfo
	s.mutex.RLock()
	for _, rm := range s.rooms {
		rms = append(rms, &RoomInfo{
			RoomID:    rm.ID,
			MasterKey: rm.masterKey,
			GuestKey:  rm.guestKey,
		})
	}
	s.mutex.RUnlock()
	RespondWithJSON(&AllRoomInfoMsg{
		true,
		rms,
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
func NewVChamberRestMux(server *Server) *mux.Router {
	restMux := mux.NewRouter().StrictSlash(true)
	restMux.HandleFunc("/room", func(w http.ResponseWriter, r *http.Request) {
		createRoom(server, w, r)
	}).Methods("GET", "POST")
	restMux.HandleFunc("/server", func(w http.ResponseWriter, r *http.Request) {
		getServerInfo(server, w, r)
	}).Methods("GET")
	restMux.HandleFunc("/server", func(w http.ResponseWriter, r *http.Request) {
		destroyServer(server, w, r)
	}).Methods("DELETE")
	restMux.HandleFunc("/allroom", func(w http.ResponseWriter, r *http.Request) {
		getAllRoomInfo(server, w, r)
	}).Methods("GET")

	// restMux.HandleFunc("/room/{rid}", func(w http.ResponseWriter, r *http.Request) {
	// 	destroyRoom(server, w, r)
	// }).Methods("DELETE")
	return restMux
}
