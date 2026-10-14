package relay

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel/aws"
	"github.com/QuantumNous/new-api/relay/channel/vertex"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/service/relayconvert"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

const compatibilityUserJSON = `{"frequency_penalty":0.78,"max_tokens":30000,"messages":[{"content":"Hi","role":"user"}],"model":"claude-opus-4-7","presence_penalty":0,"stream":false,"temperature":0.96,"top_p":0}`

// Real HTTP capture; no model calls. SetupContextForToken -> GenRelayInfo ->
// handler -> converters -> adapter -> final guard. Workers use independent SKs.
func TestRequestCompatibilityHTTP(t *testing.T) {
	service.InitHttpClient()
	for _, enabled := range []bool{true, false} {
		for _, stream := range []bool{false, true} {
			for _, mode := range []string{"chat", "openai", "openai-mapped", "openai-override", "openai-passthrough", "claude", "responses", "responses-mapped", "responses-override", "responses-passthrough", "chatresponses", "mapped", "override", "passthrough"} {
				enabled, stream, mode := enabled, stream, mode
				t.Run(mode+"/on="+common.Interface2String(enabled)+"/stream="+common.Interface2String(stream), func(t *testing.T) {
					var sent []byte
					upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						sent, _ = io.ReadAll(r.Body)
						w.Header().Set("Content-Type", "application/json")
						w.WriteHeader(400)
						_, _ = w.Write([]byte(`{"type":"error","error":{"type":"invalid_request_error","message":"local capture only"}}`))
					}))
					defer upstream.Close()
					c, _ := gin.CreateTestContext(httptest.NewRecorder())
					body := []byte(compatibilityUserJSON)
					c.Request = httptest.NewRequest("POST", "/v1/chat/completions", bytes.NewReader(body))
					require.NoError(t, middleware.SetupContextForToken(c, &model.Token{Id: 1, UserId: 1, Key: "local-only", RequestCompatibilityEnabled: &enabled}))
					channelType := constant.ChannelTypeAnthropic
					if strings.HasPrefix(mode, "responses") || strings.HasPrefix(mode, "openai") || mode == "chatresponses" {
						channelType = constant.ChannelTypeOpenAI
					}
					common.SetContextKey(c, constant.ContextKeyChannelType, channelType)
					common.SetContextKey(c, constant.ContextKeyChannelBaseUrl, upstream.URL)
					original := "claude-opus-4-7"
					if strings.Contains(mode, "mapped") {
						original = "local-alias"
						c.Set("model_mapping", `{"local-alias":"claude-opus-4-7"}`)
					}
					common.SetContextKey(c, constant.ContextKeyOriginalModel, original)
					if strings.Contains(mode, "override") {
						common.SetContextKey(c, constant.ContextKeyChannelParamOverride, map[string]any{"temperature": 0.96, "top_p": 0, "top_k": 7})
					}
					if mode == "override" {
						common.SetContextKey(c, constant.ContextKeyChannelParamOverride, map[string]any{"temperature": 0.96, "top_p": 0, "top_k": 7, "thinking": map[string]any{"type": "enabled", "budget_tokens": 4096}})
					}
					if strings.HasSuffix(mode, "passthrough") {
						common.SetContextKey(c, constant.ContextKeyChannelSetting, dto.ChannelSettings{PassThroughBodyEnabled: true})
					}
					var req dto.Request
					format := types.RelayFormatOpenAI
					switch mode {
					case "claude", "passthrough":
						r := &dto.ClaudeRequest{}
						require.NoError(t, json.Unmarshal(body, r))
						r.Stream = &stream
						req = r
						format = types.RelayFormatClaude
						if mode == "passthrough" {
							common.SetContextKey(c, constant.ContextKeyChannelSetting, dto.ChannelSettings{PassThroughBodyEnabled: true})
						}
					case "responses", "responses-mapped", "responses-override", "responses-passthrough":
						req = &dto.OpenAIResponsesRequest{Model: original, Input: json.RawMessage(`"Hi"`), MaxOutputTokens: common.GetPointer[uint](30000), Stream: &stream, Temperature: common.GetPointer(.96), TopP: common.GetPointer(0.0)}
						format = types.RelayFormatOpenAIResponses
					default:
						r := &dto.GeneralOpenAIRequest{}
						require.NoError(t, json.Unmarshal(body, r))
						r.Model = original
						r.Stream = &stream
						req = r
					}
					info, err := relaycommon.GenRelayInfo(c, format, req, nil)
					require.NoError(t, err)
					info.RequestURLPath = c.Request.URL.Path
					info.StartTime = time.Now()
					var result *types.NewAPIError
					switch format {
					case types.RelayFormatClaude:
						result = ClaudeHelper(c, info)
					case types.RelayFormatOpenAIResponses:
						result = ResponsesHelper(c, info)
					default:
						if mode == "chatresponses" {
							info.InitChannelMeta(c)
							adaptor := GetAdaptor(info.ApiType)
							adaptor.Init(info)
							_, result = chatCompletionsViaResponses(c, info, adaptor, req.(*dto.GeneralOpenAIRequest))
						} else {
							result = TextHelper(c, info)
						}
					}
					require.NotNil(t, result)
					require.NotEmpty(t, sent)
					if strings.HasSuffix(mode, "passthrough") {
						require.Equal(t, string(body), string(sent))
						return
					}
					var fields map[string]json.RawMessage
					require.NoError(t, json.Unmarshal(sent, &fields))
					if enabled {
						require.NotContains(t, fields, "temperature")
						require.NotContains(t, fields, "top_p")
						require.NotContains(t, fields, "top_k")
					} else {
						require.Equal(t, "0.96", string(fields["temperature"]))
						require.Equal(t, "0", string(fields["top_p"]))
					}
					if mode == "override" {
						if enabled {
							require.Contains(t, string(fields["thinking"]), "adaptive")
							require.NotContains(t, string(fields["thinking"]), "budget_tokens")
						} else {
							require.Contains(t, string(fields["thinking"]), "enabled")
							require.Contains(t, string(fields["thinking"]), "4096")
						}
					}
					if strings.HasPrefix(mode, "responses") || mode == "chatresponses" {
						require.Equal(t, "30000", string(fields["max_output_tokens"]))
						require.Contains(t, string(fields["input"]), "Hi")
					} else {
						require.Equal(t, "30000", string(fields["max_tokens"]))
						require.Contains(t, string(fields["messages"]), "Hi")
					}
					if channelType == constant.ChannelTypeOpenAI {
						require.NotContains(t, fields, "thinking")
						require.NotContains(t, fields, "output_config")
					}
					if dir := os.Getenv("NEWAPI_COMPAT_CAPTURE_DIR"); dir != "" {
						require.NoError(t, os.MkdirAll(dir, 0700))
						require.NoError(t, os.WriteFile(filepath.Join(dir, mode+"-"+common.Interface2String(enabled)+"-"+common.Interface2String(stream)+".json"), sent, 0600))
					}
				})
			}
		}
	}
}

func TestRequestCompatibilityConvertersAndAdapters(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		for _, provider := range []string{"claude", "aws", "vertex"} {
			for _, protocol := range []string{"chat", "responses", "native"} {
				t.Run(provider+"/"+protocol+"/"+common.Interface2String(enabled), func(t *testing.T) {
					c, _ := gin.CreateTestContext(httptest.NewRecorder())
					common.SetContextKey(c, constant.ContextKeyTokenRequestCompatibilityEnabled, &enabled)
					name := "claude-opus-4-7"
					if provider == "aws" {
						name = "us.anthropic.claude-opus-4-7-v1:0"
					}
					if provider == "vertex" {
						name = "claude-opus-4-7@20260416"
					}
					info := &relaycommon.RelayInfo{RequestCompatibilityEnabled: &enabled, ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: name}}
					var req *dto.ClaudeRequest
					var err error
					switch protocol {
					case "chat":
						result, e := relayconvert.ConvertRequest(c, info, types.RelayFormatClaude, &dto.GeneralOpenAIRequest{Model: name, Temperature: common.GetPointer(.96), TopP: common.GetPointer(0.0), ReasoningEffort: "high", Messages: []dto.Message{{Role: "user", Content: "Hi"}}})
						require.NoError(t, e)
						req = result.Value.(*dto.ClaudeRequest)
					case "responses":
						result, e := relayconvert.ConvertRequest(c, info, types.RelayFormatClaude, &dto.OpenAIResponsesRequest{Model: name, Temperature: common.GetPointer(.96), TopP: common.GetPointer(0.0), Reasoning: &dto.Reasoning{Effort: "high"}, Input: json.RawMessage(`"Hi"`)})
						require.NoError(t, e)
						req = result.Value.(*dto.ClaudeRequest)
					default:
						req = &dto.ClaudeRequest{Model: name, Thinking: &dto.Thinking{Type: "enabled", BudgetTokens: common.GetPointer(4096)}, OutputConfig: json.RawMessage(`{"effort":"medium","extension":9007199254740993}`), Temperature: common.GetPointer(.96), TopP: common.GetPointer(0.0), Messages: []dto.ClaudeMessage{{Role: "user", Content: "Hi"}}}
					}
					var converted any = req
					switch provider {
					case "aws":
						converted, err = (&aws.Adaptor{}).ConvertClaudeRequest(c, info, req)
					case "vertex":
						converted, err = (&vertex.Adaptor{}).ConvertClaudeRequest(c, info, req)
					}
					require.NoError(t, err)
					body, err := json.Marshal(converted)
					require.NoError(t, err)
					body, err = relaycommon.ApplyParamOverride(body, map[string]any{"temperature": 0.96, "top_p": 0, "top_k": 5, "thinking": map[string]any{"type": "enabled", "budget_tokens": 4096}}, nil)
					require.NoError(t, err)
					body, err = normalizeClaudeOutboundJSON(body, info, converted)
					require.NoError(t, err)
					var fields map[string]json.RawMessage
					require.NoError(t, json.Unmarshal(body, &fields))
					if enabled {
						require.NotContains(t, fields, "temperature")
						require.Contains(t, string(fields["thinking"]), "adaptive")
						require.NotContains(t, string(fields["thinking"]), "budget_tokens")
					} else {
						require.Contains(t, fields, "temperature")
						require.Contains(t, string(fields["thinking"]), "enabled")
						require.Contains(t, string(fields["thinking"]), "budget_tokens")
					}
					require.Contains(t, string(fields["messages"]), "Hi")
				})
			}
		}
	}
}
