package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTaskCopyForPublicResponseSanitizesFailureWithoutMutatingStoredTask(t *testing.T) {
	rawReason := "group ClaudeCode_AZ has no available channels"
	rawData := []byte(`{"status":"failed","error":{"message":"not authorized to access group claude-enterprise"}}`)
	task := &Task{FailReason: rawReason, Data: rawData}

	publicTask := task.CopyForPublicResponse()

	require.NotSame(t, task, publicTask)
	assert.Equal(t, rawReason, task.FailReason)
	assert.Equal(t, rawData, []byte(task.Data))
	assert.Equal(t, "请求的上游分组无可用渠道", publicTask.FailReason)
	assert.Equal(t, "请求的上游分组无可用渠道", publicTask.GetResultURL())
	assert.Nil(t, publicTask.Data)
}

func TestSanitizeTaskPublicErrorJSONHandlesEscapedGroupName(t *testing.T) {
	raw := []byte(`{"status":"failed","error":{"message":"group ClaudeCode_\\u0041Z has no available channels"}}`)

	sanitized := string(SanitizeTaskPublicErrorJSON(raw))

	assert.NotContains(t, sanitized, "ClaudeCode_AZ")
	assert.NotContains(t, sanitized, `ClaudeCode_\\u0041Z`)
	assert.Contains(t, sanitized, "请求的上游分组无可用渠道")
}

func TestTaskCopyForPublicResponsePreservesOrdinaryFailure(t *testing.T) {
	task := &Task{FailReason: "provider rejected the prompt"}

	publicTask := task.CopyForPublicResponse()

	assert.Equal(t, task.FailReason, publicTask.FailReason)
}
