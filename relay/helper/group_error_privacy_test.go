package helper

import (
	"bytes"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStreamWritersHideStructuredGroupErrorsAndKeepAdminDiagnostics(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var logs bytes.Buffer
	common.LogWriterMu.Lock()
	oldWriter := gin.DefaultErrorWriter
	gin.DefaultErrorWriter = &logs
	common.LogWriterMu.Unlock()
	t.Cleanup(func() {
		common.LogWriterMu.Lock()
		gin.DefaultErrorWriter = oldWriter
		common.LogWriterMu.Unlock()
	})

	tests := []struct {
		name  string
		write func(*gin.Context)
	}{
		{
			name: "OpenAI SSE",
			write: func(c *gin.Context) {
				require.NoError(t, StringData(c, `{"error":{"message":"无权访问 streamed-secret 分组","type":"permission_error","code":"forbidden"}}`))
			},
		},
		{
			name: "Claude SSE",
			write: func(c *gin.Context) {
				ClaudeChunkData(c, dto.ClaudeResponse{Type: "error"}, `{"type":"error","error":{"type":"permission_error","message":"not authorized to access streamed-secret group"}}`)
			},
		},
		{
			name: "Responses SSE",
			write: func(c *gin.Context) {
				require.NoError(t, ResponseChunkData(c, dto.ResponsesStreamResponse{Type: "response.failed"}, `{"type":"response.failed","response":{"error":{"message":"无权访问 streamed-secret 分组","type":"permission_error"}}}`))
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			logs.Reset()
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest("POST", "/v1/messages", nil)
			test.write(c)
			assert.NotContains(t, recorder.Body.String(), "streamed-secret")
			assert.Contains(t, recorder.Body.String(), "无权访问请求的上游分组")
			assert.Contains(t, logs.String(), "streamed-secret")
		})
	}
}

func TestStreamWriterDoesNotRedactNormalModelOutput(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
	payload := `{"choices":[{"delta":{"content":"A fictional user had no permission to access the blue group."}}]}`
	require.NoError(t, StringData(c, payload))
	assert.Contains(t, recorder.Body.String(), payload)
}
