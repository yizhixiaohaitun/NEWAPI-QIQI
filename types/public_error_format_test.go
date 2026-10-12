package types

import (
	"encoding/json"
	"errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"net/http"
	"strings"
	"testing"
)

const validationMessage = `InvokeModelWithResponseStream: operation error Bedrock Runtime: InvokeModelWithResponseStream, https response error StatusCode: 400, RequestID: upstream-123, api error ValidationException: "thinking.type.enabled" is not supported for this model. Use "thinking.type.adaptive" and "output_config.effort" to control thinking behavior. (request id: one) (request id: two) (request id: three)`

func TestPublicClaudeValidationErrorFormatting(t *testing.T) {
	raw := NewOpenAIError(errors.New(validationMessage), ErrorCodeAwsInvokeError, http.StatusBadRequest)
	before := raw.Error()
	public := PreparePublicError(raw, "local-id")
	response := public.ToClaudeError()
	assert.Equal(t, "invalid_request_error", response.Type)
	assert.Contains(t, response.Message, `"thinking.type.enabled"`)
	assert.Contains(t, response.Message, `"output_config.effort"`)
	assert.NotContains(t, response.Message, "operation error")
	assert.NotContains(t, response.Message, "upstream-123")
	assert.Equal(t, 1, strings.Count(response.Message, "(request id:"))
	assert.Contains(t, response.Message, "local-id")
	assert.Equal(t, before, raw.Error())
	assert.Equal(t, raw.GetErrorCode(), public.GetErrorCode())
	assert.Equal(t, 400, public.StatusCode)
	var original OpenAIError
	require.IsType(t, original, raw.RelayError)
	assert.Equal(t, ErrorCodeAwsInvokeError, raw.RelayError.(OpenAIError).Code)
	nilCode := WithOpenAIError(OpenAIError{Message: validationMessage, Type: "invalid_request_error"}, 400)
	assert.Nil(t, nilCode.RelayError.(OpenAIError).Code)
	assert.Equal(t, "invalid_request_error", PreparePublicError(nilCode, "id").ToClaudeError().Type)
	for _, kind := range []string{"", "<nil>", "null"} {
		raw := WithClaudeError(ClaudeError{Type: kind, Message: validationMessage}, 400)
		assert.Equal(t, "invalid_request_error", PreparePublicError(raw, "local-id").ToClaudeError().Type)
	}
	valid := WithClaudeError(ClaudeError{Type: "overloaded_error", Message: "busy"}, 529)
	assert.Equal(t, "overloaded_error", PreparePublicError(valid, "id").ToClaudeError().Type)
}
func TestPublicErrorStreamFormattingDoesNotRewriteOrdinaryContent(t *testing.T) {
	raw, _ := json.Marshal(map[string]any{"type": "error", "error": map[string]any{"type": "<nil>", "message": validationMessage, "cost": json.RawMessage(`1.1234567890123456789`), "id": json.RawMessage(`9007199254740993`)}})
	output, changed := FormatPublicErrorStreamData(string(raw), "local-id")
	assert.True(t, changed)
	assert.Contains(t, output, "9007199254740993")
	assert.Contains(t, output, "1.1234567890123456789")
	assert.Contains(t, output, "invalid_request_error")
	assert.Equal(t, 1, strings.Count(output, "(request id:"))
	for _, data := range []string{`{"type":"content_block_delta","delta":{"text":"thinking.type.enabled (request id: one)"}}`, `{"choices":[{"message":{"content":"ValidationException: original refusal"}}]}`, `{"type":"message","error":{"message":"original content"}}`} {
		output, changed := FormatPublicErrorStreamData(data, "id")
		assert.False(t, changed)
		assert.Equal(t, data, output)
	}
}
