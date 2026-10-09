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
		{name: "actual English channel selection", text: "Failed to get available channel under group hidden-west", want: publicGroupNoChannelMessage},
		{name: "current Chinese group model selection", text: "当前分组 hidden-east 下对于模型 claude 无可用渠道", want: publicGroupNoChannelMessage},
		{name: "English disabled group", text: "group hidden-disabled is disabled", want: publicGroupAccessDeniedMessage},
		{name: "Chinese disabled group", text: "分组 hidden-disabled 已停用", want: publicGroupAccessDeniedMessage},
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

func TestSanitizeGroupErrorStreamDataRebuildsRecognizedErrorEnvelope(t *testing.T) {
	t.Parallel()

	input := `{"type":"response.failed","metadata":{"group":"stream-secret"},"details":["stream-secret",{"raw":"provider trace for stream-secret"}],"response":{"id":"resp-safe","error":{"message":"Failed to get available channel under group stream-secret","type":"permission_error","code":"stream-secret","param":"stream-secret","details":["stream-secret"],"nested":{"error":"stream-secret"}}}}`
	output, changed := SanitizeGroupErrorStreamData(input)
	require.True(t, changed)
	assert.NotContains(t, output, "stream-secret")
	assert.NotContains(t, output, "provider trace")
	assert.Contains(t, output, publicGroupNoChannelMessage)

	var envelope map[string]any
	require.NoError(t, common.Unmarshal([]byte(output), &envelope))
	assert.Equal(t, "response.failed", envelope["type"])
	assert.NotContains(t, envelope, "metadata")
	response := envelope["response"].(map[string]any)
	streamErr := response["error"].(map[string]any)
	assert.Equal(t, "permission_error", streamErr["type"])
	assert.NotContains(t, streamErr, "code")
	assert.NotContains(t, streamErr, "param")
	assert.NotContains(t, streamErr, "details")
}

func TestSanitizeGroupErrorStreamDataRecognizesUnicodeEscapedMessage(t *testing.T) {
	t.Parallel()

	input := `{"type":"error","error":{"message":"\u65e0\u6743\u8bbf\u95ee escaped-secret \u5206\u7ec4","details":["escaped-secret"]}}`
	output, changed := SanitizeGroupErrorStreamData(input)
	require.True(t, changed)
	assert.NotContains(t, output, "escaped-secret")
	assert.Contains(t, output, publicGroupAccessDeniedMessage)
}

func TestSanitizeGroupErrorStreamDataScrubsArrayAndBareStringErrors(t *testing.T) {
	t.Parallel()

	for _, input := range []string{
		`{"type":"error","error":["无权访问 array-secret 分组",{"message":"array-secret","details":["array-secret"]}]}`,
		`{"type":"error","error":"group disabled: bare-secret group"}`,
		`{"type":"response.error","response":{"error":{"message":"当前分组 zh-secret 下对于模型 claude 无可用渠道","details":["zh-secret"]}}}`,
	} {
		output, changed := SanitizeGroupErrorStreamData(input)
		require.True(t, changed, input)
		assert.NotContains(t, output, "secret", input)
		assert.NotContains(t, output, "details", input)
	}
}

func TestSanitizeGroupErrorStreamDataKeepsMinimalFlatErrorContract(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		input           string
		wantType        string
		wantCode        any
		wantSequence    any
		forbiddenFields []string
	}{
		{
			name:            "flat error event",
			input:           `{"type":"error","message":"无权访问 flat-secret 分组","code":"forbidden","param":"flat-secret","metadata":{"group":"flat-secret"}}`,
			wantType:        "error",
			wantCode:        "forbidden",
			forbiddenFields: []string{"param", "metadata"},
		},
		{
			name:            "Responses response.error event",
			input:           `{"type":"response.error","message":"Failed to get available channel under group responses-secret","code":"no_available_channel","param":"responses-secret","sequence_number":17,"metadata":{"group":"responses-secret"}}`,
			wantType:        "response.error",
			wantCode:        "no_available_channel",
			wantSequence:    float64(17),
			forbiddenFields: []string{"param", "metadata"},
		},
		{
			name:            "metadata-only diagnostic",
			input:           `{"type":"error","metadata":{"message":"not authorized to access metadata-secret group","details":["metadata-secret"]}}`,
			wantType:        "error",
			forbiddenFields: []string{"metadata"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			output, changed := SanitizeGroupErrorStreamData(test.input)
			require.True(t, changed)
			assert.NotContains(t, output, "secret")

			var envelope map[string]any
			require.NoError(t, common.Unmarshal([]byte(output), &envelope))
			assert.Equal(t, test.wantType, envelope["type"])
			assert.NotEmpty(t, envelope["message"])
			assert.Contains(t, envelope["message"], "上游分组")
			if test.wantCode != nil {
				assert.Equal(t, test.wantCode, envelope["code"])
			}
			if test.wantSequence != nil {
				assert.Equal(t, test.wantSequence, envelope["sequence_number"])
			}
			for _, field := range test.forbiddenFields {
				assert.NotContains(t, envelope, field)
			}
		})
	}
}

func TestSanitizeGroupErrorStreamDataRebuildsArrayErrorWithoutLeakingElements(t *testing.T) {
	t.Parallel()

	input := `{"type":"error","error":[{"message":"无权访问 array-contract-secret 分组","code":"forbidden"},"array-contract-secret",{"metadata":{"group":"array-contract-secret"}}],"metadata":{"trace":"array-contract-secret"}}`
	output, changed := SanitizeGroupErrorStreamData(input)
	require.True(t, changed)
	assert.NotContains(t, output, "array-contract-secret")

	var envelope map[string]any
	require.NoError(t, common.Unmarshal([]byte(output), &envelope))
	streamErr, ok := envelope["error"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, publicGroupAccessDeniedMessage, streamErr["message"])
	assert.NotContains(t, envelope, "metadata")
}

func TestCopyWithPublicMessageDropsProviderControlledClassifiers(t *testing.T) {
	t.Parallel()

	original := WithOpenAIError(OpenAIError{
		Message:  "无权访问 copy-secret 分组",
		Type:     "permission_error",
		Param:    "copy-secret",
		Code:     "copy-secret",
		Metadata: []byte(`{"details":["copy-secret"]}`),
	}, 403)
	clean := original.CopyWithPublicMessage(publicGroupAccessDeniedMessage)
	serialized, err := common.Marshal(clean.ToOpenAIError())
	require.NoError(t, err)
	assert.NotContains(t, string(serialized), "copy-secret")
	assert.Equal(t, "permission_error", clean.ToOpenAIError().Type)
	assert.Empty(t, clean.ToOpenAIError().Param)
	assert.Nil(t, clean.ToOpenAIError().Code)
	assert.Contains(t, original.ToOpenAIError().Param, "copy-secret")
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

func TestPublicGroupErrorPreservesKnownLocalClassifiers(t *testing.T) {
	t.Parallel()
	original := WithOpenAIError(OpenAIError{Message: "无权访问 classifier-secret 分组", Type: "upstream_error", Code: "bad_response_status_code"}, 403)
	clean := original.CopyWithPublicMessage(publicGroupAccessDeniedMessage)
	assert.Equal(t, "upstream_error", clean.ToOpenAIError().Type)
	assert.Equal(t, "bad_response_status_code", clean.ToOpenAIError().Code)
	unknown := WithOpenAIError(OpenAIError{Message: "无权访问 classifier-secret 分组", Type: "classifier-secret", Code: "classifier-secret"}, 403).CopyWithPublicMessage(publicGroupAccessDeniedMessage)
	assert.Equal(t, "upstream_error", unknown.ToOpenAIError().Type)
	assert.Nil(t, unknown.ToOpenAIError().Code)
}
