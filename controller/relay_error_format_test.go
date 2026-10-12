package controller

import (
	"errors"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRelayClaudeValidationErrorHTTPFormatting(t *testing.T) {
	gin.SetMode(gin.TestMode)
	message := `InvokeModelWithResponseStream: operation error Bedrock Runtime: InvokeModelWithResponseStream, https response error StatusCode: 400, RequestID: upstream, ValidationException: "thinking.type.disabled" is not supported for this model. Use "thinking.type.adaptive" and "output_config.effort" to control thinking behavior. (request id: one) (request id: two) (request id: three)`
	source := types.NewOpenAIError(errors.New(message), types.ErrorCodeAwsInvokeError, 400, types.ErrOptionWithSkipRetry())
	before := source.Error()
	w := httptest.NewRecorder()
	router := gin.New()
	router.POST("/v1/messages", func(c *gin.Context) {
		writeRelayHTTPError(c, types.RelayFormatClaude, prepareFinalRelayError(source, "local-http"))
	})
	request := httptest.NewRequest("POST", "/v1/messages", nil)
	router.ServeHTTP(w, request)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	output := w.Body.String()
	assert.Contains(t, output, `"type":"invalid_request_error"`)
	assert.Contains(t, output, "thinking.type.disabled")
	assert.Contains(t, output, "output_config.effort")
	assert.NotContains(t, output, "operation error")
	assert.NotContains(t, output, "<nil>")
	assert.Equal(t, 1, strings.Count(output, "(request id:"))
	assert.Contains(t, output, "local-http")
	assert.Equal(t, before, source.Error())
}

func TestDisabledMaskedErrorHTTPAndSSEBoundary(t *testing.T) {
	message := `status_code=400, "***.***.disabled" is not supported for this model. Use "***.***.adaptive" and "output_config.effort" to control thinking behavior. (request id: upstream-one) (request id: upstream-two)`
	source := types.WithClaudeError(types.ClaudeError{Type: "<nil>", Message: message}, 400)
	public := prepareFinalRelayError(source, "local-disabled").ToClaudeError()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	writeRelayHTTPError(c, types.RelayFormatClaude, prepareFinalRelayError(source, "local-disabled"))
	require.Equal(t, 400, w.Code)
	var envelope struct {
		Error types.ClaudeError `json:"error"`
	}
	require.NoError(t, common.Unmarshal(w.Body.Bytes(), &envelope))
	assert.Equal(t, public, envelope.Error)
	assert.Equal(t, "invalid_request_error", public.Type)
	assert.Contains(t, public.Message, "***.***.disabled")
	assert.NotContains(t, public.Message, "thinking.type.disabled")
	assert.Contains(t, public.Message, "output_config.effort")
	assert.Equal(t, 1, strings.Count(public.Message, "(request id:"))
	data, err := common.Marshal(map[string]any{"type": "error", "error": map[string]any{"type": public.Type, "message": message}})
	require.NoError(t, err)
	formatted, changed := types.FormatPublicErrorStreamData(string(data), "local-disabled")
	require.True(t, changed)
	require.NoError(t, common.Unmarshal([]byte(formatted), &envelope))
	assert.Equal(t, public, envelope.Error)
	// Real SSE has HTTP 200, so an absent/invalid type without a recognized
	// ValidationException has no authoritative HTTP 400 context. Don't infer
	// protocol type from provider-controlled status_code text.
	for _, kind := range []string{"invalid_request_error", "<nil>"} {
		recorder := httptest.NewRecorder()
		context, _ := gin.CreateTestContext(recorder)
		context.Request = httptest.NewRequest("POST", "/v1/messages", nil)
		context.Set(common.RequestIdKey, "local-disabled")
		data, err := common.Marshal(map[string]any{"type": "error", "error": map[string]any{"type": kind, "message": message}})
		require.NoError(t, err)
		helper.ClaudeChunkData(context, dto.ClaudeResponse{Type: "error"}, string(data))
		expected := public
		if kind == "<nil>" {
			expected.Type = "api_error"
		}
		output := recorder.Body.String()
		start := strings.Index(output, "data: ")
		require.NotEqual(t, -1, start)
		event := strings.TrimSpace(output[start+6:])
		require.NoError(t, common.Unmarshal([]byte(event), &envelope))
		assert.Equal(t, expected, envelope.Error)
		assert.Equal(t, http.StatusOK, recorder.Code)
	}
	assert.Equal(t, message, source.Error())
}
