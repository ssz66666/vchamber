package schedule

import (
	"net/http"
	"net/url"
	"sync"

	"github.com/koding/websocketproxy"

	vserver "github.com/UoB-Cloud-Computing-2018-KLS/vchamber/server"
)

// LoadBalancedReverseProxy is a reverse proxy that serves as an entry point
// for multiple backend servers
type LoadBalancedReverseProxy struct {
	reg   ReadOnlyStorage
	conns *sync.Map
}

// NewLoadBalancedReverseProxy creates a new reverse proxy with the specific in-memory
// database (room registry) source
func NewLoadBalancedReverseProxy(roomReg ReadOnlyStorage) *LoadBalancedReverseProxy {
	var m sync.Map
	return &LoadBalancedReverseProxy{
		reg:   roomReg,
		conns: &m,
	}
}

func (r *LoadBalancedReverseProxy) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	q := req.URL.Query()
	rid := q.Get("rid")
	target := ""
	if rid != "" {
		target = r.reg.Get(rid)
	}
	if target == "" {
		http.Error(rw, vserver.ErrInvalidRoomID, http.StatusBadRequest)
		return
	}
	proxy, ok := r.conns.Load(target)
	if !ok {
		u, _ := url.Parse(target)
		proxy, _ = r.conns.LoadOrStore(target, websocketproxy.NewProxy(u))
	}
	proxy.(*websocketproxy.WebsocketProxy).ServeHTTP(rw, req)
}
