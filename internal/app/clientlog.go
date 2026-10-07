package app

import (
	"log/slog"
	"regexp"
	"strings"
	"sync"
	"time"
)

// Frontend errors are capped so a UI stuck in a loop cannot fill the log.
const (
	clientLogBurst   = 20
	clientLogWindow  = time.Minute
	clientLogMaxText = 500
	clientLogMaxStk  = 2000
)

var (
	clientLogMu    sync.Mutex
	clientLogStart time.Time
	clientLogCount int

	// appErrText is a Go binding error ("CODE" or "CODE: cause"). Only the
	// code is kept: the cause may name a domain the user looked up, and the
	// Go side logs causes where they happen.
	appErrText = regexp.MustCompile(`^([A-Z][A-Z0-9_]+)(:|$)`)
)

// LogClientError writes a frontend failure (an uncaught exception or a
// rejected call nobody handled) to the log file.
func (s *Service) LogClientError(kind, message, stack string) {
	clientLogMu.Lock()
	now := time.Now()
	if now.Sub(clientLogStart) > clientLogWindow {
		clientLogStart, clientLogCount = now, 0
	}
	clientLogCount++
	n := clientLogCount
	clientLogMu.Unlock()
	if n > clientLogBurst {
		if n == clientLogBurst+1 {
			slog.Warn("ui: too many errors, muting for a minute")
		}
		return
	}
	if m := appErrText.FindStringSubmatch(message); m != nil {
		message, stack = m[1], ""
	}
	slog.Warn("ui: "+truncate(kind, 32), "msg", truncate(message, clientLogMaxText), "stack", truncate(stack, clientLogMaxStk))
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return strings.ToValidUTF8(s[:n], "") + "…"
}
