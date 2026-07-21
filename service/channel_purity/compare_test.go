package channel_purity

import (
	"fmt"
	"testing"

	"github.com/QuantumNous/new-api/common"
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
	require.NoError(t, db.AutoMigrate(&model.ChannelPurityGroup{}, &model.ChannelPurityMember{}, &model.ChannelPurityModelComparison{}, &model.ChannelPuritySample{}, &model.ChannelPurityPairRun{}, &model.ChannelPurityAssessment{}, &model.ChannelPurityAlert{}))
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

	_, err := AggregatePairWindow(group.ID, 20, "m1", "m1", 90, 120, DefaultAggregatePolicy())
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
	tokenDetail, err := DecodeTokenSimilarityDetail(&run)
	require.NoError(t, err)
	require.NotNil(t, tokenDetail)
	assert.True(t, tokenDetail.ScoreAvailable)
	assert.Equal(t, 1, tokenDetail.BaselineValidSamples)
	assert.Equal(t, 1, tokenDetail.TargetValidSamples)
	assert.Equal(t, 1, tokenDetail.PairedCount)
	assert.Equal(t, 100, tokenDetail.BaselineMin)
	assert.Equal(t, 110, tokenDetail.TargetMax)
	assert.InDelta(t, 1.1, tokenDetail.RatioMedian, 0.0001)
	require.Len(t, tokenDetail.Pairs, 1)
	assert.False(t, tokenDetail.Pairs[0].Outside)
}

func TestCompareSamplesDoesNotTreatUnavailableTokenMetricAsZeroAnomaly(t *testing.T) {
	baseline := []model.ChannelPuritySample{{Valid: true, StructureSignature: "same", TotalTokens: -1}}
	target := []model.ChannelPuritySample{{Valid: true, StructureSignature: "same", TotalTokens: -1}}
	structure, token, _, evidence := CompareSamples(baseline, target, 1)
	assert.Equal(t, 1.0, structure)
	assert.Equal(t, 0.0, token)
	assert.NotContains(t, evidence, "token_interval_shift")
}

func TestTokenDetailRobustStatisticsAndUnavailableCompatibility(t *testing.T) {
	baseline, target := make([]model.ChannelPuritySample, 0, 5), make([]model.ChannelPuritySample, 0, 5)
	for i, value := range []int{100, 100, 100, 100, 100} {
		baseline = append(baseline, model.ChannelPuritySample{Valid: true, ActualModel: "m", TotalTokens: value})
		target = append(target, model.ChannelPuritySample{Valid: true, ActualModel: "m", TotalTokens: []int{90, 100, 110, 120, 300}[i]})
	}
	detail := BuildTokenSimilarityDetail(baseline, target, 5)
	assert.Equal(t, 5, detail.PairedCount)
	assert.InDelta(t, 1.1, detail.RatioMedian, 0.0001)
	assert.InDelta(t, 1.0, detail.Q1, 0.0001)
	assert.InDelta(t, 1.2, detail.Q3, 0.0001)
	assert.InDelta(t, 0.1, detail.MAD, 0.0001)
	assert.Equal(t, 1, detail.OutsideCount)
	assert.InDelta(t, 0.2, detail.DeviationRate, 0.0001)

	empty := BuildTokenSimilarityDetail(nil, nil, 5)
	assert.False(t, empty.ScoreAvailable)
	assert.Equal(t, 0, empty.PairedCount)

	oneMissing := BuildTokenSimilarityDetail(
		[]model.ChannelPuritySample{{Valid: true, TotalTokens: 100}},
		[]model.ChannelPuritySample{{Valid: true, TotalTokens: 0}}, 5,
	)
	assert.Equal(t, 1, oneMissing.BaselineValidSamples)
	assert.Equal(t, 0, oneMissing.TargetValidSamples)
	assert.Equal(t, 0, oneMissing.PairedCount)
	assert.False(t, oneMissing.ScoreAvailable)
	assert.Equal(t, 0.4, func() float64 { value, _ := combinedSimilarity(0.4, true, 0, oneMissing.ScoreAvailable); return value }())

	bothMissing := BuildTokenSimilarityDetail(
		[]model.ChannelPuritySample{{Valid: true, TotalTokens: 0}},
		[]model.ChannelPuritySample{{Valid: true, TotalTokens: 0}}, 5,
	)
	assert.Equal(t, 0, bothMissing.BaselineValidSamples)
	assert.Equal(t, 0, bothMissing.TargetValidSamples)
	assert.Equal(t, 0, bothMissing.BaselineMin)
	assert.Equal(t, 0, bothMissing.TargetMax)
	assert.False(t, bothMissing.ScoreAvailable)
	legacy, err := DecodeTokenSimilarityDetail(&model.ChannelPurityPairRun{})
	require.NoError(t, err)
	assert.Nil(t, legacy)
	_, err = DecodeTokenSimilarityDetail(&model.ChannelPurityPairRun{TokenSimilarityDetail: "{"})
	require.Error(t, err)
	assert.Equal(t, 0.4, func() float64 { value, _ := combinedSimilarity(0.4, true, 0, false); return value }())
}

func TestPersistedStructureSimilarityDetailUsesExactScoringInputs(t *testing.T) {
	setupPurityDB(t)
	group := createTestPurityGroup(t)
	for i, signature := range []string{"shared", "baseline-only"} {
		key := fmt.Sprintf("pair-%d", i)
		addPuritySample(t, group.ID, 10, "m1", key, signature, 100, int64(100+i))
		targetSignature := "shared"
		if i == 1 {
			targetSignature = "target-only"
		}
		addPuritySample(t, group.ID, 20, "m1", key, targetSignature, 100, int64(100+i))
	}
	_, err := AggregatePairWindow(group.ID, 20, "m1", "m1", 90, 120, DefaultAggregatePolicy())
	require.NoError(t, err)
	var run model.ChannelPurityPairRun
	require.NoError(t, model.DB.Last(&run).Error)
	detail, err := DecodeStructureSimilarityDetail(&run)
	require.NoError(t, err)
	require.NotNil(t, detail)
	assert.Equal(t, StructureSimilarityDetailVersion, detail.Version)
	assert.Equal(t, int64(90), detail.WindowStartedAt)
	assert.Equal(t, int64(120), detail.WindowEndedAt)
	assert.Equal(t, 2, detail.PairedSampleCount)
	assert.Equal(t, 1, detail.MatchedCount)
	assert.Equal(t, 1, detail.BaselineOnlyCount)
	assert.Equal(t, 1, detail.TargetOnlyCount)
	assert.Equal(t, 1, detail.IntersectionCount)
	assert.Equal(t, 3, detail.UnionCount)
	assert.False(t, detail.FieldPathsAvailable)
	assert.InDelta(t, run.StructureSimilarity, float64(detail.IntersectionCount)/float64(detail.UnionCount), 0.0001)
}

func TestStructureDetailExplainsAddedMissingTypeAndFrequencyChanges(t *testing.T) {
	profile := func(fields ...storedFieldProfile) string {
		encoded, err := common.Marshal(fields)
		require.NoError(t, err)
		return string(encoded)
	}
	baseline := []model.ChannelPuritySample{
		{Valid: true, StructureSignature: "a", StructureProfileJSON: profile(
			storedFieldProfile{Path: "response.id", Type: "string"},
			storedFieldProfile{Path: "response.old", Type: "boolean"},
			storedFieldProfile{Path: "response.freq", Type: "string"},
		)},
		{Valid: true, StructureSignature: "a", StructureProfileJSON: profile(storedFieldProfile{Path: "response.freq", Type: "string"})},
	}
	target := []model.ChannelPuritySample{
		{Valid: true, StructureSignature: "b", StructureProfileJSON: profile(
			storedFieldProfile{Path: "response.id", Type: "number"},
			storedFieldProfile{Path: "response.new", Type: "object"},
			storedFieldProfile{Path: "response.freq", Type: "string"},
		)},
		{Valid: true, StructureSignature: "b", StructureProfileJSON: profile(storedFieldProfile{Path: "response.new", Type: "object"})},
	}
	detail := BuildStructureSimilarityDetail(baseline, target)
	assert.True(t, detail.DetailAvailable)
	assert.True(t, detail.ScoreAvailable)
	byPath := map[string]FieldProfileDifference{}
	for _, difference := range detail.FieldDifferences {
		byPath[difference.Path] = difference
	}
	assert.Equal(t, "type_changed", byPath["response.id"].Change)
	assert.Equal(t, "missing", byPath["response.old"].Change)
	assert.Equal(t, "added", byPath["response.new"].Change)
	assert.Equal(t, "frequency_changed", byPath["response.freq"].Change)
}

func TestStructureDetailIncludesMatchedSafeDimensionsOnBothSides(t *testing.T) {
	profile := func(fields ...storedFieldProfile) string {
		encoded, err := common.Marshal(fields)
		require.NoError(t, err)
		return string(encoded)
	}
	metadata := func(value StructureMetadata) string {
		encoded, err := common.Marshal(value)
		require.NoError(t, err)
		return string(encoded)
	}
	sharedProfile := profile(storedFieldProfile{Path: "choices[].message.content", Type: "string"})
	sharedMetadata := metadata(StructureMetadata{Protocol: "json", HeaderPresence: map[string]bool{"x-request-id": true}})
	detail := BuildStructureSimilarityDetail(
		[]model.ChannelPuritySample{{StructureSignature: "same", StructureProfileJSON: sharedProfile, StructureMetadataJSON: sharedMetadata}},
		[]model.ChannelPuritySample{{StructureSignature: "same", StructureProfileJSON: sharedProfile, StructureMetadataJSON: sharedMetadata}},
	)
	assert.Equal(t, 1, detail.BaselineFieldProfileSamples)
	assert.Equal(t, 1, detail.TargetFieldProfileSamples)
	assert.Equal(t, 1, detail.BaselineMetadataSamples)
	assert.Equal(t, 1, detail.TargetMetadataSamples)
	assert.True(t, detail.FieldProfileCoverageComplete)
	assert.True(t, detail.MetadataCoverageComplete)
	assert.Equal(t, "sanitized_parameters_cover_all_paired_samples_values_never_retained", detail.Limitation)
	require.Len(t, detail.FieldDifferences, 1)
	assert.Equal(t, "matched", detail.FieldDifferences[0].Change)
	assert.Equal(t, 1, detail.FieldDifferences[0].BaselineCount)
	assert.Equal(t, 1, detail.FieldDifferences[0].TargetCount)
	byDimension := map[string]StructureDimensionDifference{}
	for _, difference := range detail.DimensionDifferences {
		byDimension[difference.Dimension+":"+difference.Value] = difference
	}
	assert.Equal(t, "matched", byDimension["protocol:json"].Change)
	assert.Equal(t, "matched", byDimension["header_presence:x-request-id"].Change)
}

func TestStructureDetailReportsPartialCoverageAcrossTheWholePairedWindow(t *testing.T) {
	profile, err := common.Marshal([]storedFieldProfile{{Path: "response.output[]", Type: "array"}})
	require.NoError(t, err)
	metadata, err := common.Marshal(StructureMetadata{Protocol: "sse", EventSequence: []string{"response.completed"}})
	require.NoError(t, err)
	detail := BuildStructureSimilarityDetail(
		[]model.ChannelPuritySample{
			{StructureSignature: "legacy-baseline"},
			{StructureSignature: "new-baseline", StructureProfileJSON: string(profile), StructureMetadataJSON: string(metadata)},
		},
		[]model.ChannelPuritySample{
			{StructureSignature: "legacy-target"},
			{StructureSignature: "new-target", StructureProfileJSON: string(profile), StructureMetadataJSON: string(metadata)},
		},
	)
	assert.Equal(t, 2, detail.PairedSampleCount)
	assert.Equal(t, 1, detail.BaselineFieldProfileSamples)
	assert.Equal(t, 1, detail.TargetFieldProfileSamples)
	assert.Equal(t, 1, detail.BaselineMetadataSamples)
	assert.Equal(t, 1, detail.TargetMetadataSamples)
	assert.False(t, detail.FieldProfileCoverageComplete)
	assert.False(t, detail.MetadataCoverageComplete)
	assert.True(t, detail.DetailAvailable)
	assert.Equal(t, "sanitized_parameters_cover_only_samples_collected_after_detail_upgrade", detail.Limitation)
}

func TestDecodeV2StructureDetailRestoresImplicitFullCoverage(t *testing.T) {
	run := &model.ChannelPurityPairRun{StructureSimilarityDetail: `{"version":"structure_similarity.v2","paired_sample_count":6,"field_paths_available":true,"detail_available":true,"score_available":true,"union_count":12}`}
	detail, err := DecodeStructureSimilarityDetail(run)
	require.NoError(t, err)
	require.NotNil(t, detail)
	assert.Equal(t, 6, detail.BaselineFieldProfileSamples)
	assert.Equal(t, 6, detail.TargetFieldProfileSamples)
	assert.Equal(t, 6, detail.BaselineMetadataSamples)
	assert.Equal(t, 6, detail.TargetMetadataSamples)
	assert.True(t, detail.FieldProfileCoverageComplete)
	assert.True(t, detail.MetadataCoverageComplete)
	assert.Equal(t, "sanitized_parameters_cover_all_paired_samples_values_never_retained", detail.Limitation)
}

func TestStructureDetailExplainsSensitiveLeafChanges(t *testing.T) {
	profile := func(fields ...storedFieldProfile) string {
		encoded, err := common.Marshal(fields)
		require.NoError(t, err)
		return string(encoded)
	}
	baseline := []model.ChannelPuritySample{{Valid: true, StructureSignature: "a", StructureProfileJSON: profile(
		storedFieldProfile{Path: "choices[].message.content", Type: "string"},
		storedFieldProfile{Path: "response.score", Type: "number"},
	)}}
	target := []model.ChannelPuritySample{{Valid: true, StructureSignature: "b", StructureProfileJSON: profile(
		storedFieldProfile{Path: "choices[].message.reasoning_content", Type: "string"},
		storedFieldProfile{Path: "response.score", Type: "string"},
	)}}
	detail := BuildStructureSimilarityDetail(baseline, target)
	byPath := map[string]FieldProfileDifference{}
	for _, difference := range detail.FieldDifferences {
		byPath[difference.Path] = difference
	}
	assert.Equal(t, "missing", byPath["choices[].message.content"].Change)
	assert.Equal(t, "added", byPath["choices[].message.reasoning_content"].Change)
	assert.Equal(t, "type_changed", byPath["response.score"].Change)
}

func TestStructureDetailExplainsProtocolEventHeaderAndFinishDimensions(t *testing.T) {
	metadata := func(value StructureMetadata) string {
		encoded, err := common.Marshal(value)
		require.NoError(t, err)
		return string(encoded)
	}
	baseline := []model.ChannelPuritySample{{Valid: true, StructureSignature: "a", StructureProfileJSON: `[]`, StructureMetadataJSON: metadata(StructureMetadata{
		Protocol: "json", StatusCode: 200, ModelFamily: "gpt-4o", EventSequence: []string{"response.created", "response.completed"},
		FinishReasons: []string{"stop"}, HeaderPresence: map[string]bool{"x-request-id": true},
	})}}
	target := []model.ChannelPuritySample{{Valid: true, StructureSignature: "b", StructureProfileJSON: `[]`, StructureMetadataJSON: metadata(StructureMetadata{
		Protocol: "sse", StatusCode: 201, ModelFamily: "gpt-5", EventSequence: []string{"response.completed", "response.created"},
		FinishReasons: []string{"length"}, HeaderPresence: map[string]bool{"x-reasoning-included": true},
	})}}
	detail := BuildStructureSimilarityDetail(baseline, target)
	keys := map[string]string{}
	for _, difference := range detail.DimensionDifferences {
		keys[difference.Dimension+":"+difference.Value] = difference.Change
	}
	assert.Equal(t, "missing", keys["protocol:json"])
	assert.Equal(t, "added", keys["protocol:sse"])
	assert.Equal(t, "missing", keys["status_code:200"])
	assert.Equal(t, "added", keys["status_code:201"])
	assert.Equal(t, "missing", keys["model_family:gpt-4o"])
	assert.Equal(t, "added", keys["model_family:gpt-5"])
	assert.Equal(t, "missing", keys["event_sequence:response.created → response.completed"])
	assert.Equal(t, "added", keys["event_sequence:response.completed → response.created"])
	assert.Equal(t, "missing", keys["finish_reason:stop"])
	assert.Equal(t, "added", keys["header_presence:x-reasoning-included"])
}

func TestStructureDetailMarksLegacyAndInsufficientEvidenceUnavailable(t *testing.T) {
	legacy := BuildStructureSimilarityDetail(
		[]model.ChannelPuritySample{{Valid: true, StructureSignature: "a"}},
		[]model.ChannelPuritySample{{Valid: true, StructureSignature: "b"}},
	)
	assert.False(t, legacy.DetailAvailable)
	assert.True(t, legacy.ScoreAvailable)
	assert.Equal(t, "detail_unavailable_for_legacy_anonymous_samples", legacy.Limitation)
	insufficient := BuildStructureSimilarityDetail(nil, nil)
	assert.False(t, insufficient.ScoreAvailable)
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
	_, err := AggregatePairWindow(group.ID, 20, "m1", "m1", 90, 120, policy)
	require.NoError(t, err)
	var run model.ChannelPurityPairRun
	require.NoError(t, model.DB.Last(&run).Error)
	assert.Equal(t, 10, run.PairedSampleCount)
	assert.InDelta(t, 0.3, run.TokenDeviationRate, 0.001)
	assert.Less(t, run.TokenSimilarity, 0.2, "repeated robust-interval outliers must penalize the formal score")
	assert.Contains(t, run.AnomalyEvidenceJSON, "token_interval_shift", "repeated robust-interval outliers must enter formal evidence")
}

func TestAggregatePairWindowSupportsDifferentBaselineAndTargetModels(t *testing.T) {
	setupPurityDB(t)
	group := createTestPurityGroup(t)
	addPuritySample(t, group.ID, 10, "baseline-model", "cross-model", "same", 100, 100)
	addPuritySample(t, group.ID, 20, "target-model", "cross-model", "same", 105, 101)

	assessment, err := AggregatePairWindow(group.ID, 20, "baseline-model", "target-model", 90, 120, DefaultAggregatePolicy())
	require.NoError(t, err)
	assert.Equal(t, ModelComparisonKey("baseline-model", "target-model"), assessment.ActualModel)
	var run model.ChannelPurityPairRun
	require.NoError(t, model.DB.Last(&run).Error)
	assert.Equal(t, "baseline-model", run.BaselineModel)
	assert.Equal(t, "target-model", run.TargetModel)
	assert.Equal(t, 1, run.PairedSampleCount)
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

	count, err := CountValidPairedSamples(group.ID, 10, 20, "m1", "m1", 90, 120)
	require.NoError(t, err)
	assert.Equal(t, int64(2), count)
	otherTarget, err := CountValidPairedSamples(group.ID, 10, 30, "m1", "m1", 90, 120)
	require.NoError(t, err)
	assert.Equal(t, int64(1), otherTarget)
	otherModel, err := CountValidPairedSamples(group.ID, 10, 20, "m2", "m2", 90, 120)
	require.NoError(t, err)
	assert.Equal(t, int64(1), otherModel)
}
