package channel_purity

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
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
	baselineInvalid, baselineUnmatched := sampleDiagnostics(baselineRows, len(baseline))
	targetInvalid, targetUnmatched := sampleDiagnostics(targetRows, len(target))
	structure, token, confidence, evidence := CompareSamples(baseline, target, policy.MinSamples)
	structureDetail := BuildStructureSimilarityDetail(baseline, target)
	tokenDetail := BuildTokenSimilarityDetail(baseline, target, policy.MinSamples)
	combined, _ := combinedSimilarity(structure, structureDetail.ScoreAvailable, token, tokenDetail.ScoreAvailable)
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
	deviationRate := tokenDetail.DeviationRate
	structureDetail.Version = StructureSimilarityDetailVersion
	structureDetail.WindowStartedAt = windowStart
	structureDetail.WindowEndedAt = windowEnd
	structureDetail.PairedSampleCount = len(baseline)
	encodedStructureDetail, err := common.Marshal(structureDetail)
	if err != nil {
		return nil, err
	}
	encodedTokenDetail, err := common.Marshal(tokenDetail)
	if err != nil {
		return nil, err
	}
	run := model.ChannelPurityPairRun{
		GroupID: groupID, BaselineChannelID: baselineID, TargetChannelID: targetChannelID, ActualModel: comparisonKey,
		BaselineModel: baselineModel, TargetModel: targetModel,
		WindowStartedAt: windowStart, WindowEndedAt: windowEnd, BaselineSampleCount: len(baseline), TargetSampleCount: len(target),
		PairedSampleCount: len(baseline), BaselineInvalidCount: baselineInvalid, TargetInvalidCount: targetInvalid,
		UnmatchedBaselineCount: baselineUnmatched, UnmatchedTargetCount: targetUnmatched, StructureSimilarity: structure, StructureSimilarityDetail: string(encodedStructureDetail), TokenSimilarity: token, TokenSimilarityDetail: string(encodedTokenDetail),
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
			return tx.Create(&model.ChannelPurityAlert{AssessmentID: next.ID, PairRunID: run.ID, Status: AlertStatusOpen, EvidenceJSON: evidenceJSON, OpenedAt: now, UpdatedAt: now}).Error
		}
		if resolve {
			return tx.Model(&model.ChannelPurityAlert{}).Where("assessment_id = ? AND status IN ?", next.ID, []string{AlertStatusOpen, "ACKNOWLEDGED", "SILENCED"}).Updates(map[string]any{"status": AlertStatusResolved, "resolved_at": now, "updated_at": now}).Error
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	// Keep 100 windows per target/model bucket by default (over eight hours at
	// the minimum five-minute interval) while preventing unbounded growth.
	if err := model.PrunePurityGroupHistory(groupID, group.RetentionWindows); err != nil {
		return nil, err
	}
	return &next, nil
}

type StructureDifference struct {
	Signature     string `json:"signature"`
	BaselineCount int    `json:"baseline_count"`
	TargetCount   int    `json:"target_count"`
	MatchedCount  int    `json:"matched_count"`
}
type FieldProfileDifference struct {
	Path          string   `json:"path"`
	Change        string   `json:"change"`
	BaselineTypes []string `json:"baseline_types,omitempty"`
	TargetTypes   []string `json:"target_types,omitempty"`
	BaselineCount int      `json:"baseline_count"`
	TargetCount   int      `json:"target_count"`
}
type storedFieldProfile struct {
	Path string `json:"path"`
	Type string `json:"type"`
}
type StructureDimensionDifference struct {
	Dimension     string `json:"dimension"`
	Value         string `json:"value"`
	Change        string `json:"change"`
	BaselineCount int    `json:"baseline_count"`
	TargetCount   int    `json:"target_count"`
}

const StructureSimilarityDetailVersion = "structure_similarity.v3"

type StructureSimilarityDetail struct {
	Version                      string                         `json:"version"`
	Method                       string                         `json:"method"`
	WindowStartedAt              int64                          `json:"window_started_at"`
	WindowEndedAt                int64                          `json:"window_ended_at"`
	PairedSampleCount            int                            `json:"paired_sample_count"`
	MatchedCount                 int                            `json:"matched_count"`
	BaselineOnlyCount            int                            `json:"baseline_only_count"`
	TargetOnlyCount              int                            `json:"target_only_count"`
	IntersectionCount            int                            `json:"intersection_count"`
	UnionCount                   int                            `json:"union_count"`
	Differences                  []StructureDifference          `json:"differences"`
	FieldDifferences             []FieldProfileDifference       `json:"field_differences,omitempty"`
	DimensionDifferences         []StructureDimensionDifference `json:"dimension_differences,omitempty"`
	BaselineFieldProfileSamples  int                            `json:"baseline_field_profile_samples"`
	TargetFieldProfileSamples    int                            `json:"target_field_profile_samples"`
	BaselineMetadataSamples      int                            `json:"baseline_metadata_samples"`
	TargetMetadataSamples        int                            `json:"target_metadata_samples"`
	FieldPathsAvailable          bool                           `json:"field_paths_available"`
	FieldProfileCoverageComplete bool                           `json:"field_profile_coverage_complete"`
	MetadataCoverageComplete     bool                           `json:"metadata_coverage_complete"`
	DetailAvailable              bool                           `json:"detail_available"`
	ScoreAvailable               bool                           `json:"score_available"`
	Limitation                   string                         `json:"limitation"`
}

// BuildStructureSimilarityDetail uses the same already-paired samples as CompareSamples.
func BuildStructureSimilarityDetail(baseline, target []model.ChannelPuritySample) StructureSimilarityDetail {
	bf, tf := signatureFrequency(baseline), signatureFrequency(target)
	keys := make(map[string]bool, len(bf)+len(tf))
	for key := range bf {
		keys[key] = true
	}
	for key := range tf {
		keys[key] = true
	}
	detail := StructureSimilarityDetail{
		Version: StructureSimilarityDetailVersion, Method: "multiset_jaccard",
		PairedSampleCount: len(baseline), Differences: make([]StructureDifference, 0, len(keys)),
		FieldPathsAvailable: false,
		DetailAvailable:     false,
		ScoreAvailable:      len(baseline) > 0 && len(baseline) == len(target),
		Limitation:          "detail_unavailable_for_legacy_anonymous_samples",
	}
	detail.BaselineFieldProfileSamples = samplesWithPayload(baseline, func(sample model.ChannelPuritySample) string { return sample.StructureProfileJSON })
	detail.TargetFieldProfileSamples = samplesWithPayload(target, func(sample model.ChannelPuritySample) string { return sample.StructureProfileJSON })
	detail.BaselineMetadataSamples = samplesWithPayload(baseline, func(sample model.ChannelPuritySample) string { return sample.StructureMetadataJSON })
	detail.TargetMetadataSamples = samplesWithPayload(target, func(sample model.ChannelPuritySample) string { return sample.StructureMetadataJSON })
	detail.FieldDifferences = buildFieldProfileDifferences(baseline, target)
	detail.DimensionDifferences = buildStructureDimensionDifferences(baseline, target)
	detail.FieldPathsAvailable = detail.BaselineFieldProfileSamples > 0 && detail.TargetFieldProfileSamples > 0
	detail.FieldProfileCoverageComplete = detail.FieldPathsAvailable && detail.BaselineFieldProfileSamples == len(baseline) && detail.TargetFieldProfileSamples == len(target)
	detail.MetadataCoverageComplete = detail.BaselineMetadataSamples == len(baseline) && detail.TargetMetadataSamples == len(target)
	detail.DetailAvailable = len(detail.FieldDifferences) > 0 || len(detail.DimensionDifferences) > 0
	switch {
	case detail.FieldProfileCoverageComplete && detail.MetadataCoverageComplete:
		detail.Limitation = "sanitized_parameters_cover_all_paired_samples_values_never_retained"
	case detail.DetailAvailable:
		detail.Limitation = "sanitized_parameters_cover_only_samples_collected_after_detail_upgrade"
	default:
		detail.Limitation = "detail_unavailable_for_legacy_anonymous_samples"
	}
	for key := range keys {
		matched := minInt(bf[key], tf[key])
		baseCount, targetCount := bf[key], tf[key]
		detail.IntersectionCount += matched
		detail.UnionCount += maxInt(baseCount, targetCount)
		detail.MatchedCount += matched
		if baseCount > targetCount {
			detail.BaselineOnlyCount += baseCount - targetCount
		}
		if targetCount > baseCount {
			detail.TargetOnlyCount += targetCount - baseCount
		}
		detail.Differences = append(detail.Differences, StructureDifference{Signature: key, BaselineCount: baseCount, TargetCount: targetCount, MatchedCount: matched})
	}
	sort.Slice(detail.Differences, func(i, j int) bool { return detail.Differences[i].Signature < detail.Differences[j].Signature })
	return detail
}

func samplesWithPayload(samples []model.ChannelPuritySample, value func(model.ChannelPuritySample) string) int {
	count := 0
	for _, sample := range samples {
		if strings.TrimSpace(value(sample)) != "" {
			count++
		}
	}
	return count
}

func buildFieldProfileDifferences(baseline, target []model.ChannelPuritySample) []FieldProfileDifference {
	type counts struct {
		baseline, target int
		baselineTypes    map[string]bool
		targetTypes      map[string]bool
	}
	values := map[string]*counts{}
	consume := func(samples []model.ChannelPuritySample, isBaseline bool) {
		for _, sample := range samples {
			var fields []storedFieldProfile
			if common.Unmarshal([]byte(sample.StructureProfileJSON), &fields) != nil {
				continue
			}
			seenPaths, seenTypes := map[string]bool{}, map[string]bool{}
			for _, field := range fields {
				field.Path, field.Type = strings.TrimSpace(field.Path), strings.TrimSpace(field.Type)
				key := field.Path + "\x00" + field.Type
				if field.Path == "" || field.Type == "" || seenTypes[key] {
					continue
				}
				seenTypes[key] = true
				value := values[field.Path]
				if value == nil {
					value = &counts{baselineTypes: map[string]bool{}, targetTypes: map[string]bool{}}
					values[field.Path] = value
				}
				if isBaseline {
					if !seenPaths[field.Path] {
						value.baseline++
					}
					value.baselineTypes[field.Type] = true
				} else {
					if !seenPaths[field.Path] {
						value.target++
					}
					value.targetTypes[field.Type] = true
				}
				seenPaths[field.Path] = true
			}
		}
	}
	consume(baseline, true)
	consume(target, false)
	out := make([]FieldProfileDifference, 0, len(values))
	for path, value := range values {
		baselineTypes, targetTypes := sortedSet(value.baselineTypes), sortedSet(value.targetTypes)
		change := "frequency_changed"
		switch {
		case value.baseline == 0:
			change = "added"
		case value.target == 0:
			change = "missing"
		case strings.Join(baselineTypes, "\x00") != strings.Join(targetTypes, "\x00"):
			change = "type_changed"
		case value.baseline == value.target:
			change = "matched"
		}
		out = append(out, FieldProfileDifference{Path: path, Change: change, BaselineTypes: baselineTypes, TargetTypes: targetTypes, BaselineCount: value.baseline, TargetCount: value.target})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

func buildStructureDimensionDifferences(baseline, target []model.ChannelPuritySample) []StructureDimensionDifference {
	counts := map[string][2]int{}
	consume := func(samples []model.ChannelPuritySample, side int) {
		for _, sample := range samples {
			var metadata StructureMetadata
			if common.Unmarshal([]byte(sample.StructureMetadataJSON), &metadata) != nil {
				continue
			}
			values := map[string][]string{
				"protocol": {metadata.Protocol}, "model_family": {metadata.ModelFamily},
				"finish_reason": metadata.FinishReasons,
			}
			if metadata.StatusCode > 0 {
				values["status_code"] = []string{fmt.Sprintf("%d", metadata.StatusCode)}
			}
			if len(metadata.EventSequence) > 0 {
				values["event_sequence"] = []string{strings.Join(metadata.EventSequence, " → ")}
			}
			for name, present := range metadata.HeaderPresence {
				if present {
					values["header_presence"] = append(values["header_presence"], name)
				}
			}
			if metadata.HasSignatureID {
				values["metadata"] = append(values["metadata"], "signature_id_present")
			}
			for dimension, entries := range values {
				seen := map[string]bool{}
				for _, value := range entries {
					value = strings.TrimSpace(value)
					if value == "" || seen[value] {
						continue
					}
					seen[value] = true
					key := dimension + "\x00" + value
					count := counts[key]
					count[side]++
					counts[key] = count
				}
			}
		}
	}
	consume(baseline, 0)
	consume(target, 1)
	out := make([]StructureDimensionDifference, 0, len(counts))
	for key, count := range counts {
		parts := strings.SplitN(key, "\x00", 2)
		change := "frequency_changed"
		if count[0] == 0 {
			change = "added"
		} else if count[1] == 0 {
			change = "missing"
		} else if count[0] == count[1] {
			change = "matched"
		}
		out = append(out, StructureDimensionDifference{Dimension: parts[0], Value: parts[1], Change: change, BaselineCount: count[0], TargetCount: count[1]})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Dimension == out[j].Dimension {
			return out[i].Value < out[j].Value
		}
		return out[i].Dimension < out[j].Dimension
	})
	return out
}

func sortedSet(values map[string]bool) []string {
	out := make([]string, 0, len(values))
	for value := range values {
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func DecodeStructureSimilarityDetail(run *model.ChannelPurityPairRun) (*StructureSimilarityDetail, error) {
	if strings.TrimSpace(run.StructureSimilarityDetail) == "" {
		return nil, nil
	}
	var detail StructureSimilarityDetail
	if err := common.Unmarshal([]byte(run.StructureSimilarityDetail), &detail); err != nil {
		return nil, err
	}
	// v1 predates the explicit availability flag. A non-empty persisted scoring
	// input remains a valid score, while field-level evidence stays unavailable.
	if detail.Version == "structure_similarity.v1" && (detail.UnionCount > 0 || detail.PairedSampleCount > 0) {
		detail.ScoreAvailable = true
		if !detail.FieldPathsAvailable {
			detail.DetailAvailable = false
			detail.Limitation = "detail_unavailable_for_legacy_anonymous_samples"
		}
	}
	// v2 introduced sanitized field and protocol details, and only marked them
	// available when every paired sample carried the new payload. Restore those
	// implicit coverage facts so historical v2 runs are not shown as 0 / N.
	if detail.Version == "structure_similarity.v2" && detail.FieldPathsAvailable {
		detail.BaselineFieldProfileSamples = detail.PairedSampleCount
		detail.TargetFieldProfileSamples = detail.PairedSampleCount
		detail.BaselineMetadataSamples = detail.PairedSampleCount
		detail.TargetMetadataSamples = detail.PairedSampleCount
		detail.FieldProfileCoverageComplete = true
		detail.MetadataCoverageComplete = true
		detail.Limitation = "sanitized_parameters_cover_all_paired_samples_values_never_retained"
	}
	return &detail, nil
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

func sampleDiagnostics(samples []model.ChannelPuritySample, paired int) (invalid, unmatched int) {
	valid := 0
	for _, sample := range samples {
		if !sample.Valid || strings.TrimSpace(sample.RunKey) == "" {
			invalid++
		} else {
			valid++
		}
	}
	if valid > paired {
		unmatched = valid - paired
	}
	return
}

func tokenBounds(samples []model.ChannelPuritySample) (int, int) {
	minimum, maximum, initialized := 0, 0, false
	for _, sample := range samples {
		if !sample.Valid || sample.TotalTokens <= 0 {
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

const TokenSimilarityDetailVersion = "token_similarity.v1"

type TokenPairDetail struct {
	BaselineTokens int     `json:"baseline_tokens"`
	TargetTokens   int     `json:"target_tokens"`
	Ratio          float64 `json:"ratio"`
	Outside        bool    `json:"outside"`
}

type TokenSimilarityDetail struct {
	Version              string            `json:"version"`
	BaselineValidSamples int               `json:"baseline_valid_samples"`
	TargetValidSamples   int               `json:"target_valid_samples"`
	PairedCount          int               `json:"paired_count"`
	BaselineMin          int               `json:"baseline_min"`
	BaselineMax          int               `json:"baseline_max"`
	BaselineP50          float64           `json:"baseline_p50"`
	BaselineP95          float64           `json:"baseline_p95"`
	TargetMin            int               `json:"target_min"`
	TargetMax            int               `json:"target_max"`
	TargetP50            float64           `json:"target_p50"`
	TargetP95            float64           `json:"target_p95"`
	RatioMedian          float64           `json:"ratio_median"`
	Q1                   float64           `json:"q1"`
	Q3                   float64           `json:"q3"`
	MAD                  float64           `json:"mad"`
	RobustLower          float64           `json:"robust_lower"`
	RobustUpper          float64           `json:"robust_upper"`
	OutsideCount         int               `json:"outside_count"`
	DeviationRate        float64           `json:"deviation_rate"`
	ScoreAvailable       bool              `json:"score_available"`
	Pairs                []TokenPairDetail `json:"pairs,omitempty"`
}

func BuildTokenSimilarityDetail(baseline, target []model.ChannelPuritySample, minSamples int) TokenSimilarityDetail {
	detail := TokenSimilarityDetail{Version: TokenSimilarityDetailVersion, Pairs: []TokenPairDetail{}}
	baselineValues, targetValues := make([]float64, 0, len(baseline)), make([]float64, 0, len(target))
	pairs := make([]TokenPair, 0, minInt(len(baseline), len(target)))
	for _, sample := range baseline {
		if sample.Valid && sample.TotalTokens > 0 {
			detail.BaselineValidSamples++
			baselineValues = append(baselineValues, float64(sample.TotalTokens))
		}
	}
	for _, sample := range target {
		if sample.Valid && sample.TotalTokens > 0 {
			detail.TargetValidSamples++
			targetValues = append(targetValues, float64(sample.TotalTokens))
		}
	}
	for i := 0; i < len(baseline) && i < len(target); i++ {
		pair, ok := NewTokenPair(actualModelFamily(baseline[i], target[i]), "purity_probe", target[i].TotalTokens, baseline[i].TotalTokens, target[i].TotalTokens, baseline[i].TotalTokens)
		if ok {
			pairs = append(pairs, pair)
		}
	}
	detail.PairedCount = len(pairs)
	if len(baselineValues) > 0 {
		sort.Float64s(baselineValues)
		detail.BaselineMin, detail.BaselineMax = int(baselineValues[0]), int(baselineValues[len(baselineValues)-1])
		detail.BaselineP50, detail.BaselineP95 = quantile(baselineValues, .5), quantile(baselineValues, .95)
	}
	if len(targetValues) > 0 {
		sort.Float64s(targetValues)
		detail.TargetMin, detail.TargetMax = int(targetValues[0]), int(targetValues[len(targetValues)-1])
		detail.TargetP50, detail.TargetP95 = quantile(targetValues, .5), quantile(targetValues, .95)
	}
	if len(pairs) == 0 {
		return detail
	}
	interval := AnalyzeTokenRatios(pairs[0].ModelFamily, "purity_probe", pairs, minSamples)
	detail.ScoreAvailable = true
	detail.RatioMedian, detail.Q1, detail.Q3, detail.MAD = interval.Median, interval.Q1, interval.Q3, interval.MAD
	detail.RobustLower, detail.RobustUpper = interval.Lower, interval.Upper
	for _, pair := range pairs {
		outside := pair.Ratio < interval.Lower || pair.Ratio > interval.Upper
		if outside {
			detail.OutsideCount++
		}
		detail.Pairs = append(detail.Pairs, TokenPairDetail{BaselineTokens: pair.BaselineUnified, TargetTokens: pair.TargetUnified, Ratio: pair.Ratio, Outside: outside})
	}
	detail.DeviationRate = float64(detail.OutsideCount) / float64(detail.PairedCount)
	return detail
}

func combinedSimilarity(structure float64, structureAvailable bool, token float64, tokenAvailable bool) (float64, bool) {
	switch {
	case structureAvailable && tokenAvailable:
		return structure*.65 + token*.35, true
	case structureAvailable:
		return structure, true
	case tokenAvailable:
		return token, true
	default:
		return 0, false
	}
}

func DecodeTokenSimilarityDetail(run *model.ChannelPurityPairRun) (*TokenSimilarityDetail, error) {
	if strings.TrimSpace(run.TokenSimilarityDetail) == "" {
		return nil, nil
	}
	var detail TokenSimilarityDetail
	if err := common.Unmarshal([]byte(run.TokenSimilarityDetail), &detail); err != nil {
		return nil, err
	}
	if detail.Version == "" {
		return nil, errors.New("unknown token similarity detail")
	}
	return &detail, nil
}
