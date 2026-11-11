package relay

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"sync"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/service"
	"github.com/stretchr/testify/require"
)

const signatureToolRequest = `{"model":"claude-opus-4-7","max_tokens":64000,"thinking":{"type":"adaptive","display":"summarized"},"output_config":{"effort":"high"},"extension":900719925474099312345,"messages":[{"role":"user","content":"start"},{"role":"assistant","content":[{"type":"thinking","thinking":"first","signature":"signed-prefix-1"},{"type":"redacted_thinking","data":"encrypted"},{"type":"text","text":"plan"},{"type":"tool_use","id":"a","name":"local","input":{"n":9007199254740993}}]},{"role":"user","content":[{"type":"tool_result","tool_use_id":"a","content":"done"}]},{"role":"assistant","content":[{"type":"thinking","thinking":"second","signature":"signed-prefix-2"},{"type":"tool_use","id":"b","name":"local","input":{}}]},{"role":"user","content":[{"type":"tool_result","tool_use_id":"b","content":"done"}]}]}`

// Compare against an independent raw-value deletion oracle, not the production
// recovery function. Neither assertion failures nor logs disclose user content.
func assertOnlyThinkingRemoved(t *testing.T, before, after []byte, wantRemoved int) {
	t.Helper()
	decode := func(b []byte) map[string]any {
		var value map[string]any
		require.NoError(t, common.UnmarshalUseNumber(b, &value))
		return value
	}
	want, got := decode(before), decode(after)
	removed := 0
	for _, raw := range want["messages"].([]any) {
		message := raw.(map[string]any)
		if message["role"] != "assistant" {
			continue
		}
		blocks, ok := message["content"].([]any)
		if !ok {
			continue
		}
		kept := make([]any, 0, len(blocks))
		for _, block := range blocks {
			kind := block.(map[string]any)["type"]
			if kind == "thinking" || kind == "redacted_thinking" {
				removed++
				continue
			}
			kept = append(kept, block)
		}
		message["content"] = kept
	}
	require.Equal(t, wantRemoved, removed)
	require.True(t, objectsEqualJSON(want, got), "retry changed fields other than assistant thinking")
}

func objectsEqualJSON(a, b any) bool {
	aa, ea := json.Marshal(a)
	bb, eb := json.Marshal(b)
	return ea == nil && eb == nil && bytes.Equal(aa, bb)
}

func TestClaudeToolSignatureRecovery(t *testing.T) {
	body := []byte(signatureToolRequest)
	out, changed, lossy := prepareClaudeSignatureRecovery(body)
	require.True(t, changed)
	require.True(t, lossy)
	assertOnlyThinkingRemoved(t, body, out, 3)
	again, changed, lossy := prepareClaudeSignatureRecovery(out)
	require.False(t, changed)
	require.False(t, lossy)
	require.Equal(t, out, again)
	// Explicit real user boundary still takes the old, lossless-history path.
	out, changed, lossy = prepareClaudeSignatureRecovery([]byte(signatureHistoryRequest))
	require.True(t, changed)
	require.False(t, lossy)
	expected, _ := stripHistoricalClaudeThinking([]byte(signatureHistoryRequest))
	require.Equal(t, expected, out)
}

func TestClaudeToolSignatureStringHistory(t *testing.T) {
	body := bytes.Replace([]byte(signatureToolRequest), []byte(`"messages":[`), []byte(`"messages":[{"role":"user","content":"old"},{"role":"assistant","content":"completed answer"},`), 1)
	out, changed, lossy := prepareClaudeSignatureRecovery(body)
	require.True(t, changed)
	require.True(t, lossy)
	assertOnlyThinkingRemoved(t, body, out, 3)
}

func TestClaudeSignatureRecoveryAtomicHistory(t *testing.T) {
	body := []byte(`{"messages":[{"role":"assistant","content":[{"type":"thinking","signature":"first"},{"type":"text","text":"answer"}]},{"role":"assistant","content":[{"type":"thinking","signature":"keep-dependent"}]},{"role":"user","content":"next"}]}`)
	out, changed, lossy := prepareClaudeSignatureRecovery(body)
	require.False(t, changed)
	require.False(t, lossy)
	require.Equal(t, body, out)
}

func TestClaudeToolSignatureConservativeBoundaries(t *testing.T) {
	for _, tc := range []struct{ name, old, replacement string }{
		{"unmatched tool", `"tool_use_id":"b"`, `"tool_use_id":"unknown"`},
		{"duplicate call", `"id":"b"`, `"id":"a"`},
		{"duplicate result", `"tool_use_id":"b"`, `"tool_use_id":"a"`},
		{"mixed user content", `"tool_use_id":"b","content":"done"}`, `"tool_use_id":"b","content":"done"},{"type":"text","text":"new input"}`},
		{"server tool", `"type":"tool_use","id":"b"`, `"type":"server_tool_use","id":"b"`},
		{"unknown content", `"type":"text","text":"plan"`, `"type":"future_output","value":9007199254740993`},
		{"thinking-only", `,{"type":"tool_use","id":"b","name":"local","input":{}}`, ``},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := bytes.Replace([]byte(signatureToolRequest), []byte(tc.old), []byte(tc.replacement), 1)
			require.NotEqual(t, []byte(signatureToolRequest), body)
			out, changed, lossy := prepareClaudeSignatureRecovery(body)
			require.False(t, changed)
			require.False(t, lossy)
			require.Equal(t, body, out)
		})
	}
	for _, body := range [][]byte{[]byte(`{`), []byte(`{"messages":null}`), []byte(`{"messages":[{"role":"assistant","content":[{"type":"thinking"}]}]}`)} {
		out, changed, lossy := prepareClaudeSignatureRecovery(body)
		require.False(t, changed)
		require.False(t, lossy)
		require.Equal(t, body, out)
	}
	// No assistant prefill recovery, even with an otherwise complete tool pair.
	var req map[string]json.RawMessage
	require.NoError(t, json.Unmarshal([]byte(signatureToolRequest), &req))
	var messages []json.RawMessage
	require.NoError(t, json.Unmarshal(req["messages"], &messages))
	messages = messages[:len(messages)-1]
	req["messages"], _ = json.Marshal(messages)
	body, _ := json.Marshal(req)
	out, changed, lossy := prepareClaudeSignatureRecovery(body)
	require.False(t, changed)
	require.False(t, lossy)
	require.Equal(t, body, out)
}

func TestClaudeSignatureRetryBudgetAcrossHelperReentry(t *testing.T) {
	setupSignatureBilling(t)
	service.InitHttpClient()
	for _, aws := range []bool{false, true} {
		for _, stream := range []bool{false, true} {
			name := "claude"
			if aws {
				name = "aws"
			}
			if stream {
				name += "/stream"
			} else {
				name += "/nonstream"
			}
			t.Run(name, func(t *testing.T) {
				var mu sync.Mutex
				calls := 0
				upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					_, _ = io.Copy(io.Discard, r.Body)
					mu.Lock()
					calls++
					mu.Unlock()
					w.Header().Set("Content-Type", "application/json")
					w.Header().Set("X-Amzn-Errortype", "ValidationException")
					w.WriteHeader(429)
					if aws {
						_ = json.NewEncoder(w).Encode(map[string]string{"message": signatureValidationMessage})
					} else {
						_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"type": "invalid_request_error", "message": signatureValidationMessage}})
					}
				}))
				defer upstream.Close()
				target, err := url.Parse(upstream.URL)
				require.NoError(t, err)
				client := service.GetHttpClient()
				saved := client.Transport
				client.Transport = signatureLoopbackTransport{target: target, base: http.DefaultTransport}
				defer func() { client.Transport = saved }()
				c, info, _ := signatureTestContext(t, []byte(signatureToolRequest), upstream.URL, aws, stream, false, true)
				common.SetContextKey(c, constant.ContextKeyChannelId, 101)
				require.NotNil(t, ClaudeHelper(c, info))
				mu.Lock()
				require.Equal(t, 2, calls)
				mu.Unlock()
				// The controller re-enters with the original request, same channel,
				// potentially another credential. No second compatibility retry.
				require.NotNil(t, ClaudeHelper(c, info))
				mu.Lock()
				require.Equal(t, 3, calls)
				mu.Unlock()
				common.SetContextKey(c, constant.ContextKeyChannelId, 102)
				require.NotNil(t, ClaudeHelper(c, info))
				mu.Lock()
				require.Equal(t, 5, calls)
				mu.Unlock()
			})
		}
	}
}

// Optional exact request replay: payload remains exclusively on localhost. This
// mock proves conversion/retention/retry boundaries, NOT signature validity or
// acceptance by the real provider.
func TestClaudeToolSignatureAttachmentReplay(t *testing.T) {
	path := os.Getenv("NEWAPI_CLAUDE_TOOL_SIGNATURE_ATTACHMENT")
	if path == "" {
		t.Skip("set NEWAPI_CLAUDE_TOOL_SIGNATURE_ATTACHMENT for localhost-only replay")
	}
	body, err := os.ReadFile(path)
	require.NoError(t, err)
	var original map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(body, &original))
	var messages []json.RawMessage
	require.NoError(t, json.Unmarshal(original["messages"], &messages))
	require.Len(t, messages, 57)
	runClaudeToolSignatureReplay(t, body, 19, true)
}

func TestClaudeToolSignaturePrecisionHTTP(t *testing.T) {
	// Existing DTO/SDK any conversion may already round the first payload.
	// Only assert this recovery adds no changes beyond deleting thinking.
	runClaudeToolSignatureReplay(t, []byte(signatureToolRequest), 3, false)
}

func runClaudeToolSignatureReplay(t *testing.T, body []byte, removed int, compareInitial bool) {
	var original map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(body, &original))
	setupSignatureBilling(t)
	service.InitHttpClient()
	for _, aws := range []bool{false, true} {
		for _, stream := range []bool{false, true} {
			name := "claude"
			if aws {
				name = "aws"
			}
			if stream {
				name += "/stream"
			} else {
				name += "/nonstream"
			}
			t.Run(name, func(t *testing.T) {
				var bodies [][]byte
				var mu sync.Mutex
				upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					b, readErr := io.ReadAll(r.Body)
					if readErr != nil {
						w.WriteHeader(500)
						return
					}
					mu.Lock()
					defer mu.Unlock()
					bodies = append(bodies, b)
					if len(bodies) == 1 {
						w.Header().Set("Content-Type", "application/json")
						w.Header().Set("X-Amzn-Errortype", "ValidationException")
						w.WriteHeader(429)
						payload := map[string]any{"error": map[string]string{"type": "invalid_request_error", "message": "messages.44.content.0: " + signatureValidationMessage}}
						if aws {
							payload = map[string]any{"message": signatureValidationMessage}
						}
						_ = json.NewEncoder(w).Encode(payload)
						return
					}
					writeSignatureSuccess(w, aws, stream)
				}))
				defer upstream.Close()
				target, err := url.Parse(upstream.URL)
				require.NoError(t, err)
				client := service.GetHttpClient()
				saved := client.Transport
				client.Transport = signatureLoopbackTransport{target: target, base: http.DefaultTransport}
				defer func() { client.Transport = saved }()
				c, info, recorder := signatureTestContext(t, body, upstream.URL, aws, stream, false, true)
				require.Nil(t, ClaudeHelper(c, info))
				require.Contains(t, recorder.Body.String(), "hello")
				mu.Lock()
				defer mu.Unlock()
				require.Len(t, bodies, 2)
				// First transmission preserves all 57 messages, including all signatures.
				var first map[string]json.RawMessage
				require.NoError(t, json.Unmarshal(bodies[0], &first))
				var want, got any
				require.NoError(t, common.UnmarshalUseNumber(original["messages"], &want))
				require.NoError(t, common.UnmarshalUseNumber(first["messages"], &got))
				if compareInitial {
					require.True(t, objectsEqualJSON(want, got), "initial attachment messages changed")
				}
				assertOnlyThinkingRemoved(t, bodies[0], bodies[1], removed)
				// The precise fallback event must be distinct from safe historical cleanup.
				admin := map[string]interface{}{}
				service.AppendRelayCompatibilityAdminInfo(c, admin)
				events := admin["compatibility_events"].([]service.RelayCompatibilityEvent)
				require.Len(t, events, 1)
				require.Equal(t, "QIQI-EC-006", events[0].RuleID)
			})
		}
	}
}
