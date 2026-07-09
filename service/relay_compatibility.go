package service

import "github.com/gin-gonic/gin"

const ginKeyRelayCompatibilityEvents = "relay_compatibility_events"

type RelayCompatibilityEvent struct {
	Key     string `json:"key"`
	Action  string `json:"action"`
	Outcome string `json:"outcome"`
	ItemID  string `json:"item_id,omitempty"`
	Count   int    `json:"count,omitempty"`
	Retried bool   `json:"retried,omitempty"`
}

func RecordRelayCompatibilityEvent(c *gin.Context, event RelayCompatibilityEvent) {
	if c == nil || event.Key == "" {
		return
	}
	events := make([]RelayCompatibilityEvent, 0, 1)
	if existing, ok := c.Get(ginKeyRelayCompatibilityEvents); ok {
		if typed, ok := existing.([]RelayCompatibilityEvent); ok {
			events = append(events, typed...)
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
