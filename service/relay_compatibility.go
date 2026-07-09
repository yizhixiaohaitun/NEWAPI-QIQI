package service

import "github.com/gin-gonic/gin"

const ginKeyRelayCompatibilityEvents = "relay_compatibility_events"

const (
	RelayCompatibilityEventTypeApplied        = "applied"
	RelayCompatibilityEventTypeRecommendation = "recommendation"
)

type RelayCompatibilityRule struct {
	ID         string
	Key        string
	SettingKey string
}

var ResponsesMissingReasoningItemRule = RelayCompatibilityRule{
	ID:         "QIQI-EC-001",
	Key:        "responses_missing_reasoning_item_retry",
	SettingKey: "qiqi_setting.responses_missing_reasoning_item_retry_enabled",
}

type RelayCompatibilityEvent struct {
	RuleID     string `json:"rule_id"`
	Key        string `json:"key"`
	SettingKey string `json:"setting_key"`
	EventType  string `json:"event_type"`
	Action     string `json:"action"`
	Outcome    string `json:"outcome"`
	ItemID     string `json:"item_id,omitempty"`
	Count      int    `json:"count,omitempty"`
	Retried    bool   `json:"retried,omitempty"`
}

func NewRelayCompatibilityEvent(rule RelayCompatibilityRule, eventType string) RelayCompatibilityEvent {
	return RelayCompatibilityEvent{
		RuleID:     rule.ID,
		Key:        rule.Key,
		SettingKey: rule.SettingKey,
		EventType:  eventType,
	}
}

func RecordRelayCompatibilityEvent(c *gin.Context, event RelayCompatibilityEvent) {
	if c == nil || event.RuleID == "" || event.Key == "" || event.EventType == "" {
		return
	}
	events := make([]RelayCompatibilityEvent, 0, 1)
	if existing, ok := c.Get(ginKeyRelayCompatibilityEvents); ok {
		if typed, ok := existing.([]RelayCompatibilityEvent); ok {
			events = append(events, typed...)
		}
	}
	for _, existing := range events {
		if existing.RuleID == event.RuleID &&
			existing.EventType == event.EventType &&
			existing.Action == event.Action &&
			existing.ItemID == event.ItemID {
			return
		}
	}
	events = append(events, event)
	c.Set(ginKeyRelayCompatibilityEvents, events)
}

func AppendRelayCompatibilityAdminInfo(c *gin.Context, adminInfo map[string]interface{}) {
	if c == nil || adminInfo == nil {
		return
	}
	existing, ok := c.Get(ginKeyRelayCompatibilityEvents)
	if !ok {
		return
	}
	events, ok := existing.([]RelayCompatibilityEvent)
	if !ok || len(events) == 0 {
		return
	}
	adminInfo["compatibility_events"] = events
}
