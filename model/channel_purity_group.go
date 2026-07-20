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
	SuspectThreshold     float64                        `json:"suspect_threshold" gorm:"not null;default:0.72"`
	AlertThreshold       float64                        `json:"alert_threshold" gorm:"not null;default:0.55"`
	AlertWindows         int                            `json:"alert_windows" gorm:"not null;default:3"`
	RecoveryWindows      int                            `json:"recovery_windows" gorm:"not null;default:2"`
	RetentionWindows     int                            `json:"retention_windows" gorm:"not null;default:100"`
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
	ID                   uint   `json:"id" gorm:"primaryKey"`
	GroupID              uint   `json:"group_id" gorm:"not null;index:idx_purity_sample_window,priority:1"`
	ChannelID            int    `json:"channel_id" gorm:"not null;index:idx_purity_sample_window,priority:2"`
	ActualModel          string `json:"actual_model" gorm:"type:varchar(255);not null;index:idx_purity_sample_window,priority:3"`
	RunKey               string `json:"run_key" gorm:"type:varchar(64);not null;index"`
	Protocol             string `json:"protocol" gorm:"type:varchar(32);not null"`
	StructureSignature   string `json:"structure_signature" gorm:"type:varchar(512);not null"`
	StructureProfileJSON string `json:"-" gorm:"type:text;not null;default:''"`
	PromptTokens         int    `json:"prompt_tokens"`
	CompletionTokens     int    `json:"completion_tokens"`
	TotalTokens          int    `json:"total_tokens"`
	Valid                bool   `json:"valid" gorm:"not null"`
	ErrorClass           string `json:"error_class,omitempty" gorm:"type:varchar(64)"`
	ObservedAt           int64  `json:"observed_at" gorm:"bigint;not null;index:idx_purity_sample_window,priority:4"`
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
	BaselineInvalidCount      int     `json:"baseline_invalid_count"`
	TargetInvalidCount        int     `json:"target_invalid_count"`
	UnmatchedBaselineCount    int     `json:"unmatched_baseline_count"`
	UnmatchedTargetCount      int     `json:"unmatched_target_count"`
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
	ID              uint   `json:"id" gorm:"primaryKey"`
	AssessmentID    uint   `json:"assessment_id" gorm:"not null;index"`
	PairRunID       uint   `json:"pair_run_id" gorm:"not null"`
	Status          string `json:"status" gorm:"type:varchar(24);not null;index"`
	EvidenceJSON    string `json:"-" gorm:"type:text;not null"`
	Note            string `json:"note,omitempty" gorm:"type:text"`
	SilenceUntil    int64  `json:"silence_until,omitempty" gorm:"bigint"`
	AcknowledgedAt  int64  `json:"acknowledged_at,omitempty" gorm:"bigint"`
	FalsePositiveAt int64  `json:"false_positive_at,omitempty" gorm:"bigint"`
	OpenedAt        int64  `json:"opened_at" gorm:"bigint;not null"`
	ResolvedAt      int64  `json:"resolved_at,omitempty" gorm:"bigint"`
	UpdatedAt       int64  `json:"updated_at,omitempty" gorm:"bigint"`
}

func (ChannelPurityAlert) TableName() string { return "qiqi_channel_purity_alerts" }

type ChannelPurityAlertAudit struct {
	ID        uint   `json:"id" gorm:"primaryKey"`
	AlertID   uint   `json:"alert_id" gorm:"not null;index"`
	Action    string `json:"action" gorm:"type:varchar(32);not null"`
	Note      string `json:"note,omitempty" gorm:"type:text"`
	CreatedAt int64  `json:"created_at" gorm:"bigint;not null"`
}

func (ChannelPurityAlertAudit) TableName() string { return "qiqi_channel_purity_alert_audits" }

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
	if group.SuspectThreshold == 0 {
		group.SuspectThreshold = .72
	}
	if group.AlertThreshold == 0 {
		group.AlertThreshold = .55
	}
	if group.AlertWindows == 0 {
		group.AlertWindows = 3
	}
	if group.RecoveryWindows == 0 {
		group.RecoveryWindows = 2
	}
	if group.RetentionWindows == 0 {
		group.RetentionWindows = 100
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
	if group.AlertThreshold <= 0 || group.SuspectThreshold >= 1 || group.AlertThreshold >= group.SuspectThreshold {
		return errors.New("alert_threshold must be greater than 0 and lower than suspect_threshold; suspect_threshold must be lower than 1")
	}
	if group.AlertWindows < 1 || group.AlertWindows > 20 || group.RecoveryWindows < 1 || group.RecoveryWindows > 20 {
		return errors.New("alert_windows and recovery_windows must be between 1 and 20")
	}
	if group.RetentionWindows < 10 || group.RetentionWindows > 1000 {
		return errors.New("retention max_windows_per_target_model must be between 10 and 1000")
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
			"suspect_threshold": group.SuspectThreshold, "alert_threshold": group.AlertThreshold,
			"alert_windows": group.AlertWindows, "recovery_windows": group.RecoveryWindows,
			"retention_windows": group.RetentionWindows,
			"next_run_at":       group.NextRunAt, "updated_at": group.UpdatedAt,
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
			var alertIDs []uint
			if err := tx.Model(&ChannelPurityAlert{}).Where("assessment_id IN ?", assessmentIDs).Pluck("id", &alertIDs).Error; err != nil {
				return err
			}
			if len(alertIDs) > 0 {
				if err := tx.Where("alert_id IN ?", alertIDs).Delete(&ChannelPurityAlertAudit{}).Error; err != nil {
					return err
				}
			}
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

var ErrPurityGroupDetectionRunning = errors.New("channel purity detection is pending or running")

// ClearPurityGroupHistory removes detector outputs while preserving the group configuration.
// The active system-task check and deletes share one transaction so a pending/running group
// detection cannot be cleared underneath its writer.
func ClearPurityGroupHistory(id uint) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		var group ChannelPurityGroup
		if err := tx.First(&group, id).Error; err != nil {
			return err
		}
		var activeTasks []SystemTask
		if err := tx.Where("type = ? AND status IN ?", SystemTaskTypeChannelPurityAggregate, activeSystemTaskStatuses()).Find(&activeTasks).Error; err != nil {
			return err
		}
		for i := range activeTasks {
			var payload struct {
				GroupID uint `json:"group_id"`
			}
			if activeTasks[i].DecodePayload(&payload) == nil && (payload.GroupID == 0 || payload.GroupID == id) {
				return ErrPurityGroupDetectionRunning
			}
		}
		var assessmentIDs []uint
		if err := tx.Model(&ChannelPurityAssessment{}).Where("group_id = ?", id).Pluck("id", &assessmentIDs).Error; err != nil {
			return err
		}
		if len(assessmentIDs) > 0 {
			var alertIDs []uint
			if err := tx.Model(&ChannelPurityAlert{}).Where("assessment_id IN ?", assessmentIDs).Pluck("id", &alertIDs).Error; err != nil {
				return err
			}
			if len(alertIDs) > 0 {
				if err := tx.Where("alert_id IN ?", alertIDs).Delete(&ChannelPurityAlertAudit{}).Error; err != nil {
					return err
				}
			}
			if err := tx.Where("assessment_id IN ?", assessmentIDs).Delete(&ChannelPurityAlert{}).Error; err != nil {
				return err
			}
		}
		if err := tx.Where("group_id = ?", id).Delete(&ChannelPurityAssessment{}).Error; err != nil {
			return err
		}
		if err := tx.Where("group_id = ?", id).Delete(&ChannelPurityPairRun{}).Error; err != nil {
			return err
		}
		return tx.Where("group_id = ?", id).Delete(&ChannelPuritySample{}).Error
	})
}

// PrunePurityGroupHistory keeps a bounded number of pair-run rows per result bucket.
// Assessments always point at the newest row, so only older, unreferenced rows are removed.
func PrunePurityGroupHistory(groupID uint, keepPerBucket int) error {
	if keepPerBucket <= 0 {
		keepPerBucket = 100
	}
	var assessments []ChannelPurityAssessment
	if err := DB.Where("group_id = ?", groupID).Find(&assessments).Error; err != nil {
		return err
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		for _, assessment := range assessments {
			var keepIDs []uint
			if err := tx.Model(&ChannelPurityPairRun{}).
				Where("group_id = ? AND target_channel_id = ? AND actual_model = ?", groupID, assessment.TargetChannelID, assessment.ActualModel).
				Order("window_ended_at desc, id desc").Limit(keepPerBucket).Pluck("id", &keepIDs).Error; err != nil {
				return err
			}
			keepIDs = append(keepIDs, assessment.LatestPairRunID)
			var alertRunIDs []uint
			if err := tx.Model(&ChannelPurityAlert{}).Where("assessment_id = ? AND status IN ?", assessment.ID, []string{"OPEN", "ACKNOWLEDGED", "SILENCED"}).Pluck("pair_run_id", &alertRunIDs).Error; err != nil {
				return err
			}
			keepIDs = append(keepIDs, alertRunIDs...)
			query := tx.Where("group_id = ? AND target_channel_id = ? AND actual_model = ?", groupID, assessment.TargetChannelID, assessment.ActualModel)
			if len(keepIDs) > 0 {
				query = query.Where("id NOT IN ?", keepIDs)
			}
			if err := query.Delete(&ChannelPurityPairRun{}).Error; err != nil {
				return err
			}
		}
		return nil
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
	err := DB.Where("assessment_id = ? AND status IN ?", assessmentID, []string{"OPEN", "ACKNOWLEDGED", "SILENCED"}).Order("opened_at desc").Find(&values).Error
	return values, err
}
func ListPurityAlertAudits(alertID uint) ([]ChannelPurityAlertAudit, error) {
	var values []ChannelPurityAlertAudit
	err := DB.Where("alert_id = ?", alertID).Order("created_at desc, id desc").Find(&values).Error
	return values, err
}
func GetPurityAlertForGroup(groupID, alertID uint) (*ChannelPurityAlert, error) {
	var value ChannelPurityAlert
	err := DB.Table("qiqi_channel_purity_alerts AS alerts").
		Select("alerts.*").Joins("JOIN qiqi_channel_purity_assessments AS assessments ON assessments.id = alerts.assessment_id").
		Where("alerts.id = ? AND assessments.group_id = ?", alertID, groupID).First(&value).Error
	return &value, err
}
func UpdatePurityAlertAction(alert *ChannelPurityAlert, action, note string, now int64) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(alert).Error; err != nil {
			return err
		}
		return tx.Create(&ChannelPurityAlertAudit{AlertID: alert.ID, Action: action, Note: note, CreatedAt: now}).Error
	})
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
func ListPurityHistory(groupID uint, state, query string, offset, limit int) ([]ChannelPurityPairRun, int64, error) {
	q := DB.Table("qiqi_channel_purity_pair_runs AS runs").
		Joins("LEFT JOIN qiqi_channel_purity_groups AS groups ON groups.id = runs.group_id").
		Joins("LEFT JOIN channels AS channels ON channels.id = runs.target_channel_id")
	if groupID != 0 {
		q = q.Where("runs.group_id = ?", groupID)
	}
	if state != "" {
		q = q.Where("runs.state = ?", state)
	}
	if query = strings.TrimSpace(query); query != "" {
		like := "%" + query + "%"
		q = q.Where("runs.baseline_model LIKE ? OR runs.target_model LIKE ? OR runs.actual_model LIKE ? OR groups.name LIKE ? OR channels.name LIKE ?", like, like, like, like, like)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var values []ChannelPurityPairRun
	err := q.Select("runs.*").Order("runs.window_ended_at desc, runs.id desc").Offset(offset).Limit(limit).Scan(&values).Error
	return values, total, err
}
func PurityHistoryPreview(groupID uint) (samples, pairRuns, assessments, alerts, audits int64, err error) {
	if err = DB.Model(&ChannelPuritySample{}).Where("group_id = ?", groupID).Count(&samples).Error; err != nil {
		return
	}
	if err = DB.Model(&ChannelPurityPairRun{}).Where("group_id = ?", groupID).Count(&pairRuns).Error; err != nil {
		return
	}
	if err = DB.Model(&ChannelPurityAssessment{}).Where("group_id = ?", groupID).Count(&assessments).Error; err != nil {
		return
	}
	var assessmentIDs []uint
	if err = DB.Model(&ChannelPurityAssessment{}).Where("group_id = ?", groupID).Pluck("id", &assessmentIDs).Error; err != nil {
		return
	}
	if len(assessmentIDs) == 0 {
		return
	}
	if err = DB.Model(&ChannelPurityAlert{}).Where("assessment_id IN ?", assessmentIDs).Count(&alerts).Error; err != nil {
		return
	}
	var alertIDs []uint
	if err = DB.Model(&ChannelPurityAlert{}).Where("assessment_id IN ?", assessmentIDs).Pluck("id", &alertIDs).Error; err != nil || len(alertIDs) == 0 {
		return
	}
	err = DB.Model(&ChannelPurityAlertAudit{}).Where("alert_id IN ?", alertIDs).Count(&audits).Error
	return
}
