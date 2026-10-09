package types

import (
	"errors"
	"regexp"
	"strings"

	"github.com/QuantumNous/new-api/common"
)

type groupErrorKind int

const (
	groupErrorNone groupErrorKind = iota
	groupErrorAccessDenied
	groupErrorNoChannel
)

const (
	publicGroupAccessDeniedMessage = "无权访问请求的上游分组"
	publicGroupNoChannelMessage    = "请求的上游分组无可用渠道"
)

var (
	chineseGroupAccessDeniedPatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)(?:无权|无权限|没有权限|未授权|禁止).{0,48}(?:访问|使用).{0,96}(?:分组|群组)`),
		regexp.MustCompile(`(?i)(?:分组|群组).{0,96}(?:无权|无权限|没有权限|未授权|禁止访问|已停用|已禁用|不可用)`),
	}
	englishGroupAccessDeniedPatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)(?:no permission|not authorized|not allowed|unauthorized|forbidden)\s+(?:to\s+)?(?:access|use)\s+(?:the\s+)?.{0,96}\bgroup\b`),
		regexp.MustCompile(`(?i)\bgroup\b.{0,96}(?:\baccess\s+(?:is\s+)?denied\b|\bis\s+(?:disabled|inactive)\b|\bdisabled\b)`),
	}
	chineseGroupNoChannelPatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)(?:分组|群组).{0,160}(?:无可用(?:渠道|通道)|没有可用(?:渠道|通道)|可用(?:渠道|通道)不存在)`),
		regexp.MustCompile(`(?i)(?:无可用(?:渠道|通道)|没有可用(?:渠道|通道)|可用(?:渠道|通道)不存在).{0,160}(?:分组|群组)`),
	}
	englishGroupNoChannelPatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)(?:no available (?:upstream )?channels?).{0,160}\b(?:for|under|in)\s+(?:the\s+)?group\b`),
		regexp.MustCompile(`(?i)(?:failed to get|cannot get|unable to get).{0,48}available (?:upstream )?channels?.{0,160}\b(?:for|under|in)\s+(?:the\s+)?group\b`),
		regexp.MustCompile(`(?i)\bgroup\b.{0,160}(?:no available (?:upstream )?channels?)`),
	}
)

func classifyGroupError(text string) groupErrorKind {
	for _, pattern := range chineseGroupAccessDeniedPatterns {
		if pattern.MatchString(text) {
			return groupErrorAccessDenied
		}
	}
	for _, pattern := range englishGroupAccessDeniedPatterns {
		if pattern.MatchString(text) {
			return groupErrorAccessDenied
		}
	}
	for _, pattern := range chineseGroupNoChannelPatterns {
		if pattern.MatchString(text) {
			return groupErrorNoChannel
		}
	}
	for _, pattern := range englishGroupNoChannelPatterns {
		if pattern.MatchString(text) {
			return groupErrorNoChannel
		}
	}
	return groupErrorNone
}

func publicGroupErrorMessage(kind groupErrorKind) string {
	if kind == groupErrorNoChannel {
		return publicGroupNoChannelMessage
	}
	return publicGroupAccessDeniedMessage
}

// PublicGroupErrorMessage recognizes upstream group authorization/channel
// selection failures. It intentionally requires both group semantics and a
// denial/no-channel phrase so ordinary provider errors and model output are not
// broadly redacted.
func PublicGroupErrorMessage(text string) (string, bool) {
	kind := classifyGroupError(text)
	if kind == groupErrorNone {
		return "", false
	}
	return publicGroupErrorMessage(kind), true
}

// CopyWithPublicMessage keeps status, error code/type and retry/logging flags,
// while removing provider-controlled message and metadata from client output.
func (e *NewAPIError) CopyWithPublicMessage(message string) *NewAPIError {
	if e == nil {
		return nil
	}
	copyErr := *e
	copyErr.Err = errors.New(message)
	copyErr.Metadata = nil
	switch relayErr := e.RelayError.(type) {
	case OpenAIError:
		relayCopy := relayErr
		relayCopy.Message = message
		relayCopy.Param = ""
		relayCopy.Code = safePublicErrorClassifier(relayCopy.Code)
		relayCopy.Type = safePublicErrorType(relayCopy.Type)
		relayCopy.Metadata = nil
		copyErr.RelayError = relayCopy
	case ClaudeError:
		relayCopy := relayErr
		relayCopy.Message = message
		relayCopy.Type = safePublicErrorType(relayCopy.Type)
		copyErr.RelayError = relayCopy
	default:
		copyErr.RelayError = nil
	}
	return &copyErr
}

// SanitizeGroupErrorStreamData only rewrites structured error envelopes. A
// normal completion containing the same words remains byte-for-byte unchanged.
func SanitizeGroupErrorStreamData(data string) (string, bool) {
	var envelope map[string]any
	if err := common.Unmarshal([]byte(data), &envelope); err != nil || !isErrorEnvelope(envelope) {
		return data, false
	}

	// Classify decoded structured values rather than raw JSON. Otherwise an
	// upstream can hide Chinese error text behind JSON unicode escapes.
	diagnostic, err := common.Marshal(envelope)
	if err != nil {
		return data, false
	}
	kind := classifyGroupError(string(diagnostic))
	if kind == groupErrorNone {
		return data, false
	}

	envelope = rebuildPublicErrorEnvelope(envelope, kind)
	encoded, err := common.Marshal(envelope)
	if err != nil {
		return data, false
	}
	return string(encoded), true
}

func isErrorEnvelope(envelope map[string]any) bool {
	if value, ok := envelope["error"]; ok && value != nil {
		return true
	}
	eventType, _ := envelope["type"].(string)
	switch strings.ToLower(strings.TrimSpace(eventType)) {
	case "error", "response.error", "response.failed":
		return true
	}
	if response, ok := envelope["response"].(map[string]any); ok {
		return response["error"] != nil
	}
	return false
}

var safeGroupErrorTypes = map[string]struct{}{
	"error": {}, "permission_error": {}, "authentication_error": {},
	"authorization_error": {}, "invalid_request_error": {}, "api_error": {},
	"server_error": {}, "rate_limit_error": {}, "overloaded_error": {},
}

var safeGroupErrorCodes = map[string]struct{}{
	"forbidden": {}, "permission_denied": {}, "access_denied": {},
	"unauthorized": {}, "not_authorized": {}, "no_available_channel": {},
}

func safePublicErrorType(value string) string {
	normalized := strings.ToLower(strings.TrimSpace(value))
	if _, ok := safeGroupErrorTypes[normalized]; ok {
		return value
	}
	return ""
}

func safePublicErrorClassifier(value any) any {
	switch typed := value.(type) {
	case nil, float64, float32, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return typed
	case string:
		normalized := strings.ToLower(strings.TrimSpace(typed))
		if _, ok := safeGroupErrorCodes[normalized]; ok {
			return typed
		}
	}
	return nil
}

// rebuildPublicErrorEnvelope deliberately reconstructs a minimal standard
// error envelope after a group failure has been recognized. Provider-owned
// metadata, details, params, raw bodies, arrays and nested diagnostics are not
// copied, so a group identifier cannot move to a field that lacks the complete
// error phrase. Non-error stream payloads never enter this function.
func rebuildPublicErrorEnvelope(envelope map[string]any, kind groupErrorKind) map[string]any {
	result := make(map[string]any, 2)
	if eventType, ok := envelope["type"].(string); ok {
		switch strings.ToLower(strings.TrimSpace(eventType)) {
		case "error", "response.error", "response.failed":
			result["type"] = eventType
		}
	}
	if value, ok := envelope["error"]; ok && value != nil {
		result["error"] = rebuildPublicErrorObject(value, kind)
	}
	if response, ok := envelope["response"].(map[string]any); ok && response["error"] != nil {
		result["response"] = map[string]any{
			"error": rebuildPublicErrorObject(response["error"], kind),
		}
	}
	return result
}

func rebuildPublicErrorObject(value any, kind groupErrorKind) map[string]any {
	result := map[string]any{"message": publicGroupErrorMessage(kind)}
	original, ok := value.(map[string]any)
	if !ok {
		return result
	}
	if errorType, ok := original["type"].(string); ok {
		if safe := safePublicErrorType(errorType); safe != "" {
			result["type"] = safe
		}
	}
	if code := safePublicErrorClassifier(original["code"]); code != nil {
		result["code"] = code
	}
	return result
}
