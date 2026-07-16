package model

import "gorm.io/gorm"

const (
	ChannelPurityStatusPending   = "pending"
	ChannelPurityStatusRunning   = "running"
	ChannelPurityStatusCompleted = "completed"
	ChannelPurityStatusFailed    = "failed"

	ChannelPurityConclusionUnknown       = "unknown"
	ChannelPurityConclusionNoObviousRisk = "no_obvious_risk"
	ChannelPurityConclusionRisk          = "risk"

	ChannelPurityRiskUnknown = "unknown"
	ChannelPurityRiskLow     = "low"
	ChannelPurityRiskMedium  = "medium"
)

// ChannelPurityScan represents one bounded Quick probe. It never stores credentials.
type ChannelPurityScan struct {
	ID             uint                 `json:"id" gorm:"primaryKey"`
	ChannelID      int                  `json:"channel_id" gorm:"not null;index:idx_purity_scan_channel_created,priority:1"`
	ChannelName    string               `json:"channel_name" gorm:"type:varchar(255);not null"`
	RequestedModel string               `json:"model" gorm:"type:varchar(255);not null;index:idx_purity_scan_channel_created,priority:2"`
	Protocol       string               `json:"protocol" gorm:"type:varchar(32);not null"`
	Status         string               `json:"status" gorm:"type:varchar(32);not null;index"`
	Conclusion     string               `json:"conclusion" gorm:"type:varchar(32);not null"`
	Risk           string               `json:"risk" gorm:"type:varchar(32);not null"`
	Coverage       int                  `json:"coverage" gorm:"not null"`
	Summary        string               `json:"summary" gorm:"type:text;not null"`
	ErrorClass     string               `json:"error_class,omitempty" gorm:"type:varchar(64)"`
	CreatedBy      int                  `json:"created_by" gorm:"not null"`
	CreatedAt      int64                `json:"created_at" gorm:"bigint;not null;index:idx_purity_scan_channel_created,priority:3,sort:desc"`
	StartedAt      int64                `json:"started_at" gorm:"bigint"`
	CompletedAt    int64                `json:"completed_at" gorm:"bigint"`
	Result         *ChannelPurityResult `json:"result,omitempty" gorm:"foreignKey:ScanID;constraint:OnDelete:CASCADE"`
}

func (ChannelPurityScan) TableName() string { return "qiqi_channel_purity_scans" }

// ChannelPurityResult stores normalized structural observations only.
type ChannelPurityResult struct {
	ID               uint   `json:"id" gorm:"primaryKey"`
	ScanID           uint   `json:"scan_id" gorm:"not null;uniqueIndex"`
	ChannelID        int    `json:"channel_id" gorm:"not null;index"`
	DeclaredModel    string `json:"declared_model,omitempty" gorm:"type:varchar(255)"`
	HTTPStatus       int    `json:"http_status"`
	LatencyMS        int64  `json:"latency_ms" gorm:"bigint"`
	PromptTokens     int    `json:"prompt_tokens"`
	CompletionTokens int    `json:"completion_tokens"`
	TotalTokens      int    `json:"total_tokens"`
	HasModelField    bool   `json:"has_model_field"`
	HasUsage         bool   `json:"has_usage"`
	HasOutput        bool   `json:"has_output"`
	ProtocolValid    bool   `json:"protocol_valid"`
	EvidenceJSON     string `json:"-" gorm:"type:text;not null"`
	CreatedAt        int64  `json:"created_at" gorm:"bigint;not null"`
}

func (ChannelPurityResult) TableName() string { return "qiqi_channel_purity_results" }

func CreateChannelPurityScan(scan *ChannelPurityScan) error { return DB.Create(scan).Error }

func MarkChannelPurityScanRunning(id uint, startedAt int64) error {
	return DB.Model(&ChannelPurityScan{}).Where("id = ? AND status = ?", id, ChannelPurityStatusPending).
		Updates(map[string]any{"status": ChannelPurityStatusRunning, "started_at": startedAt}).Error
}

func FinishChannelPurityScan(scan *ChannelPurityScan, result *ChannelPurityResult) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		if result != nil {
			result.ScanID = scan.ID
			if err := tx.Create(result).Error; err != nil {
				return err
			}
		}
		return tx.Model(&ChannelPurityScan{}).Where("id = ?", scan.ID).Updates(map[string]any{
			"status": scan.Status, "conclusion": scan.Conclusion, "risk": scan.Risk,
			"coverage": scan.Coverage, "summary": scan.Summary, "error_class": scan.ErrorClass,
			"completed_at": scan.CompletedAt,
		}).Error
	})
}

func GetChannelPurityScan(id uint) (*ChannelPurityScan, error) {
	var scan ChannelPurityScan
	if err := DB.Preload("Result").First(&scan, id).Error; err != nil {
		return nil, err
	}
	return &scan, nil
}

func GetLatestChannelPurityScan(channelID int, modelName string) (*ChannelPurityScan, error) {
	query := DB.Where("channel_id = ?", channelID)
	if modelName != "" {
		query = query.Where("requested_model = ?", modelName)
	}
	var scan ChannelPurityScan
	if err := query.Order("created_at DESC").Order("id DESC").Preload("Result").First(&scan).Error; err != nil {
		return nil, err
	}
	return &scan, nil
}

func ListChannelPurityScans(offset, limit int) ([]ChannelPurityScan, int64, error) {
	query := DB.Model(&ChannelPurityScan{})
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var scans []ChannelPurityScan
	err := query.Order("created_at DESC").Order("id DESC").Offset(offset).Limit(limit).Preload("Result").Find(&scans).Error
	return scans, total, err
}

func ListChannelPurityResults(offset, limit int) ([]ChannelPurityScan, int64, error) {
	return ListChannelPurityScans(offset, limit)
}
