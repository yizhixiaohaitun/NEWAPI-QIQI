package types

import (
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strings"

	"github.com/QuantumNous/new-api/common"
)

var bedrockValidationWrapper = regexp.MustCompile(`(?s)^(?:InvokeModel(?:WithResponseStream)?: )?operation error Bedrock Runtime: InvokeModel(?:WithResponseStream)?, https response error StatusCode: 400, RequestID: [^,]*, api error ValidationException: (.*)$`)

// PublicErrorMessage removes only a recognized SDK ValidationException
// wrapper. Other provider messages (including normal refusals) aren't parsed.
func PublicErrorMessage(message string) string {
	if match := bedrockValidationWrapper.FindStringSubmatch(message); len(match) > 0 {
		message = "ValidationException: " + match[1]
	}
	return common.MaskSensitiveInfo(message)
}

func invalidClaudeErrorType(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "<nil>", "nil", "null":
		return true
	}
	return false
}

func claudeErrorTypeForStatus(status int) string {
	switch status {
	case http.StatusBadRequest, http.StatusUnprocessableEntity:
		return "invalid_request_error"
	case http.StatusUnauthorized:
		return "authentication_error"
	case http.StatusForbidden:
		return "permission_error"
	case http.StatusNotFound:
		return "not_found_error"
	case http.StatusRequestEntityTooLarge:
		return "request_too_large"
	case http.StatusTooManyRequests:
		return "rate_limit_error"
	case http.StatusServiceUnavailable, 529:
		return "overloaded_error"
	default:
		return "api_error"
	}
}

func protocolClaudeErrorType(value string, status int) string {
	switch value {
	case "invalid_request_error", "authentication_error", "permission_error", "not_found_error", "request_too_large", "rate_limit_error", "api_error", "overloaded_error":
		return value
	}
	return claudeErrorTypeForStatus(status)
}

// PreparePublicError returns a copy; the raw error remains available to
// logging, retry policy, status mapping and billing. Classifiers stay intact.
func PreparePublicError(e *NewAPIError, requestID string) *NewAPIError {
	if e == nil {
		return nil
	}
	result := *e
	message := PublicErrorMessage(e.Error())
	if requestID != "" {
		message = common.MessageWithRequestId(message, requestID)
	}
	result.Err = errors.New(message)
	switch original := e.RelayError.(type) {
	case OpenAIError:
		original.Message = message
		result.RelayError = original
	case ClaudeError:
		original.Message = message
		result.RelayError = original
	}
	return &result
}

// FormatPublicErrorStreamData touches only explicit error envelopes and only
// their message/invalid type. RawMessage preserves numbers and unknown fields.
func FormatPublicErrorStreamData(data, requestID string) (string, bool) {
	var envelope map[string]json.RawMessage
	if common.Unmarshal([]byte(data), &envelope) != nil || envelope == nil {
		return data, false
	}
	var event string
	_ = common.Unmarshal(envelope["type"], &event)
	raw, hasError := envelope["error"]
	if !hasError || string(raw) == "null" {
		return data, false
	}
	// Don't rewrite arbitrary error-like values inside normal message content.
	if event != "" && event != "error" && event != "response.error" && event != "response.failed" {
		return data, false
	}
	var fields map[string]json.RawMessage
	if common.Unmarshal(raw, &fields) != nil || fields == nil {
		return data, false
	}
	var message string
	if common.Unmarshal(fields["message"], &message) != nil {
		return data, false
	}
	formatted := PublicErrorMessage(message)
	if requestID != "" {
		formatted = common.MessageWithRequestId(formatted, requestID)
	}
	changed := formatted != message
	if changed {
		fields["message"], _ = common.Marshal(formatted)
	}
	if event == "error" {
		var kind string
		_ = common.Unmarshal(fields["type"], &kind)
		if invalidClaudeErrorType(kind) {
			status := http.StatusInternalServerError
			if strings.Contains(formatted, "ValidationException:") {
				status = http.StatusBadRequest
			}
			fields["type"], _ = common.Marshal(claudeErrorTypeForStatus(status))
			changed = true
		}
	}
	if !changed {
		return data, false
	}
	patched, err := common.Marshal(fields)
	if err != nil {
		return data, false
	}
	envelope["error"] = patched
	result, err := common.Marshal(envelope)
	if err != nil {
		return data, false
	}
	return string(result), true
}
