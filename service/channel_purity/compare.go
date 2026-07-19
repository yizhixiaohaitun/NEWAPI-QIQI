package channel_purity

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"math"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"gorm.io/gorm"
)

func ModelComparisonKey(baselineModel, targetModel string) string {
	baselineModel, targetModel = strings.TrimSpace(baselineModel), strings.TrimSpace(targetModel)
	display := baselineModel + " → " + targetModel
	if len(display) <= 240 {
		return display
	}
	sum := sha256.Sum256([]byte(display))
	return display[:220] + "#" + hex.EncodeToString(sum[:8])
}

// AggregatePairWindow persists one independent group+target+model-comparison result.
// Only valid baseline/target rows matched one-to-one by the same non-empty RunKey
// participate in any formal statistic.
func AggregatePairWindow(groupID uint, targetChannelID int, baselineModel, targetModel string, windowStart, windowEnd int64, policy AggregatePolicy) (*model.ChannelPurityAssessment, error) {
	comparisonKey := ModelComparisonKey(baselineModel, targetModel)
	group, err := model.GetPurityGroup(groupID)
	if err != nil {
		return nil, err
	}
	baselineID, targetMember := 0, false
	for _, member := range group.Members {
		if member.IsBaseline {
			baselineID = member.ChannelID
		}
		if member.ChannelID == targetChannelID && !member.IsBaseline {
			targetMember = true
		}
	}
	if baselineID == 0 || !targetMember {
		return nil, errors.New("target channel is not a non-baseline member of group")
	}
	var baselineRows, targetRows []model.ChannelPuritySample
	query := func(channelID int, modelName string, into *[]model.ChannelPuritySample) error {
		return model.DB.Where("group_id = ? AND channel_id = ? AND actual_model = ? AND observed_at >= ? AND observed_at < ?", groupID, channelID, modelName, windowStart, windowEnd).Order("observed_at asc, id asc").Find(into).Error
	}
	if err = query(baselineID, baselineModel, &baselineRows); err != nil {
		return nil, err
	}
	if err = query(targetChannelID, targetModel, &targetRows); err != nil {
		return nil, err
	}
	baseline, target := matchValidSamples(baselineRows, targetRows)
	structure, token, confidence, evidence := CompareSamples(baseline, target, policy.MinSamples)
	combined := structure*.65 + token*.35
	window := WindowResult{BaselineAvailable: len(baseline) > 0, BaselineSamples: len(baseline), TargetSamples: len(target), Similarity: combined, Confidence: confidence}
	var previous model.ChannelPurityAssessment
	err = model.DB.Where("group_id = ? AND target_channel_id = ? AND actual_model = ?", groupID, targetChannelID, comparisonKey).First(&previous).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	next := Advance(previous, window, policy)
	now := time.Now().Unix()
	if next.FirstSeenAt == 0 {
		next.FirstSeenAt = now
	}
	next.GroupID, next.TargetChannelID, next.ActualModel, next.UpdatedAt = groupID, targetChannelID, comparisonKey, now
	evidenceJSON := "[]"
	if encoded, e := common.Marshal(evidence); e == nil {
		evidenceJSON = string(encoded)
	}
	baselineMin, baselineMax := tokenBounds(baseline)
	targetMin, targetMax := tokenBounds(target)
	deviationRate := pairedTokenDeviationRate(baseline, target, policy.MinSamples)
	run := model.ChannelPurityPairRun{
		GroupID: groupID, BaselineChannelID: baselineID, TargetChannelID: targetChannelID, ActualModel: comparisonKey,
		BaselineModel: baselineModel, TargetModel: targetModel,
		WindowStartedAt: windowStart, WindowEndedAt: windowEnd, BaselineSampleCount: len(baseline), TargetSampleCount: len(target),
		PairedSampleCount: len(baseline), StructureSimilarity: structure, TokenSimilarity: token,
		BaselineTokenMin: baselineMin, BaselineTokenMax: baselineMax, TargetTokenMin: targetMin, TargetTokenMax: targetMax,
		TokenDeviationRate: deviationRate, AnomalyEvidenceJSON: evidenceJSON, Confidence: confidence, State: next.State, CreatedAt: now,
	}
	err = model.DB.Transaction(func(tx *gorm.DB) error {
		if e := tx.Create(&run).Error; e != nil {
			return e
		}
		next.LatestPairRunID = run.ID
		if next.ID == 0 {
			if e := tx.Create(&next).Error; e != nil {
				return e
			}
		} else if e := tx.Save(&next).Error; e != nil {
			return e
		}
		open, resolve := AlertTransition(previous.State, next.State)
		if open {
			return tx.Create(&model.ChannelPurityAlert{AssessmentID: next.ID, PairRunID: run.ID, Status: AlertStatusOpen, EvidenceJSON: evidenceJSON, OpenedAt: now}).Error
		}
		if resolve {
			return tx.Model(&model.ChannelPurityAlert{}).Where("assessment_id = ? AND status = ?", next.ID, AlertStatusOpen).Updates(map[string]any{"status": AlertStatusResolved, "resolved_at": now}).Error
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &next, nil
}

func matchValidSamples(baseline, target []model.ChannelPuritySample) ([]model.ChannelPuritySample, []model.ChannelPuritySample) {
	byKey := make(map[string][]model.ChannelPuritySample)
	for _, sample := range baseline {
		if sample.Valid && sample.RunKey != "" {
			byKey[sample.RunKey] = append(byKey[sample.RunKey], sample)
		}
	}
	matchedBaseline := make([]model.ChannelPuritySample, 0)
	matchedTarget := make([]model.ChannelPuritySample, 0)
	for _, sample := range target {
		candidates := byKey[sample.RunKey]
		if !sample.Valid || sample.RunKey == "" || len(candidates) == 0 {
			continue
		}
		matchedBaseline = append(matchedBaseline, candidates[0])
		matchedTarget = append(matchedTarget, sample)
		byKey[sample.RunKey] = candidates[1:]
	}
	return matchedBaseline, matchedTarget
}

// CountValidPairedSamples returns the existing formal pair count for one isolated window quota.
func CountValidPairedSamples(groupID uint, baselineChannelID, targetChannelID int, baselineModel, targetModel string, windowStart, windowEnd int64) (int64, error) {
	var baseline, target []model.ChannelPuritySample
	load := func(channelID int, modelName string, into *[]model.ChannelPuritySample) error {
		return model.DB.Where("group_id = ? AND channel_id = ? AND actual_model = ? AND observed_at >= ? AND observed_at < ?", groupID, channelID, modelName, windowStart, windowEnd).Order("observed_at asc, id asc").Find(into).Error
	}
	if err := load(baselineChannelID, baselineModel, &baseline); err != nil {
		return 0, err
	}
	if err := load(targetChannelID, targetModel, &target); err != nil {
		return 0, err
	}
	paired, _ := matchValidSamples(baseline, target)
	return int64(len(paired)), nil
}

func tokenBounds(samples []model.ChannelPuritySample) (int, int) {
	minimum, maximum, initialized := 0, 0, false
	for _, sample := range samples {
		if !sample.Valid || sample.TotalTokens < 0 {
			continue
		}
		if !initialized || sample.TotalTokens < minimum {
			minimum = sample.TotalTokens
		}
		if !initialized || sample.TotalTokens > maximum {
			maximum = sample.TotalTokens
		}
		initialized = true
	}
	return minimum, maximum
}

func pairedTokenDeviationRate(baseline, target []model.ChannelPuritySample, minSamples int) float64 {
	pairs := make([]TokenPair, 0, len(baseline))
	for i := range baseline {
		pair, ok := NewTokenPair(baseline[i].ActualModel, "purity_probe", target[i].TotalTokens, baseline[i].TotalTokens, target[i].TotalTokens, baseline[i].TotalTokens)
		if ok {
			pairs = append(pairs, pair)
		}
	}
	if len(pairs) == 0 {
		return 0
	}
	interval := AnalyzeTokenRatios(pairs[0].ModelFamily, "purity_probe", pairs, minSamples)
	outside := 0
	for _, pair := range pairs {
		if pair.Ratio < interval.Lower || pair.Ratio > interval.Upper {
			outside++
		}
	}
	return math.Min(1, float64(outside)/float64(len(pairs)))
}
