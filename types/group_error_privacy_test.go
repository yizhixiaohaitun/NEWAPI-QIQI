package types

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPublicGroupErrorMessageRecognizesGroupFailuresWithoutFixedNames(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		text string
		want string
	}{
		{name: "reported Chinese access denial", text: "无权访问 ClaudeCode_AZ 分组 (request id: upstream-1)", want: publicGroupAccessDeniedMessage},
		{name: "random Chinese group", text: "没有权限访问 tenant-random-9 群组", want: publicGroupAccessDeniedMessage},
		{name: "English access denial", text: "not authorized to access private-blue group", want: publicGroupAccessDeniedMessage},
		{name: "Chinese no channel", text: "分组 corp-secret 下模型 claude 无可用渠道", want: publicGroupNoChannelMessage},
		{name: "English no channel", text: "no available upstream channel for group hidden-west", want: publicGroupNoChannelMessage},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			message, matched := PublicGroupErrorMessage(test.text)
			require.True(t, matched)
			assert.Equal(t, test.want, message)
		})
	}
}

func TestPublicGroupErrorMessageLeavesUnrelatedErrorsAndRefusalsAlone(t *testing.T) {
	t.Parallel()

	for _, text := range []string{
		"content policy refused this prompt",
		"permission denied for model claude-4",
		"no available channel for model gpt-5",
	} {
		message, matched := PublicGroupErrorMessage(text)
		assert.False(t, matched, text)
		assert.Empty(t, message, text)
	}
}

func TestSanitizeGroupErrorStreamDataScrubsNestedAndRawErrorFields(t *testing.T) {
	t.Parallel()

	input := `{"type":"response.failed","response":{"error":{"message":"not authorized to access random-secret group","type":"permission_error","code":"forbidden","group":"random-secret","nested":{"body":"{\"error\":{\"message\":\"无权访问 random-secret 分组\"}}"}}}}`
	output, changed := SanitizeGroupErrorStreamData(input)
	require.True(t, changed)
	assert.NotContains(t, output, "random-secret")
	assert.NotContains(t, output, "not authorized")
	assert.Contains(t, output, publicGroupAccessDeniedMessage)

	var envelope map[string]any
	require.NoError(t, common.Unmarshal([]byte(output), &envelope))
	response := envelope["response"].(map[string]any)
	streamErr := response["error"].(map[string]any)
	assert.Equal(t, "permission_error", streamErr["type"])
	assert.Equal(t, "forbidden", streamErr["code"])
	assert.NotContains(t, streamErr, "group")
}

func TestSanitizeGroupErrorStreamDataDoesNotRedactNormalModelOutput(t *testing.T) {
	t.Parallel()

	input := `{"id":"chatcmpl-1","choices":[{"delta":{"content":"The story says no permission to access the blue group."}}]}`
	output, changed := SanitizeGroupErrorStreamData(input)
	assert.False(t, changed)
	assert.Equal(t, input, output)

	refusal := `{"error":{"message":"Your request was rejected by the content policy","type":"invalid_request_error","code":"content_policy"}}`
	output, changed = SanitizeGroupErrorStreamData(refusal)
	assert.False(t, changed)
	assert.Equal(t, refusal, output)
}
