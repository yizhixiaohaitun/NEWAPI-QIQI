package channel_purity

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTokenRatioUsesMatchingCohortAndKeepsProviderCounters(t *testing.T) {
	pair, ok := NewTokenPair("gpt-4o", "chat", 120, 100, 110, 100)
	require.True(t, ok)
	assert.Equal(t, 120, pair.TargetProvider)
	assert.Equal(t, 100, pair.BaselineProvider)
	assert.InDelta(t, 1.1, pair.Ratio, .0001)
	pairs := []TokenPair{pair, {ModelFamily: "other", RequestType: "chat", Ratio: 9}}
	result := AnalyzeTokenRatios("gpt-4o", "chat", pairs, 5)
	assert.Equal(t, 1, result.Samples)
	assert.False(t, result.Alert, "one deviation must never alert")
}

func TestTokenIntervalMedianMADQuantilesAndConfidence(t *testing.T) {
	values := []float64{.98, 1, 1.01, 1.02, 1.03, 1.04, 1.05, 1.9, 2.0, 2.1}
	pairs := make([]TokenPair, 0, len(values))
	for _, ratio := range values {
		pairs = append(pairs, TokenPair{ModelFamily: "gpt-4o", RequestType: "responses", Ratio: ratio})
	}
	result := AnalyzeTokenRatios("gpt-4o", "responses", pairs, 5)
	assert.InDelta(t, 1.035, result.Median, .001)
	assert.Greater(t, result.MAD, 0.0)
	assert.Greater(t, result.Q3, result.Q1)
	assert.Equal(t, 1.0, result.Confidence)
	assert.True(t, result.Alert, "repeated high ratios should alert")
}
