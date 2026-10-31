package dto

import (
	"encoding/json"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGeminiNativeContentFidelity(t *testing.T) {
	input := `{"role":"model","extension":{"big":9007199254740993,"decimal":1.234567890123456789},"parts":[{"text":"","thought":false,"extra":{"n":9007199254740995}},{"text":"original","thought":true,"function_response":{"name":"tool","response":{"big":9007199254740993,"nested":[1.234567890123456789,{"v":9007199254740995}]}}}]}`
	var content GeminiChatContent
	require.NoError(t, common.Unmarshal([]byte(input), &content))
	encoded, err := common.Marshal(content)
	require.NoError(t, err)
	var out map[string]json.RawMessage
	require.NoError(t, common.Unmarshal(encoded, &out))
	assert.Equal(t, `{"big":9007199254740993,"decimal":1.234567890123456789}`, string(out["extension"]))
	var parts []map[string]json.RawMessage
	require.NoError(t, common.Unmarshal(out["parts"], &parts))
	require.Len(t, parts, 2)
	assert.Equal(t, `""`, string(parts[0]["text"]))
	assert.Equal(t, `false`, string(parts[0]["thought"]))
	assert.Equal(t, `{"n":9007199254740995}`, string(parts[0]["extra"]))
	var response map[string]json.RawMessage
	require.NoError(t, common.Unmarshal(parts[1]["functionResponse"], &response))
	assert.JSONEq(t, `{"big":9007199254740993,"nested":[1.234567890123456789,{"v":9007199254740995}]}`, string(response["response"]))
	assert.Contains(t, string(response["response"]), `9007199254740993`)
	assert.Contains(t, string(response["response"]), `1.234567890123456789`)
	assert.Contains(t, string(response["response"]), `9007199254740995`)

	content.Role = "user"
	content.Parts[0].Text = "edited"
	content.Parts[0].Thought = true
	content.Parts[1].Text = ""
	content.Parts[1].Thought = false
	content.Parts[1].FunctionResponse.Name = "edited-tool"
	content.Parts[1].FunctionResponse.Response["big"] = json.Number("99999999999999999999")
	encoded, err = common.Marshal(content)
	require.NoError(t, err)
	require.NoError(t, common.Unmarshal(encoded, &out))
	assert.Equal(t, `"user"`, string(out["role"]))
	assert.Equal(t, `{"big":9007199254740993,"decimal":1.234567890123456789}`, string(out["extension"]))
	parts = nil
	require.NoError(t, common.Unmarshal(out["parts"], &parts))

	assert.Equal(t, `"edited"`, string(parts[0]["text"]))
	assert.Equal(t, `true`, string(parts[0]["thought"]))
	assert.Equal(t, `""`, string(parts[1]["text"]))
	assert.NotEqual(t, `true`, string(parts[1]["thought"]))
	require.NoError(t, common.Unmarshal(parts[1]["functionResponse"], &response))
	assert.Equal(t, `"edited-tool"`, string(response["name"]))
	assert.Contains(t, string(response["response"]), `99999999999999999999`)
	assert.Contains(t, string(response["response"]), `1.234567890123456789`)

	// Clearing a formerly populated known field must not restore its snapshot.
	content.Parts[1].FunctionResponse = nil
	encoded, err = common.Marshal(content)
	require.NoError(t, err)
	out = nil
	require.NoError(t, common.Unmarshal(encoded, &out))
	parts = nil
	require.NoError(t, common.Unmarshal(out["parts"], &parts))
	assert.NotContains(t, parts[1], "functionResponse")
	assert.NotContains(t, parts[1], "function_response")
}

func TestGeminiNativeProgrammaticContent(t *testing.T) {
	content := GeminiChatContent{Role: "user", Parts: []GeminiPart{{Text: "constructed", FunctionResponse: &GeminiFunctionResponse{Name: "tool", Response: map[string]interface{}{"result": "new"}}}}}
	encoded, err := common.Marshal(content)
	require.NoError(t, err)
	assert.JSONEq(t, `{"role":"user","parts":[{"text":"constructed","functionResponse":{"name":"tool","response":{"result":"new"}}}]}`, string(encoded))
}
