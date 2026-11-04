package service

import (
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRelayCompatibilitySignatureReference(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/messages", nil)
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{}}
	before := GenerateTextOtherInfo(c, info, 1, 1, 1, 0, 1, 0, 1)
	require.NotContains(t, before["admin_info"], "compatibility_events")

	event := NewRelayCompatibilityEvent(ClaudeHistoricalThinkingSignatureRetryRule, RelayCompatibilityEventTypeApplied)
	event.Action = "strip_historical_thinking_and_retry"
	event.Outcome = "retry_started"
	event.Retried = true
	RecordRelayCompatibilityEvent(c, event)
	RecordRelayCompatibilityEvent(c, event)
	other := GenerateTextOtherInfo(c, info, 1, 1, 1, 0, 1, 0, 1)
	admin, ok := other["admin_info"].(map[string]interface{})
	require.True(t, ok)
	require.Equal(t, []RelayCompatibilityEvent{event}, admin["compatibility_events"])
	data, err := common.Marshal(admin["compatibility_events"])
	require.NoError(t, err)
	assert.JSONEq(t, `[{"rule_id":"QIQI-EC-005","key":"claude_historical_thinking_signature_retry","setting_key":"request_compatibility_enabled","event_type":"applied","action":"strip_historical_thinking_and_retry","outcome":"retry_started","retried":true}]`, string(data))
	// No block count or recovery result is invented; no request/signature is logged.
	assert.NotContains(t, string(data), "accepted")
	assert.NotContains(t, string(data), "count")
}

func TestRelayCompatibilityEmptyReference(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	RecordRelayCompatibilityEvent(nil, RelayCompatibilityEvent{})
	RecordRelayCompatibilityEvent(c, RelayCompatibilityEvent{})
	admin := map[string]interface{}{}
	AppendRelayCompatibilityAdminInfo(nil, admin)
	AppendRelayCompatibilityAdminInfo(c, nil)
	AppendRelayCompatibilityAdminInfo(c, admin)
	assert.Empty(t, admin)
}
