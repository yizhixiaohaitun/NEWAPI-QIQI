package dto

import (
	"encoding/json"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGeminiNativeFunctionCallRoundTrip(t *testing.T) {
	for _, tc := range []struct {
		name string
		raw  string
		args string
	}{
		{"precise-arguments", `{"name":"lookup","id":"server-call-1","args":{"integer":9007199254740993,"decimal":1.234567890123456789,"flag":false,"nested":[0,{"value":9007199254740995}]}}`, `{"integer":9007199254740993,"decimal":1.234567890123456789,"flag":false,"nested":[0,{"value":9007199254740995}]}`},
		{"empty-object", `{"name":"lookup","args":{}}`, `{}`},
		{"explicit-null", `{"name":"lookup","args":null}`, `null`},
		{"absent-arguments", `{"name":"lookup"}`, ``},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var call FunctionCall
			require.NoError(t, common.Unmarshal([]byte(tc.raw), &call))
			encoded, err := common.Marshal(call)
			require.NoError(t, err)
			var fields map[string]json.RawMessage
			require.NoError(t, common.Unmarshal(encoded, &fields))
			assert.Equal(t, tc.args, string(fields["args"]), "do not round signed call arguments through float64")
			assert.Equal(t, `"lookup"`, string(fields["name"]))
			if tc.name == "precise-arguments" {
				assert.Equal(t, `"server-call-1"`, string(fields["id"]))
			} else {
				assert.NotContains(t, fields, "id", "do not invent missing IDs")
			}
		})
	}
	// Converters construct this same DTO with ordinary Go maps.
	encoded, err := common.Marshal(FunctionCall{FunctionName: "lookup", Arguments: map[string]any{"q": "x"}})
	require.NoError(t, err)
	assert.JSONEq(t, `{"name":"lookup","args":{"q":"x"}}`, string(encoded))
}

func TestGeminiNativeSnakeCaseToolHistory(t *testing.T) {
	raw := []byte(`{"contents":[
		{"role":"model","parts":[
			{"thought":true,"text":"keep this thought"},
			{"function_call":{"name":"lookup","id":"a","args":{"q":"first"}},"thought_signature":"c2lnLzE9PQ=="},
			{"function_call":{"name":"lookup","id":"b","args":{"q":"second"}}},
			{"text":"","thought_signature":"c2lnLzI9PQ=="}
		]},
		{"role":"user","parts":[
			{"function_response":{"name":"lookup","id":"a","response":{"ok":true}}},
			{"function_response":{"name":"lookup","id":"b","response":{"ok":true}}}
		]}
	]}`)
	var request GeminiChatRequest
	require.NoError(t, common.Unmarshal(raw, &request))
	require.Len(t, request.Contents, 2)
	parts := request.Contents[0].Parts
	require.Len(t, parts, 4)
	assert.True(t, parts[0].Thought)
	assert.Equal(t, "keep this thought", parts[0].Text)
	require.NotNil(t, parts[1].FunctionCall)
	require.NotNil(t, parts[2].FunctionCall)
	assert.Equal(t, `"a"`, string(parts[1].FunctionCall.ID))
	assert.Equal(t, `"b"`, string(parts[2].FunctionCall.ID))
	assert.Equal(t, `"c2lnLzE9PQ=="`, string(parts[1].ThoughtSignature))
	assert.Empty(t, parts[2].ThoughtSignature)
	assert.Equal(t, `"c2lnLzI9PQ=="`, string(parts[3].ThoughtSignature))
	encoded, err := common.Marshal(request)
	require.NoError(t, err)
	var result GeminiChatRequest
	require.NoError(t, common.Unmarshal(encoded, &result))
	assert.Equal(t, request, result, "normalization must be idempotent and retain signature-only parts")
	for i, id := range []string{`"a"`, `"b"`} {
		require.NotNil(t, result.Contents[1].Parts[i].FunctionResponse)
		assert.Equal(t, id, string(result.Contents[1].Parts[i].FunctionResponse.ID))
	}
}
