package helper

import (
	"encoding/json"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestClaudeAndOpenAIErrorSSEFormatting(t *testing.T) {
	message := `InvokeModel: operation error Bedrock Runtime: InvokeModel, https response error StatusCode: 400, RequestID: upstream, ValidationException: "thinking.type.disabled" not supported. Use "thinking.type.adaptive" and "output_config.effort". (request id: one) (request id: two) (request id: three)`
	data, _ := json.Marshal(map[string]any{"type": "error", "error": map[string]any{"type": "<nil>", "message": message, "large": json.RawMessage(`9007199254740993`)}})
	for _, mode := range []string{"claude", "oai", "responses"} {
		t.Run(mode, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest("POST", "/local", nil)
			c.Set(common.RequestIdKey, "local-sse")
			switch mode {
			case "claude":
				ClaudeChunkData(c, dto.ClaudeResponse{Type: "error"}, string(data))
			case "oai":
				require.NoError(t, StringData(c, string(data)))
			case "responses":
				require.NoError(t, ResponseChunkData(c, dto.ResponsesStreamResponse{Type: "error"}, string(data)))
			}
			output := w.Body.String()
			assert.Contains(t, output, "invalid_request_error")
			assert.Contains(t, output, "thinking.type.disabled")
			assert.Contains(t, output, "output_config.effort")
			assert.NotContains(t, output, "operation error")
			assert.Contains(t, output, "9007199254740993")
			assert.Equal(t, 1, strings.Count(output, "(request id:"))
			assert.Contains(t, output, "local-sse")
		})
	}
	original := `{"type":"content_block_delta","delta":{"text":"ValidationException: thinking.type.disabled (request id: original)"}}`
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/local", nil)
	c.Set(common.RequestIdKey, "id")
	ClaudeChunkData(c, dto.ClaudeResponse{Type: "content_block_delta"}, original)
	assert.Contains(t, w.Body.String(), original)
}
