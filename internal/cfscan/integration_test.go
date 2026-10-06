//go:build integration

package cfscan_test

import (
	"context"
	"math/rand/v2"
	"net"
	"testing"
	"time"

	"github.com/hashcott/ghostline/internal/cfscan"
	"github.com/stretchr/testify/require"
)

func TestIntegration_RealScan(t *testing.T) {
	ips := cfscan.Sample(cfscan.Ranges(), 200, rand.New(rand.NewPCG(uint64(time.Now().UnixNano()), 1)))
	p := cfscan.Prober{Dial: (&net.Dialer{}).DialContext, Host: "speed.cloudflare.com", Timeout: 2 * time.Second}
	rs, err := cfscan.Scan(context.Background(), ips, p, cfscan.Options{Concurrency: 64, Want: 10, Limiter: cfscan.NewLimiter(200)})
	require.NoError(t, err)
	require.NotEmpty(t, rs)
	require.True(t, rs[0].OK, rs[0].Reason)
	t.Logf("best: %+v", rs[0])
}
