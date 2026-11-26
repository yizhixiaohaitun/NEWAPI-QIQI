package relay

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/service"
	"github.com/stretchr/testify/require"
)

// Synthetic signed and unsigned thinking across two tool steps, a system
// notice, and a final tool result followed by client text.
func TestClaudeSignatureInterleavedToolHistoryHTTP(t *testing.T) {
	body := []byte(`{"model":"claude-opus-5-5","max_tokens":512,"stream":true,"thinking":{"type":"adaptive"},"output_config":{"effort":"medium"},"messages":[{"role":"user","content":"previous request"},{"role":"assistant","content":[{"type":"thinking","thinking":"","signature":"opaque-completed"},{"type":"text","text":"previous answer"}]},{"role":"user","content":"continue using tools"},{"role":"assistant","content":[{"type":"thinking","thinking":"partly recorded","signature":""},{"type":"tool_use","id":"tool-a","name":"local","input":{}}]},{"role":"user","content":[{"type":"tool_result","tool_use_id":"tool-a","content":"result a"}]},{"role":"system","content":"injected notice stays in place"},{"role":"assistant","content":[{"type":"thinking","thinking":"","signature":"opaque-active"},{"type":"tool_use","id":"tool-b","name":"local","input":{}}]},{"role":"user","content":[{"type":"tool_result","tool_use_id":"tool-b","content":"result b"},{"type":"text","text":"keep client reminder"}]}]}`)
	runClaudeToolSignatureReplay(t, body, 3, true)
}

// Native signed frames must survive the actual stream writer and the next
// request without dropping, substituting or moving a signature. This is a
// transport test; these opaque values are not real provider signatures.
func TestClaudeSignatureNativeToolRoundTrip(t *testing.T) {
	setupSignatureBilling(t)
	service.InitHttpClient()
	initial := []byte(`{"model":"claude-opus-5-5","max_tokens":512,"stream":true,"thinking":{"type":"adaptive"},"messages":[{"role":"user","content":"begin"}]}`)
	frames := []string{
		`{"type":"message_start","message":{"id":"synthetic-message","type":"message","role":"assistant","model":"claude-opus-5-5","content":[],"usage":{"input_tokens":1,"output_tokens":0}}}`,
		`{"type":"content_block_start","index":0,"content_block":{"type":"thinking","thinking":"","signature":""}}`,
		`{"type":"content_block_delta","index":0,"delta":{"type":"signature_delta","signature":"opaque-signature-one+/="}}`,
		`{"type":"content_block_stop","index":0}`,
		`{"type":"content_block_start","index":1,"content_block":{"type":"thinking","thinking":"","signature":""}}`,
		`{"type":"content_block_delta","index":1,"delta":{"type":"thinking_delta","thinking":"work"}}`,
		`{"type":"content_block_delta","index":1,"delta":{"type":"signature_delta","signature":"opaque-signature-two+/="}}`,
		`{"type":"content_block_stop","index":1}`,
		`{"type":"content_block_start","index":2,"content_block":{"type":"tool_use","id":"tool-a","name":"local","input":{}}}`,
		`{"type":"content_block_stop","index":2}`,
		`{"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"output_tokens":1}}`,
		`{"type":"message_stop"}`,
	}
	var mu sync.Mutex
	var requests [][]byte
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		mu.Lock()
		requests = append(requests, body)
		count := len(requests)
		mu.Unlock()
		if count != 1 {
			writeSignatureSuccess(w, false, true)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		for _, frame := range frames {
			_, _ = fmt.Fprintf(w, "data: %s\n\n", frame)
		}
	}))
	defer upstream.Close()
	c, info, first := signatureTestContext(t, initial, upstream.URL, false, true, false, true)
	common.SetContextKey(c, constant.ContextKeyChannelId, 0)
	require.Nil(t, ClaudeHelper(c, info))

	// Rebuild the native assistant turn from the client-visible frames only.
	var blocks []map[string]any
	var blockFrames []string
	for _, line := range strings.Split(first.Body.String(), "\n") {
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		var frame struct {
			Type  string
			Index int
			Block map[string]any `json:"content_block"`
			Delta map[string]any
		}
		require.NoError(t, common.UnmarshalJsonStr(data, &frame))
		switch frame.Type {
		case "content_block_start":
			require.Len(t, blocks, frame.Index)
			blocks = append(blocks, frame.Block)
		case "content_block_delta":
			switch frame.Delta["type"] {
			case "thinking_delta":
				blocks[frame.Index]["thinking"] = frame.Delta["thinking"]
			case "signature_delta":
				blocks[frame.Index]["signature"] = frame.Delta["signature"]
			}
		case "content_block_stop":
		default:
			continue
		}
		blockFrames = append(blockFrames, data)
	}
	require.True(t, slices.Equal(frames[1:10], blockFrames), "native thinking, signature and tool frames changed")

	var request map[string]any
	require.NoError(t, common.Unmarshal(initial, &request))
	request["messages"] = append(request["messages"].([]any),
		map[string]any{"role": "assistant", "content": blocks},
		map[string]any{"role": "user", "content": []any{map[string]any{"type": "tool_result", "tool_use_id": "tool-a", "content": "ok"}}},
	)
	continued, err := common.Marshal(request)
	require.NoError(t, err)
	c, info, _ = signatureTestContext(t, continued, upstream.URL, false, true, false, true)
	common.SetContextKey(c, constant.ContextKeyChannelId, 0)
	require.Nil(t, ClaudeHelper(c, info))
	mu.Lock()
	defer mu.Unlock()
	require.Len(t, requests, 2)
	var sent struct{ Messages []json.RawMessage }
	require.NoError(t, common.Unmarshal(requests[1], &sent))
	require.Len(t, sent.Messages, 3)
	var got any
	require.NoError(t, common.UnmarshalUseNumber(sent.Messages[1], &got))
	require.True(t, objectsEqualJSON(map[string]any{"role": "assistant", "content": blocks}, got), "the next request changed the signed assistant turn")
}
