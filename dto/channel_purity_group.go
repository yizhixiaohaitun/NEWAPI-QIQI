package dto

type ChannelPurityGroupMemberRequest struct {
	ChannelID  int  `json:"channel_id"`
	IsBaseline bool `json:"is_baseline"`
}
type ChannelPuritySamplingRequest struct {
	WindowMinutes       int `json:"window_minutes"`
	MinimumSamples      int `json:"minimum_samples"`
	MaxSamplesPerWindow int `json:"max_samples_per_window"`
}
type ChannelPurityModelComparisonRequest struct {
	BaselineModel string `json:"baseline_model"`
	TargetModel   string `json:"target_model"`
}
type ChannelPurityPolicyRequest struct {
	SuspectThreshold float64 `json:"suspect_threshold"`
	AlertThreshold   float64 `json:"alert_threshold"`
	AlertWindows     int     `json:"alert_windows"`
	RecoveryWindows  int     `json:"recovery_windows"`
}
type ChannelPurityRetentionRequest struct {
	MaxWindowsPerTargetModel int    `json:"max_windows_per_target_model"`
	Policy                   string `json:"policy"`
}
type ChannelPurityFieldProfile struct {
	Path string `json:"path"`
	Type string `json:"type"`
}
type ChannelPurityGroupRequest struct {
	Name                 string                                `json:"name"`
	Enabled              bool                                  `json:"enabled"`
	IntervalMinutes      int                                   `json:"interval_minutes"`
	RandomPairingEnabled bool                                  `json:"random_pairing_enabled"`
	ChannelIDs           []int                                 `json:"channel_ids"`
	BaselineChannelID    int                                   `json:"baseline_channel_id"`
	Sampling             ChannelPuritySamplingRequest          `json:"sampling"`
	Policy               ChannelPurityPolicyRequest            `json:"policy"`
	Retention            ChannelPurityRetentionRequest         `json:"retention"`
	Members              []ChannelPurityGroupMemberRequest     `json:"members,omitempty"`
	ModelComparisons     []ChannelPurityModelComparisonRequest `json:"model_comparisons"`
}
type ChannelPuritySampleRequest struct {
	GroupID            uint                        `json:"group_id"`
	ChannelID          int                         `json:"channel_id"`
	ActualModel        string                      `json:"actual_model"`
	StructureSignature string                      `json:"structure_signature"`
	StructureProfile   []ChannelPurityFieldProfile `json:"structure_profile"`
	PromptTokens       int                         `json:"prompt_tokens"`
	CompletionTokens   int                         `json:"completion_tokens"`
	TotalTokens        int                         `json:"total_tokens"`
	Valid              bool                        `json:"valid"`
	ErrorClass         string                      `json:"error_class"`
	ObservedAt         int64                       `json:"observed_at"`
}
