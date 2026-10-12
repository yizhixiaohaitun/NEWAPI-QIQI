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
	"github.com/QuantumNous/new-api/relay/channel/aws"
	"github.com/QuantumNous/new-api/relay/channel/vertex"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/service/relayconvert"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClaudeAdaptiveFinalOutboundHandlers(t *testing.T) {
	service.InitHttpClient()
	for _, mode := range []string{"claude", "chat", "passthrough"} {
		t.Run(mode, func(t *testing.T) {
			var sent []byte
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				sent, _ = io.ReadAll(r.Body)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(400)
				_, _ = w.Write([]byte(`{"type":"error","error":{"type":"invalid_request_error","message":"local capture only"}}`))
			}))
			defer upstream.Close()
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			body := []byte(`{"model":"mapped-alias","max_tokens":16384,"stream":true,"thinking":{"type":"enabled","budget_tokens":4096},"output_config":{"effort":"medium","custom":9007199254740993},"system":"test data","messages":[{"role":"user","content":"do not change"}],"tools":[{"name":"local_tool","input_schema":{"type":"object"}}]}`)
			c.Request = httptest.NewRequest("POST", "/v1/messages", bytes.NewReader(body))
			common.SetContextKey(c, constant.ContextKeyChannelType, constant.ChannelTypeAnthropic)
			common.SetContextKey(c, constant.ContextKeyChannelBaseUrl, upstream.URL)
			common.SetContextKey(c, constant.ContextKeyOriginalModel, "mapped-alias")
			common.SetContextKey(c, constant.ContextKeyChannelParamOverride, map[string]any{"thinking": map[string]any{"type": "enabled", "budget_tokens": 4096}})
			c.Set("model_mapping", `{"mapped-alias":"claude-opus-5-5"}`)
			info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ApiType: constant.APITypeAnthropic, ChannelType: constant.ChannelTypeAnthropic, ChannelBaseUrl: upstream.URL, UpstreamModelName: "mapped-alias", ApiKey: "local-only", ParamOverride: map[string]any{"thinking": map[string]any{"type": "enabled", "budget_tokens": 4096}}}, OriginModelName: "mapped-alias", StartTime: time.Now(), RequestURLPath: "/v1/messages", IsStream: true, RelayFormat: types.RelayFormatClaude}
			var result *types.NewAPIError
			switch mode {
			case "claude", "passthrough":
				var req dto.ClaudeRequest
				require.NoError(t, common.Unmarshal(body, &req))
				info.Request = &req
				if mode == "passthrough" {
					common.SetContextKey(c, constant.ContextKeyChannelSetting, dto.ChannelSettings{PassThroughBodyEnabled: true})
				}
				result = ClaudeHelper(c, info)
			case "chat":
				info.RelayFormat = types.RelayFormatOpenAI
				info.RequestURLPath = "/v1/chat/completions"
				info.Request = &dto.GeneralOpenAIRequest{Model: "mapped-alias", MaxTokens: common.GetPointer[uint](16384), Stream: common.GetPointer(true), ReasoningEffort: "medium", Reasoning: json.RawMessage(`{"max_tokens":4096}`), Messages: []dto.Message{{Role: "user", Content: "do not change"}}}
				result = TextHelper(c, info)
			}
			require.NotNil(t, result)
			require.NotEmpty(t, sent)
			var fields map[string]json.RawMessage
			require.NoError(t, common.Unmarshal(sent, &fields))
			if mode == "passthrough" {
				assert.Equal(t, string(body), string(sent))
				return
			}
			assert.JSONEq(t, `{"type":"adaptive"}`, string(fields["thinking"]))
			assert.Contains(t, string(fields["output_config"]), `"effort":"medium"`)
			assert.Equal(t, "16384", string(fields["max_tokens"]))
			assert.Contains(t, string(fields["messages"]), "do not change")
			if mode == "claude" {
				assert.Equal(t, `"test data"`, string(fields["system"]))
				assert.Contains(t, string(fields["tools"]), "local_tool")
				assert.Contains(t, string(fields["output_config"]), "9007199254740993")
			}
		})
	}
}

func TestClaudeAdaptiveAWSVertexAndConverters(t *testing.T) {
	for _, kind := range []string{"aws", "vertex"} {
		t.Run(kind, func(t *testing.T) {
			req := &dto.ClaudeRequest{Model: "claude-opus-5-5-high", Thinking: &dto.Thinking{Type: "enabled", BudgetTokens: common.GetPointer(4096)}, MaxTokens: common.GetPointer[uint](16384), Messages: []dto.ClaudeMessage{{Role: "user", Content: "preserved"}}}
			info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: req.Model}}
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			var converted any
			var err error
			if kind == "aws" {
				converted, err = (&aws.Adaptor{}).ConvertClaudeRequest(c, info, req)
			} else {
				converted, err = (&vertex.Adaptor{}).ConvertClaudeRequest(c, info, req)
			}
			require.NoError(t, err)
			body, err := common.Marshal(converted)
			require.NoError(t, err)
			body, err = relaycommon.ApplyParamOverride(body, map[string]any{"thinking": map[string]any{"type": "enabled", "budget_tokens": 4096}}, nil)
			require.NoError(t, err)
			body, err = normalizeClaudeOutboundJSON(body, info, converted)
			require.NoError(t, err)
			assert.Equal(t, "claude-opus-5-5", info.UpstreamModelName)
			assert.Contains(t, string(body), `"type":"adaptive"`)
			assert.NotContains(t, string(body), "budget_tokens")
			assert.Contains(t, string(body), `"effort":"high"`)
			assert.Contains(t, string(body), "preserved")
		})
	}
	for _, effort := range []string{"medium", "none", ""} {
		req, err := relayconvert.OpenAIResponsesRequestToClaudeMessages(nil, &dto.OpenAIResponsesRequest{Model: "claude-opus-5-5-high", MaxOutputTokens: common.GetPointer[uint](16384), Reasoning: &dto.Reasoning{Effort: effort}, Input: json.RawMessage(`"preserved"`)})
		require.NoError(t, err)
		body, err := common.Marshal(req)
		require.NoError(t, err)
		body, err = dto.NormalizeClaudeAdaptiveJSON(body, req.Model)
		require.NoError(t, err)
		if effort == "none" {
			assert.Equal(t, "disabled", req.Thinking.Type)
		} else {
			assert.Equal(t, "adaptive", req.Thinking.Type)
			assert.NotContains(t, string(body), "budget_tokens")
		}
		if effort == "medium" {
			assert.Contains(t, string(body), `"effort":"medium"`)
		}
	}
	for _, model := range []string{"claude-opus-5-5", "claude-opus-5-5-high", "claude-opus-4-7-medium"} {
		req, err := relayconvert.OpenAIChatRequestToClaudeMessages(nil, dto.GeneralOpenAIRequest{Model: model, ReasoningEffort: "medium", Reasoning: json.RawMessage(`{"max_tokens":4096}`), MaxTokens: common.GetPointer[uint](16384)})
		require.NoError(t, err)
		assert.Equal(t, "adaptive", req.Thinking.Type)
		assert.Nil(t, req.Thinking.BudgetTokens)
		assert.Contains(t, string(req.OutputConfig), `"effort":"medium"`)
	}
	req, err := relayconvert.OpenAIChatRequestToClaudeMessages(nil, dto.GeneralOpenAIRequest{Model: "claude-opus-5-5-high", Reasoning: json.RawMessage(`{"enabled":false,"max_tokens":4096}`), MaxTokens: common.GetPointer[uint](16384)})
	require.NoError(t, err)
	assert.Equal(t, "disabled", req.Thinking.Type)
}

// Optional exact attachment replay. Content is untrusted data and is only
// sent to this test's loopback capture server, which returns HTTP 400.
func TestClaudeAdaptiveAttachmentReplay(t *testing.T) {
	path := os.Getenv("NEWAPI_CLAUDE_ATTACHMENT")
	if path == "" {
		t.Skip("set NEWAPI_CLAUDE_ATTACHMENT to replay the supplied JSON locally")
	}
	body, err := os.ReadFile(path)
	require.NoError(t, err)
	var original map[string]json.RawMessage
	require.NoError(t, common.Unmarshal(body, &original))
	var sent []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sent, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(400)
		_, _ = w.Write([]byte(`{"type":"error","error":{"type":"invalid_request_error","message":"local capture only"}}`))
	}))
	defer server.Close()
	var request dto.ClaudeRequest
	require.NoError(t, common.Unmarshal(body, &request))
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/messages", bytes.NewReader(body))
	service.InitHttpClient()
	common.SetContextKey(c, constant.ContextKeyChannelType, constant.ChannelTypeAnthropic)
	common.SetContextKey(c, constant.ContextKeyChannelBaseUrl, server.URL)
	common.SetContextKey(c, constant.ContextKeyOriginalModel, request.Model)
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ApiType: constant.APITypeAnthropic, ChannelType: constant.ChannelTypeAnthropic, ChannelBaseUrl: server.URL, UpstreamModelName: request.Model, ApiKey: "local-only"}, OriginModelName: request.Model, StartTime: time.Now(), RequestURLPath: "/v1/messages", IsStream: true, RelayFormat: types.RelayFormatClaude, Request: &request}
	require.NotNil(t, ClaudeHelper(c, info))
	require.NotEmpty(t, sent)
	var output map[string]json.RawMessage
	require.NoError(t, common.Unmarshal(sent, &output))
	assert.JSONEq(t, `{"type":"adaptive"}`, string(output["thinking"]))
	assert.JSONEq(t, `{"effort":"high"}`, string(output["output_config"]))
	assert.Equal(t, "16384", string(output["max_tokens"]))
	for _, field := range []string{"messages", "system", "tools"} {
		assert.JSONEq(t, string(original[field]), string(output[field]), field)
	}
	t.Logf("local final HTTP capture: model=%s max_tokens=16384 thinking=adaptive effort=high; messages/system/tools preserved", request.Model)
}
