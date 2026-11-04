package relay

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"
	"github.com/aws/aws-sdk-go-v2/aws/protocol/eventstream"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

const signatureValidationMessage = "Invalid `signature` in `thinking` block"
const signatureHistoryRequest = `{"model":"claude-opus-5-5","max_tokens":512,"thinking":{"type":"adaptive","display":"summarized"},"output_config":{"effort":"medium","extension":9007199254740993},"messages":[{"role":"user","content":"first"},{"role":"assistant","content":[{"type":"thinking","thinking":"old","signature":"synthetic-signature"},{"type":"redacted_thinking","data":"synthetic-redacted"},{"type":"text","text":"answer","extension":9},{"type":"tool_use","id":"old-tool","name":"local","input":{"number":9,"decimal":0.125}}]},{"role":"user","content":[{"type":"tool_result","tool_use_id":"old-tool","content":"result"}]},{"role":"assistant","content":[{"type":"text","text":"done"}]},{"role":"user","content":"next"}]}`
const signatureSuccessResponse = `{"id":"local-success","type":"message","role":"assistant","model":"claude-opus-5-5","content":[{"type":"text","text":"hello"}],"stop_reason":"end_turn","usage":{"input_tokens":0,"output_tokens":0}}`

// signatureLoopbackTransport sends even signed SDK requests only to the local
// mock. No endpoint or real credentials are used by these tests.
type signatureLoopbackTransport struct {
	target     *url.URL
	base       http.RoundTripper
	afterClose func()
}

func (rt signatureLoopbackTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	copy := r.Clone(r.Context())
	target := *r.URL
	target.Scheme, target.Host = rt.target.Scheme, rt.target.Host
	copy.URL = &target
	copy.Host = rt.target.Host
	resp, err := rt.base.RoundTrip(copy)
	if err == nil && rt.afterClose != nil {
		resp.Body = signatureCloseBody{ReadCloser: resp.Body, afterClose: rt.afterClose}
	}
	return resp, err
}

type signatureCloseBody struct {
	io.ReadCloser
	afterClose func()
}

func (b signatureCloseBody) Close() error { err := b.ReadCloser.Close(); b.afterClose(); return err }

func signatureTestContext(t *testing.T, body []byte, server string, aws, stream, raw, enabled bool) (*gin.Context, *relaycommon.RelayInfo, *httptest.ResponseRecorder) {
	t.Helper()
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body))
	channelType, apiType, key := constant.ChannelTypeAnthropic, constant.APITypeAnthropic, "local-only"
	if aws {
		channelType, apiType, key = constant.ChannelTypeAws, constant.APITypeAws, "synthetic-ak|synthetic-sk|us-east-1"
	}
	common.SetContextKey(c, constant.ContextKeyChannelType, channelType)
	common.SetContextKey(c, constant.ContextKeyChannelBaseUrl, server)
	common.SetContextKey(c, constant.ContextKeyOriginalModel, "claude-opus-5-5")
	common.SetContextKey(c, constant.ContextKeyChannelKey, key)
	common.SetContextKey(c, constant.ContextKeyChannelSetting, dto.ChannelSettings{PassThroughBodyEnabled: raw})
	common.SetContextKey(c, constant.ContextKeyTokenRequestCompatibilityEnabled, &enabled)
	var req dto.ClaudeRequest
	require.NoError(t, common.Unmarshal(body, &req))
	req.Stream = common.GetPointer(stream)
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ApiType: apiType, ChannelType: channelType, ChannelBaseUrl: server, UpstreamModelName: req.Model, ApiKey: key}, OriginModelName: req.Model, StartTime: time.Now(), RequestURLPath: "/v1/messages", IsStream: stream, RelayFormat: types.RelayFormatClaude, Request: &req, RequestCompatibilityEnabled: &enabled}
	return c, info, recorder
}

func writeSignatureSuccess(w http.ResponseWriter, aws, stream bool) {
	if !stream {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, signatureSuccessResponse)
		return
	}
	events := []string{
		`{"type":"message_start","message":{"id":"local-success","type":"message","role":"assistant","model":"claude-opus-5-5","content":[],"usage":{"input_tokens":0,"output_tokens":0}}}`,
		`{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
		`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"hello"}}`,
		`{"type":"content_block_stop","index":0}`,
		`{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":0}}`,
		`{"type":"message_stop"}`,
	}
	if aws {
		w.Header().Set("Content-Type", "application/vnd.amazon.eventstream")
		enc := eventstream.NewEncoder()
		for _, event := range events {
			payload, _ := common.Marshal(map[string][]byte{"bytes": []byte(event)})
			_ = enc.Encode(w, eventstream.Message{Headers: eventstream.Headers{
				{Name: ":message-type", Value: eventstream.StringValue("event")},
				{Name: ":event-type", Value: eventstream.StringValue("chunk")},
				{Name: ":content-type", Value: eventstream.StringValue("application/json")},
			}, Payload: payload})
		}
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	for _, event := range events {
		_, _ = io.WriteString(w, "data: "+event+"\n\n")
	}
}

// Isolate the real helper success/settlement path from any persistent state.
func setupSignatureBilling(t *testing.T) {
	t.Helper()
	savedDB, savedRedis, savedLogs := model.DB, common.RedisEnabled, common.LogConsumeEnabled
	savedBatch, savedTimeout, savedDebug := common.BatchUpdateEnabled, constant.StreamingTimeout, common.DebugEnabled
	common.BatchUpdateEnabled, constant.StreamingTimeout, common.DebugEnabled = false, 30, false
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.Exec("CREATE TABLE users (id integer primary key, used_quota integer, request_count integer, deleted_at datetime)").Error)
	require.NoError(t, db.Exec("CREATE TABLE channels (id integer primary key, used_quota integer)").Error)
	model.DB, common.RedisEnabled, common.LogConsumeEnabled = db, false, false
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() {
		model.DB, common.RedisEnabled, common.LogConsumeEnabled = savedDB, savedRedis, savedLogs
		common.BatchUpdateEnabled, constant.StreamingTimeout, common.DebugEnabled = savedBatch, savedTimeout, savedDebug
		_ = sqlDB.Close()
	})
}

func TestClaudeSignatureRetryHTTP(t *testing.T) {
	setupSignatureBilling(t)
	service.InitHttpClient()
	for _, aws := range []bool{false, true} {
		for _, stream := range []bool{false, true} {
			for _, tc := range []struct {
				name                                                                             string
				firstStatus                                                                      int
				message                                                                          string
				secondFailure, secondSignature, raw, disabled, cancel, timeout, deadline, active bool
				written, mapping                                                                 string
				expected                                                                         int
			}{
				{name: "success unchanged", expected: 1},
				{name: "precise signature then success", firstStatus: 400, message: signatureValidationMessage, expected: 2},
				{name: "signature stops after one retry", firstStatus: 400, message: signatureValidationMessage, secondSignature: true, expected: 2},
				{name: "mapped raw400 retries", firstStatus: 400, message: signatureValidationMessage, mapping: `{"400":"503"}`, expected: 2},
				{name: "mapped non400 does not retry", firstStatus: 403, message: signatureValidationMessage, mapping: `{"403":"400"}`, expected: 1},
				{name: "second failure remains real", firstStatus: 400, message: signatureValidationMessage, secondFailure: true, expected: 2},
				{name: "other validation", firstStatus: 400, message: "other validation failure", expected: 1},
				{name: "signature non400", firstStatus: 403, message: signatureValidationMessage, expected: 1},
				{name: "disabled", firstStatus: 400, message: signatureValidationMessage, disabled: true, expected: 1},
				{name: "raw", firstStatus: 400, message: signatureValidationMessage, raw: true, expected: 1},
				{name: "cancel", firstStatus: 400, message: signatureValidationMessage, cancel: true, expected: 1},
				{name: "already expired total deadline", firstStatus: 400, message: signatureValidationMessage, deadline: true, expected: 0},
				{name: "upstream canceled", firstStatus: 400, message: signatureValidationMessage, timeout: true, expected: 1},
				{name: "already output", firstStatus: 400, message: signatureValidationMessage, written: "body", expected: 1},
				{name: "already flushed only", firstStatus: 400, message: signatureValidationMessage, written: "flush", expected: 1},
				{name: "already ping", firstStatus: 400, message: signatureValidationMessage, written: "ping", expected: 1},
				{name: "already stream start", firstStatus: 400, message: signatureValidationMessage, written: "start", expected: 1},
				{name: "current tool continuation only", firstStatus: 400, message: signatureValidationMessage, active: true, expected: 1},
			} {
				name := "claude/"
				if aws {
					name = "aws/"
				}
				if stream {
					name += "stream/"
				} else {
					name += "nonstream/"
				}
				t.Run(name+tc.name, func(t *testing.T) {
					var bodies [][]byte
					var bodiesMu sync.Mutex
					var cancel context.CancelFunc
					upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						b, _ := io.ReadAll(r.Body)
						bodiesMu.Lock()
						defer bodiesMu.Unlock()
						bodies = append(bodies, b)

						if len(bodies) == 1 && tc.firstStatus != 0 || len(bodies) == 2 && (tc.secondFailure || tc.secondSignature) {
							status, msg := tc.firstStatus, tc.message
							if len(bodies) == 2 {
								status, msg = 400, "second upstream rejection"
								if tc.secondSignature {
									msg = signatureValidationMessage
								}
							}
							w.Header().Set("Content-Type", "application/json")
							if aws {
								w.Header().Set("X-Amzn-Errortype", "ValidationException")
							}
							w.WriteHeader(status)
							var payload []byte
							if aws {
								payload, _ = common.Marshal(map[string]string{"message": msg})
							} else {
								payload, _ = common.Marshal(map[string]any{"type": "error", "error": map[string]string{"type": "invalid_request_error", "message": msg}})
							}
							_, _ = w.Write(payload)
							return
						}
						writeSignatureSuccess(w, aws, stream)
					}))
					defer upstream.Close()
					target, err := url.Parse(upstream.URL)
					require.NoError(t, err)
					client := service.GetHttpClient()
					saved := client.Transport
					client.Transport = signatureLoopbackTransport{target: target, base: http.DefaultTransport, afterClose: func() {
						if cancel != nil {
							cancel()
						}
					}}
					defer func() { client.Transport = saved }()
					body := []byte(signatureHistoryRequest)
					if tc.active {
						body = []byte(`{"model":"claude-opus-5-5","max_tokens":512,"thinking":{"type":"adaptive"},"messages":[{"role":"user","content":"first"},{"role":"assistant","content":[{"type":"thinking","thinking":"active","signature":"active-synthetic"},{"type":"tool_use","id":"a","name":"local","input":{}}]},{"role":"user","content":[{"type":"tool_result","tool_use_id":"a","content":"result"}]}]}`)
					}
					c, info, recorder := signatureTestContext(t, body, upstream.URL, aws, stream, tc.raw, !tc.disabled)
					if tc.cancel {
						ctx, fn := context.WithCancel(c.Request.Context())
						cancel = fn
						defer fn()
						c.Request = c.Request.WithContext(ctx)
					}
					if tc.timeout {
						ctx, fn := context.WithCancel(context.Background())
						cancel = fn
						defer fn()
						info.UpstreamContext = ctx
					}
					if tc.deadline {
						ctx, fn := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
						defer fn()
						info.UpstreamContext = ctx
					}
					if tc.mapping != "" {
						c.Set("status_code_mapping", tc.mapping)
					}
					switch tc.written {
					case "body":
						_, err := c.Writer.WriteString("already-sent")
						require.NoError(t, err)
					case "flush":
						c.Writer.Flush()
					case "ping":
						_, err := c.Writer.WriteString(`: PING

`)
						require.NoError(t, err)
						c.Writer.Flush()
					case "start":
						_, err := c.Writer.WriteString(`event: message_start
data: {}

`)
						require.NoError(t, err)
						c.Writer.Flush()
					}
					result := ClaudeHelper(c, info)
					bodiesMu.Lock()
					defer bodiesMu.Unlock()
					require.Len(t, bodies, tc.expected)
					if tc.deadline {
						require.NotNil(t, result)
						assert.ErrorIs(t, info.UpstreamContext.Err(), context.DeadlineExceeded)
						return
					}
					if tc.firstStatus == 0 || tc.expected == 2 && !tc.secondFailure && !tc.secondSignature {
						require.Nil(t, result)
						assert.Contains(t, recorder.Body.String(), "hello")
						assert.NotContains(t, recorder.Body.String(), signatureValidationMessage)
					} else {
						require.NotNil(t, result)
						if tc.cancel || tc.timeout {
							assert.Equal(t, http.StatusBadRequest, result.StatusCode)
							assert.Contains(t, result.Error(), signatureValidationMessage, "the precise failure reaches the cancellation gate")
						}
						if tc.secondFailure {
							assert.Contains(t, result.Error(), "second upstream rejection")
							assert.NotContains(t, result.Error(), signatureValidationMessage)
						}
					}
					if tc.raw && !aws {
						assert.Equal(t, body, bodies[0])
					}
					var sent map[string]json.RawMessage
					require.NoError(t, common.Unmarshal(bodies[0], &sent))
					var original map[string]json.RawMessage
					require.NoError(t, common.Unmarshal(body, &original))
					// Compare exact content bytes after JSON compaction, not float64 JSONEq.
					var originalMessages, sentMessages any
					require.NoError(t, common.UnmarshalUseNumber(original["messages"], &originalMessages))
					require.NoError(t, common.UnmarshalUseNumber(sent["messages"], &sentMessages))
					assert.Equal(t, originalMessages, sentMessages, "first request must retain the original history")
					if tc.expected == 2 {
						require.NoError(t, common.Unmarshal(bodies[1], &sent))
						expected, changed := stripHistoricalClaudeThinking(bodies[0])
						require.True(t, changed)
						var want, got any
						require.NoError(t, common.UnmarshalUseNumber(expected, &want))
						require.NoError(t, common.UnmarshalUseNumber(bodies[1], &got))
						assert.Equal(t, want, got, "retry changes only completed historical thinking")
						assert.JSONEq(t, `{"type":"adaptive","display":"summarized"}`, string(sent["thinking"]))
						assert.JSONEq(t, `{"effort":"medium","extension":9007199254740993}`, string(sent["output_config"]))
						assert.NotContains(t, string(sent["messages"]), "synthetic-signature")
						assert.NotContains(t, string(sent["messages"]), "synthetic-redacted")
					}
				})
			}
		}
	}
}

func TestClaudeSignatureConservativeBoundaries(t *testing.T) {
	for _, content := range []string{
		`[{"type":"tool_result","tool_use_id":"x"},{"type":"text","text":"ambiguous"}]`,
		`[{"type":"future_input","value":9007199254740993}]`,
	} {
		body := []byte(`{"messages":[{"role":"assistant","content":[{"type":"thinking","signature":"keep"},{"type":"text","text":"answer"}]},{"role":"user","content":` + content + `}]}`)
		got, changed := stripHistoricalClaudeThinking(body)
		require.False(t, changed)
		assert.Equal(t, body, got, "ambiguous user boundary must not change the request")
	}
}

// Optional current attachment replay, never transmitted outside a loopback mock
// and never saved or printed. Unlike the old adaptive fixture, compare the actual
// summarized display and medium effort instead of imposing another request.
func TestClaudeSignatureAttachmentReplay(t *testing.T) {
	path := os.Getenv("NEWAPI_CLAUDE_SIGNATURE_ATTACHMENT")
	if path == "" {
		t.Skip("set NEWAPI_CLAUDE_SIGNATURE_ATTACHMENT for local-only replay")
	}
	setupSignatureBilling(t)
	body, err := os.ReadFile(path)
	require.NoError(t, err)
	var original map[string]json.RawMessage
	require.NoError(t, common.Unmarshal(body, &original))
	var messages []json.RawMessage
	require.NoError(t, common.Unmarshal(original["messages"], &messages))
	require.Len(t, messages, 54)
	require.JSONEq(t, `{"display":"summarized","type":"adaptive"}`, string(original["thinking"]))
	require.JSONEq(t, `{"effort":"medium"}`, string(original["output_config"]))
	service.InitHttpClient()
	for _, aws := range []bool{false, true} {
		t.Run(map[bool]string{false: "claude", true: "aws"}[aws], func(t *testing.T) {
			var bodies [][]byte
			var bodiesMu sync.Mutex
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				b, _ := io.ReadAll(r.Body)
				bodiesMu.Lock()
				defer bodiesMu.Unlock()
				bodies = append(bodies, b)
				if len(bodies) == 1 {
					w.Header().Set("Content-Type", "application/json")
					w.Header().Set("X-Amzn-Errortype", "ValidationException")
					w.WriteHeader(400)
					if aws {
						payload, _ := common.Marshal(map[string]string{"message": signatureValidationMessage})
						_, _ = w.Write(payload)
					} else {
						payload, _ := common.Marshal(map[string]any{"type": "error", "error": map[string]string{"type": "invalid_request_error", "message": signatureValidationMessage}})
						_, _ = w.Write(payload)
					}
					return
				}
				writeSignatureSuccess(w, aws, true)
			}))
			defer upstream.Close()
			target, err := url.Parse(upstream.URL)
			require.NoError(t, err)
			client := service.GetHttpClient()
			saved := client.Transport
			client.Transport = signatureLoopbackTransport{target: target, base: http.DefaultTransport}
			defer func() { client.Transport = saved }()
			c, info, _ := signatureTestContext(t, body, upstream.URL, aws, true, false, true)
			require.Nil(t, ClaudeHelper(c, info))
			bodiesMu.Lock()
			defer bodiesMu.Unlock()
			require.Len(t, bodies, 2)
			for index, b := range bodies {
				var sent map[string]json.RawMessage
				require.NoError(t, common.Unmarshal(b, &sent))
				for _, field := range []string{"thinking", "output_config", "tools", "system"} {
					var want, got any
					require.NoError(t, common.UnmarshalUseNumber(original[field], &want))
					require.NoError(t, common.UnmarshalUseNumber(sent[field], &got))
					// Avoid printing user content on a failed assertion.
					require.True(t, assert.ObjectsAreEqual(want, got), "attachment field %s differs at attempt %d", field, index+1)
				}
				var wantBody []byte
				if index == 0 {
					wantBody = body
				} else {
					var changed bool
					wantBody, changed = stripHistoricalClaudeThinking(body)
					require.True(t, changed)
				}
				var wantFields map[string]json.RawMessage
				require.NoError(t, common.Unmarshal(wantBody, &wantFields))
				var want, got any
				require.NoError(t, common.UnmarshalUseNumber(wantFields["messages"], &want))
				require.NoError(t, common.UnmarshalUseNumber(sent["messages"], &got))
				require.True(t, assert.ObjectsAreEqual(want, got), "attachment history differs at attempt %d", index+1)
				// Count using generic messages since some user content is a string.
				var parsed []map[string]json.RawMessage
				require.NoError(t, common.Unmarshal(sent["messages"], &parsed))
				count := 0
				for _, message := range parsed {
					var blocks []map[string]json.RawMessage
					if common.Unmarshal(message["content"], &blocks) != nil {
						continue
					}
					for _, block := range blocks {
						if string(block["type"]) == `"thinking"` {
							count++
						}
					}
				}
				if index == 0 {
					assert.Equal(t, 12, count)
				} else {
					assert.Zero(t, count)
				}
			}
		})
	}
}
