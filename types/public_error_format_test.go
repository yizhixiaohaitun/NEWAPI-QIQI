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
func TestPublicErrorExactBedrockWrapperVariants(t *testing.T) {
	const exactMessage = `InvokeModelWithResponseStream: operation error Bedrock Runtime: InvokeModelWithResponseStream, https response error StatusCode: 400, RequestID: b5e65650-65b3-42c9-b332-427377f0997f, ValidationException: "***.***.enabled" is not supported for this model. Use "***.***.adaptive" and "output_config.effort" to control thinking behavior. (request id: 202610121212427215037068268d9d6ml2pemJO) (request id: 202610121212426949872588268d9d6GWrLQuuw) (request id: 202610121212425334071308268d9d64UsvU1O2)`
	for name, message := range map[string]string{
		"without_sdk_api_prefix": exactMessage,
		"with_sdk_api_prefix":    strings.Replace(exactMessage, ", ValidationException:", ", api error ValidationException:", 1),
	} {
		t.Run(name, func(t *testing.T) {
			raw := WithOpenAIError(OpenAIError{Message: message}, http.StatusBadRequest)
			public := PreparePublicError(raw, "local-id").ToClaudeError()
			assert.Equal(t, "invalid_request_error", public.Type)
			assert.True(t, strings.HasPrefix(public.Message, "ValidationException:"), public.Message)
			assert.NotContains(t, public.Message, "InvokeModel")
			assert.NotContains(t, public.Message, "b5e65650")
			assert.Contains(t, public.Message, `"output_config.effort"`)
			assert.Equal(t, 1, strings.Count(public.Message, "(request id:"))
			assert.Equal(t, message, raw.Error())
			data, err := json.Marshal(map[string]any{"type": "error", "error": map[string]any{"type": "<nil>", "message": message}})
			require.NoError(t, err)
			formatted, changed := FormatPublicErrorStreamData(string(data), "local-id")
			require.True(t, changed)
			var envelope struct {
				Error ClaudeError `json:"error"`
			}
			require.NoError(t, json.Unmarshal([]byte(formatted), &envelope))
			assert.Equal(t, public, envelope.Error)
		})
	}
	for _, message := range []string{
		strings.Replace(exactMessage, "StatusCode: 400", "StatusCode: 500", 1),
		strings.Replace(exactMessage, "Bedrock Runtime:", "Other Runtime:", 1),
		strings.Replace(exactMessage, "ValidationException:", "InternalServerException:", 1),
		"normal content mentioning " + exactMessage,
	} {
		assert.True(t, strings.Contains(PublicErrorMessage(message), "operation error"))
	}
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
