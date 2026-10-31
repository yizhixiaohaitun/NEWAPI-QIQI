package dto

import (
	"encoding/json"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGeminiNativeMediaAliasesAndEdits(t *testing.T) {
	for _, tc := range []struct {
		name string
		raw  string
	}{
		{"camel", `{"parts":[{"fileData":{"mimeType":"image/png","fileUri":"old"}},{"inlineData":{"mimeType":"image/png","data":"abc"}},{"mediaResolution":{"v":9007199254740993}},{"videoMetadata":{"v":9007199254740995}},{"executableCode":{"language":"PYTHON","code":"old"}},{"codeExecutionResult":{"outcome":"OK","output":"old"}}]}`},
		{"snake", `{"parts":[{"file_data":{"mime_type":"image/png","file_uri":"old"}},{"inline_data":{"mime_type":"image/png","data":"abc"}},{"media_resolution":{"v":9007199254740993}},{"video_metadata":{"v":9007199254740995}},{"executable_code":{"language":"PYTHON","code":"old"}},{"code_execution_result":{"outcome":"OK","output":"old"}}]}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var content GeminiChatContent
			require.NoError(t, common.Unmarshal([]byte(tc.raw), &content))
			require.Len(t, content.Parts, 6)
			require.NotNil(t, content.Parts[0].FileData)
			require.NotNil(t, content.Parts[1].InlineData)
			require.NotNil(t, content.Parts[4].ExecutableCode)
			require.NotNil(t, content.Parts[5].CodeExecutionResult)
			assert.Equal(t, "image/png", content.Parts[0].FileData.MimeType)
			assert.Equal(t, "old", content.Parts[0].FileData.FileUri)
			content.Parts[0].FileData.MimeType = "image/jpeg"
			content.Parts[0].FileData.FileUri = "new"
			content.Parts[1].InlineData.MimeType = "image/jpeg"
			content.Parts[4].ExecutableCode.Code = "new"
			content.Parts[5].CodeExecutionResult.Output = "new"
			encoded, err := common.Marshal(content)
			require.NoError(t, err)
			var out struct {
				Parts []map[string]json.RawMessage `json:"parts"`
			}
			require.NoError(t, common.Unmarshal(encoded, &out))
			assert.JSONEq(t, `{"mimeType":"image/jpeg","fileUri":"new"}`, string(out.Parts[0]["fileData"]))
			assert.JSONEq(t, `{"mimeType":"image/jpeg","data":"abc"}`, string(out.Parts[1]["inlineData"]))
			assert.Equal(t, `{"v":9007199254740993}`, string(out.Parts[2]["mediaResolution"]))
			assert.Equal(t, `{"v":9007199254740995}`, string(out.Parts[3]["videoMetadata"]))
			assert.JSONEq(t, `{"language":"PYTHON","code":"new"}`, string(out.Parts[4]["executableCode"]))
			assert.JSONEq(t, `{"outcome":"OK","output":"new"}`, string(out.Parts[5]["codeExecutionResult"]))
			assert.NotContains(t, string(encoded), "file_data")
			assert.NotContains(t, string(encoded), "media_resolution")
			assert.NotContains(t, string(encoded), "video_metadata")
			assert.NotContains(t, string(encoded), "executable_code")
			assert.NotContains(t, string(encoded), "code_execution_result")
		})
	}
}
