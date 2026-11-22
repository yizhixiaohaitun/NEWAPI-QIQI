package relay

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFable52ThinkingActualOutbound(t *testing.T) {
	setupSignatureBilling(t)
	service.InitHttpClient()
	const fable, legacy = "claude-fable-5-2", "claude-opus-4-5-20251101"
	const disabled, missing, adaptive = `{"type":"disabled"}`, `{"type":"enabled"}`, `{"type":"adaptive"}`
	const disabledError = `"thinking.type.disabled" is not supported for this model. Use "thinking.type.adaptive" and "output_config.effort" to control thinking behavior.`
	const budgetError = `***.enabled.budget_tokens: Field required`
	for _, tc := range []struct {
		name, model, mapped, overrideModel, overrideThinking, thinking, effort, wantThinking, wantEffort, reject string
		maxTokens                                                                                                uint
		stream, aws, off, channelRaw, globalRaw, explicit                                                        bool
	}{
		{name: "native default disabled", model: fable, thinking: disabled, wantThinking: adaptive, wantEffort: "low", maxTokens: 4096, stream: true},
		{name: "native default missing budget", model: fable, thinking: missing, effort: "low", wantThinking: adaptive, wantEffort: "low", maxTokens: 1000000},
		{name: "mapped fable with late thinking override", model: "local-alias", mapped: fable, thinking: adaptive, overrideThinking: disabled, effort: "high", wantThinking: adaptive, wantEffort: "high", maxTokens: 4096, explicit: true},
		{name: "native override to fable", model: legacy, thinking: missing, overrideModel: fable, effort: "medium", wantThinking: adaptive, wantEffort: "medium", maxTokens: 8192},
		{name: "native override away from fable", model: fable, thinking: disabled, overrideModel: legacy, wantThinking: disabled, maxTokens: 8192},
		{name: "SK off", model: fable, thinking: missing, effort: "low", wantThinking: missing, wantEffort: "low", maxTokens: 1000000, off: true, reject: budgetError},
		{name: "channel raw", model: fable, thinking: disabled, wantThinking: disabled, maxTokens: 4096, channelRaw: true, reject: disabledError},
		{name: "global raw", model: fable, thinking: missing, effort: "low", wantThinking: missing, wantEffort: "low", maxTokens: 1000000, globalRaw: true, reject: budgetError},
		{name: "AWS SDK stream selects its own model", model: fable, thinking: disabled, overrideModel: legacy, wantThinking: adaptive, wantEffort: "low", maxTokens: 4096, aws: true, stream: true},
		{name: "AWS SDK JSON selects its own model", model: fable, thinking: missing, overrideModel: legacy, effort: "low", wantThinking: adaptive, wantEffort: "low", maxTokens: 1000000, aws: true},
		{name: "AWS SDK does not let body model trigger Fable logic", model: legacy, thinking: disabled, overrideModel: fable, wantThinking: disabled, maxTokens: 4096, aws: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			global := model_setting.GetGlobalSettings()
			originalRaw := global.PassThroughRequestEnabled
			global.PassThroughRequestEnabled = tc.globalRaw
			t.Cleanup(func() { global.PassThroughRequestEnabled = originalRaw })

			var captured [][]byte
			var paths []string
			var mu sync.Mutex
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				b, err := io.ReadAll(r.Body)
				if err != nil {
					w.WriteHeader(http.StatusInternalServerError)
					return
				}
				mu.Lock()
				captured = append(captured, b)
				paths = append(paths, r.URL.Path)
				mu.Unlock()

				var parsed struct {
					Model    string `json:"model"`
					Thinking struct {
						Type   string `json:"type"`
						Budget *int   `json:"budget_tokens"`
					} `json:"thinking"`
				}
				if common.Unmarshal(b, &parsed) != nil {
					w.WriteHeader(http.StatusInternalServerError)
					return
				}
				if parsed.Model == fable || (tc.aws && strings.HasPrefix(r.URL.Path, "/model/"+fable+"/")) {
					message := ""
					switch {
					case parsed.Thinking.Type == "disabled":
						message = disabledError
					case parsed.Thinking.Type == "enabled" && parsed.Thinking.Budget == nil:
						message = budgetError
					}
					if message != "" {
						var response any = map[string]any{"type": "error", "error": map[string]string{"type": "invalid_request_error", "message": message}}
						if tc.aws {
							w.Header().Set("X-Amzn-Errortype", "ValidationException")
							response = map[string]string{"message": message}
						}
						w.Header().Set("Content-Type", "application/json")
						w.WriteHeader(http.StatusBadRequest)
						b, _ := common.Marshal(response)
						_, _ = w.Write(b)
						return
					}
				}
				writeSignatureSuccess(w, tc.aws, tc.stream)
			}))
			t.Cleanup(upstream.Close)
			if tc.aws {
				target, err := url.Parse(upstream.URL)
				require.NoError(t, err)
				client := service.GetHttpClient()
				saved := client.Transport
				client.Transport = signatureLoopbackTransport{target: target, base: upstream.Client().Transport}
				t.Cleanup(func() { client.Transport = saved })
			}

			body := fmt.Sprintf(`{"model":%q,"max_tokens":%d,"stream":%t,"thinking":%s,"system":"preserve this system","messages":[{"role":"user","content":"preserve this message"}],"tools":[{"name":"local_tool","input_schema":{"type":"object"}}]`, tc.model, tc.maxTokens, tc.stream, tc.thinking)
			if tc.effort != "" {
				body += fmt.Sprintf(`,"output_config":{"effort":%q,"extension":9007199254740993}`, tc.effort)
			}
			body += "}"
			c, info, recorder := signatureTestContext(t, []byte(body), upstream.URL, tc.aws, tc.stream, tc.channelRaw, !tc.off)
			common.SetContextKey(c, constant.ContextKeyOriginalModel, tc.model)
			if !tc.off && !tc.explicit {
				info.RequestCompatibilityEnabled = nil
				common.SetContextKey(c, constant.ContextKeyTokenRequestCompatibilityEnabled, (*bool)(nil))
			}
			if tc.mapped != "" {
				c.Set("model_mapping", fmt.Sprintf(`{%q:%q}`, tc.model, tc.mapped))
			}
			override := map[string]any{}
			if tc.overrideModel != "" {
				override["model"] = tc.overrideModel
			}
			if tc.overrideThinking != "" {
				var thinking map[string]any
				require.NoError(t, common.Unmarshal([]byte(tc.overrideThinking), &thinking))
				override["thinking"] = thinking
			}
			common.SetContextKey(c, constant.ContextKeyChannelParamOverride, override)

			apiErr := ClaudeHelper(c, info)
			mu.Lock()
			defer mu.Unlock()
			require.Len(t, captured, 1, "the gateway must not resend the request")
			if tc.reject != "" {
				require.NotNil(t, apiErr)
				assert.Equal(t, http.StatusBadRequest, apiErr.StatusCode)
				assert.Contains(t, apiErr.Error(), tc.reject)
			} else {
				require.Nil(t, apiErr)
				assert.Contains(t, recorder.Body.String(), "hello")
			}
			if tc.channelRaw || tc.globalRaw {
				assert.Equal(t, body, string(captured[0]))
			}
			var original, sent map[string]json.RawMessage
			require.NoError(t, common.Unmarshal([]byte(body), &original))
			require.NoError(t, common.Unmarshal(captured[0], &sent))
			assert.JSONEq(t, tc.wantThinking, string(sent["thinking"]))
			if tc.wantEffort == "" {
				assert.Empty(t, sent["output_config"])
			} else {
				var config map[string]json.RawMessage
				require.NoError(t, common.Unmarshal(sent["output_config"], &config))
				assert.Equal(t, fmt.Sprintf("%q", tc.wantEffort), string(config["effort"]))
				if tc.effort != "" {
					assert.Equal(t, "9007199254740993", string(config["extension"]))
				}
			}
			for _, field := range []string{"system", "messages", "tools", "max_tokens"} {
				assert.JSONEq(t, string(original[field]), string(sent[field]), field)
			}
			if tc.aws {
				op := "/invoke"
				if tc.stream {
					op += "-with-response-stream"
				}
				modelID := tc.model
				if tc.model == legacy {
					modelID = "us.anthropic.claude-opus-4-5-20251101-v1:0"
				}
				assert.Equal(t, []string{"/model/" + modelID + op}, paths)
				assert.Empty(t, sent["model"])
				assert.NotEmpty(t, sent["anthropic_version"])
			} else {
				expected := tc.model
				if tc.mapped != "" {
					expected = tc.mapped
				}
				if tc.overrideModel != "" {
					expected = tc.overrideModel
				}
				assert.Equal(t, fmt.Sprintf("%q", expected), string(sent["model"]))
				assert.Equal(t, []string{"/v1/messages"}, paths)
			}
		})
	}
}
