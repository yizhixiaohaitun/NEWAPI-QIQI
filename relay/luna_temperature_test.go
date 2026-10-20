package relay

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type lunaRequestCase struct {
	mode, origin, target, temperature, raw string
	enabled, stream                        bool
	override                               map[string]any
}

type lunaCapture struct {
	body, original []byte
	path           string
}

// Exercise token context -> model mapping -> real protocol converter/adaptor ->
// final overrides -> HTTP capture. Only this local server can receive a request.
func captureLunaRequest(t *testing.T, tc lunaRequestCase) lunaCapture {
	t.Helper()
	var sent []byte
	var sentPath string
	calls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		sent, _ = io.ReadAll(r.Body)
		sentPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"code":"unsupported_value","param":"temperature","message":"Unsupported value: 'temperature' does not support 0.3 with this model. Only the default (1) value is supported."}}`))
	}))
	defer upstream.Close()

	if tc.origin == "" {
		tc.origin = "gpt-6-luna"
	}
	format := types.RelayFormatOpenAI
	path := "/v1/chat/completions"
	fields := map[string]any{
		"model": tc.origin, "stream": tc.stream, "top_p": 0.42,
		"messages": []any{map[string]any{"role": "system", "content": "translator instructions"}, map[string]any{"role": "user", "content": "text"}},
		"tools":    []any{map[string]any{"type": "function", "function": map[string]any{"name": "probe", "description": "test only", "parameters": map[string]any{"type": "object", "properties": map[string]any{}}}}},
	}
	if strings.Contains(tc.mode, "responses") && tc.mode != "advanced-chat-responses" && tc.mode != "chat-via-responses" {
		format = types.RelayFormatOpenAIResponses
		path = "/v1/responses"
		delete(fields, "messages")
		fields["instructions"] = "translator instructions"
		fields["input"] = []any{map[string]any{"role": "user", "content": "text"}}
		fields["tools"] = []any{map[string]any{"type": "function", "name": "probe", "description": "test only", "parameters": map[string]any{"type": "object", "properties": map[string]any{}}}}
	} else if tc.mode == "advanced-claude-chat" {
		format = types.RelayFormatClaude
		path = "/v1/messages"
		fields["system"] = "translator instructions"
		fields["messages"] = []any{map[string]any{"role": "user", "content": "text"}}
		fields["max_tokens"] = 30000
		fields["tools"] = []any{map[string]any{"name": "probe", "description": "test only", "input_schema": map[string]any{"type": "object", "properties": map[string]any{}}}}
	}
	if tc.temperature != "absent" {
		fields["temperature"] = json.RawMessage(tc.temperature)
	}
	if strings.HasPrefix(tc.mode, "advanced-gemini-") {
		format = types.RelayFormatGemini
		action := "generateContent"
		if tc.stream {
			action = "streamGenerateContent"
		}
		path = "/v1beta/models/" + tc.origin + ":" + action
		generationConfig := map[string]any{"topP": 0.42}
		if tc.temperature != "absent" {
			generationConfig["temperature"] = json.RawMessage(tc.temperature)
		}
		fields = map[string]any{
			"generationConfig":  generationConfig,
			"systemInstruction": map[string]any{"parts": []any{map[string]any{"text": "translator instructions"}}},
			"contents":          []any{map[string]any{"role": "user", "parts": []any{map[string]any{"text": "text"}}}},
			"tools":             []any{map[string]any{"functionDeclarations": []any{map[string]any{"name": "probe", "description": "test only", "parameters": map[string]any{"type": "object", "properties": map[string]any{}}}}}},
		}
	}
	body, err := common.Marshal(fields)
	require.NoError(t, err)
	// Distinct whitespace makes raw passthrough a byte-level assertion.
	body = append(append([]byte(" \n"), body...), '\n')
	var req dto.Request
	switch format {
	case types.RelayFormatOpenAIResponses:
		req = &dto.OpenAIResponsesRequest{}
	case types.RelayFormatClaude:
		req = &dto.ClaudeRequest{}
	case types.RelayFormatGemini:
		req = &dto.GeminiChatRequest{}
	default:
		req = &dto.GeneralOpenAIRequest{}
	}
	require.NoError(t, common.Unmarshal(body, req))
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", path, bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	require.NoError(t, middleware.SetupContextForToken(c, &model.Token{Id: 1, UserId: 1, Key: "luna-local-test-only", RequestCompatibilityEnabled: &tc.enabled}))
	channelType := constant.ChannelTypeOpenAI
	if strings.HasPrefix(tc.mode, "advanced") {
		channelType = constant.ChannelTypeAdvancedCustom
	}
	common.SetContextKey(c, constant.ContextKeyChannelType, channelType)
	common.SetContextKey(c, constant.ContextKeyChannelBaseUrl, upstream.URL)
	common.SetContextKey(c, constant.ContextKeyOriginalModel, tc.origin)
	if tc.target != "" {
		mapping, err := common.Marshal(map[string]string{tc.origin: tc.target})
		require.NoError(t, err)
		c.Set("model_mapping", string(mapping))
	}
	if len(tc.override) > 0 {
		common.SetContextKey(c, constant.ContextKeyChannelParamOverride, tc.override)
	}
	oldGlobal := *model_setting.GetGlobalSettings()
	model_setting.GetGlobalSettings().PassThroughRequestEnabled = tc.raw == "global"
	model_setting.GetGlobalSettings().ChatCompletionsToResponsesPolicy = model_setting.ChatCompletionsToResponsesPolicy{Enabled: tc.mode == "chat-via-responses", AllChannels: true, ModelPatterns: []string{".*"}}
	defer func() { *model_setting.GetGlobalSettings() = oldGlobal }()
	common.SetContextKey(c, constant.ContextKeyChannelSetting, dto.ChannelSettings{PassThroughBodyEnabled: tc.raw == "channel"})
	if channelType == constant.ChannelTypeAdvancedCustom {
		converter, outPath := relayconvert.ConverterNone, path
		switch tc.mode {
		case "advanced-chat-responses":
			converter, outPath = relayconvert.ConverterOpenAIChatToOpenAIResponses, "/v1/responses"
		case "advanced-responses-chat":
			converter, outPath = relayconvert.ConverterOpenAIResponsesToOpenAIChat, "/v1/chat/completions"
		case "advanced-claude-chat":
			converter, outPath = relayconvert.ConverterClaudeMessagesToOpenAIChat, "/v1/chat/completions"
		case "advanced-gemini-chat":
			converter, outPath = relayconvert.ConverterGeminiContentToOpenAIChat, "/v1/chat/completions"
		}
		common.SetContextKey(c, constant.ContextKeyChannelOtherSetting, dto.ChannelOtherSettings{AdvancedCustom: &dto.AdvancedCustomConfig{Routes: []dto.AdvancedCustomRoute{{IncomingPath: path, UpstreamPath: outPath, Converter: converter}}}})
	}
	info, err := relaycommon.GenRelayInfo(c, format, req, nil)
	require.NoError(t, err)
	info.RequestURLPath, info.StartTime = path, time.Now()
	var result *types.NewAPIError
	switch format {
	case types.RelayFormatClaude:
		result = ClaudeHelper(c, info)
	case types.RelayFormatGemini:
		result = GeminiHelper(c, info)
	case types.RelayFormatOpenAIResponses:
		result = ResponsesHelper(c, info)
	default:
		result = TextHelper(c, info)
	}
	require.NotNil(t, result, "capture intentionally returns a local upstream error")
	require.NotEmpty(t, sent, "must reach HTTP transport, not fail protocol conversion")
	assert.Equal(t, 1, calls, "unsupported temperature must not trigger automatic resend")
	return lunaCapture{body: sent, original: body, path: sentPath}
}

func TestLunaTemperatureHTTP(t *testing.T) {
	service.InitHttpClient()
	for _, mode := range []string{"chat", "responses", "chat-via-responses", "advanced-chat-none", "advanced-responses-none", "advanced-chat-responses", "advanced-responses-chat", "advanced-claude-chat", "advanced-gemini-chat"} {
		for _, enabled := range []bool{true, false} {
			for _, stream := range []bool{false, true} {
				for _, temperature := range []string{"0.3", "0", "1", "absent"} {
					t.Run(mode+"/on="+common.Interface2String(enabled)+"/stream="+common.Interface2String(stream)+"/temp="+temperature, func(t *testing.T) {
						capture := captureLunaRequest(t, lunaRequestCase{mode: mode, enabled: enabled, stream: stream, temperature: temperature})
						var fields map[string]json.RawMessage
						require.NoError(t, common.Unmarshal(capture.body, &fields))
						if temperature == "absent" || enabled && temperature != "1" {
							assert.NotContains(t, fields, "temperature")
						} else {
							assert.Equal(t, temperature, string(fields["temperature"]))
						}
						assert.Equal(t, `"gpt-6-luna"`, string(fields["model"]))
						assert.Equal(t, "0.42", string(fields["top_p"]), "do not guess other unsupported parameters")
						assert.Equal(t, common.Interface2String(stream), string(fields["stream"]))
						assert.Contains(t, string(fields["tools"]), "probe", "normal tools conversion remains")
						if mode == "chat" || mode == "responses" || strings.HasSuffix(mode, "-none") {
							var original map[string]json.RawMessage
							require.NoError(t, common.Unmarshal(capture.original, &original))
							for _, key := range []string{"messages", "input", "instructions", "tools"} {
								if raw, ok := original[key]; ok {
									assert.JSONEq(t, string(raw), string(fields[key]), key+" remains unchanged")
								}
							}
						}
						if capture.path == "/v1/responses" {
							assert.Contains(t, string(fields["input"]), "text")
							assert.Contains(t, string(capture.body), "translator instructions")
						} else {
							assert.Contains(t, string(fields["messages"]), "translator instructions")
							assert.Contains(t, string(fields["messages"]), `"role":"system"`)
							assert.Contains(t, string(fields["messages"]), "text")
						}
					})
				}
			}
		}
	}
}

func TestLunaTemperatureMappingAndOverrides(t *testing.T) {
	service.InitHttpClient()
	for _, mode := range []string{"chat", "responses", "chat-via-responses", "advanced-chat-none", "advanced-responses-none", "advanced-chat-responses", "advanced-responses-chat", "advanced-claude-chat", "advanced-gemini-chat"} {
		for _, enabled := range []bool{false, true} {
			for _, tc := range []struct {
				name, origin, target, wantModel string
				override                        map[string]any
				omit                            bool
			}{
				{"mapped-in", "translator-alias", "gpt-6-luna", "gpt-6-luna", nil, true},
				{"mapped-out", "gpt-6-luna", "gpt-6-other", "gpt-6-other", nil, false},
				{"unrelated", "gpt-6", "", "gpt-6", nil, false},
				{"similar-name", "gpt-6-luna-preview", "", "gpt-6-luna-preview", nil, false},
				{"override-temperature", "gpt-6-luna", "", "gpt-6-luna", map[string]any{"temperature": 0.3, "top_p": 0, "top_k": 7}, true},
				{"override-model-in", "gpt-6-other", "", "gpt-6-luna", map[string]any{"model": "gpt-6-luna", "temperature": 0.3}, true},
				{"override-model-out", "gpt-6-luna", "", "gpt-6-other", map[string]any{"model": "gpt-6-other", "temperature": 0.3}, false},
			} {
				t.Run(mode+"/on="+common.Interface2String(enabled)+"/"+tc.name, func(t *testing.T) {
					capture := captureLunaRequest(t, lunaRequestCase{mode: mode, origin: tc.origin, target: tc.target, temperature: "0.3", enabled: enabled, override: tc.override})
					var fields map[string]json.RawMessage
					require.NoError(t, common.Unmarshal(capture.body, &fields))
					assert.Equal(t, `"`+tc.wantModel+`"`, string(fields["model"]))
					if enabled && tc.omit {
						assert.NotContains(t, fields, "temperature")
					} else {
						assert.Equal(t, "0.3", string(fields["temperature"]))
					}
					if tc.name == "override-temperature" {
						assert.Equal(t, "0", string(fields["top_p"]))
						if mode == "chat-via-responses" {
							// Responses has no top_k field; preserve normal protocol conversion.
							assert.NotContains(t, fields, "top_k")
						} else {
							assert.Equal(t, "7", string(fields["top_k"]))
						}
					}
				})
			}
		}
	}
}

func TestLunaTemperatureRawPassthrough(t *testing.T) {
	service.InitHttpClient()
	for _, mode := range []string{"chat", "responses", "advanced-chat-none", "advanced-responses-none", "advanced-gemini-none"} {
		for _, raw := range []string{"global", "channel"} {
			for _, enabled := range []bool{false, true} {
				for _, stream := range []bool{false, true} {
					for _, origin := range []string{"gpt-6-luna", "translator-alias"} {
						t.Run(mode+"/"+raw+"/on="+common.Interface2String(enabled)+"/stream="+common.Interface2String(stream)+"/"+origin, func(t *testing.T) {
							capture := captureLunaRequest(t, lunaRequestCase{mode: mode, raw: raw, origin: origin, target: "gpt-6-luna", temperature: "0.3", enabled: enabled, stream: stream, override: map[string]any{"temperature": 0.9}})
							assert.Equal(t, string(capture.original), string(capture.body), "raw keeps model, temperature and all bytes despite mapping/overrides")
						})
					}
				}
			}
		}
	}
}
