package store

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTestDomains_SplitsNormalisesAndDedupes(t *testing.T) {
	require.Equal(t, []string{"www.google.com"}, TestDomains("www.google.com"))
	require.Equal(t, []string{"www.google.com", "youtube.com"}, TestDomains(" WWW.Google.com \n youtube.com, www.google.com\n\n"))
	require.Empty(t, TestDomains("  "))
}

func TestValidateTestDomains(t *testing.T) {
	require.NoError(t, ValidateTestDomains("www.google.com\nyoutube.com"))
	for _, bad := range []string{"", "google", "http://google.com", "a b", "a.com\nnot a domain"} {
		require.Error(t, ValidateTestDomains(bad), bad)
	}
	require.Error(t, ValidateTestDomains("a.com b.com c.com d.com e.com f.com"), "at most 5")
}
