package winutil

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDPAPI_RoundTrip(t *testing.T) {
	enc, err := ProtectString("s3cret ✓")
	require.NoError(t, err)
	require.NotContains(t, enc, "s3cret")
	got, err := UnprotectString(enc)
	require.NoError(t, err)
	require.Equal(t, "s3cret ✓", got)

	_, err = UnprotectString("not base64 !!")
	require.Error(t, err)
	_, err = UnprotectString("aGVsbG8=") // valid base64, not a DPAPI blob
	require.Error(t, err)
}

func TestProtectMachine_RoundTrip(t *testing.T) {
	enc, err := ProtectMachine([]byte("key bytes"))
	require.NoError(t, err)
	require.NotContains(t, string(enc), "key bytes")
	got, err := UnprotectMachine(enc)
	require.NoError(t, err)
	require.Equal(t, []byte("key bytes"), got)
	_, err = UnprotectMachine([]byte("junk"))
	require.Error(t, err)
}
