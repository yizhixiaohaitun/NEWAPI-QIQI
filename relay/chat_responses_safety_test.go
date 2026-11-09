package relay

import (
	"net/http/httptest"
	"strings"
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestSafeChatResponsesBridge(t *testing.T) {
	tests := []struct {
		body string
		safe bool
	}{
		{`{"model":"m","messages":[{"role":"user","content":"hello"}],"stream":true,"stream_options":{"include_usage":true}}`, true},
		{`{"model":"m","prompt_cache_key":"cache-会话","messages":[{"role":"system","content":"a"},{"role":"user","content":"b"}]}`, true},
		{`{"model":"m","messages":[{"role":"user","content":[{"type":"image_url","image_url":{"url":"x","detail":"high"}}]}]}`, false},
		{`{"model":"m","messages":[{"role":"user","content":"x"}],"tools":[]}`, false},
		{`{"model":"m","messages":[{"role":"user","content":"x"}],"unknown":1}`, false},
		{`{"model":"m","messages":[{"role":"user","content":"x"}],"max_tokens":1,"max_completion_tokens":2}`, false},
	}
	for _, tt := range tests {
		t.Run(tt.body, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(tt.body))
			require.Equal(t, tt.safe, safeChatResponsesBridge(c, &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{}}))
		})
	}
}

func TestHeaderOnlyOverrides(t *testing.T) {
	require.True(t, headerOnlyOverrides(map[string]interface{}{"operations": []map[string]interface{}{{"mode": "pass_headers", "value": []string{"session_id"}, "keep_origin": true}}}))
	require.False(t, headerOnlyOverrides(map[string]interface{}{"operations": []map[string]interface{}{{"mode": "set", "path": "messages", "value": "lost"}}}))
	require.False(t, headerOnlyOverrides(map[string]interface{}{"temperature": 0.5}))
}
