package schedule

import (
	"net/http"
	"net/url"
	"sync"
	"time"

	hostpool "github.com/bitly/go-hostpool"
)

// configurable constants
const (
	SchedulingUpdatePeriod = 30 * time.Second
)

// Scheduler implements a RESTful API to create rooms, with the same API as implemented
// in the underlying backend servers. It delegates requests to a backend and register
// it with the room registry
type Scheduler struct {
	store        Storage
	info         *ScheduleInfo
	pool         hostpool.HostPool
	orchestrator url.URL
	mutex        *sync.RWMutex
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
func NewScheduler(orAPI url.URL, s Storage) *Scheduler {
	return &Scheduler{
		store:        s,
		info:         NewScheduleInfo(),
		pool:         nil,
		orchestrator: orAPI,
		mutex:        &sync.RWMutex{},
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
	ticker := time.NewTicker(SchedulingUpdatePeriod)
	for {
		select {
		case <-ticker.C:
			sch.PollSchedulingInfo()
		}
	}
}

// PollSchedulingInfo polls update from orchestrator and update the backend pool
func (sch *Scheduler) PollSchedulingInfo() {
	// poll from orchestrator
	// TODO: implement this if time allows
	sch.mutex.Lock()
	defer sch.mutex.Unlock()
}

func (sch *Scheduler) ServeHTTP(rw http.ResponseWriter, req *http.Request) {

}
