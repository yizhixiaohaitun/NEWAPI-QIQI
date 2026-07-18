package dto

// ChannelPurityQuickScanRequest starts one bounded, non-streaming probe.
type ChannelPurityQuickScanRequest struct {
	ChannelID int    `json:"channel_id" binding:"required"`
	Model     string `json:"model" binding:"required"`
}

// ChannelPurityInspectionSettings is the persistent scheduler configuration.
type ChannelPurityInspectionSettings struct {
	Enabled         bool `json:"enabled"`
	IntervalMinutes int  `json:"interval_minutes"`
}

// ChannelPurityInspectionStatus combines persistent settings with task state.
type ChannelPurityInspectionStatus struct {
	Enabled           bool  `json:"enabled"`
	IntervalMinutes   int   `json:"interval_minutes"`
	Running           bool  `json:"running"`
	EnabledChannels   int   `json:"enabled_channels"`
	ModelCombinations int   `json:"model_combinations"`
	LastRunAt         int64 `json:"last_run_at,omitempty"`
	NextRunAt         int64 `json:"next_run_at,omitempty"`
	Task              any   `json:"task,omitempty"`
}

// ChannelPurityUsage is the normalized, non-sensitive usage subset retained by a scan.
type ChannelPurityUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// ChannelPurityEvidence contains structural observations only. Response content and credentials are never retained.
type ChannelPurityEvidence struct {
	HTTPStatus       int                `json:"http_status"`
	ContentType      string             `json:"content_type,omitempty"`
	Object           string             `json:"object,omitempty"`
	ResponseIDPrefix string             `json:"response_id_prefix,omitempty"`
	DeclaredModel    string             `json:"declared_model,omitempty"`
	MappedModel      string             `json:"mapped_model,omitempty"`
	HasModelField    bool               `json:"has_model_field"`
	HasUsage         bool               `json:"has_usage"`
	HasOutput        bool               `json:"has_output"`
	HasChoices       bool               `json:"has_choices"`
	Usage            ChannelPurityUsage `json:"usage"`
	Warnings         []string           `json:"warnings,omitempty"`
}

// ChannelPurityScanResponse is the credential-free API representation of a scan and its result.
type ChannelPurityScanResponse struct {
	ID          uint                         `json:"id"`
	ChannelID   int                          `json:"channel_id"`
	ChannelName string                       `json:"channel_name"`
	Model       string                       `json:"model"`
	Protocol    string                       `json:"protocol"`
	Status      string                       `json:"status"`
	Conclusion  string                       `json:"conclusion"`
	Risk        string                       `json:"risk"`
	Coverage    int                          `json:"coverage"`
	Summary     string                       `json:"summary"`
	ErrorClass  string                       `json:"error_class,omitempty"`
	CreatedBy   int                          `json:"created_by"`
	CreatedAt   int64                        `json:"created_at"`
	StartedAt   int64                        `json:"started_at,omitempty"`
	CompletedAt int64                        `json:"completed_at,omitempty"`
	Result      *ChannelPurityResultResponse `json:"result,omitempty"`
}

// ChannelPurityResultResponse exposes normalized probe observations without upstream content.
type ChannelPurityResultResponse struct {
	ID            uint                  `json:"id"`
	ScanID        uint                  `json:"scan_id"`
	ChannelID     int                   `json:"channel_id"`
	Model         string                `json:"model"`
	Protocol      string                `json:"protocol"`
	Status        string                `json:"status"`
	Conclusion    string                `json:"conclusion"`
	Risk          string                `json:"risk"`
	Coverage      int                   `json:"coverage"`
	Summary       string                `json:"summary"`
	DeclaredModel string                `json:"declared_model,omitempty"`
	LatencyMS     int64                 `json:"latency_ms"`
	HTTPStatus    int                   `json:"http_status"`
	ErrorClass    string                `json:"error_class,omitempty"`
	Usage         ChannelPurityUsage    `json:"usage"`
	Evidence      ChannelPurityEvidence `json:"evidence"`
	CreatedAt     int64                 `json:"created_at"`
}
