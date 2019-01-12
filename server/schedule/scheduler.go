package schedule

import (
	"bytes"
	"encoding/json"
	"errors"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"sync"
	"time"

	vserver "github.com/UoB-Cloud-Computing-2018-KLS/vchamber/server"
	hostpool "github.com/bitly/go-hostpool"
	"github.com/go-redis/redis"
)

// configurable constants
const (
	SchedulingUpdatePeriod = 30 * time.Second
	SchedulePubSubChannel  = "schedule"
)

// url schemes for our backends
var (
	BackendWSScheme, _   = url.Parse("ws://example.com:8080")
	BackendRESTScheme, _ = url.Parse("http://example.com:8080")
)

// Scheduler implements a RESTful API to create rooms, with the same API as implemented
// in the underlying backend servers. It delegates requests to a backend and register
// it with the room registry
type Scheduler struct {
	store  Storage
	info   *ScheduleInfo
	pool   hostpool.HostPool
	pubsub *redis.PubSub
	mutex  *sync.RWMutex
}

// SchedulingStrategy enum
type SchedulingStrategy int

// SchedulingStrategy enum values
const (
	SchedulingStrategyBalance SchedulingStrategy = iota
	SchedulingStrategyCompact
)

// Backend type for serialisation
type Backend string

// ServerLoad type for serialisation
type ServerLoad float64

// ScheduleInfo defines the message format used by scheduler and orchestrator
type ScheduleInfo struct {
	Backends map[Backend]ServerLoad `json:"backends"`
	Strategy SchedulingStrategy     `json:"strategy"`
}

// NewScheduleInfo creates an empty scheduleinfo message
func NewScheduleInfo() *ScheduleInfo {
	return &ScheduleInfo{make(map[Backend]ServerLoad), SchedulingStrategyBalance}
}

// NewScheduler creates a runnable scheduler with given orchestrator and room registry
func NewScheduler(rclient *redis.Client, s Storage) *Scheduler {
	ps := rclient.Subscribe(SchedulePubSubChannel)
	return &Scheduler{
		store:  s,
		info:   NewScheduleInfo(),
		pool:   hostpool.New([]string{""}),
		pubsub: ps,
		mutex:  &sync.RWMutex{},
	}
}

// RebuildPool recreate the backend pool base on current scheduleinfo,
// NOT thread-safe
func (sch *Scheduler) RebuildPool() {
	hosts := make([]string, 0, len(sch.info.Backends))
	for h := range sch.info.Backends {
		hosts = append(hosts, string(h))
	}
	sch.pool = hostpool.New(hosts) // just round-robin
}

// NextBackend returns a backend string using the current scheduling strategy
func (sch *Scheduler) NextBackend() string {
	// TODO: decide on room scheduling based on
	// broadcasted info

	// randomly choose a backend
	sch.mutex.RLock()
	h := sch.pool.Get().Host()
	sch.mutex.RUnlock()
	return h

}

// RunScheduler runs the scheduler daemon that periodically polls update
// from orchestrator
func (sch *Scheduler) RunScheduler() {
	ch := sch.pubsub.Channel()
	for {
		select {
		case m := <-ch:
			log.Printf("received new schedule info update %s", m)
			var s ScheduleInfo
			json.Unmarshal([]byte(m.Payload), &s)
			sch.mutex.Lock()
			sch.info = &s
			sch.RebuildPool()
			sch.mutex.Unlock()
		}
	}
}

// ProxyDirector returns a Director function for the reverseproxy
func (sch *Scheduler) ProxyDirector() func(*http.Request) {
	return func(req *http.Request) {
		req.URL.Scheme = BackendRESTScheme.Scheme
		req.URL.Host = sch.NextBackend()
		if _, ok := req.Header["User-Agent"]; !ok {
			req.Header.Set("User-Agent", "")
		}
	}
}

// RoomRegister returns a ModifyResponse function for the reverseproxy
func (sch *Scheduler) RoomRegister() func(*http.Response) error {
	return func(rsp *http.Response) error {
		if rsp.StatusCode == http.StatusOK {
			// register the room
			b, err := ioutil.ReadAll(rsp.Body)
			if err != nil {
				return err
			}
			err = rsp.Body.Close()
			if err != nil {
				return err
			}
			var m vserver.RoomCreatedMsg
			if err := json.Unmarshal(b, &m); err != nil {
				return errors.New("Internal error during room creation")
			}
			sch.store.Set(m.RoomID, rsp.Request.URL.Host)
			// put the original content back
			rsp.Body = ioutil.NopCloser(bytes.NewReader(b))
		}
		return nil
	}
}

// GetProxy returns the reverse proxy http.Handler
func (sch *Scheduler) GetProxy() *httputil.ReverseProxy {
	return &httputil.ReverseProxy{Director: sch.ProxyDirector(), ModifyResponse: sch.RoomRegister()}
}
