package game

import (
	"context"
	"net"
	"slices"
	"sync"
	"time"
)

// SDRCluster represents a Valve Steam Datagram Relay gateway location.
type SDRCluster struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	IP   string `json:"ip"`
	Port int    `json:"port"`
}

// DefaultSDRClusters contains the official Valve SDR relays serving Asia-Pacific players.
var DefaultSDRClusters = []SDRCluster{
	{ID: "sgp", Name: "Singapore (Primary SEA)", IP: "103.10.124.1", Port: 27015},
	{ID: "sgp2", Name: "Singapore (Secondary)", IP: "103.28.54.1", Port: 27015},
	{ID: "hkg", Name: "Hong Kong (East Asia)", IP: "153.254.86.1", Port: 27015},
	{ID: "tyo", Name: "Tokyo (Japan)", IP: "45.121.184.1", Port: 27015},
	{ID: "seo", Name: "Seoul (Korea)", IP: "153.254.87.1", Port: 27015},
}

// SDRProbeResult is the ping and quality measurement to a Valve game relay.
type SDRProbeResult struct {
	ClusterID   string `json:"clusterId"`
	ClusterName string `json:"clusterName"`
	IP          string `json:"ip"`
	LatencyMs   int64  `json:"latencyMs"`
	OK          bool   `json:"ok"`
	Quality     string `json:"quality"` // excellent (<50ms), good (<80ms), fair (<120ms), poor (>=120ms)
}

// ProbeSDRClusters measures RTT to all default Valve SDR relay locations concurrently.
func ProbeSDRClusters(ctx context.Context, timeout time.Duration) []SDRProbeResult {
	if timeout <= 0 {
		timeout = 2500 * time.Millisecond
	}

	clusters := DefaultSDRClusters
	results := make([]SDRProbeResult, len(clusters))
	var wg sync.WaitGroup

	for i, c := range clusters {
		wg.Add(1)
		go func(idx int, target SDRCluster) {
			defer wg.Done()
			results[idx] = probeCluster(ctx, target, timeout)
		}(i, c)
	}

	wg.Wait()

	// Sort working relays by latency ascending (lowest ping first)
	slices.SortStableFunc(results, func(a, b SDRProbeResult) int {
		if a.OK != b.OK {
			if a.OK {
				return -1
			}
			return 1
		}
		return int(a.LatencyMs - b.LatencyMs)
	})

	return results
}

func probeCluster(ctx context.Context, c SDRCluster, timeout time.Duration) SDRProbeResult {
	res := SDRProbeResult{
		ClusterID:   c.ID,
		ClusterName: c.Name,
		IP:          c.IP,
		LatencyMs:   -1,
		OK:          false,
		Quality:     "unreachable",
	}

	// Try UDP echo / connect probe first
	addr := net.JoinHostPort(c.IP, "27015")
	start := time.Now()

	d := net.Dialer{Timeout: timeout}
	conn, err := d.DialContext(ctx, "udp", addr)
	if err == nil {
		defer conn.Close()
		// UDP connect succeeded (socket bound to destination)
		// Valve SDR UDP responds to standard ping or connection setup
		_ = conn.SetDeadline(time.Now().Add(timeout))
		// Send a lightweight Valve SDR ping byte
		_, writeErr := conn.Write([]byte{0xff, 0xff, 0xff, 0xff, 'p', 'i', 'n', 'g', 0x00})
		if writeErr == nil {
			buf := make([]byte, 64)
			n, _ := conn.Read(buf)
			rtt := time.Since(start).Milliseconds()
			if n > 0 || rtt > 0 {
				res.LatencyMs = rtt
				res.OK = true
				res.Quality = classifyQuality(rtt)
				return res
			}
		}
	}

	// Fallback to TCP probe on common edge port (443 / 80) if UDP was filtered
	tcpStart := time.Now()
	tcpConn, tcpErr := d.DialContext(ctx, "tcp", net.JoinHostPort(c.IP, "443"))
	if tcpErr == nil {
		_ = tcpConn.Close()
		rtt := time.Since(tcpStart).Milliseconds()
		res.LatencyMs = rtt
		res.OK = true
		res.Quality = classifyQuality(rtt)
		return res
	}

	return res
}

func classifyQuality(ms int64) string {
	switch {
	case ms < 50:
		return "excellent"
	case ms < 80:
		return "good"
	case ms < 120:
		return "fair"
	default:
		return "poor"
	}
}
