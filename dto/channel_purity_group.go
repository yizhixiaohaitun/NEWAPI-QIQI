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
type ChannelPurityGroupRequest struct {
	Name                 string                                `json:"name"`
	Enabled              bool                                  `json:"enabled"`
	IntervalMinutes      int                                   `json:"interval_minutes"`
	RandomPairingEnabled bool                                  `json:"random_pairing_enabled"`
	ChannelIDs           []int                                 `json:"channel_ids"`
	BaselineChannelID    int                                   `json:"baseline_channel_id"`
	Sampling             ChannelPuritySamplingRequest          `json:"sampling"`
	Members              []ChannelPurityGroupMemberRequest     `json:"members,omitempty"`
	ModelComparisons     []ChannelPurityModelComparisonRequest `json:"model_comparisons"`
}
type ChannelPuritySampleRequest struct {
	GroupID            uint   `json:"group_id"`
	ChannelID          int    `json:"channel_id"`
	ActualModel        string `json:"actual_model"`
	StructureSignature string `json:"structure_signature"`
	PromptTokens       int    `json:"prompt_tokens"`
	CompletionTokens   int    `json:"completion_tokens"`
	TotalTokens        int    `json:"total_tokens"`
	Valid              bool   `json:"valid"`
	ErrorClass         string `json:"error_class"`
	ObservedAt         int64  `json:"observed_at"`
}
