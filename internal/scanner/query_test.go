package scanner_test

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/hashcott/ghostline/internal/scanner"
	"github.com/miekg/dns"
	"github.com/stretchr/testify/require"
)

func TestExchange_SetsDO(t *testing.T) {
	var got *dns.Msg
	u := &answerUp{fn: func(_ context.Context, req *dns.Msg) (*dns.Msg, error) {
		got = req
		return withA("8.8.8.8")(context.Background(), req)
	}}
	m, d, err := scanner.Exchange(context.Background(), u, "Example.com", dns.TypeAAAA, true)
	require.NoError(t, err)
	require.NotNil(t, m)
	require.GreaterOrEqual(t, d.Nanoseconds(), int64(0))
	require.Equal(t, "Example.com.", got.Question[0].Name)
	require.Equal(t, dns.TypeAAAA, got.Question[0].Qtype)
	require.True(t, got.RecursionDesired)
	opt := got.IsEdns0()
	require.NotNil(t, opt)
	require.True(t, opt.Do())

	_, _, err = scanner.Exchange(context.Background(), u, "example.com", dns.TypeA, false)
	require.NoError(t, err)
	require.Nil(t, got.IsEdns0())
}

type timeoutErr struct{}

func (timeoutErr) Error() string   { return "i/o deadline" }
func (timeoutErr) Timeout() bool   { return true }
func (timeoutErr) Temporary() bool { return true }

func TestClassify(t *testing.T) {
	require.Equal(t, "bootstrap", scanner.Classify(errors.New("couldn't bootstrap dns.google"), nil))
	require.Equal(t, "timeout", scanner.Classify(context.DeadlineExceeded, nil))
	require.Equal(t, "timeout", scanner.Classify(errors.New("x"), context.DeadlineExceeded))
	var ne net.Error = timeoutErr{}
	require.Equal(t, "timeout", scanner.Classify(ne, nil))
	require.Equal(t, "error", scanner.Classify(errors.New("refused"), nil))
}

func TestExchange_HonoursContextWhenUpstreamDoesNot(t *testing.T) {
	block := make(chan struct{})
	defer close(block)
	u := &answerUp{fn: func(context.Context, *dns.Msg) (*dns.Msg, error) {
		<-block // ignores ctx, like dnsproxy's plain upstream
		return nil, errors.New("late")
	}}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, _, err := scanner.Exchange(ctx, u, "example.com", dns.TypeA, false)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Less(t, time.Since(start), time.Second)
	require.Equal(t, "timeout", scanner.Classify(err, ctx.Err()))
}
