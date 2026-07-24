package service

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func intPointer(value int) *int { return &value }

func TestEvaluateActiveModelProbeRequiresBothSignals(t *testing.T) {
	result := &model.ModelProbeResult{
		DeclaredModel:  "models/GPT-4O",
		ActualModel:    "openai/gpt-4o",
		ExpectedTokens: intPointer(100),
		ActualTokens:   intPointer(104),
	}
	EvaluateActiveModelProbe(result)
	require.Equal(t, ProbeStatusMatch, result.IdStatus)
	assert.Equal(t, ProbeStatusMatch, result.TokenStatus)
	assert.Equal(t, 5, result.TokenTolerance)
	assert.Equal(t, 4, *result.TokenDelta)
	assert.Equal(t, ProbeConclusionPassed, result.Conclusion)
}

func TestEvaluateActiveModelProbeMismatchAndUnknown(t *testing.T) {
	mismatch := &model.ModelProbeResult{
		DeclaredModel: "gpt-4o", ActualModel: "gpt-4o-mini",
		ExpectedTokens: intPointer(10), ActualTokens: intPointer(20),
	}
	EvaluateActiveModelProbe(mismatch)
	assert.Equal(t, ProbeStatusMismatch, mismatch.IdStatus)
	assert.Equal(t, ProbeStatusMismatch, mismatch.TokenStatus)
	assert.Equal(t, ProbeConclusionSuspicious, mismatch.Conclusion)

	unknown := &model.ModelProbeResult{DeclaredModel: "gpt-4o", ActualModel: "gpt-4o"}
	EvaluateActiveModelProbe(unknown)
	assert.Equal(t, ProbeStatusUnknown, unknown.TokenStatus)
	assert.Equal(t, ProbeConclusionUnknown, unknown.Conclusion)
}

func TestEvaluateActiveModelProbeFailureWins(t *testing.T) {
	result := &model.ModelProbeResult{DeclaredModel: "gpt-4o", Error: "upstream timeout"}
	EvaluateActiveModelProbe(result)
	assert.Equal(t, ProbeConclusionFailed, result.Conclusion)
}

func TestStoreAndNotifyModelProbePersistsBeforeNotificationAndDeduplicates(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.ModelProbeResult{}))

	originalDB := model.DB
	originalNotifier := modelProbeNotifier
	model.DB = db
	modelProbeNotifyState.Lock()
	modelProbeNotifyState.last = make(map[string]time.Time)
	modelProbeNotifyState.Unlock()
	t.Cleanup(func() {
		model.DB = originalDB
		modelProbeNotifier = originalNotifier
	})

	notifications := 0
	modelProbeNotifier = func(_ string, _ string, _ string) {
		var count int64
		require.NoError(t, db.Model(&model.ModelProbeResult{}).Count(&count).Error)
		require.Equal(t, int64(1), count, "notification must run after persistence")
		notifications++
	}

	first := &model.ModelProbeResult{ChannelId: 7, ChannelName: "test", DeclaredModel: "gpt-4o", ActualModel: "gpt-4o-mini"}
	require.NoError(t, StoreAndNotifyModelProbe(first))
	second := &model.ModelProbeResult{ChannelId: 7, ChannelName: "test", DeclaredModel: "gpt-4o", ActualModel: "gpt-4o-mini"}
	require.NoError(t, StoreAndNotifyModelProbe(second))

	assert.Equal(t, 1, notifications)
	var stored []model.ModelProbeResult
	require.NoError(t, db.Order("id").Find(&stored).Error)
	require.Len(t, stored, 2)
	assert.Equal(t, ProbeConclusionSuspicious, stored[0].Conclusion)
	assert.Equal(t, ProbeConclusionSuspicious, stored[1].Conclusion)
}
