package model

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetContextLogDecisionGlobalDisabledOverridesCaptureRule(t *testing.T) {
	setting := operation_setting.GetQiqiSetting()
	previous := setting.ContextRequestLoggingEnabled
	setting.ContextRequestLoggingEnabled = false
	t.Cleanup(func() { setting.ContextRequestLoggingEnabled = previous })
	contextLogRuleCache.Lock()
	oldRules, oldLoadedAt := contextLogRuleCache.rules, contextLogRuleCache.loadedAt
	contextLogRuleCache.rules = []ContextRequestLogRule{{Id: 1, Decision: ContextLogDecisionCapture, Enabled: true}}
	contextLogRuleCache.loadedAt = time.Now()
	contextLogRuleCache.Unlock()
	t.Cleanup(func() {
		contextLogRuleCache.Lock()
		contextLogRuleCache.rules, contextLogRuleCache.loadedAt = oldRules, oldLoadedAt
		contextLogRuleCache.Unlock()
	})
	decision := GetContextLogDecision(7, "gpt-4o")
	require.False(t, decision.Capture)
	assert.Equal(t, "global_disabled", decision.Source)
}
