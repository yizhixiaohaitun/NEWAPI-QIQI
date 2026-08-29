package middleware

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
)

func TestValidVideoContentSignature(t *testing.T) {
	taskID := "task_public"
	signature := common.GenerateHMAC("video-content:" + taskID)

	assert.True(t, validVideoContentSignature(taskID, signature))
	assert.False(t, validVideoContentSignature(taskID, signature+"00"))
	assert.False(t, validVideoContentSignature("task_other", signature))
	assert.False(t, validVideoContentSignature("", signature))
	assert.False(t, validVideoContentSignature(taskID, ""))
}
