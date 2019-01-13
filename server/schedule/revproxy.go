package schedule

import (
	"net/http"
	"net/url"

	vsv "github.com/UoB-Cloud-Computing-2018-KLS/vchamber/server"

	"github.com/koding/websocketproxy"
)

// LoadBalancedReverseProxy is a reverse proxy that serves as an entry point
// for multiple backend servers
type LoadBalancedReverseProxy struct {
	reg ReadOnlyStorage
}

// NewLoadBalancedReverseProxy creates a new reverse proxy with the specific in-memory
// database (room registry) source
func NewLoadBalancedReverseProxy(roomReg ReadOnlyStorage) *LoadBalancedReverseProxy {
	return &LoadBalancedReverseProxy{reg: roomReg}
}

func (r *LoadBalancedReverseProxy) ProxyBackend() func(*http.Request) *url.URL {
	return func(req *http.Request) *url.URL {
		q := req.URL.Query()
		rid := q.Get("rid")
		target := ""
		if rid != "" {
			target, _ = r.reg.Get(rid)
		}
		if target == "" {
			return nil
		}
		u := *BackendWSScheme
		u.Host = target
		u.Fragment = req.URL.Fragment
		u.Path = req.URL.Path
		u.RawQuery = req.URL.RawQuery
		return &u
	}

}

// GetProxy returns a websocket reverse proxy object with registry-backed backend
func (r *LoadBalancedReverseProxy) GetProxy() *websocketproxy.WebsocketProxy {
	return &websocketproxy.WebsocketProxy{
		Backend:  r.ProxyBackend(),
		Upgrader: vsv.GetWSUpgrader(),
	}
}
