package relay

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/stretchr/testify/require"
)

func TestClaudeRejectedHistoricalThinkingOnlyMessage(t *testing.T) {
	body := []byte(`{"model":"claude-opus-5-5","thinking":{"type":"adaptive"},"messages":[{"role":"user","content":"old question"},{"role":"assistant","content":[{"type":"thinking","thinking":"","signature":"synthetic-signature"},{"type":"redacted_thinking","data":"opaque reasoning"}]},{"role":"user","content":"next question"}]}`)

	out, changed := stripHistoricalClaudeThinking(body)
	require.False(t, changed, "ordinary history cleanup must retain the existing pure-thinking guard")
	require.Equal(t, body, out)

	out, changed, lossy := prepareClaudeSignatureRecovery(body)
	require.True(t, changed)
	require.False(t, lossy)
	want := []byte(`{"model":"claude-opus-5-5","thinking":{"type":"adaptive"},"messages":[{"role":"user","content":"old question"},{"role":"user","content":"next question"}]}`)
	assertJSONValuesEqual(t, want, out)

	for _, tc := range []struct {
		name, assistant string
	}{
		{"visible assistant text", `{"role":"assistant","content":[{"type":"thinking","thinking":"reasoning","signature":"s"},{"type":"text","text":"visible answer"}]}`},
		{"tool call", `{"role":"assistant","content":[{"type":"thinking","thinking":"reasoning","signature":"s"},{"type":"tool_use","id":"tool-1","name":"lookup","input":{"query":"keep"}}]}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			request := []byte(`{"messages":[{"role":"user","content":"old"},` + tc.assistant + `,{"role":"user","content":"next"}]}`)
			out, changed, lossy := prepareClaudeSignatureRecovery(request)
			require.False(t, lossy)
			require.True(t, changed)
			var got map[string]json.RawMessage
			require.NoError(t, common.Unmarshal(out, &got))
			var messages []json.RawMessage
			require.NoError(t, common.Unmarshal(got["messages"], &messages))
			require.Len(t, messages, 3, "assistant text and tool semantics must not be dropped")
			var assistant map[string]json.RawMessage
			require.NoError(t, common.Unmarshal(messages[1], &assistant))
			var blocks []map[string]json.RawMessage
			require.NoError(t, common.Unmarshal(assistant["content"], &blocks))
			require.Len(t, blocks, 1)
			encodedBlock, err := common.Marshal(blocks[0])
			require.NoError(t, err)
			if tc.name == "visible assistant text" {
				require.JSONEq(t, `{"type":"text","text":"visible answer"}`, string(encodedBlock))
			} else {
				require.JSONEq(t, `{"type":"tool_use","id":"tool-1","name":"lookup","input":{"query":"keep"}}`, string(encodedBlock))
			}
		})
	}

	for _, tc := range []struct {
		name, assistant string
	}{
		{"message metadata", `{"role":"assistant","stop_reason":"end_turn","content":[{"type":"thinking","thinking":"reasoning","signature":"s"}]}`},
		{"unknown thinking metadata", `{"role":"assistant","content":[{"type":"thinking","thinking":"reasoning","signature":"s","extension":"unknown semantics"}]}`},
		{"null thinking", `{"role":"assistant","content":[{"type":"thinking","thinking":null,"signature":"s"}]}`},
		{"null signature", `{"role":"assistant","content":[{"type":"thinking","thinking":"work","signature":null}]}`},
		{"missing redacted data", `{"role":"assistant","content":[{"type":"redacted_thinking"}]}`},
	} {
		t.Run("unsafe "+tc.name, func(t *testing.T) {
			request := []byte(`{"messages":[{"role":"user","content":"old"},` + tc.assistant + `,{"role":"user","content":"next"}]}`)
			out, changed, lossy := prepareClaudeSignatureRecovery(request)
			require.False(t, lossy)
			require.False(t, changed)
			require.Equal(t, request, out)
		})
	}
}

func TestClaudeSignatureSystemAndMixedToolBoundaries(t *testing.T) {
	body := []byte(`{"model":"claude-opus-5-5","thinking":{"type":"adaptive","budget_tokens":4096},"messages":[{"role":"user","content":"old question"},{"role":"assistant","content":[{"type":"thinking","thinking":"old reasoning","signature":"old-signature"},{"type":"text","text":"visible answer"}]},{"role":"system","content":"middle system text"},{"role":"user","content":"continue"},{"role":"system","content":[{"type":"text","text":"system before tools","extension":900719925474099312345}]},{"role":"assistant","content":[{"type":"thinking","thinking":"active reasoning","signature":"active-signature"},{"type":"tool_use","id":"call-1","name":"lookup","input":{"query":"preserve"}}]},{"role":"user","content":[{"type":"tool_result","tool_use_id":"call-1","content":"result"},{"type":"text","text":"client reminder","extension":900719925474099312345}]},{"role":"system","content":"trailing system text"}]}`)
	out, changed, lossy := prepareClaudeSignatureRecovery(body)
	require.True(t, changed)
	require.True(t, lossy)
	want := []byte(`{"model":"claude-opus-5-5","thinking":{"type":"adaptive","budget_tokens":4096},"messages":[{"role":"user","content":"old question"},{"role":"assistant","content":[{"type":"text","text":"visible answer"}]},{"role":"system","content":"middle system text"},{"role":"user","content":"continue"},{"role":"system","content":[{"type":"text","text":"system before tools","extension":900719925474099312345}]},{"role":"assistant","content":[{"type":"tool_use","id":"call-1","name":"lookup","input":{"query":"preserve"}}]},{"role":"user","content":[{"type":"tool_result","tool_use_id":"call-1","content":"result"},{"type":"text","text":"client reminder","extension":900719925474099312345}]},{"role":"system","content":"trailing system text"}]}`)
	assertJSONValuesEqual(t, want, out)

	mixed := json.RawMessage(`[{"type":"tool_result","tool_use_id":"x","content":"result"},{"type":"text","text":"reminder"}]`)
	genuine, known := claudeSignatureUserBoundary(mixed, true)
	require.False(t, genuine, "mixed text/tool results are continuations, not a new user boundary")
	require.True(t, known)
	genuine, known = claudeGenuineUserContent(mixed)
	require.False(t, genuine)
	require.False(t, known, "ordinary history cleanup remains strict")

	withoutActiveThinking := []byte(`{"messages":[{"role":"user","content":"old"},{"role":"assistant","content":[{"type":"thinking","thinking":"completed reasoning","signature":"old"},{"type":"text","text":"visible answer"}]},{"role":"user","content":"current"},{"role":"system","content":"system text"},{"role":"assistant","content":[{"type":"tool_use","id":"call","name":"lookup","input":{}}]},{"role":"user","content":[{"type":"tool_result","tool_use_id":"call","content":"result"},{"type":"text","text":"keep reminder"}]}]}`)
	out, changed, lossy = prepareClaudeSignatureRecovery(withoutActiveThinking)
	require.True(t, changed, "a complete unsigned tool turn must not block historical recovery")
	require.False(t, lossy)
	assertOnlyThinkingRemoved(t, withoutActiveThinking, out, 1)

	unmatched := bytes.Replace(withoutActiveThinking, []byte(`"tool_use_id":"call"`), []byte(`"tool_use_id":"unknown"`), 1)
	out, changed, lossy = prepareClaudeSignatureRecovery(unmatched)
	require.False(t, changed)
	require.False(t, lossy)
	require.Equal(t, unmatched, out)

	for _, tc := range []struct {
		name, userContent, systemContent string
	}{
		{"media mixed with result", `[{"type":"tool_result","tool_use_id":"x"},{"type":"image","source":{}}]`, `"keep"`},
		{"text before result", `[{"type":"text","text":"reminder"},{"type":"tool_result","tool_use_id":"x"}]`, `"keep"`},
		{"missing text", `[{"type":"tool_result","tool_use_id":"x"},{"type":"text"}]`, `"keep"`},
		{"null text", `[{"type":"tool_result","tool_use_id":"x"},{"type":"text","text":null}]`, `"keep"`},
		{"opaque system", `[{"type":"tool_result","tool_use_id":"x"},{"type":"text","text":"reminder"}]`, `{"unexpected":true}`},
		{"non-text system block", `[{"type":"tool_result","tool_use_id":"x"},{"type":"text","text":"reminder"}]`, `[{"type":"image"}]`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			request := []byte(`{"messages":[{"role":"user","content":"start"},{"role":"assistant","content":[{"type":"thinking","thinking":"active","signature":"s"},{"type":"tool_use","id":"x","name":"lookup","input":{}}]},{"role":"user","content":` + tc.userContent + `},{"role":"system","content":` + tc.systemContent + `}]}`)
			out, changed, lossy := prepareClaudeSignatureRecovery(request)
			require.False(t, changed)
			require.False(t, lossy)
			require.Equal(t, request, out)
		})
	}
}

func TestClaudeSignatureHistoricalSystemBoundaryHTTP(t *testing.T) {
	setupSignatureBilling(t)
	service.InitHttpClient()

	cases := []struct {
		name   string
		body   string
		status int
		lossy  bool
	}{
		{
			name:   "rejected pure-thinking completion and trailing system",
			status: http.StatusBadRequest,
			body:   `{"model":"claude-opus-5-5","max_tokens":512,"thinking":{"type":"adaptive"},"messages":[{"role":"user","content":"old question"},{"role":"assistant","content":[{"type":"thinking","thinking":"","signature":"synthetic-signature"}]},{"role":"user","content":"next question"},{"role":"system","content":[{"type":"text","text":"keep trailing system"}]}]}`,
		},
		{
			name:   "mixed tool result and trailing system",
			status: http.StatusTooManyRequests,
			lossy:  true,
			body:   `{"model":"claude-opus-5-5","max_tokens":512,"thinking":{"type":"adaptive"},"messages":[{"role":"user","content":"start"},{"role":"assistant","content":[{"type":"thinking","thinking":"active reasoning","signature":"synthetic-signature"},{"type":"tool_use","id":"call-1","name":"lookup","input":{"query":"keep"}}]},{"role":"user","content":[{"type":"tool_result","tool_use_id":"call-1","content":"result"},{"type":"text","text":"keep reminder"}]},{"role":"system","content":"keep trailing system"}]}`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body := []byte(tc.body)
			var mu sync.Mutex
			var received [][]byte
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requestBody, err := io.ReadAll(r.Body)
				if err != nil {
					w.WriteHeader(http.StatusInternalServerError)
					return
				}
				mu.Lock()
				received = append(received, requestBody)
				attempt := len(received)
				mu.Unlock()
				if attempt == 1 {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(tc.status)
					_, _ = io.WriteString(w, `{"error":{"type":"invalid_request_error","message":"`+signatureValidationMessage+`"}}`)
					return
				}
				writeSignatureSuccess(w, false, false)
			}))
			defer upstream.Close()

			target, err := url.Parse(upstream.URL)
			require.NoError(t, err)
			client := service.GetHttpClient()
			savedTransport := client.Transport
			client.Transport = signatureLoopbackTransport{target: target, base: http.DefaultTransport}
			defer func() { client.Transport = savedTransport }()

			c, info, recorder := signatureTestContext(t, body, upstream.URL, false, false, false, true)
			require.Nil(t, ClaudeHelper(c, info))
			require.Contains(t, recorder.Body.String(), "hello")

			mu.Lock()
			captured := append([][]byte(nil), received...)
			mu.Unlock()
			require.Len(t, captured, 2)
			var original, first map[string]json.RawMessage
			require.NoError(t, common.Unmarshal(body, &original))
			require.NoError(t, common.Unmarshal(captured[0], &first))
			assertJSONValuesEqual(t, original["messages"], first["messages"])

			if tc.lossy {
				assertOnlyThinkingRemoved(t, captured[0], captured[1], 1)
			} else {
				first["messages"] = json.RawMessage(`[{"role":"user","content":"old question"},{"role":"user","content":"next question"},{"role":"system","content":[{"type":"text","text":"keep trailing system"}]}]`)
				expectedRetry, err := common.Marshal(first)
				require.NoError(t, err)
				assertJSONValuesEqual(t, expectedRetry, captured[1])
			}

			adminInfo := map[string]interface{}{}
			service.AppendRelayCompatibilityAdminInfo(c, adminInfo)
			events, ok := adminInfo["compatibility_events"].([]service.RelayCompatibilityEvent)
			require.True(t, ok)
			require.Len(t, events, 1)
			wantRule := "QIQI-EC-005"
			if tc.lossy {
				wantRule = "QIQI-EC-006"
			}
			require.Equal(t, wantRule, events[0].RuleID)
		})
	}
}

// System messages remain opaque, in place, and are not a new user boundary.
func TestClaudeSignatureSystemBoundary(t *testing.T) {
	for _, system := range []string{
		`{"role":"system","content":"keep system verbatim"}`,
		`{"role":"system","content":[{"type":"text","text":"keep blocks","extension":900719925474099312345}]}`,
	} {
		for _, position := range []string{"before user", "after user", "both"} {
			t.Run(position+"/"+system[:20], func(t *testing.T) {
				body := []byte(signatureHistoryRequest)
				if position != "after user" {
					body = bytes.Replace(body, []byte(`{"role":"user","content":"next"}`), []byte(system+`,{"role":"user","content":"next"}`), 1)
				}
				if position != "before user" {
					body = append(append([]byte{}, body[:len(body)-2]...), []byte(","+system+"]}")...)
				}
				out, changed, lossy := prepareClaudeSignatureRecovery(body)
				require.True(t, changed)
				require.False(t, lossy)
				assertOnlyThinkingRemoved(t, body, out, 2)
			})
		}
	}
	for _, tc := range []struct{ name, body string }{
		{"unknown trailing role", signatureHistoryRequest[:len(signatureHistoryRequest)-2] + `,{"role":"developer","content":"unknown"}]}`},
		{"unknown user content", `{"messages":[{"role":"assistant","content":[{"type":"thinking"},{"type":"text","text":"old"}]},{"role":"user","content":[{"type":"unknown"}]},{"role":"system","content":"keep"}]}`},
		{"atomic thinking only", `{"messages":[{"role":"assistant","content":[{"type":"thinking"},{"type":"text","text":"old"}]},{"role":"assistant","content":[{"type":"thinking"}]},{"role":"user","content":"next"},{"role":"system","content":"keep"}]}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, changed, lossy := prepareClaudeSignatureRecovery([]byte(tc.body))
			require.False(t, changed)
			require.False(t, lossy)
			require.Equal(t, []byte(tc.body), out)
		})
	}
}

func assertJSONValuesEqual(t *testing.T, want, got []byte) {
	t.Helper()
	var expected, actual any
	require.NoError(t, common.UnmarshalUseNumber(want, &expected))
	require.NoError(t, common.UnmarshalUseNumber(got, &actual))
	require.True(t, objectsEqualJSON(expected, actual), "JSON values differ")
}
