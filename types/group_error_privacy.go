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
		regexp.MustCompile(`(?i)(?:分组|群组).{0,96}(?:无权|无权限|没有权限|未授权|禁止访问)`),
	}
	englishGroupAccessDeniedPatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)(?:no permission|not authorized|not allowed|unauthorized|forbidden)\s+(?:to\s+)?(?:access|use)\s+(?:the\s+)?.{0,96}\bgroup\b`),
		regexp.MustCompile(`(?i)\bgroup\b.{0,96}\baccess\s+(?:is\s+)?denied\b`),
	}
	chineseGroupNoChannelPatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)(?:分组|群组).{0,160}(?:无可用(?:渠道|通道)|没有可用(?:渠道|通道)|可用(?:渠道|通道)不存在)`),
		regexp.MustCompile(`(?i)(?:无可用(?:渠道|通道)|没有可用(?:渠道|通道)|可用(?:渠道|通道)不存在).{0,160}(?:分组|群组)`),
	}
	englishGroupNoChannelPatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)(?:no available (?:upstream )?channels?).{0,160}\b(?:for|under|in)\s+(?:the\s+)?group\b`),
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
		relayCopy.Metadata = nil
		copyErr.RelayError = relayCopy
	case ClaudeError:
		relayCopy := relayErr
		relayCopy.Message = message
		copyErr.RelayError = relayCopy
	default:
		copyErr.RelayError = nil
	}
	return &copyErr
}

// SanitizeGroupErrorStreamData only rewrites structured error envelopes. A
// normal completion containing the same words remains byte-for-byte unchanged.
func SanitizeGroupErrorStreamData(data string) (string, bool) {
	kind := classifyGroupError(data)
	if kind == groupErrorNone {
		return data, false
	}

	var envelope map[string]any
	if err := common.Unmarshal([]byte(data), &envelope); err != nil || !isErrorEnvelope(envelope) {
		return data, false
	}

	scrubGroupErrorValue(envelope, kind)
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

func scrubGroupErrorValue(value any, kind groupErrorKind) {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			normalizedKey := strings.ToLower(strings.TrimSpace(key))
			switch normalizedKey {
			case "group", "groups", "selected_group", "selectedgroup":
				delete(typed, key)
				continue
			}
			if text, ok := child.(string); ok && classifyGroupError(text) != groupErrorNone {
				typed[key] = publicGroupErrorMessage(kind)
				continue
			}
			scrubGroupErrorValue(child, kind)
		}
	case []any:
		for _, child := range typed {
			scrubGroupErrorValue(child, kind)
		}
	}
}
