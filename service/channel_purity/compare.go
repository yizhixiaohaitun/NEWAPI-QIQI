package channel_purity

import (
	"errors"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"gorm.io/gorm"
)

// AggregatePairWindow persists one independent group+target+actual-model result.
// It deliberately never reads quick-probe tables and never produces a group score.
func AggregatePairWindow(groupID uint, targetChannelID int, actualModel string, windowStart, windowEnd int64, policy AggregatePolicy) (*model.ChannelPurityAssessment, error) {
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
	if !targetMember {
		return nil, errors.New("target channel is not a non-baseline member of group")
	}
	var baseline, target []model.ChannelPuritySample
	query := func(channelID int, into *[]model.ChannelPuritySample) error {
		return model.DB.Where("group_id = ? AND channel_id = ? AND actual_model = ? AND observed_at >= ? AND observed_at < ?", groupID, channelID, actualModel, windowStart, windowEnd).Order("observed_at asc").Find(into).Error
	}
	if baselineID != 0 {
		if err = query(baselineID, &baseline); err != nil {
			return nil, err
		}
	}
	if err = query(targetChannelID, &target); err != nil {
		return nil, err
	}
	structure, token, confidence, evidence := CompareSamples(baseline, target)
	combined := structure*.65 + token*.35
	window := WindowResult{BaselineAvailable: baselineID != 0 && len(baseline) > 0, BaselineSamples: len(baseline), TargetSamples: len(target), Similarity: combined, Confidence: confidence}
	var previous model.ChannelPurityAssessment
	err = model.DB.Where("group_id = ? AND target_channel_id = ? AND actual_model = ?", groupID, targetChannelID, actualModel).First(&previous).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	next := Advance(previous, window, policy)
	now := time.Now().Unix()
	if next.FirstSeenAt == 0 {
		next.FirstSeenAt = now
	}
	next.GroupID = groupID
	next.TargetChannelID = targetChannelID
	next.ActualModel = actualModel
	next.UpdatedAt = now
	evidenceJSON := "[]"
	if encoded, e := common.Marshal(evidence); e == nil {
		evidenceJSON = string(encoded)
	}
	baselineMin, baselineMax := tokenBounds(baseline)
	targetMin, targetMax := tokenBounds(target)
	pairedCount := pairedSampleCount(baseline, target)
	deviationRate := tokenDeviationRate(baselineMin, baselineMax, target)
	run := model.ChannelPurityPairRun{
		GroupID: groupID, BaselineChannelID: baselineID, TargetChannelID: targetChannelID, ActualModel: actualModel,
		WindowStartedAt: windowStart, WindowEndedAt: windowEnd, BaselineSampleCount: len(baseline), TargetSampleCount: len(target),
		PairedSampleCount: pairedCount, StructureSimilarity: structure, TokenSimilarity: token,
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

func tokenBounds(samples []model.ChannelPuritySample) (int, int) {
	minimum, maximum := 0, 0
	for _, sample := range samples {
		if !sample.Valid || sample.TotalTokens < 0 {
			continue
		}
		if minimum == 0 || sample.TotalTokens < minimum {
			minimum = sample.TotalTokens
		}
		if sample.TotalTokens > maximum {
			maximum = sample.TotalTokens
		}
	}
	return minimum, maximum
}

func pairedSampleCount(baseline, target []model.ChannelPuritySample) int {
	baselineKeys := map[string]int{}
	for _, sample := range baseline {
		if sample.Valid && sample.RunKey != "" {
			baselineKeys[sample.RunKey]++
		}
	}
	count := 0
	for _, sample := range target {
		if sample.Valid && baselineKeys[sample.RunKey] > 0 {
			count++
			baselineKeys[sample.RunKey]--
		}
	}
	return count
}

func tokenDeviationRate(minimum, maximum int, target []model.ChannelPuritySample) float64 {
	if len(target) == 0 || maximum < minimum {
		return 0
	}
	valid, outside := 0, 0
	for _, sample := range target {
		if !sample.Valid {
			continue
		}
		valid++
		if sample.TotalTokens < minimum || sample.TotalTokens > maximum {
			outside++
		}
	}
	if valid == 0 {
		return 0
	}
	return float64(outside) / float64(valid)
}
