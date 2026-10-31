package traefik

import (
	"strconv"

	"git.juzo.io/juzo/hashi/internal/services"
)

// StreamEntryPoint names a stream's entry point (04 §3): tcp-<port> or udp-<port>.
func StreamEntryPoint(s services.Service) string { return s.Kind + "-" + strconv.Itoa(s.PublicPort) }

// streamEntryPoints are the static entry points of every renderable stream, ":<port>/tcp" or ":<port>/udp".
func streamEntryPoints(svcs []services.Service) map[string]entryPoint {
	out := map[string]entryPoint{}
	for _, s := range svcs {
		if s.Stream() && s.Renderable() {
			out[StreamEntryPoint(s)] = entryPoint{Address: ":" + strconv.Itoa(s.PublicPort) + "/" + s.Kind}
		}
	}
	return out
}

// streamServices renders 20-hashi-stream-services.yml (04 §4): per stream one router on its own entry point
// (raw TCP matches HostSNI(`*`), UDP has no rule) and one load balancer over the resolved targets. Weights do not
// apply: Traefik's TCP/UDP load balancers take addresses only. nil when no stream is renderable.
func streamServices(svcs []services.Service) *dynamicConfig {
	tcp := &tcpConfig{Routers: map[string]tcpRouter{}, Services: map[string]tcpService{}}
	udp := &udpConfig{Routers: map[string]udpRouter{}, Services: map[string]udpService{}}
	for _, s := range svcs {
		if !s.Stream() || !s.Renderable() {
			continue
		}
		name, ep := "svc-"+s.Slug, StreamEntryPoint(s)
		targets := s.Targets
		if targets == nil {
			targets = []services.Target{s.Target}
		}
		servers := make([]streamServer, 0, len(targets))
		for _, t := range targets {
			servers = append(servers, streamServer{Address: t.HostPort()})
		}
		if s.Kind == services.KindTCP {
			lb := &tcpLoadBalancer{Servers: servers}
			if s.ProxyProtocol {
				lb.ProxyProtocol = &proxyProtocol{Version: 2}
			}
			tcp.Routers[name] = tcpRouter{EntryPoints: []string{ep}, Rule: "HostSNI(`*`)", Service: name}
			tcp.Services[name] = tcpService{LoadBalancer: lb}
			continue
		}
		udp.Routers[name] = udpRouter{EntryPoints: []string{ep}, Service: name}
		udp.Services[name] = udpService{LoadBalancer: &udpLoadBalancer{Servers: servers}}
	}
	c := &dynamicConfig{}
	if len(tcp.Routers) > 0 {
		c.TCP = tcp
	}
	if len(udp.Routers) > 0 {
		c.UDP = udp
	}
	if c.TCP == nil && c.UDP == nil {
		return nil
	}
	return c
}
