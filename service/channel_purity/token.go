package channel_purity

import (
	"math"
	"sort"
)

type TokenPair struct {
	ModelFamily      string  `json:"model_family"`
	RequestType      string  `json:"request_type"`
	TargetProvider   int     `json:"target_provider"`
	BaselineProvider int     `json:"baseline_provider"`
	TargetUnified    int     `json:"target_unified"`
	BaselineUnified  int     `json:"baseline_unified"`
	Ratio            float64 `json:"ratio"`
}

type TokenInterval struct {
	ModelFamily string  `json:"model_family"`
	RequestType string  `json:"request_type"`
	Samples     int     `json:"samples"`
	Median      float64 `json:"median"`
	MAD         float64 `json:"mad"`
	Q1          float64 `json:"q1"`
	Q3          float64 `json:"q3"`
	Lower       float64 `json:"lower"`
	Upper       float64 `json:"upper"`
	Confidence  float64 `json:"confidence"`
	Alert       bool    `json:"alert"`
}

func NewTokenPair(modelFamily, requestType string, targetProvider, baselineProvider, targetUnified, baselineUnified int) (TokenPair, bool) {
	if baselineUnified <= 0 || targetUnified < 0 {
		return TokenPair{}, false
	}
	return TokenPair{ModelFamily: modelFamily, RequestType: requestType, TargetProvider: targetProvider, BaselineProvider: baselineProvider, TargetUnified: targetUnified, BaselineUnified: baselineUnified, Ratio: float64(targetUnified) / float64(baselineUnified)}, true
}

// AnalyzeTokenRatios compares only an identical model family and request type.
// Alerting requires repeated evidence; one outlier can never alert.
func AnalyzeTokenRatios(modelFamily, requestType string, pairs []TokenPair, minSamples int) TokenInterval {
	if minSamples < 5 {
		minSamples = 5
	}
	values := make([]float64, 0, len(pairs))
	for _, pair := range pairs {
		if pair.ModelFamily == modelFamily && pair.RequestType == requestType && pair.Ratio >= 0 && !math.IsNaN(pair.Ratio) && !math.IsInf(pair.Ratio, 0) {
			values = append(values, pair.Ratio)
		}
	}
	sort.Float64s(values)
	result := TokenInterval{ModelFamily: modelFamily, RequestType: requestType, Samples: len(values)}
	if len(values) == 0 {
		return result
	}
	result.Median = quantile(values, .5)
	result.Q1, result.Q3 = quantile(values, .25), quantile(values, .75)
	deviations := make([]float64, len(values))
	for i, value := range values {
		deviations[i] = math.Abs(value - result.Median)
	}
	sort.Float64s(deviations)
	result.MAD = quantile(deviations, .5)
	spread := 1.4826 * result.MAD
	if spread <= 0 {
		spread = (result.Q3 - result.Q1) / 1.349
	}
	if spread < .02 {
		spread = .02
	}
	iqrLower := result.Q1 - 1.5*(result.Q3-result.Q1)
	iqrUpper := result.Q3 + 1.5*(result.Q3-result.Q1)
	result.Lower = math.Max(0, math.Max(iqrLower, result.Median-3*spread))
	result.Upper = math.Min(iqrUpper, result.Median+3*spread)
	result.Confidence = math.Min(1, float64(len(values))/float64(minSamples*2))
	if len(values) >= minSamples {
		outside := 0
		for _, value := range values {
			if value < result.Lower || value > result.Upper {
				outside++
			}
		}
		result.Alert = outside >= 2 && float64(outside)/float64(len(values)) >= .2
	}
	return result
}

func quantile(sorted []float64, q float64) float64 {
	if len(sorted) == 1 {
		return sorted[0]
	}
	position := q * float64(len(sorted)-1)
	lower := int(math.Floor(position))
	upper := int(math.Ceil(position))
	if lower == upper {
		return sorted[lower]
	}
	weight := position - float64(lower)
	return sorted[lower]*(1-weight) + sorted[upper]*weight
}
