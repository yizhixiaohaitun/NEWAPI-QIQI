package common

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestMaskSensitiveInfoProtectsOnlyProtocolPaths(t *testing.T) {
	for _, text := range []string{`"thinking.type.enabled" is not supported. Use "thinking.type.adaptive" and "output_config.effort".`, "thinking.budget_tokens thinking.type.disabled"} {
		assert.Equal(t, text, MaskSensitiveInfo(text))
	}
	for _, text := range []string{"https://thinking.type.enabled", "thinking.type.enabled.evil.com", "user@thinking.type.enabled", "evil.thinking.type.enabled", "foo.thinking.budget_tokens.com", "config.effort.com", "api_key:secretkey123", "example.com"} {
		assert.NotEqual(t, text, MaskSensitiveInfo(text), text)
	}
	assert.Equal(t, "message (request id: here)", MessageWithRequestId("message (request id: one) (request id: two) (request id: three)", "here"))
	assert.Equal(t, "(request id: embedded) message (request id: here)", MessageWithRequestId("(request id: embedded) message", "here"))
}
