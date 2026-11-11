package relay

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// The optional attachment is read as data and sent only to a local capture server.
func TestSonnet55ChatOutbound(t *testing.T) {
	body := []byte(`{"model":"claude-sonnet-5-5","reasoning_effort":"high","stream":true,"messages":[{"role":"system","content":"preserve system"},{"role":"user","content":"preserve user"}],"tools":[{"type":"function","function":{"name":"lookup","description":"local","parameters":{"type":"object","properties":{"x":{"type":"string"}}}}}]}`)
	if path := os.Getenv("NEWAPI_SONNET55_ATTACHMENT"); path != "" {
		var err error
		body, err = os.ReadFile(path)
		require.NoError(t, err)
	}
	var original dto.GeneralOpenAIRequest
	require.NoError(t, common.Unmarshal(body, &original))
	require.Equal(t, "claude-sonnet-5-5", original.Model)
	require.Equal(t, "high", original.ReasoningEffort)
	service.InitHttpClient()
	cases := []struct {
		name, model, mapped              string
		enabled, raw, override, adaptive bool
	}{
		{"on", "claude-sonnet-5-5", "", true, false, false, true},
		{"off", "claude-sonnet-5-5", "", false, false, false, false},
		{"raw", "claude-sonnet-5-5", "", true, true, false, false},
		{"mapping-in", "local-alias", "claude-sonnet-5-5", true, false, false, true},
		{"mapping-out", "claude-sonnet-5-5", "claude-sonnet-4-5", true, false, false, false},
		{"override", "claude-sonnet-5-5", "", true, false, true, true},
		{"old", "claude-sonnet-4-5", "", true, false, false, false},
		{"bedrock-alias", "us.anthropic.claude-sonnet-5-5-v1:0", "", true, false, false, true},
		{"vertex-alias", "claude-sonnet-5-5@20261001", "", true, false, false, true},
		{"future", "claude-sonnet-5-6", "", true, false, false, false},
		{"near-name", "claude-sonnet-5-50", "", true, false, false, false},
	}
	var reference map[string]json.RawMessage
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var sent []byte
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				sent, _ = io.ReadAll(r.Body)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(400)
				_, _ = w.Write([]byte(`{"type":"error","error":{"type":"invalid_request_error","message":"local capture"}}`))
			}))
			defer server.Close()
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest("POST", "/v1/chat/completions", bytes.NewReader(body))
			require.NoError(t, middleware.SetupContextForToken(c, &model.Token{Id: 1, UserId: 1, Key: "local-only", RequestCompatibilityEnabled: &tc.enabled}))
			common.SetContextKey(c, constant.ContextKeyChannelType, constant.ChannelTypeAnthropic)
			common.SetContextKey(c, constant.ContextKeyChannelBaseUrl, server.URL)
			common.SetContextKey(c, constant.ContextKeyOriginalModel, tc.model)
			if tc.mapped != "" {
				mapping, err := common.Marshal(map[string]string{tc.model: tc.mapped})
				require.NoError(t, err)
				c.Set("model_mapping", string(mapping))
			}
			if tc.raw {
				common.SetContextKey(c, constant.ContextKeyChannelSetting, dto.ChannelSettings{PassThroughBodyEnabled: true})
			}
			if tc.override {
				common.SetContextKey(c, constant.ContextKeyChannelParamOverride, map[string]any{"thinking": map[string]any{"type": "enabled", "budget_tokens": 4096}})
			}
			var req dto.GeneralOpenAIRequest
			require.NoError(t, common.Unmarshal(body, &req))
			req.Model = tc.model
			info, err := relaycommon.GenRelayInfo(c, types.RelayFormatOpenAI, &req, nil)
			require.NoError(t, err)
			info.RequestURLPath = c.Request.URL.Path
			info.StartTime = time.Now()
			require.NotNil(t, TextHelper(c, info))
			require.NotEmpty(t, sent)
			if tc.raw {
				require.Equal(t, string(body), string(sent))
				return
			}
			var out map[string]json.RawMessage
			require.NoError(t, common.Unmarshal(sent, &out))
			// Check the original text and every tool's schema survive protocol conversion,
			// not merely that the test cases agree with one another.
			for _, message := range original.Messages {
				if text, ok := message.Content.(string); ok {
					var system, messages any
					if len(out["system"]) > 0 {
						require.NoError(t, common.Unmarshal(out["system"], &system))
					}
					require.NoError(t, common.Unmarshal(out["messages"], &messages))
					encoded, err := common.Marshal([]any{system, messages})
					require.NoError(t, err)
					quoted, err := common.Marshal(text)
					require.NoError(t, err)
					require.Contains(t, string(encoded), string(quoted))
				}
			}
			var tools []struct {
				Name        string         `json:"name"`
				Description string         `json:"description"`
				InputSchema map[string]any `json:"input_schema"`
			}
			require.NoError(t, common.Unmarshal(out["tools"], &tools))
			require.Len(t, tools, len(original.Tools))
			for i, tool := range original.Tools {
				require.Equal(t, tool.Function.Name, tools[i].Name)
				require.Equal(t, tool.Function.Description, tools[i].Description)
				params, ok := tool.Function.Parameters.(map[string]any)
				require.True(t, ok)
				for key, value := range params {
					require.Equal(t, value, tools[i].InputSchema[key], key)
				}
			}
			if reference == nil {
				reference = out
			} else {
				for _, field := range []string{"system", "messages", "tools"} {
					require.JSONEq(t, string(reference[field]), string(out[field]), field)
				}
			}
			if tc.adaptive {
				require.JSONEq(t, `{"type":"adaptive"}`, string(out["thinking"]))
				require.JSONEq(t, `{"effort":"high"}`, string(out["output_config"]))
			} else {
				require.Contains(t, string(out["thinking"]), `"enabled"`)
				require.Contains(t, string(out["thinking"]), `"budget_tokens"`)
			}
		})
	}
}
