package model

import (
	"errors"
	"strings"

	"gorm.io/gorm"
)

const (
	ChannelPurityStateBaselineUnavailable = "BASELINE_UNAVAILABLE"
	ChannelPurityStateLowSample           = "LOW_SAMPLE"
	ChannelPurityStateWarmingUp           = "WARMING_UP"
	ChannelPurityStateHealthy             = "HEALTHY"
	ChannelPurityStateSuspect             = "SUSPECT"
	ChannelPurityStateAlert               = "ALERT"
	ChannelPurityStateDetectorError       = "DETECTOR_ERROR"
)

// ChannelPurityGroup is the isolation boundary for baseline comparison.
type ChannelPurityGroup struct {
	ID              uint                  `json:"id" gorm:"primaryKey"`
	Name            string                `json:"name" gorm:"type:varchar(255);not null;uniqueIndex"`
	Enabled         bool                  `json:"enabled" gorm:"not null;default:true"`
	IntervalMinutes int                   `json:"interval_minutes" gorm:"not null;default:5"`
	CreatedAt       int64                 `json:"created_at" gorm:"bigint;not null"`
	UpdatedAt       int64                 `json:"updated_at" gorm:"bigint;not null"`
	Members         []ChannelPurityMember `json:"members,omitempty" gorm:"foreignKey:GroupID;constraint:OnDelete:CASCADE"`
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
	ID                  uint    `json:"id" gorm:"primaryKey"`
	GroupID             uint    `json:"group_id" gorm:"not null;index:idx_purity_pair_history,priority:1"`
	BaselineChannelID   int     `json:"baseline_channel_id" gorm:"not null"`
	TargetChannelID     int     `json:"target_channel_id" gorm:"not null;index:idx_purity_pair_history,priority:2"`
	ActualModel         string  `json:"actual_model" gorm:"type:varchar(255);not null;index:idx_purity_pair_history,priority:3"`
	WindowStartedAt     int64   `json:"window_started_at" gorm:"bigint;not null"`
	WindowEndedAt       int64   `json:"window_ended_at" gorm:"bigint;not null;index:idx_purity_pair_history,priority:4,sort:desc"`
	BaselineSampleCount int     `json:"baseline_sample_count"`
	TargetSampleCount   int     `json:"target_sample_count"`
	StructureSimilarity float64 `json:"structure_similarity"`
	TokenSimilarity     float64 `json:"token_similarity"`
	AnomalyEvidenceJSON string  `json:"-" gorm:"type:text;not null"`
	Confidence          float64 `json:"confidence"`
	State               string  `json:"state" gorm:"type:varchar(32);not null"`
	ErrorClass          string  `json:"error_class,omitempty" gorm:"type:varchar(64)"`
	CreatedAt           int64   `json:"created_at" gorm:"bigint;not null"`
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
	if group.Name == "" {
		return errors.New("group name is required")
	}
	if group.IntervalMinutes < 5 || group.IntervalMinutes > 10 {
		return errors.New("interval_minutes must be between 5 and 10")
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
		if err := tx.Model(&ChannelPurityGroup{}).Where("id = ?", group.ID).Updates(map[string]any{"name": group.Name, "enabled": group.Enabled, "interval_minutes": group.IntervalMinutes, "updated_at": group.UpdatedAt}).Error; err != nil {
			return err
		}
		if err := tx.Where("group_id = ?", group.ID).Delete(&ChannelPurityMember{}).Error; err != nil {
			return err
		}
		for i := range group.Members {
			group.Members[i].ID = 0
			group.Members[i].GroupID = group.ID
		}
		return tx.Create(&group.Members).Error
	})
}
func GetPurityGroup(id uint) (*ChannelPurityGroup, error) {
	var v ChannelPurityGroup
	err := DB.Preload("Members").First(&v, id).Error
	return &v, err
}
func ListPurityGroups() ([]ChannelPurityGroup, error) {
	var v []ChannelPurityGroup
	err := DB.Preload("Members").Order("id asc").Find(&v).Error
	return v, err
}
func DeletePurityGroup(id uint) error                      { return DB.Delete(&ChannelPurityGroup{}, id).Error }
func CreatePuritySample(sample *ChannelPuritySample) error { return DB.Create(sample).Error }
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
