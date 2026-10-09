package helper

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
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

func TestStreamWritersRebuildArrayAndNestedResponseErrors(t *testing.T) {
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

	for _, payload := range []string{
		`{"type":"error","metadata":{"group":"writer-secret"},"error":["无权访问 writer-secret 分组",{"details":["writer-secret"]}]}`,
		`{"type":"response.failed","details":["writer-secret"],"response":{"error":{"message":"Failed to get available channel under group writer-secret","type":"permission_error","code":"writer-secret","param":"writer-secret"}}}`,
	} {
		logs.Reset()
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		c.Request = httptest.NewRequest("POST", "/v1/responses", nil)
		require.NoError(t, StringData(c, payload))
		assert.NotContains(t, recorder.Body.String(), "writer-secret")
		assert.NotContains(t, recorder.Body.String(), "details")
		assert.Contains(t, logs.String(), "writer-secret")
	}
}

func TestWebSocketWritersHideStructuredGroupErrorsAndPreserveNormalFrames(t *testing.T) {
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

	upgrader := websocket.Upgrader{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws, err := upgrader.Upgrade(w, r, nil)
		require.NoError(t, err)
		defer ws.Close()
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = r
		require.NoError(t, WssPublicString(c, ws, `{"type":"error","error":{"message":"\u65e0\u6743\u8bbf\u95ee ws-secret \u5206\u7ec4","details":["ws-secret"]}}`))
		require.NoError(t, WssPublicObject(c, ws, map[string]any{
			"type": "response.failed",
			"response": map[string]any{"error": map[string]any{
				"message": "Failed to get available channel under group object-secret",
				"type":    "permission_error", "code": "forbidden", "details": []string{"object-secret"},
			}},
		}))
		require.NoError(t, WssPublicString(c, ws, `{"type":"message","text":"normal frame"}`))
		require.NoError(t, WssString(c, ws, `{"type":"error","error":{"message":"无权访问 client-input 分组"}}`))
		require.NoError(t, WssObject(c, ws, map[string]any{"type": "error", "error": map[string]any{"message": "无权访问 client-object 分组"}}))
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	client, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err)
	defer client.Close()

	for _, secret := range []string{"ws-secret", "object-secret"} {
		_, payload, readErr := client.ReadMessage()
		require.NoError(t, readErr)
		assert.NotContains(t, string(payload), secret)
		assert.Contains(t, string(payload), "上游分组")
	}
	_, normal, err := client.ReadMessage()
	require.NoError(t, err)
	assert.Equal(t, `{"type":"message","text":"normal frame"}`, string(normal))
	_, raw, readErr := client.ReadMessage()
	require.NoError(t, readErr)
	assert.Equal(t, `{"type":"error","error":{"message":"无权访问 client-input 分组"}}`, string(raw))
	_, rawObject, readErr := client.ReadMessage()
	require.NoError(t, readErr)
	assert.Contains(t, string(rawObject), "client-object")
	assert.NotContains(t, logs.String(), "client-input")
	assert.NotContains(t, logs.String(), "client-object")
	assert.Contains(t, logs.String(), "ws-secret")
	assert.Contains(t, logs.String(), "object-secret")
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
