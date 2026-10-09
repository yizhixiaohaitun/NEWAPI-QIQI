package controller

import (
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
)

func TestTaskListAndDetailSanitizePublicFailureCopies(t *testing.T) {
	rawReason := "group ClaudeCode_AZ has no available channels"
	task := &model.Task{
		FailReason: rawReason,
		Data:       []byte(`{"status":"failed","error":{"message":"not authorized to access group claude-enterprise"}}`),
	}

	listItem := tasksToDto([]*model.Task{task}, false)[0]
	detail := taskToDetailDto(task, false)

	for _, response := range []string{listItem.FailReason, detail.FailReason} {
		assert.Equal(t, "请求的上游分组无可用渠道", response)
	}
	assert.Nil(t, listItem.Data)
	assert.Nil(t, detail.Data)
	assert.Equal(t, rawReason, task.FailReason)
	assert.Contains(t, string(task.Data), "claude-enterprise")
}
