package controller

import (
	"errors"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRelayClaudeValidationErrorHTTPFormatting(t *testing.T) {
	gin.SetMode(gin.TestMode)
	message := `InvokeModelWithResponseStream: operation error Bedrock Runtime: InvokeModelWithResponseStream, https response error StatusCode: 400, RequestID: upstream, api error ValidationException: "thinking.type.enabled" is not supported for this model. Use "thinking.type.adaptive" and "output_config.effort" to control thinking behavior. (request id: one) (request id: two) (request id: three)`
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
	assert.Contains(t, output, "thinking.type.enabled")
	assert.Contains(t, output, "output_config.effort")
	assert.NotContains(t, output, "operation error")
	assert.NotContains(t, output, "<nil>")
	assert.Equal(t, 1, strings.Count(output, "(request id:"))
	assert.Contains(t, output, "local-http")
	assert.Equal(t, before, source.Error())
}
