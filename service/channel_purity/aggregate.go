package channel_purity

import (
	"math"
	"sort"

	"github.com/QuantumNous/new-api/model"
)

type AggregatePolicy struct {
	MinSamples       int
	AlertWindows     int
	RecoveryWindows  int
	SuspectThreshold float64
	AlertThreshold   float64
}

func DefaultAggregatePolicy() AggregatePolicy {
	return AggregatePolicy{MinSamples: 3, AlertWindows: 3, RecoveryWindows: 2, SuspectThreshold: .72, AlertThreshold: .55}
}

// CompareSamples compares only a target against its own group's baseline.
func CompareSamples(baseline, target []model.ChannelPuritySample) (structure, token, confidence float64, evidence []string) {
	if len(baseline) == 0 || len(target) == 0 {
		return 0, 0, 0, []string{"missing_comparable_samples"}
	}
	bf, tf := signatureFrequency(baseline), signatureFrequency(target)
	var intersection, union int
	keys := map[string]bool{}
	for k := range bf {
		keys[k] = true
	}
	for k := range tf {
		keys[k] = true
	}
	for k := range keys {
		intersection += minInt(bf[k], tf[k])
		union += maxInt(bf[k], tf[k])
	}
	if union > 0 {
		structure = float64(intersection) / float64(union)
	}
	bm, tm := medianTokens(baseline), medianTokens(target)
	if maxInt(bm, tm) == 0 {
		token = 1
	} else {
		token = 1 - math.Abs(float64(bm-tm))/float64(maxInt(bm, tm))
		if token < 0 {
			token = 0
		}
	}
	confidence = math.Min(1, float64(minInt(len(baseline), len(target)))/10)
	if structure < .72 {
		evidence = append(evidence, "structure_distribution_shift")
	}
	if token < .70 {
		evidence = append(evidence, "token_interval_shift")
	}
	return
}
func signatureFrequency(samples []model.ChannelPuritySample) map[string]int {
	out := map[string]int{}
	for _, s := range samples {
		if s.Valid {
			out[s.StructureSignature]++
		}
	}
	return out
}
func medianTokens(samples []model.ChannelPuritySample) int {
	v := make([]int, 0, len(samples))
	for _, s := range samples {
		if s.Valid {
			v = append(v, s.TotalTokens)
		}
	}
	if len(v) == 0 {
		return 0
	}
	sort.Ints(v)
	return v[len(v)/2]
}
func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
