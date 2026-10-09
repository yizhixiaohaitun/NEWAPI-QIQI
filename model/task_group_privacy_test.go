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

func TestTaskCopyForPublicResponseSanitizesUnicodeStructuredError(t *testing.T) {
	raw := []byte(`{"status":"failed","error":{"message":"\u65e0\u6743\u8bbf\u95ee private-task \u5206\u7ec4","metadata":{"group":"private-task"},"details":["private-task"]}}`)
	task := &Task{Data: raw}

	publicTask := task.CopyForPublicResponse()

	assert.NotContains(t, string(publicTask.Data), "private-task")
	assert.Contains(t, string(publicTask.Data), "上游分组")
	assert.Equal(t, raw, []byte(task.Data))
}

func TestTaskCopyForPublicResponseRebuildsFlatFailedError(t *testing.T) {
	raw := []byte(`{"id":"task_public","status":"failed","message":"group private-task-flat has no available channels","metadata":{"group":"private-task-flat"},"details":["private-task-flat"],"prompt":"ordinary prompt"}`)
	task := &Task{Data: raw}

	publicTask := task.CopyForPublicResponse()

	assert.NotContains(t, string(publicTask.Data), "private-task-flat")
	assert.Contains(t, string(publicTask.Data), "上游分组")
	assert.Contains(t, string(publicTask.Data), `"id":"task_public"`)
	assert.Contains(t, string(publicTask.Data), `"prompt":"ordinary prompt"`)
	assert.Equal(t, raw, []byte(task.Data))
}

func TestTaskCopyForPublicResponseRebuildsErrorArray(t *testing.T) {
	raw := []byte(`{"error":["group private-task-array has no available channels","private-task-array"]}`)
	task := &Task{Data: raw}

	publicTask := task.CopyForPublicResponse()

	assert.NotContains(t, string(publicTask.Data), "private-task-array")
	assert.Contains(t, string(publicTask.Data), "上游分组")
	assert.Equal(t, raw, []byte(task.Data))
}

func TestTaskCopyForPublicResponseRemovesParentDiagnosticsForExplicitError(t *testing.T) {
	raw := []byte(`{"status":"failed","id":"task_public","error":{"message":"group private-parent has no available channels"},"metadata":{"group":"private-parent"},"details":["private-parent"],"debug":"private-parent","prompt":"ordinary prompt","output":"ordinary output"}`)
	task := &Task{Data: raw}

	publicTask := task.CopyForPublicResponse()

	assert.NotContains(t, string(publicTask.Data), "private-parent")
	assert.NotContains(t, string(publicTask.Data), `"metadata"`)
	assert.NotContains(t, string(publicTask.Data), `"details"`)
	assert.NotContains(t, string(publicTask.Data), `"debug"`)
	assert.Contains(t, string(publicTask.Data), `"id":"task_public"`)
	assert.Contains(t, string(publicTask.Data), `"prompt":"ordinary prompt"`)
	assert.Contains(t, string(publicTask.Data), `"output":"ordinary output"`)
	assert.Equal(t, raw, []byte(task.Data))
}

func TestTaskCopyForPublicResponseSanitizesFailReasonWithoutStatus(t *testing.T) {
	raw := []byte(`{"id":"task_public","fail_reason":"group private-no-status has no available channels","metadata":{"group":"private-no-status"}}`)
	task := &Task{Data: raw}

	publicTask := task.CopyForPublicResponse()

	assert.NotContains(t, string(publicTask.Data), "private-no-status")
	assert.Contains(t, string(publicTask.Data), "上游分组")
	assert.Contains(t, string(publicTask.Data), `"id":"task_public"`)
	assert.Equal(t, raw, []byte(task.Data))
}

func TestTaskCopyForPublicResponseSanitizesDiagnosticResultFields(t *testing.T) {
	raw := []byte(`{"status":"failed","message":"group private-result has no available channels","result":"group private-result has no available channels","result_url":"group private-result has no available channels","url":"group private-result has no available channels"}`)
	task := &Task{Data: raw}

	publicTask := task.CopyForPublicResponse()

	assert.NotContains(t, string(publicTask.Data), "private-result")
	assert.Contains(t, string(publicTask.Data), "上游分组")
	assert.Equal(t, raw, []byte(task.Data))
}
