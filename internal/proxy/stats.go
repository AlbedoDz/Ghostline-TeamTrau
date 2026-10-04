package proxy

import "net/netip"

// Stats is a snapshot of proxy counters.
type Stats struct {
	Open       int               `json:"open"`
	LANClients int               `json:"lanClients"`
	BytesIn    uint64            `json:"bytesIn"`  // client → server
	BytesOut   uint64            `json:"bytesOut"` // server → client
	ByOutcome  map[string]uint64 `json:"byOutcome"`
}

// stats is guarded by Server.mu.
type stats struct {
	nOpen     int
	clients   map[netip.Addr]int
	in, out   uint64
	byOutcome map[string]uint64
}

func (st *stats) open(ip netip.Addr) {
	st.nOpen++
	if st.clients == nil {
		st.clients = map[netip.Addr]int{}
	}
	st.clients[ip]++
}

func (st *stats) close(ip netip.Addr) {
	st.nOpen--
	if st.clients[ip]--; st.clients[ip] <= 0 {
		delete(st.clients, ip)
	}
}

func (st *stats) outcome(o string) {
	if o == "" {
		return
	}
	if st.byOutcome == nil {
		st.byOutcome = map[string]uint64{}
	}
	st.byOutcome[o]++
}

func (s *Server) addBytes(in, out uint64) {
	s.mu.Lock()
	s.stats.in += in
	s.stats.out += out
	s.mu.Unlock()
}

// Stats returns a copy of the counters.
func (s *Server) Stats() Stats {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := Stats{Open: s.stats.nOpen, BytesIn: s.stats.in, BytesOut: s.stats.out, ByOutcome: map[string]uint64{}}
	for ip := range s.stats.clients {
		if !ip.IsLoopback() {
			out.LANClients++
		}
	}
	for k, v := range s.stats.byOutcome {
		out.ByOutcome[k] = v
	}
	return out
}
