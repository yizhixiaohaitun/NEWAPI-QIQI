package dto

type ChannelPurityGroupMemberRequest struct {
	ChannelID  int  `json:"channel_id"`
	IsBaseline bool `json:"is_baseline"`
}
type ChannelPurityGroupRequest struct {
	Name            string                            `json:"name"`
	Enabled         bool                              `json:"enabled"`
	IntervalMinutes int                               `json:"interval_minutes"`
	Members         []ChannelPurityGroupMemberRequest `json:"members"`
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
