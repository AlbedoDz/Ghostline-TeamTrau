package game

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestProbeSDRClusters(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()

	results := ProbeSDRClusters(ctx, 2*time.Second)
	require.NotEmpty(t, results)
	require.Len(t, results, len(DefaultSDRClusters))

	// Ensure Singapore cluster exists in results
	foundSGP := false
	for _, r := range results {
		if r.ClusterID == "sgp" {
			foundSGP = true
			require.Equal(t, "103.10.124.1", r.IP)
		}
	}
	require.True(t, foundSGP)
}

func TestClassifyQuality(t *testing.T) {
	require.Equal(t, "excellent", classifyQuality(30))
	require.Equal(t, "good", classifyQuality(65))
	require.Equal(t, "fair", classifyQuality(100))
	require.Equal(t, "poor", classifyQuality(150))
}
