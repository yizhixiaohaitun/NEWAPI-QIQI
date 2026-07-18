package channel_purity

import (
	"fmt"
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupPurityDB(t *testing.T) {
	t.Helper()
	previous := model.DB
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:purity-%s?mode=memory&cache=shared", t.Name())), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.ChannelPurityGroup{}, &model.ChannelPurityMember{}, &model.ChannelPuritySample{}, &model.ChannelPurityPairRun{}, &model.ChannelPurityAssessment{}, &model.ChannelPurityAlert{}))
	model.DB = db
	t.Cleanup(func() { model.DB = previous })
}

func createTestPurityGroup(t *testing.T) model.ChannelPurityGroup {
	t.Helper()
	one := 1
	group := model.ChannelPurityGroup{Name: "formal-pairs", Enabled: true, IntervalMinutes: 5, WindowMinutes: 30, MinimumSamples: 1, MaxSamplesPerWindow: 2,
		Members: []model.ChannelPurityMember{{ChannelID: 10, IsBaseline: true, BaselineSlot: &one}, {ChannelID: 20}, {ChannelID: 30}}}
	require.NoError(t, model.DB.Create(&group).Error)
	return group
}

func addPuritySample(t *testing.T, groupID uint, channel int, actualModel, runKey, signature string, tokens int, observedAt int64) {
	t.Helper()
	require.NoError(t, model.DB.Create(&model.ChannelPuritySample{GroupID: groupID, ChannelID: channel, ActualModel: actualModel, RunKey: runKey, StructureSignature: signature, TotalTokens: tokens, Valid: true, ObservedAt: observedAt}).Error)
}

func TestAggregatePairWindowExcludesUnpairedRowsFromAllStatistics(t *testing.T) {
	setupPurityDB(t)
	group := createTestPurityGroup(t)
	addPuritySample(t, group.ID, 10, "m1", "paired", "same", 100, 100)
	addPuritySample(t, group.ID, 20, "m1", "paired", "same", 110, 101)
	addPuritySample(t, group.ID, 10, "m1", "baseline-only", "noise-a", 999, 102)
	addPuritySample(t, group.ID, 20, "m1", "target-only", "noise-b", 1, 103)

	_, err := AggregatePairWindow(group.ID, 20, "m1", 90, 120, DefaultAggregatePolicy())
	require.NoError(t, err)
	var run model.ChannelPurityPairRun
	require.NoError(t, model.DB.Last(&run).Error)
	assert.Equal(t, 1, run.PairedSampleCount)
	assert.Equal(t, 1, run.BaselineSampleCount)
	assert.Equal(t, 1, run.TargetSampleCount)
	assert.Equal(t, 100, run.BaselineTokenMin)
	assert.Equal(t, 100, run.BaselineTokenMax)
	assert.Equal(t, 110, run.TargetTokenMin)
	assert.Equal(t, 110, run.TargetTokenMax)
	assert.Equal(t, 1.0, run.StructureSimilarity)
}

func TestFormalAssessmentUsesRobustPairedTokenDistribution(t *testing.T) {
	setupPurityDB(t)
	group := createTestPurityGroup(t)
	ratios := []int{98, 100, 101, 102, 103, 104, 105, 190, 200, 210}
	for i, targetTokens := range ratios {
		key := fmt.Sprintf("pair-%d", i)
		addPuritySample(t, group.ID, 10, "m1", key, "same", 100, int64(100+i))
		addPuritySample(t, group.ID, 20, "m1", key, "same", targetTokens, int64(100+i))
	}
	policy := DefaultAggregatePolicy()
	policy.MinSamples = 5
	_, err := AggregatePairWindow(group.ID, 20, "m1", 90, 120, policy)
	require.NoError(t, err)
	var run model.ChannelPurityPairRun
	require.NoError(t, model.DB.Last(&run).Error)
	assert.Equal(t, 10, run.PairedSampleCount)
	assert.InDelta(t, 0.3, run.TokenDeviationRate, 0.001)
	assert.Less(t, run.TokenSimilarity, 0.2, "repeated robust-interval outliers must penalize the formal score")
	assert.Contains(t, run.AnomalyEvidenceJSON, "token_interval_shift", "repeated robust-interval outliers must enter formal evidence")
}

func TestWindowQuotaCountIsolatedByTargetAndActualModel(t *testing.T) {
	setupPurityDB(t)
	group := createTestPurityGroup(t)
	for i := 0; i < 2; i++ {
		key := fmt.Sprintf("m1-t20-%d", i)
		addPuritySample(t, group.ID, 10, "m1", key, "s", 100, int64(100+i))
		addPuritySample(t, group.ID, 20, "m1", key, "s", 100, int64(100+i))
	}
	addPuritySample(t, group.ID, 10, "m1", "m1-t30", "s", 100, 105)
	addPuritySample(t, group.ID, 30, "m1", "m1-t30", "s", 100, 105)
	addPuritySample(t, group.ID, 10, "m2", "m2-t20", "s", 100, 106)
	addPuritySample(t, group.ID, 20, "m2", "m2-t20", "s", 100, 106)
	addPuritySample(t, group.ID, 10, "m1", "old", "s", 100, 10)
	addPuritySample(t, group.ID, 20, "m1", "old", "s", 100, 10)

	count, err := CountValidPairedSamples(group.ID, 10, 20, "m1", 90, 120)
	require.NoError(t, err)
	assert.Equal(t, int64(2), count)
	otherTarget, err := CountValidPairedSamples(group.ID, 10, 30, "m1", 90, 120)
	require.NoError(t, err)
	assert.Equal(t, int64(1), otherTarget)
	otherModel, err := CountValidPairedSamples(group.ID, 10, 20, "m2", 90, 120)
	require.NoError(t, err)
	assert.Equal(t, int64(1), otherModel)
}
