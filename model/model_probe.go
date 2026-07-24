package model

import "time"

// ModelProbeResult persists only fixed-probe metadata. Probe request and response
// bodies are deliberately excluded from this schema.
type ModelProbeResult struct {
	Id             int    `json:"id"`
	CreatedAt      int64  `json:"created_at" gorm:"bigint;index"`
	ChannelId      int    `json:"channel_id" gorm:"index"`
	ChannelName    string `json:"channel_name" gorm:"type:varchar(191)"`
	RequestModel   string `json:"request_model" gorm:"type:varchar(191);index"`
	DeclaredModel  string `json:"declared_model" gorm:"type:varchar(191);index"`
	ActualModel    string `json:"actual_model" gorm:"type:varchar(191)"`
	IdStatus       string `json:"id_status" gorm:"type:varchar(24)"`
	ExpectedTokens *int   `json:"expected_tokens"`
	ActualTokens   *int   `json:"actual_tokens"`
	TokenDelta     *int   `json:"token_delta"`
	TokenTolerance int    `json:"token_tolerance"`
	TokenStatus    string `json:"token_status" gorm:"type:varchar(24)"`
	Conclusion     string `json:"conclusion" gorm:"type:varchar(24);index"`
	Error          string `json:"error" gorm:"type:varchar(500)"`
}

func (ModelProbeResult) TableName() string { return "model_probe_results" }

func CreateModelProbeResult(result *ModelProbeResult) error {
	if result == nil || DB == nil {
		return nil
	}
	if result.CreatedAt == 0 {
		result.CreatedAt = time.Now().Unix()
	}
	return DB.Create(result).Error
}

func ListModelProbeResults(limit int) ([]ModelProbeResult, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	results := make([]ModelProbeResult, 0, limit)
	err := DB.Order("CASE WHEN conclusion = 'suspicious' THEN 0 WHEN conclusion = 'failed' THEN 1 ELSE 2 END").
		Order("id DESC").Limit(limit).Find(&results).Error
	return results, err
}
