package controller

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/stretchr/testify/assert"
)

func TestContextRequestLogCleanupHandlerDisabledAtZeroRetention(t *testing.T) {
	setting := operation_setting.GetQiqiSetting()
	original := *setting
	t.Cleanup(func() { *setting = original })

	setting.ContextRequestLogRetentionDays = 0
	handler := contextRequestLogCleanupHandler{}
	assert.False(t, handler.Enabled())
	assert.Equal(t, model.SystemTaskTypeContextLogCleanup, handler.Type())
	assert.Equal(t, 24*time.Hour, handler.Interval())
}

func TestContextRequestLogCleanupPayloadUsesRetentionThreshold(t *testing.T) {
	setting := operation_setting.GetQiqiSetting()
	original := *setting
	t.Cleanup(func() { *setting = original })

	setting.ContextRequestLogRetentionDays = 30
	handler := contextRequestLogCleanupHandler{}
	before := time.Now().Unix()
	payload := handler.NewPayload().(contextRequestLogCleanupPayload)
	after := time.Now().Unix()

	assert.True(t, handler.Enabled())
	assert.Equal(t, 30, payload.RetentionDays)
	assert.Equal(t, contextRequestLogCleanupBatchSize, payload.BatchSize)
	assert.GreaterOrEqual(t, payload.Cutoff, before-int64(30*24*time.Hour/time.Second))
	assert.LessOrEqual(t, payload.Cutoff, after-int64(30*24*time.Hour/time.Second))
}
