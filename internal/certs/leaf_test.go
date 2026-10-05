package certs_test

import (
	"testing"
	"time"

	"github.com/hashcott/ghostline/internal/certs"
	"github.com/stretchr/testify/require"
)

func TestIssuer_Covers(t *testing.T) {
	ca, err := certs.NewSessionCA([]string{"youtube.com", "googlevideo.com"}, t0)
	require.NoError(t, err)
	is := certs.NewIssuer(ca, func() time.Time { return t0 })
	for h, want := range map[string]bool{"youtube.com": true, "m.youtube.com": true, "r1.googlevideo.com": true,
		"vercel.com": false, "notyoutube.com": false, "youtube.com.evil.net": false} {
		require.Equal(t, want, is.Covers(h), h)
	}
}
