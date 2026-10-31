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
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type geminiHistoryPart struct {
	Text             json.RawMessage            `json:"text"`
	FuturePart       json.RawMessage            `json:"futurePart"`
	Thought          bool                       `json:"thought"`
	ThoughtSignature json.RawMessage            `json:"thoughtSignature"`
	FunctionCall     map[string]json.RawMessage `json:"functionCall"`
	FunctionResponse map[string]json.RawMessage `json:"functionResponse"`
}

// Compare object fields without converting numeric tokens to float64 or depending on key order.
func assertGeminiHistoryObject(t *testing.T, expected string, actual json.RawMessage, message string) {
	t.Helper()
	var want, got map[string]json.RawMessage
	require.NoError(t, common.Unmarshal([]byte(expected), &want))
	if !assert.NoError(t, common.Unmarshal(actual, &got), message) {
		return
	}
	assert.Equal(t, want, got, message)
}

// Local HTTP capture only: the mock upstream returns 400 before billing and no tool is executed.
func TestGeminiNativeHistory(t *testing.T) {
	service.InitHttpClient()
	const (
		bigArg  = `9007199254740993`
		decArg  = `1.234567890123456789`
		args1   = `{"big":` + bigArg + `,"dec":` + decArg + `,"city":"Paris"}`
		args2   = `{"second":` + bigArg + `}`
		resp1   = `{"big":` + bigArg + `,"city":"Paris","dec":` + decArg + `,"temp":"20C"}`
		resp2   = `{"ok":true}`
		future  = `{"big":` + bigArg + `,"dec":` + decArg + `}`
		sigCall = `"sig-first-call"`
		sigText = `"sig-empty-text"`
	)
	for _, stream := range []bool{false, true} {
		for _, snake := range []bool{false, true} {
			name := "json"
			if stream {
				name = "sse"
			}
			if snake {
				name += "/snake"
			} else {
				name += "/camel"
			}
			t.Run(name, func(t *testing.T) {
				keys := map[string]string{"call": "functionCall", "resp": "functionResponse", "sig": "thoughtSignature"}
				if snake {
					keys = map[string]string{"call": "function_call", "resp": "function_response", "sig": "thought_signature"}
				}
				var sent []byte
				var sentPath, sentQuery string
				calls := 0
				upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls++
					sent, _ = io.ReadAll(r.Body)
					sentPath, sentQuery = r.URL.Path, r.URL.RawQuery
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusBadRequest)
					_, _ = w.Write([]byte(`{"error":{"code":400,"message":"local capture stop","status":"INVALID_ARGUMENT"}}`))
				}))
				defer upstream.Close()

				body := `{
"contents":[
 {"role":"user","parts":[{"text":"weather?"}]},
 {"role":"model","futureContent":` + future + `,"parts":[
  {"text":"thinking","thought":true,"futurePart":` + future + `},
  {"` + keys["call"] + `":{"id":"call-1","name":"get_weather","args":` + args1 + `},"` + keys["sig"] + `":` + sigCall + `},
  {"` + keys["call"] + `":{"id":"call-2","name":"get_time","args":` + args2 + `}},
  {"text":"","` + keys["sig"] + `":` + sigText + `}
 ]},
 {"role":"user","parts":[
  {"` + keys["resp"] + `":{"id":"call-1","name":"get_weather","response":` + resp1 + `}},
  {"` + keys["resp"] + `":{"id":"call-2","name":"get_time","response":` + resp2 + `}}
 ]}
],
"systemInstruction":{"role":"user","futureContent":` + future + `,"parts":[{"text":"original system","futurePart":` + future + `}]},
"generationConfig":{"temperature":0.8,"thinkingConfig":{"includeThoughts":true,"thinkingLevel":"high"}}
}`
				action := "generateContent"
				if stream {
					action = "streamGenerateContent"
				}
				path := "/v1beta/models/gemini-3-pro-preview:" + action
				var req dto.GeminiChatRequest
				require.NoError(t, common.Unmarshal([]byte(body), &req))

				c, _ := gin.CreateTestContext(httptest.NewRecorder())
				c.Request = httptest.NewRequest("POST", path, bytes.NewReader([]byte(body)))
				c.Request.Header.Set("Content-Type", "application/json")
				require.NoError(t, middleware.SetupContextForToken(c, &model.Token{Id: 1, UserId: 1, Key: "gemini-history-local-test-only"}))
				common.SetContextKey(c, constant.ContextKeyChannelType, constant.ChannelTypeGemini)
				common.SetContextKey(c, constant.ContextKeyChannelBaseUrl, upstream.URL)
				common.SetContextKey(c, constant.ContextKeyChannelKey, "local-test-key")
				common.SetContextKey(c, constant.ContextKeyOriginalModel, "gemini-3-pro-preview")
				common.SetContextKey(c, constant.ContextKeyChannelSetting, dto.ChannelSettings{
					SystemPrompt: "channel system", SystemPromptOverride: true,
				})
				common.SetContextKey(c, constant.ContextKeyChannelParamOverride, map[string]interface{}{
					"operations": []interface{}{map[string]interface{}{
						"mode": "set", "path": "generationConfig.temperature", "value": 0.25,
					}},
				})
				c.Set("model_mapping", `{"gemini-3-pro-preview":"gemini-3-pro-upstream"}`)
				info, err := relaycommon.GenRelayInfo(c, types.RelayFormatGemini, &req, nil)
				require.NoError(t, err)
				info.RequestURLPath, info.StartTime = path, time.Now()

				result := GeminiHelper(c, info)
				require.NotNil(t, result, "local upstream returns 400")
				require.NotEmpty(t, sent, "must reach HTTP transport")
				assert.Equal(t, 1, calls)
				assert.Contains(t, sentPath, "gemini-3-pro-upstream:"+action, "model mapping must still apply")
				if stream {
					assert.Contains(t, sentQuery, "alt=sse")
				}

				var out struct {
					Contents []struct {
						Role          string              `json:"role"`
						Parts         []geminiHistoryPart `json:"parts"`
						FutureContent json.RawMessage     `json:"futureContent"`
					} `json:"contents"`
					SystemInstruction struct {
						Parts         []geminiHistoryPart `json:"parts"`
						FutureContent json.RawMessage     `json:"futureContent"`
					} `json:"systemInstruction"`
					GenerationConfig struct {
						Temperature    json.RawMessage `json:"temperature"`
						ThinkingConfig struct {
							IncludeThoughts bool   `json:"includeThoughts"`
							ThinkingLevel   string `json:"thinkingLevel"`
						} `json:"thinkingConfig"`
					} `json:"generationConfig"`
				}
				require.NoError(t, common.Unmarshal(sent, &out), string(sent))
				require.Len(t, out.Contents, 3)
				assert.True(t, out.GenerationConfig.ThinkingConfig.IncludeThoughts)
				assert.Equal(t, "high", out.GenerationConfig.ThinkingConfig.ThinkingLevel)
				assert.Equal(t, `0.25`, string(out.GenerationConfig.Temperature), "channel parameter override must still apply")
				require.Len(t, out.SystemInstruction.Parts, 1, string(sent))
				assert.Equal(t, `"channel system\noriginal system"`, string(out.SystemInstruction.Parts[0].Text), "channel system prompt must still merge")
				assertGeminiHistoryObject(t, future, out.SystemInstruction.FutureContent, "system instruction content extension must survive")
				assertGeminiHistoryObject(t, future, out.SystemInstruction.Parts[0].FuturePart, "system instruction part extension must survive")

				model := out.Contents[1]
				assert.Equal(t, "model", model.Role)
				assertGeminiHistoryObject(t, future, model.FutureContent, "unknown content field must retain exact numbers")
				require.Len(t, model.Parts, 4, string(sent))
				assert.True(t, model.Parts[0].Thought)
				assert.Equal(t, `"thinking"`, string(model.Parts[0].Text))
				assertGeminiHistoryObject(t, future, model.Parts[0].FuturePart, "unknown part field must retain exact numbers")
				assert.Nil(t, model.Parts[0].FunctionCall)

				first, second := model.Parts[1], model.Parts[2]
				require.NotNil(t, first.FunctionCall, string(sent))
				require.NotNil(t, second.FunctionCall, string(sent))
				assert.Equal(t, `"call-1"`, string(first.FunctionCall["id"]))
				assert.Equal(t, `"get_weather"`, string(first.FunctionCall["name"]))
				assert.Equal(t, args1, string(first.FunctionCall["args"]))
				assert.Equal(t, sigCall, string(first.ThoughtSignature))
				assert.Equal(t, `"call-2"`, string(second.FunctionCall["id"]))
				assert.Equal(t, `"get_time"`, string(second.FunctionCall["name"]))
				assert.Equal(t, args2, string(second.FunctionCall["args"]))
				assert.Empty(t, string(second.ThoughtSignature), "no signature may be invented for the second call")
				assert.Equal(t, `""`, string(model.Parts[3].Text), "explicit empty text must remain present")
				assert.Equal(t, sigText, string(model.Parts[3].ThoughtSignature))
				assert.Equal(t, 1, strings.Count(string(sent), sigCall), "signature must not be duplicated")
				assert.Equal(t, 1, strings.Count(string(sent), sigText), "signature must not be duplicated")
				assert.NotContains(t, string(sent), "skip_thought_signature_validator")

				user := out.Contents[2]
				require.Len(t, user.Parts, 2)
				require.NotNil(t, user.Parts[0].FunctionResponse, string(sent))
				require.NotNil(t, user.Parts[1].FunctionResponse, string(sent))
				assert.Equal(t, `"call-1"`, string(user.Parts[0].FunctionResponse["id"]))
				assert.Equal(t, `"get_weather"`, string(user.Parts[0].FunctionResponse["name"]))
				assertGeminiHistoryObject(t, resp1, user.Parts[0].FunctionResponse["response"], "tool response must retain exact numbers")
				assert.Equal(t, `"call-2"`, string(user.Parts[1].FunctionResponse["id"]))
				assert.Equal(t, `"get_time"`, string(user.Parts[1].FunctionResponse["name"]))
				assert.Equal(t, resp2, string(user.Parts[1].FunctionResponse["response"]))
				assert.NotContains(t, string(sent), "function_call")
				assert.NotContains(t, string(sent), "thought_signature")
			})
		}
	}
}
