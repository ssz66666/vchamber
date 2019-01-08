package schedule

import (
	"net/http"
	"net/url"
	"sync"
	"time"

	hostpool "github.com/bitly/go-hostpool"
)

const (
	SchedulingUpdatePeriod = 30 * time.Second
)

type Scheduler struct {
	store        Storage
	info         *ScheduleInfo
	pool         hostpool.HostPool
	orchestrator url.URL
	mutex        *sync.RWMutex
}

type SchedulingStrategy int

const (
	SchedulingStrategyBalance SchedulingStrategy = iota
	SchedulingStrategyCompact
)

type Backend string
type ServerLoad float64

type ScheduleInfo struct {
	Backends map[Backend]ServerLoad `json:"backends"`
	Strategy SchedulingStrategy     `json:"strategy"`
}

func NewScheduleInfo() *ScheduleInfo {
	return &ScheduleInfo{make(map[Backend]ServerLoad), SchedulingStrategyBalance}
}

func NewScheduler(orAPI url.URL, s Storage) *Scheduler {
	return &Scheduler{
		store:        s,
		info:         NewScheduleInfo(),
		pool:         nil,
		orchestrator: orAPI,
		mutex:        &sync.RWMutex{},
	}
}

func (sch *Scheduler) RebuildPool() {
	hosts := make([]string, 0, len(sch.info.Backends))
	for h := range sch.info.Backends {
		hosts = append(hosts, string(h))
	}
	sch.pool = hostpool.New(hosts) // just round-robin
}

func (sch *Scheduler) NextBackend() string {
	// TODO: decide on room scheduling based on
	// broadcasted info

	// randomly choose a backend
	sch.mutex.RLock()
	h := sch.pool.Get().Host()
	sch.mutex.RUnlock()
	return h

}

func (sch *Scheduler) RunScheduler() {
	ticker := time.NewTicker(SchedulingUpdatePeriod)
	for {
		select {
		case <-ticker.C:
			sch.PollSchedulingInfo()
		}
	}
}

func (sch *Scheduler) PollSchedulingInfo() {
	// poll from orchestrator
	// TODO: implement this if time allows
	sch.mutex.Lock()
	defer sch.mutex.Unlock()
}

func (sch *Scheduler) ServeHTTP(rw http.ResponseWriter, req *http.Request) {

}
