package relay

import (
	"bytes"
	"encoding/json"
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
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestOpus55DisabledOutboundHTTP(t *testing.T) {
	service.InitHttpClient()
	for _, stream := range []bool{false, true} {
		for _, mode := range []string{"claude", "chat", "passthrough"} {
			t.Run(mode+"/"+common.Interface2String(stream), func(t *testing.T) {
				var sent []byte
				upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					sent, _ = io.ReadAll(r.Body)
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(400)
					_, _ = w.Write([]byte(`{"type":"error","error":{"type":"invalid_request_error","message":"local capture only"}}`))
				}))
				defer upstream.Close()
				body := []byte(`{"model":"mapped-alias","max_tokens":64000,"thinking":{"type":"disabled","budget_tokens":4096},"output_config":{"effort":"high","extension":9007199254740993},"messages":[{"role":"user","content":"unchanged"}]}`)
				c, _ := gin.CreateTestContext(httptest.NewRecorder())
				c.Request = httptest.NewRequest("POST", "/v1/messages", bytes.NewReader(body))
				common.SetContextKey(c, constant.ContextKeyChannelType, constant.ChannelTypeAnthropic)
				common.SetContextKey(c, constant.ContextKeyChannelBaseUrl, upstream.URL)
				common.SetContextKey(c, constant.ContextKeyOriginalModel, "mapped-alias")
				override := map[string]any{"thinking": map[string]any{"type": "disabled", "budget_tokens": 4096}}
				common.SetContextKey(c, constant.ContextKeyChannelParamOverride, override)
				c.Set("model_mapping", `{"mapped-alias":"claude-opus-5-5"}`)
				info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ApiType: constant.APITypeAnthropic, ChannelType: constant.ChannelTypeAnthropic, ChannelBaseUrl: upstream.URL, UpstreamModelName: "mapped-alias", ApiKey: "local-only", ParamOverride: override}, OriginModelName: "mapped-alias", StartTime: time.Now(), RequestURLPath: "/v1/messages", IsStream: stream, RelayFormat: types.RelayFormatClaude}
				var result *types.NewAPIError
				if mode == "chat" {
					info.RelayFormat = types.RelayFormatOpenAI
					info.RequestURLPath = "/v1/chat/completions"
					info.Request = &dto.GeneralOpenAIRequest{Model: "mapped-alias", MaxTokens: common.GetPointer[uint](64000), Stream: common.GetPointer(stream), ReasoningEffort: "none", Messages: []dto.Message{{Role: "user", Content: "unchanged"}}}
					result = TextHelper(c, info)
				} else {
					var req dto.ClaudeRequest
					require.NoError(t, common.Unmarshal(body, &req))
					req.Stream = common.GetPointer(stream)
					info.Request = &req
					if mode == "passthrough" {
						common.SetContextKey(c, constant.ContextKeyChannelSetting, dto.ChannelSettings{PassThroughBodyEnabled: true})
					}
					result = ClaudeHelper(c, info)
				}
				require.NotNil(t, result)
				require.NotEmpty(t, sent)
				if mode == "passthrough" {
					assert.Equal(t, string(body), string(sent))
					return
				}
				var fields map[string]json.RawMessage
				require.NoError(t, common.Unmarshal(sent, &fields))
				assert.JSONEq(t, `{"type":"adaptive"}`, string(fields["thinking"]))
				assert.Equal(t, "64000", string(fields["max_tokens"]))
				assert.Contains(t, string(fields["messages"]), "unchanged")
				if mode == "claude" {
					assert.JSONEq(t, `{"effort":"high","extension":9007199254740993}`, string(fields["output_config"]))
				} else {
					assert.JSONEq(t, `{"effort":"low"}`, string(fields["output_config"]))
				}
				t.Logf("captured %s stream=%t: adaptive, no budget, max_tokens=64000", mode, stream)
			})
		}
	}
}

func TestOpus55DisabledAWSVertexFinalOverride(t *testing.T) {
	for _, stream := range []bool{false, true} {
		for _, kind := range []string{"aws", "vertex"} {
			t.Run(kind+"/"+common.Interface2String(stream), func(t *testing.T) {
				model := "us.anthropic.claude-opus-5-5-v1:0"
				if kind == "vertex" {
					model = "claude-opus-5-5@20261001"
				}
				req := &dto.ClaudeRequest{Model: model, Stream: common.GetPointer(stream), MaxTokens: common.GetPointer[uint](64000), Thinking: &dto.Thinking{Type: "disabled"}, OutputConfig: json.RawMessage(`{"effort":"medium","extension":9007199254740993}`), Messages: []dto.ClaudeMessage{{Role: "user", Content: "unchanged"}}}
				info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: model}}
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
				body, err = relaycommon.ApplyParamOverride(body, map[string]any{"thinking": map[string]any{"type": "disabled", "budget_tokens": 4096}}, nil)
				require.NoError(t, err)
				body, err = normalizeClaudeOutboundJSON(body, info, converted)
				require.NoError(t, err)
				var fields map[string]json.RawMessage
				require.NoError(t, common.Unmarshal(body, &fields))
				assert.JSONEq(t, `{"type":"adaptive"}`, string(fields["thinking"]))
				assert.JSONEq(t, `{"effort":"medium","extension":9007199254740993}`, string(fields["output_config"]))
				assert.Equal(t, "64000", string(fields["max_tokens"]))
				assert.Contains(t, string(fields["messages"]), "unchanged")
			})
		}
	}
}

func TestOpus55DisabledChatResponsesConverters(t *testing.T) {
	for _, model := range []string{"claude-opus-5-5", "claude-opus-5-5-high", "claude-opus-4-7", "claude-opus-4-8"} {
		for _, stream := range []bool{false, true} {
			chat, err := relayconvert.OpenAIChatRequestToClaudeMessages(nil, dto.GeneralOpenAIRequest{Model: model, Stream: common.GetPointer(stream), ReasoningEffort: "none", MaxTokens: common.GetPointer[uint](64000)})
			require.NoError(t, err)
			response, err := relayconvert.OpenAIResponsesRequestToClaudeMessages(nil, &dto.OpenAIResponsesRequest{Model: model, Stream: common.GetPointer(stream), Reasoning: &dto.Reasoning{Effort: "none"}, MaxOutputTokens: common.GetPointer[uint](64000), Input: json.RawMessage(`"unchanged"`)})
			require.NoError(t, err)
			for _, req := range []*dto.ClaudeRequest{chat, response} {
				expected := "disabled"
				if model == "claude-opus-5-5" || model == "claude-opus-5-5-high" {
					expected = "adaptive"
					assert.JSONEq(t, `{"effort":"low"}`, string(req.OutputConfig))
				}
				assert.Equal(t, expected, req.Thinking.Type, model)
				assert.Nil(t, req.Thinking.BudgetTokens)
				assert.Equal(t, uint(64000), *req.MaxTokens)
				body, err := common.Marshal(req)
				require.NoError(t, err)
				out, err := dto.NormalizeClaudeAdaptiveJSON(body, req.Model)
				require.NoError(t, err)
				assert.JSONEq(t, string(body), string(out))
			}
		}
	}
}
