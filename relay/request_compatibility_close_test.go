package relay

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/service/relayconvert"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type compatibilityCaptureTransport func(*http.Request) (*http.Response, error)

func (f compatibilityCaptureTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

// Capture the actual final HTTP request, including provider transport URL. The
// transport replacement cannot access the network; no installed instance used.
func TestRequestCompatibilityRawPassthroughTransport(t *testing.T) {
	service.InitHttpClient()
	client := service.GetHttpClient()
	oldTransport := client.Transport
	oldGlobal := model_setting.GetGlobalSettings().PassThroughRequestEnabled
	defer func() {
		client.Transport = oldTransport
		model_setting.GetGlobalSettings().PassThroughRequestEnabled = oldGlobal
	}()
	for _, provider := range []int{constant.ChannelTypeAnthropic, constant.ChannelTypeAws, constant.ChannelTypeVertexAi} {
		for _, global := range []bool{false, true} {
			for _, stream := range []bool{false, true} {
				t.Run(common.Interface2String(provider)+"/global="+common.Interface2String(global)+"/stream="+common.Interface2String(stream), func(t *testing.T) {
					model_setting.GetGlobalSettings().PassThroughRequestEnabled = global
					name := "claude-opus-4-7-thinking"
					if provider == constant.ChannelTypeAws {
						name = "us.anthropic.claude-opus-4-7-v1:0-thinking"
					}
					if provider == constant.ChannelTypeVertexAi {
						name = "claude-opus-4-7@20260416-thinking"
					}
					body := []byte(`{ "model":"` + name + `", "max_tokens":30000, "messages":[{"role":"user","content":"Hi"}], "thinking":{"type":"enabled","budget_tokens":4096}, "output_config":["not-an-object"], "temperature":0.96, "top_p":0 }`)
					c, _ := gin.CreateTestContext(httptest.NewRecorder())
					c.Request = httptest.NewRequest("POST", "/v1/messages", bytes.NewReader(body))
					require.NoError(t, middleware.SetupContextForToken(c, &model.Token{Id: 1, UserId: 1, Key: "test-only-raw", RequestCompatibilityEnabled: common.GetPointer(true)}))
					common.SetContextKey(c, constant.ContextKeyChannelType, provider)
					common.SetContextKey(c, constant.ContextKeyChannelBaseUrl, "https://local-capture.invalid")
					common.SetContextKey(c, constant.ContextKeyOriginalModel, name)
					common.SetContextKey(c, constant.ContextKeyChannelKey, "test-only-key|us-east-1")
					common.SetContextKey(c, constant.ContextKeyChannelOtherSetting, dto.ChannelOtherSettings{AwsKeyType: dto.AwsKeyTypeApiKey, VertexKeyType: dto.VertexKeyTypeAPIKey})
					common.SetContextKey(c, constant.ContextKeyChannelSetting, dto.ChannelSettings{PassThroughBodyEnabled: !global})
					req := &dto.ClaudeRequest{}
					require.NoError(t, json.Unmarshal(body, req))
					req.Stream = &stream
					info, err := relaycommon.GenRelayInfo(c, types.RelayFormatClaude, req, nil)
					require.NoError(t, err)
					info.StartTime = time.Now()
					var sent []byte
					var transportURL string
					client.Transport = compatibilityCaptureTransport(func(r *http.Request) (*http.Response, error) {
						sent, _ = io.ReadAll(r.Body)
						transportURL = r.URL.String()
						return &http.Response{StatusCode: 400, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"error":{"type":"invalid_request_error","message":"local capture"}}`)), Request: r}, nil
					})
					result := ClaudeHelper(c, info)
					require.NotNil(t, result)
					require.NotEmpty(t, sent, "must reach transport, not fail conversion")
					if provider == constant.ChannelTypeAws {
						// Bedrock SDK already removes model into the URL and serializes its
						// mandatory envelope; the collection must not change those fields.
						var fields map[string]json.RawMessage
						require.NoError(t, json.Unmarshal(body, &fields))
						delete(fields, "model")
						expected, err := json.Marshal(fields)
						require.NoError(t, err)
						require.JSONEq(t, string(expected), string(sent))
					} else {
						require.Equal(t, string(body), string(sent), "raw bytes must precede all collection rewrites")
					}
					require.Equal(t, name, info.UpstreamModelName)
					if provider != constant.ChannelTypeAnthropic {
						decodedURL, err := url.PathUnescape(transportURL)
						require.NoError(t, err)
						require.Contains(t, decodedURL, name, "provider model transport must retain suffix")
					}
				})
			}
		}
	}
}

func TestRequestCompatibilityOpenAIAndAdvancedHTTP(t *testing.T) {
	service.InitHttpClient()
	for _, enabled := range []bool{false, true} {
		for _, mode := range []string{"chat", "responses", "advanced-chat-none", "advanced-responses-none", "advanced-claude-chat"} {
			for _, suffix := range []string{"", "-thinking", "-high"} {
				t.Run(mode+suffix+"/on="+common.Interface2String(enabled), func(t *testing.T) {
					var sent []byte
					upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						sent, _ = io.ReadAll(r.Body)
						w.Header().Set("Content-Type", "application/json")
						w.WriteHeader(400)
						_, _ = w.Write([]byte(`{"error":{"type":"invalid_request_error","message":"local capture"}}`))
					}))
					defer upstream.Close()
					format := types.RelayFormatOpenAI
					path := "/v1/chat/completions"
					if strings.Contains(mode, "responses") {
						format = types.RelayFormatOpenAIResponses
						path = "/v1/responses"
					}
					if mode == "advanced-claude-chat" {
						format = types.RelayFormatClaude
						path = "/v1/messages"
					}
					origin := "test-alias" + suffix
					target := "claude-opus-4-7" + suffix
					// The reverse converter must not synthesize -thinking after mapping removes it.
					if mode == "advanced-claude-chat" {
						target = "claude-opus-4-7"
					}
					var req dto.Request
					if format == types.RelayFormatClaude {
						req = &dto.ClaudeRequest{Model: origin, MaxTokens: common.GetPointer[uint](30000), Temperature: common.GetPointer(.96), TopP: common.GetPointer(0.0), System: "system kept", Thinking: &dto.Thinking{Type: "enabled", BudgetTokens: common.GetPointer(4096)}, Messages: []dto.ClaudeMessage{{Role: "user", Content: "Hi"}}, Tools: []dto.Tool{{Name: "probe", Description: "test only", InputSchema: map[string]any{"type": "object"}}}}
					} else if format == types.RelayFormatOpenAIResponses {
						req = &dto.OpenAIResponsesRequest{Model: origin, MaxOutputTokens: common.GetPointer[uint](30000), Input: json.RawMessage(`"Hi"`), Temperature: common.GetPointer(.96), TopP: common.GetPointer(0.0)}
					} else {
						req = &dto.GeneralOpenAIRequest{Model: origin, MaxTokens: common.GetPointer[uint](30000), FrequencyPenalty: common.GetPointer(.78), PresencePenalty: common.GetPointer(0.0), Stream: common.GetPointer(false), Temperature: common.GetPointer(.96), TopP: common.GetPointer(0.0), Messages: []dto.Message{{Role: "user", Content: "Hi"}}}
					}
					body, err := json.Marshal(req)
					require.NoError(t, err)
					c, _ := gin.CreateTestContext(httptest.NewRecorder())
					c.Request = httptest.NewRequest("POST", path, bytes.NewReader(body))
					require.NoError(t, middleware.SetupContextForToken(c, &model.Token{Id: 1, UserId: 1, Key: "test-only-close", RequestCompatibilityEnabled: &enabled}))
					channelType := constant.ChannelTypeOpenAI
					if strings.HasPrefix(mode, "advanced") {
						channelType = constant.ChannelTypeAdvancedCustom
					}
					common.SetContextKey(c, constant.ContextKeyChannelType, channelType)
					common.SetContextKey(c, constant.ContextKeyChannelBaseUrl, upstream.URL)
					common.SetContextKey(c, constant.ContextKeyOriginalModel, origin)
					mapping, _ := json.Marshal(map[string]string{origin: target})
					c.Set("model_mapping", string(mapping))
					common.SetContextKey(c, constant.ContextKeyChannelParamOverride, map[string]any{"temperature": .96, "top_p": 0, "top_k": 7})
					if channelType == constant.ChannelTypeAdvancedCustom {
						converter := relayconvert.ConverterNone
						if format == types.RelayFormatClaude {
							converter = relayconvert.ConverterClaudeMessagesToOpenAIChat
						}
						common.SetContextKey(c, constant.ContextKeyChannelOtherSetting, dto.ChannelOtherSettings{AdvancedCustom: &dto.AdvancedCustomConfig{Routes: []dto.AdvancedCustomRoute{{IncomingPath: path, UpstreamPath: "/v1/chat/completions", Converter: converter}}}})
					}
					info, err := relaycommon.GenRelayInfo(c, format, req, nil)
					require.NoError(t, err)
					info.RequestURLPath = path
					info.StartTime = time.Now()
					switch format {
					case types.RelayFormatClaude:
						require.NotNil(t, ClaudeHelper(c, info))
					case types.RelayFormatOpenAIResponses:
						require.NotNil(t, ResponsesHelper(c, info))
					default:
						require.NotNil(t, TextHelper(c, info))
					}
					require.NotEmpty(t, sent, "actual supported HTTP route must send")
					var fields map[string]json.RawMessage
					require.NoError(t, json.Unmarshal(sent, &fields))
					expectedModel := target
					if enabled {
						expectedModel = "claude-opus-4-7"
					}
					require.Equal(t, `"`+expectedModel+`"`, string(fields["model"]))
					if enabled {
						for _, key := range []string{"temperature", "top_p", "top_k"} {
							require.NotContains(t, fields, key)
						}
					} else {
						require.Equal(t, "0.96", string(fields["temperature"]))
						require.Equal(t, "0", string(fields["top_p"]))
						require.Equal(t, "7", string(fields["top_k"]))
					}
					require.NotContains(t, fields, "thinking")
					require.NotContains(t, fields, "output_config")
					if format == types.RelayFormatOpenAI {
						require.Equal(t, "0.78", string(fields["frequency_penalty"]))
						require.Equal(t, "0", string(fields["presence_penalty"]))
					}
					if format == types.RelayFormatOpenAIResponses {
						require.Contains(t, string(fields["input"]), "Hi")
						require.Equal(t, "30000", string(fields["max_output_tokens"]))
					} else {
						require.Contains(t, string(fields["messages"]), "Hi")
						require.Equal(t, "30000", string(fields["max_tokens"]))
					}
					if format == types.RelayFormatClaude {
						require.Contains(t, string(fields["tools"]), "probe")
						require.Contains(t, string(fields["messages"]), "system kept")
					}
				})
			}
		}
	}
}
