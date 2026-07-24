package service

import (
	"fmt"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
)

const (
	ProbeStatusMatch          = "match"
	ProbeStatusMismatch       = "mismatch"
	ProbeStatusUnknown        = "unknown"
	ProbeConclusionPassed     = "passed"
	ProbeConclusionSuspicious = "suspicious"
	ProbeConclusionFailed     = "failed"
	ProbeConclusionUnknown    = "unknown"

	modelProbeNotifyCooldown = 30 * time.Minute
)

var modelProbeNotifyState = struct {
	sync.Mutex
	last map[string]time.Time
}{last: make(map[string]time.Time)}

var modelProbeNotifier = NotifyRootUser

func NormalizeProbeModelID(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	for _, prefix := range []string{"models/", "openai/"} {
		value = strings.TrimPrefix(value, prefix)
	}
	return value
}

func ModelProbeTolerance(expected int) int {
	if expected <= 0 {
		return 0
	}
	return max(3, int(math.Ceil(float64(expected)*0.05)))
}

func EvaluateActiveModelProbe(result *model.ModelProbeResult) {
	if result == nil {
		return
	}
	declared := NormalizeProbeModelID(result.DeclaredModel)
	actual := NormalizeProbeModelID(result.ActualModel)
	switch {
	case declared == "" || actual == "":
		result.IdStatus = ProbeStatusUnknown
	case declared == actual:
		result.IdStatus = ProbeStatusMatch
	default:
		result.IdStatus = ProbeStatusMismatch
	}

	if result.ExpectedTokens == nil || result.ActualTokens == nil {
		result.TokenStatus = ProbeStatusUnknown
	} else {
		delta := *result.ActualTokens - *result.ExpectedTokens
		result.TokenDelta = &delta
		result.TokenTolerance = ModelProbeTolerance(*result.ExpectedTokens)
		if absProbeInt(delta) <= result.TokenTolerance {
			result.TokenStatus = ProbeStatusMatch
		} else {
			result.TokenStatus = ProbeStatusMismatch
		}
	}

	switch {
	case result.Error != "":
		result.Conclusion = ProbeConclusionFailed
	case result.IdStatus == ProbeStatusMismatch || result.TokenStatus == ProbeStatusMismatch:
		result.Conclusion = ProbeConclusionSuspicious
	case result.IdStatus == ProbeStatusMatch && result.TokenStatus == ProbeStatusMatch:
		result.Conclusion = ProbeConclusionPassed
	default:
		result.Conclusion = ProbeConclusionUnknown
	}
}

func StoreAndNotifyModelProbe(result *model.ModelProbeResult) error {
	EvaluateActiveModelProbe(result)
	if err := model.CreateModelProbeResult(result); err != nil {
		return err
	}
	if result.Conclusion != ProbeConclusionSuspicious && result.Conclusion != ProbeConclusionFailed {
		return nil
	}

	key := fmt.Sprintf("%d:%s:%s", result.ChannelId, NormalizeProbeModelID(result.DeclaredModel), result.Conclusion)
	now := time.Now()
	modelProbeNotifyState.Lock()
	last := modelProbeNotifyState.last[key]
	if !last.IsZero() && now.Sub(last) < modelProbeNotifyCooldown {
		modelProbeNotifyState.Unlock()
		return nil
	}
	modelProbeNotifyState.last[key] = now
	modelProbeNotifyState.Unlock()

	content := fmt.Sprintf("渠道 #%d %s 的模型抽检结果为 %s；声明模型=%s，实际模型=%s，ID=%s，Token=%s。", result.ChannelId, result.ChannelName, result.Conclusion, result.DeclaredModel, result.ActualModel, result.IdStatus, result.TokenStatus)
	// This is an explicit admin action, so invoke the existing notification
	// chain immediately after persistence. Avoid spawning an unbounded goroutine.
	modelProbeNotifier(dto.NotifyTypeChannelTest, "模型抽检异常", content)
	return nil
}

func absProbeInt(value int) int {
	if value < 0 {
		return -value
	}
	return value
}

func LogModelProbeStoreError(err error) {
	if err != nil {
		common.SysError("failed to store model probe result: " + err.Error())
	}
}
