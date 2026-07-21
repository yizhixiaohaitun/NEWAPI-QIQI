package channel_purity

import (
	"math"

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

// CompareSamples compares only samples already matched by RunKey.
func CompareSamples(baseline, target []model.ChannelPuritySample, minimumSamples ...int) (structure, token, confidence float64, evidence []string) {
	minSamples := 5
	if len(minimumSamples) > 0 {
		minSamples = minimumSamples[0]
	}
	if len(baseline) == 0 || len(target) == 0 || len(baseline) != len(target) {
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

	pairs := make([]TokenPair, 0, len(baseline))
	for i := range baseline {
		pair, ok := NewTokenPair(actualModelFamily(baseline[i], target[i]), "purity_probe", target[i].TotalTokens, baseline[i].TotalTokens, target[i].TotalTokens, baseline[i].TotalTokens)
		if ok {
			pairs = append(pairs, pair)
		}
	}
	modelFamily := ""
	if len(pairs) > 0 {
		modelFamily = pairs[0].ModelFamily
	}
	interval := AnalyzeTokenRatios(modelFamily, "purity_probe", pairs, minSamples)
	if interval.Samples == 0 {
		token = 0
	} else {
		token = 1 - math.Abs(interval.Median-1)/math.Max(1, interval.Median)
		if interval.Alert {
			outside := 0
			for _, pair := range pairs {
				if pair.Ratio < interval.Lower || pair.Ratio > interval.Upper {
					outside++
				}
			}
			// Repeated robust-interval outliers must affect the formal score,
			// rather than merely being attached as evidence beside a median score.
			token *= math.Max(0, 1-3*float64(outside)/float64(len(pairs)))
		}
		if token < 0 {
			token = 0
		}
	}
	confidence = math.Min(1, float64(len(baseline))/10)
	if structure < .72 {
		evidence = append(evidence, "structure_distribution_shift")
	}
	if interval.Samples > 0 && (token < .70 || interval.Alert) {
		evidence = append(evidence, "token_interval_shift")
	}
	return
}

func actualModelFamily(baseline, target model.ChannelPuritySample) string {
	if baseline.ActualModel == target.ActualModel {
		return baseline.ActualModel
	}
	return ""
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
