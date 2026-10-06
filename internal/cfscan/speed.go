package cfscan

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/netip"
	"os"
	"time"
)

// ErrNoSpeedEndpoint means the probe host does not serve /__down.
var ErrNoSpeedEndpoint = errors.New("cfscan: host has no /__down")

// Speed downloads bytes from https://<Host>/__down through ip and returns
// the rate in Mbit/s. At limit it stops and returns the rate so far.
func (p Prober) Speed(ctx context.Context, ip netip.Addr, bytes int, limit time.Duration) (float64, error) {
	ctx, cancel := context.WithTimeout(ctx, limit)
	defer cancel()
	c, _, _, err := p.connect(ctx, ip, "http/1.1")
	if err != nil {
		return 0, err
	}
	defer c.Close()
	start := time.Now()
	resp, err := p.get(c, fmt.Sprintf("/__down?bytes=%d", bytes))
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusNotFound:
		return 0, ErrNoSpeedEndpoint
	case resp.StatusCode != http.StatusOK:
		return 0, fmt.Errorf("cfscan: speed: %s", resp.Status)
	}
	n, err := io.Copy(io.Discard, io.LimitReader(resp.Body, int64(bytes)))
	// Reaching the limit is not an error: the socket deadline (set from ctx)
	// can fire a moment before ctx itself reports it.
	if err != nil && ctx.Err() == nil && !errors.Is(err, os.ErrDeadlineExceeded) {
		return 0, err
	}
	if n == 0 {
		return 0, errors.New("cfscan: speed: nothing received")
	}
	// The monotonic clock can read 0 for a very short local transfer.
	secs := max(time.Since(start).Seconds(), 0.001)
	return float64(n) * 8 / secs / 1e6, nil
}

// SpeedTop measures, one at a time, the first top OK results of rs and
// stores the rate in their Mbps. ErrNoSpeedEndpoint stops it at once; other
// failures leave that result's Mbps at 0.
func SpeedTop(ctx context.Context, rs []Result, p Prober, top, bytes int, limit time.Duration, onEach func(Result)) error {
	done := 0
	for i := range rs {
		if done >= top || ctx.Err() != nil {
			break
		}
		if !rs[i].OK {
			continue
		}
		done++
		ip, err := netip.ParseAddr(rs[i].IP)
		if err != nil {
			continue
		}
		mbps, err := p.Speed(ctx, ip, bytes, limit)
		if errors.Is(err, ErrNoSpeedEndpoint) {
			return err
		}
		if err == nil {
			rs[i].Mbps = mbps
		}
		if onEach != nil {
			onEach(rs[i])
		}
	}
	return ctx.Err()
}
