package model

import (
	"errors"
	"strings"

	"gorm.io/gorm"
)

const (
	ChannelPurityStateBaselineUnavailable = "BASELINE_UNAVAILABLE"
	ChannelPurityStateLowSample           = "LOW_SAMPLE"
	ChannelPurityStateNoTraffic           = "NO_TRAFFIC"
	ChannelPurityStateWarmingUp           = "WARMING_UP"
	ChannelPurityStateHealthy             = "HEALTHY"
	ChannelPurityStateSuspect             = "SUSPECT"
	ChannelPurityStateAlert               = "ALERT"
	ChannelPurityStateDetectorError       = "DETECTOR_ERROR"
)

// ChannelPurityGroup is the isolation boundary for baseline comparison.
type ChannelPurityGroup struct {
	ID                   uint                           `json:"id" gorm:"primaryKey"`
	Name                 string                         `json:"name" gorm:"type:varchar(255);not null;uniqueIndex"`
	Enabled              bool                           `json:"enabled" gorm:"not null;default:true"`
	IntervalMinutes      int                            `json:"interval_minutes" gorm:"not null;default:5"`
	RandomPairingEnabled bool                           `json:"random_pairing_enabled" gorm:"not null;default:false"`
	WindowMinutes        int                            `json:"window_minutes" gorm:"not null;default:30"`
	MinimumSamples       int                            `json:"minimum_samples" gorm:"not null;default:5"`
	MaxSamplesPerWindow  int                            `json:"max_samples_per_window" gorm:"not null;default:200"`
	LastRunAt            int64                          `json:"last_run_at" gorm:"bigint;not null;default:0"`
	NextRunAt            int64                          `json:"next_run_at" gorm:"bigint;not null;default:0;index"`
	LastError            string                         `json:"last_error,omitempty" gorm:"type:varchar(512)"`
	CreatedAt            int64                          `json:"created_at" gorm:"bigint;not null"`
	UpdatedAt            int64                          `json:"updated_at" gorm:"bigint;not null"`
	Members              []ChannelPurityMember          `json:"members,omitempty" gorm:"foreignKey:GroupID;constraint:OnDelete:CASCADE"`
	ModelComparisons     []ChannelPurityModelComparison `json:"model_comparisons,omitempty" gorm:"foreignKey:GroupID;constraint:OnDelete:CASCADE"`
}

// ChannelPurityModelComparison is an explicit baseline-model to target-model contract.
type ChannelPurityModelComparison struct {
	ID            uint   `json:"id" gorm:"primaryKey"`
	GroupID       uint   `json:"group_id" gorm:"not null;uniqueIndex:uq_purity_group_model_pair,priority:1"`
	BaselineModel string `json:"baseline_model" gorm:"type:varchar(255);not null;uniqueIndex:uq_purity_group_model_pair,priority:2"`
	TargetModel   string `json:"target_model" gorm:"type:varchar(255);not null;uniqueIndex:uq_purity_group_model_pair,priority:3"`
	CreatedAt     int64  `json:"created_at" gorm:"bigint;not null"`
}

func (ChannelPurityModelComparison) TableName() string {
	return "qiqi_channel_purity_model_comparisons"
}

func (ChannelPurityGroup) TableName() string { return "qiqi_channel_purity_groups" }

// BaselineSlot is 1 only for the baseline and NULL for targets. The composite
// unique index therefore enforces one baseline per group on SQLite/MySQL/Postgres.
type ChannelPurityMember struct {
	ID           uint  `json:"id" gorm:"primaryKey"`
	GroupID      uint  `json:"group_id" gorm:"not null;uniqueIndex:uq_purity_group_channel,priority:1;uniqueIndex:uq_purity_group_baseline,priority:1"`
	ChannelID    int   `json:"channel_id" gorm:"not null;uniqueIndex:uq_purity_group_channel,priority:2;index"`
	IsBaseline   bool  `json:"is_baseline" gorm:"not null;default:false"`
	BaselineSlot *int  `json:"-" gorm:"uniqueIndex:uq_purity_group_baseline,priority:2"`
	CreatedAt    int64 `json:"created_at" gorm:"bigint;not null"`
}

func (ChannelPurityMember) TableName() string { return "qiqi_channel_purity_members" }

// ChannelPuritySample is passive detector input; quick-probe rows are never read by aggregation.
type ChannelPuritySample struct {
	ID                 uint   `json:"id" gorm:"primaryKey"`
	GroupID            uint   `json:"group_id" gorm:"not null;index:idx_purity_sample_window,priority:1"`
	ChannelID          int    `json:"channel_id" gorm:"not null;index:idx_purity_sample_window,priority:2"`
	ActualModel        string `json:"actual_model" gorm:"type:varchar(255);not null;index:idx_purity_sample_window,priority:3"`
	RunKey             string `json:"run_key" gorm:"type:varchar(64);not null;index"`
	Protocol           string `json:"protocol" gorm:"type:varchar(32);not null"`
	StructureSignature string `json:"structure_signature" gorm:"type:varchar(512);not null"`
	PromptTokens       int    `json:"prompt_tokens"`
	CompletionTokens   int    `json:"completion_tokens"`
	TotalTokens        int    `json:"total_tokens"`
	Valid              bool   `json:"valid" gorm:"not null"`
	ErrorClass         string `json:"error_class,omitempty" gorm:"type:varchar(64)"`
	ObservedAt         int64  `json:"observed_at" gorm:"bigint;not null;index:idx_purity_sample_window,priority:4"`
}

func (ChannelPuritySample) TableName() string { return "qiqi_channel_purity_samples" }

type ChannelPurityPairRun struct {
	ID                        uint    `json:"id" gorm:"primaryKey"`
	GroupID                   uint    `json:"group_id" gorm:"not null;index:idx_purity_pair_history,priority:1"`
	BaselineChannelID         int     `json:"baseline_channel_id" gorm:"not null"`
	TargetChannelID           int     `json:"target_channel_id" gorm:"not null;index:idx_purity_pair_history,priority:2"`
	ActualModel               string  `json:"actual_model" gorm:"type:varchar(255);not null;index:idx_purity_pair_history,priority:3"`
	BaselineModel             string  `json:"baseline_model" gorm:"type:varchar(255);not null;default:''"`
	TargetModel               string  `json:"target_model" gorm:"type:varchar(255);not null;default:''"`
	WindowStartedAt           int64   `json:"window_started_at" gorm:"bigint;not null"`
	WindowEndedAt             int64   `json:"window_ended_at" gorm:"bigint;not null;index:idx_purity_pair_history,priority:4,sort:desc"`
	BaselineSampleCount       int     `json:"baseline_sample_count"`
	TargetSampleCount         int     `json:"target_sample_count"`
	PairedSampleCount         int     `json:"paired_sample_count"`
	StructureSimilarity       float64 `json:"structure_similarity"`
	StructureSimilarityDetail string  `json:"-" gorm:"type:text;not null;default:''"`
	TokenSimilarity           float64 `json:"token_similarity"`
	BaselineTokenMin          int     `json:"baseline_token_min"`
	BaselineTokenMax          int     `json:"baseline_token_max"`
	TargetTokenMin            int     `json:"target_token_min"`
	TargetTokenMax            int     `json:"target_token_max"`
	TokenDeviationRate        float64 `json:"token_deviation_rate"`
	AnomalyEvidenceJSON       string  `json:"-" gorm:"type:text;not null"`
	Confidence                float64 `json:"confidence"`
	State                     string  `json:"state" gorm:"type:varchar(32);not null"`
	ErrorClass                string  `json:"error_class,omitempty" gorm:"type:varchar(64)"`
	CreatedAt                 int64   `json:"created_at" gorm:"bigint;not null"`
}

func (ChannelPurityPairRun) TableName() string { return "qiqi_channel_purity_pair_runs" }

type ChannelPurityAssessment struct {
	ID                   uint    `json:"id" gorm:"primaryKey"`
	GroupID              uint    `json:"group_id" gorm:"not null;uniqueIndex:uq_purity_assessment_key,priority:1"`
	TargetChannelID      int     `json:"target_channel_id" gorm:"not null;uniqueIndex:uq_purity_assessment_key,priority:2"`
	ActualModel          string  `json:"actual_model" gorm:"type:varchar(255);not null;uniqueIndex:uq_purity_assessment_key,priority:3"`
	LatestPairRunID      uint    `json:"latest_pair_run_id" gorm:"not null"`
	State                string  `json:"state" gorm:"type:varchar(32);not null;index"`
	ConsecutiveAnomalies int     `json:"consecutive_anomalies"`
	ConsecutiveHealthy   int     `json:"consecutive_healthy"`
	Confidence           float64 `json:"confidence"`
	FirstSeenAt          int64   `json:"first_seen_at" gorm:"bigint;not null"`
	UpdatedAt            int64   `json:"updated_at" gorm:"bigint;not null"`
}

func (ChannelPurityAssessment) TableName() string { return "qiqi_channel_purity_assessments" }

type ChannelPurityAlert struct {
	ID           uint   `json:"id" gorm:"primaryKey"`
	AssessmentID uint   `json:"assessment_id" gorm:"not null;index"`
	PairRunID    uint   `json:"pair_run_id" gorm:"not null"`
	Status       string `json:"status" gorm:"type:varchar(16);not null;index"`
	EvidenceJSON string `json:"-" gorm:"type:text;not null"`
	OpenedAt     int64  `json:"opened_at" gorm:"bigint;not null"`
	ResolvedAt   int64  `json:"resolved_at,omitempty" gorm:"bigint"`
}

func (ChannelPurityAlert) TableName() string { return "qiqi_channel_purity_alerts" }

func ValidateChannelPurityGroup(group *ChannelPurityGroup) error {
	group.Name = strings.TrimSpace(group.Name)
	if group.IntervalMinutes == 0 {
		group.IntervalMinutes = 5
	}
	if group.WindowMinutes == 0 {
		group.WindowMinutes = 30
	}
	if group.MinimumSamples == 0 {
		group.MinimumSamples = 5
	}
	if group.MaxSamplesPerWindow == 0 {
		group.MaxSamplesPerWindow = 200
	}
	if group.Name == "" {
		return errors.New("group name is required")
	}
	if group.IntervalMinutes != 5 && group.IntervalMinutes != 10 {
		return errors.New("interval_minutes must be 5 or 10")
	}
	if group.WindowMinutes < group.IntervalMinutes || group.WindowMinutes > 1440 {
		return errors.New("window_minutes must be between interval_minutes and 1440")
	}
	if group.MinimumSamples < 1 || group.MinimumSamples > 1000 {
		return errors.New("minimum_samples must be between 1 and 1000")
	}
	if group.MaxSamplesPerWindow < group.MinimumSamples || group.MaxSamplesPerWindow > 5000 {
		return errors.New("max_samples_per_window must be between minimum_samples and 5000")
	}
	if len(group.Members) < 2 {
		return errors.New("a group requires at least two channels")
	}
	baseline, seen := 0, map[int]bool{}
	one := 1
	for i := range group.Members {
		m := &group.Members[i]
		if m.ChannelID <= 0 || seen[m.ChannelID] {
			return errors.New("channel ids must be positive and unique within a group")
		}
		seen[m.ChannelID] = true
		if m.IsBaseline {
			baseline++
			m.BaselineSlot = &one
		} else {
			m.BaselineSlot = nil
		}
	}
	if baseline != 1 {
		return errors.New("a group requires exactly one baseline")
	}
	seenComparisons := map[string]bool{}
	for i := range group.ModelComparisons {
		comparison := &group.ModelComparisons[i]
		comparison.BaselineModel = strings.TrimSpace(comparison.BaselineModel)
		comparison.TargetModel = strings.TrimSpace(comparison.TargetModel)
		if comparison.BaselineModel == "" || comparison.TargetModel == "" {
			return errors.New("baseline_model and target_model are required")
		}
		key := comparison.BaselineModel + "\x00" + comparison.TargetModel
		if seenComparisons[key] {
			return errors.New("model comparisons must be unique")
		}
		seenComparisons[key] = true
	}
	return nil
}

func CreatePurityGroup(group *ChannelPurityGroup) error {
	if err := ValidateChannelPurityGroup(group); err != nil {
		return err
	}
	return DB.Transaction(func(tx *gorm.DB) error { return tx.Create(group).Error })
}
func UpdatePurityGroup(group *ChannelPurityGroup) error {
	if err := ValidateChannelPurityGroup(group); err != nil {
		return err
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&ChannelPurityGroup{}).Where("id = ?", group.ID).Updates(map[string]any{
			"name": group.Name, "enabled": group.Enabled, "interval_minutes": group.IntervalMinutes,
			"random_pairing_enabled": group.RandomPairingEnabled, "window_minutes": group.WindowMinutes,
			"minimum_samples": group.MinimumSamples, "max_samples_per_window": group.MaxSamplesPerWindow,
			"next_run_at": group.NextRunAt, "updated_at": group.UpdatedAt,
		}).Error; err != nil {
			return err
		}
		if err := tx.Where("group_id = ?", group.ID).Delete(&ChannelPurityMember{}).Error; err != nil {
			return err
		}
		for i := range group.Members {
			group.Members[i].ID = 0
			group.Members[i].GroupID = group.ID
		}
		if err := tx.Create(&group.Members).Error; err != nil {
			return err
		}
		if err := tx.Where("group_id = ?", group.ID).Delete(&ChannelPurityModelComparison{}).Error; err != nil {
			return err
		}
		for i := range group.ModelComparisons {
			group.ModelComparisons[i].ID = 0
			group.ModelComparisons[i].GroupID = group.ID
		}
		if len(group.ModelComparisons) > 0 {
			return tx.Create(&group.ModelComparisons).Error
		}
		return nil
	})
}
func GetPurityGroup(id uint) (*ChannelPurityGroup, error) {
	var v ChannelPurityGroup
	err := DB.Preload("Members").Preload("ModelComparisons").First(&v, id).Error
	return &v, err
}
func ListPurityGroups() ([]ChannelPurityGroup, error) {
	var v []ChannelPurityGroup
	err := DB.Preload("Members").Preload("ModelComparisons").Order("id asc").Find(&v).Error
	return v, err
}
func DeletePurityGroup(id uint) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		var assessmentIDs []uint
		if err := tx.Model(&ChannelPurityAssessment{}).Where("group_id = ?", id).Pluck("id", &assessmentIDs).Error; err != nil {
			return err
		}
		if len(assessmentIDs) > 0 {
			if err := tx.Where("assessment_id IN ?", assessmentIDs).Delete(&ChannelPurityAlert{}).Error; err != nil {
				return err
			}
		}
		for _, target := range []any{&ChannelPurityAssessment{}, &ChannelPurityPairRun{}, &ChannelPuritySample{}, &ChannelPurityModelComparison{}, &ChannelPurityMember{}} {
			if err := tx.Where("group_id = ?", id).Delete(target).Error; err != nil {
				return err
			}
		}
		return tx.Delete(&ChannelPurityGroup{}, id).Error
	})
}
func CreatePuritySample(sample *ChannelPuritySample) error { return DB.Create(sample).Error }
func ListPurityGroupIDsForChannel(channelID int) ([]uint, error) {
	var groupIDs []uint
	err := DB.Table("qiqi_channel_purity_members AS members").
		Joins("JOIN qiqi_channel_purity_groups AS groups ON groups.id = members.group_id").
		Where("members.channel_id = ? AND groups.enabled = ?", channelID, true).
		Order("members.group_id ASC").Pluck("members.group_id", &groupIDs).Error
	return groupIDs, err
}
func ListDuePurityGroups(now int64) ([]ChannelPurityGroup, error) {
	var groups []ChannelPurityGroup
	err := DB.Preload("Members").Preload("ModelComparisons").Where("enabled = ? AND (next_run_at = 0 OR next_run_at <= ?)", true, now).Order("id asc").Find(&groups).Error
	return groups, err
}
func MarkPurityGroupRun(id uint, lastRunAt, nextRunAt int64, lastError string) error {
	return DB.Model(&ChannelPurityGroup{}).Where("id = ?", id).Updates(map[string]any{
		"last_run_at": lastRunAt, "next_run_at": nextRunAt, "last_error": lastError, "updated_at": lastRunAt,
	}).Error
}
func HasEnabledPurityGroups() bool {
	var count int64
	return DB.Model(&ChannelPurityGroup{}).Where("enabled = ?", true).Limit(1).Count(&count).Error == nil && count > 0
}
func ListPurityAssessments(groupID uint) ([]ChannelPurityAssessment, error) {
	var values []ChannelPurityAssessment
	err := DB.Where("group_id = ?", groupID).Order("target_channel_id asc, actual_model asc").Find(&values).Error
	return values, err
}
func GetPurityPairRun(id uint) (*ChannelPurityPairRun, error) {
	var value ChannelPurityPairRun
	err := DB.First(&value, id).Error
	return &value, err
}
func ListRecentPurityPairRuns(groupID uint, targetID int, actualModel string, limit int) ([]ChannelPurityPairRun, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	var values []ChannelPurityPairRun
	err := DB.Where("group_id = ? AND target_channel_id = ? AND actual_model = ?", groupID, targetID, actualModel).
		Order("window_ended_at desc, id desc").Limit(limit).Find(&values).Error
	return values, err
}
func ListOpenPurityAlerts(assessmentID uint) ([]ChannelPurityAlert, error) {
	var values []ChannelPurityAlert
	err := DB.Where("assessment_id = ? AND status = ?", assessmentID, "OPEN").Order("opened_at desc").Find(&values).Error
	return values, err
}
func GetLatestPurityAssessment(groupID uint, targetID int, actualModel string) (*ChannelPurityAssessment, error) {
	var v ChannelPurityAssessment
	err := DB.Where("group_id = ? AND target_channel_id = ? AND actual_model = ?", groupID, targetID, actualModel).First(&v).Error
	return &v, err
}
func ListPurityPairRuns(groupID uint, targetID int, actualModel string, offset, limit int) ([]ChannelPurityPairRun, int64, error) {
	q := DB.Model(&ChannelPurityPairRun{}).Where("group_id = ? AND target_channel_id = ? AND actual_model = ?", groupID, targetID, actualModel)
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var v []ChannelPurityPairRun
	err := q.Order("window_ended_at desc, id desc").Offset(offset).Limit(limit).Find(&v).Error
	return v, total, err
}
