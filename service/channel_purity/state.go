package channel_purity

import "github.com/QuantumNous/new-api/model"

type WindowResult struct {
	BaselineAvailable bool
	DetectorError     bool
	BaselineSamples   int
	TargetSamples     int
	Similarity        float64
	Confidence        float64
}

// Advance applies consecutive-window debounce and slower recovery. Assessments
// are keyed externally by group+target channel+actual model; no group score exists.
func Advance(previous model.ChannelPurityAssessment, window WindowResult, policy AggregatePolicy) model.ChannelPurityAssessment {
	next := previous
	next.Confidence = window.Confidence
	if window.DetectorError {
		next.State = model.ChannelPurityStateDetectorError
		return next
	}
	if !window.BaselineAvailable {
		next.State = model.ChannelPurityStateBaselineUnavailable
		next.ConsecutiveAnomalies = 0
		next.ConsecutiveHealthy = 0
		return next
	}
	if window.BaselineSamples < policy.MinSamples || window.TargetSamples < policy.MinSamples {
		next.State = model.ChannelPurityStateLowSample
		next.ConsecutiveAnomalies = 0
		next.ConsecutiveHealthy = 0
		return next
	}
	anomalous := window.Similarity < policy.SuspectThreshold
	severe := window.Similarity < policy.AlertThreshold
	if anomalous {
		next.ConsecutiveAnomalies++
		next.ConsecutiveHealthy = 0
		if severe && next.ConsecutiveAnomalies >= policy.AlertWindows {
			next.State = model.ChannelPurityStateAlert
		} else {
			next.State = model.ChannelPurityStateSuspect
		}
		return next
	}
	next.ConsecutiveHealthy++
	next.ConsecutiveAnomalies = 0
	if previous.State == model.ChannelPurityStateAlert || previous.State == model.ChannelPurityStateSuspect {
		if next.ConsecutiveHealthy < policy.RecoveryWindows {
			next.State = previous.State
			return next
		}
	}
	if previous.State == "" || previous.State == model.ChannelPurityStateLowSample || previous.State == model.ChannelPurityStateBaselineUnavailable {
		next.State = model.ChannelPurityStateWarmingUp
	} else {
		next.State = model.ChannelPurityStateHealthy
	}
	return next
}
