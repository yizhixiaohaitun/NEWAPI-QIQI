package model

import (
	"encoding/json"
	"strings"
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSanitizeTaskDetailJSONRedactsSecretsAndPreservesMediaSignature(t *testing.T) {
	raw := json.RawMessage(`{
		"authorization":"Bearer top-secret",
		"nested":{"api_key":"sk-test","prompt":"Authorization: Bearer abc123"},
		"reference_images":[
			"https://cdn.example/ref.png?sign=media-signature&t=123",
			"https://cdn.example/ref.png?api_key=secret-value",
			{"base64":"very-large-payload"},
			"data:image/png;base64,inline-payload"
		]
	}`)

	sanitized := SanitizeTaskDetailJSON(raw)
	text := string(sanitized)
	assert.NotContains(t, text, "top-secret")
	assert.NotContains(t, text, "sk-test")
	assert.NotContains(t, text, "abc123")
	assert.NotContains(t, text, "secret-value")
	assert.NotContains(t, text, "very-large-payload")
	assert.NotContains(t, text, "inline-payload")
	assert.Contains(t, text, "media-signature")
	assert.Contains(t, text, "%5BREDACTED%5D")
	assert.Contains(t, text, "[OMITTED BASE64]")
	assert.Contains(t, text, "[OMITTED DATA URL]")
}

func TestInitTaskPersistsSanitizedRequestSnapshot(t *testing.T) {
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{},
		TaskRelayInfo: &relaycommon.TaskRelayInfo{
			TaskInput:           "rainy alley",
			ReferenceResources:  []string{"https://cdn.example/ref.png"},
			TaskRequestSnapshot: json.RawMessage(`{"model":"sd_2.0_discount","prompt":"rainy alley","api_key":"secret"}`),
		},
	}

	task := InitTask("seedance", info)
	require.NotEmpty(t, task.Properties.RequestSnapshot)
	assert.Contains(t, string(task.Properties.RequestSnapshot), "sd_2.0_discount")
	assert.NotContains(t, string(task.Properties.RequestSnapshot), "secret\"")
	assert.True(t, strings.Contains(string(task.Properties.RequestSnapshot), "[REDACTED]") || strings.Contains(string(task.Properties.RequestSnapshot), "\\u005bREDACTED\\u005d"))
}
