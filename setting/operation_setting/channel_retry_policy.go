package operation_setting

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"sync/atomic"

	"github.com/QuantumNous/new-api/common"
)

const (
	ChannelRetryPolicyOption  = "ChannelRetryPolicy"
	DefaultChannelRetryPolicy = "[]"
	maxChannelRetryRules      = 128
)

// StreamHedgeParallelScope describes what the configuration opted into. The
// relay still applies the protocol, channel, permissions and replay checks.
type StreamHedgeParallelScope int

const (
	StreamHedgeParallelOff StreamHedgeParallelScope = iota
	StreamHedgeParallelClaude
	StreamHedgeParallelAll
)

type channelRetryRule struct {
	channelID       int
	status          int
	errorCode       string
	messageContains string
	retry           bool
}

// Only complete, validated snapshots are published. Requests never need the
// option-map lock or a database lookup to make a retry decision.
type channelRetrySnapshot struct {
	rules                 []channelRetryRule
	hedgeDelayMS          int
	hedgeSameChannelFirst bool
	hedgeConfigured       bool
	hedgeModels           []string
	hedgeParallelModels   []string
	hedgeParallelScope    StreamHedgeParallelScope
}

var channelRetryPolicy atomic.Pointer[channelRetrySnapshot]

func parseChannelRetryPolicy(value string) (*channelRetrySnapshot, error) {
	if len(value) > 64*1024 {
		return nil, fmt.Errorf("channel retry policy exceeds 64 KiB")
	}
	var items []map[string]json.RawMessage
	if common.GetJsonType([]byte(value)) != "array" || common.UnmarshalJsonStr(value, &items) != nil {
		return nil, fmt.Errorf("channel retry policy must be a JSON array of rules")
	}
	if len(items) > maxChannelRetryRules+1 {
		return nil, fmt.Errorf("channel retry policy accepts at most %d rules", maxChannelRetryRules)
	}
	snapshot := &channelRetrySnapshot{rules: make([]channelRetryRule, 0, len(items))}
	for i, item := range items {
		if raw, ok := item["stream_hedge"]; ok {
			if len(item) != 1 || snapshot.hedgeConfigured {
				return nil, fmt.Errorf("stream_hedge must be the only field of a single dedicated entry")
			}
			var fields map[string]json.RawMessage
			if common.GetJsonType(raw) != "object" || common.Unmarshal(raw, &fields) != nil {
				return nil, fmt.Errorf("stream_hedge must be an object")
			}
			var parallelScope string
			for key, value := range fields {
				var dst any
				switch key {
				case "delay_ms":
					dst = &snapshot.hedgeDelayMS
				case "retry_same_channel_first":
					dst = &snapshot.hedgeSameChannelFirst
				case "models":
					dst = &snapshot.hedgeModels
				case "parallel_backup_models":
					dst = &snapshot.hedgeParallelModels
				case "parallel_backup_scope":
					dst = &parallelScope
				default:
					return nil, fmt.Errorf("stream_hedge: unknown field %q", key)
				}
				if common.GetJsonType(value) == "null" || common.Unmarshal(value, dst) != nil {
					return nil, fmt.Errorf("stream_hedge: invalid value for %q", key)
				}
			}
			if _, ok := fields["delay_ms"]; !ok || snapshot.hedgeDelayMS < 0 || snapshot.hedgeDelayMS > 300_000 {
				return nil, fmt.Errorf("stream_hedge.delay_ms must be an integer between 0 and 300000")
			}
			for _, modelList := range []struct {
				name   string
				models []string
			}{{"models", snapshot.hedgeModels}, {"parallel_backup_models", snapshot.hedgeParallelModels}} {
				for i, model := range modelList.models {
					model = strings.TrimSpace(model)
					if model == "" {
						return nil, fmt.Errorf("stream_hedge.%s must contain non-blank model names", modelList.name)
					}
					modelList.models[i] = model
				}
			}
			if _, hasScope := fields["parallel_backup_scope"]; hasScope {
				if _, hasModels := fields["parallel_backup_models"]; hasModels {
					return nil, fmt.Errorf("stream_hedge.parallel_backup_scope and parallel_backup_models cannot be combined")
				}
				switch parallelScope {
				case "claude":
					snapshot.hedgeParallelScope = StreamHedgeParallelClaude
				case "all":
					snapshot.hedgeParallelScope = StreamHedgeParallelAll
				default:
					return nil, fmt.Errorf("stream_hedge.parallel_backup_scope must be claude or all")
				}
			}
			if (len(snapshot.hedgeParallelModels) != 0 || snapshot.hedgeParallelScope != StreamHedgeParallelOff) && !snapshot.hedgeSameChannelFirst {
				return nil, fmt.Errorf("stream_hedge parallel backups require retry_same_channel_first to be true")
			}
			if snapshot.hedgeModels != nil {
				for _, model := range snapshot.hedgeParallelModels {
					if !slices.Contains(snapshot.hedgeModels, model) {
						return nil, fmt.Errorf("stream_hedge.parallel_backup_models must be a subset of models when both are provided")
					}
				}
			}
			snapshot.hedgeConfigured = true
			continue
		}
		var rule channelRetryRule
		var action string
		for key, raw := range item {
			var dst any
			switch key {
			case "channel_id":
				dst = &rule.channelID
			case "status":
				dst = &rule.status
			case "error_code":
				dst = &rule.errorCode
			case "message_contains":
				dst = &rule.messageContains
			case "action":
				dst = &action
			default:
				return nil, fmt.Errorf("channel retry rule %d: unknown field %q", i+1, key)
			}
			if common.GetJsonType(raw) == "null" || common.Unmarshal(raw, dst) != nil {
				return nil, fmt.Errorf("channel retry rule %d: invalid value for %q", i+1, key)
			}
		}
		rule.errorCode = strings.ToLower(strings.TrimSpace(rule.errorCode))
		rule.messageContains = strings.ToLower(strings.TrimSpace(rule.messageContains))
		if _, ok := item["channel_id"]; ok && rule.channelID <= 0 {
			return nil, fmt.Errorf("channel retry rule %d: channel_id must be a positive integer", i+1)
		}
		if _, ok := item["status"]; ok && (rule.status < 400 || rule.status > 599) {
			return nil, fmt.Errorf("channel retry rule %d: status must be between 400 and 599", i+1)
		}
		if _, ok := item["error_code"]; ok && rule.errorCode == "" {
			return nil, fmt.Errorf("channel retry rule %d: error_code must not be blank", i+1)
		}
		if _, ok := item["message_contains"]; ok && rule.messageContains == "" {
			return nil, fmt.Errorf("channel retry rule %d: message_contains must not be blank", i+1)
		}
		if rule.status == 0 && rule.errorCode == "" && rule.messageContains == "" {
			return nil, fmt.Errorf("channel retry rule %d: specify status, error_code or message_contains", i+1)
		}
		switch action {
		case "retry":
			rule.retry = true
		case "stop":
		default:
			return nil, fmt.Errorf("channel retry rule %d: action must be retry or stop", i+1)
		}
		snapshot.rules = append(snapshot.rules, rule)
	}
	if len(snapshot.rules) > maxChannelRetryRules {
		return nil, fmt.Errorf("channel retry policy accepts at most %d rules", maxChannelRetryRules)
	}
	return snapshot, nil
}

func ValidateChannelRetryPolicy(value string) error {
	_, err := parseChannelRetryPolicy(value)
	return err
}

func SetChannelRetryPolicy(value string) error {
	snapshot, err := parseChannelRetryPolicy(value)
	if err != nil {
		return err
	}
	channelRetryPolicy.Store(snapshot)
	return nil
}

// GetChannelRetryStreamHedge resolves the JSON entry for the requested model.
// An explicit disable or a model outside the list must not fall back to the
// legacy startup setting. Only an absent JSON entry allows that fallback.
func GetChannelRetryStreamHedge(model string) (delayMS int, sameChannelFirst bool, parallelScope StreamHedgeParallelScope, configured bool) {
	snapshot := channelRetryPolicy.Load()
	if snapshot == nil || !snapshot.hedgeConfigured {
		return 0, false, StreamHedgeParallelOff, false
	}
	if snapshot.hedgeModels != nil && !slices.Contains(snapshot.hedgeModels, model) {
		return 0, false, StreamHedgeParallelOff, true
	}
	parallelScope = snapshot.hedgeParallelScope
	if slices.Contains(snapshot.hedgeParallelModels, model) {
		parallelScope = StreamHedgeParallelClaude
	}
	return snapshot.hedgeDelayMS, snapshot.hedgeSameChannelFirst, parallelScope, true
}

// MatchChannelRetryPolicy returns the first matching action. All fields on a
// rule must match. A miss leaves the original keyword/status policy in charge.
func MatchChannelRetryPolicy(channelID, status int, errorCode, message string) (retry, matched bool) {
	snapshot := channelRetryPolicy.Load()
	if snapshot == nil || len(snapshot.rules) == 0 || status < 400 || status > 599 {
		return false, false
	}
	errorCode = strings.ToLower(strings.TrimSpace(errorCode))
	message = strings.ToLower(message)
	for _, rule := range snapshot.rules {
		if rule.channelID != 0 && rule.channelID != channelID || rule.status != 0 && rule.status != status ||
			rule.errorCode != "" && rule.errorCode != errorCode || rule.messageContains != "" && !strings.Contains(message, rule.messageContains) {
			continue
		}
		return rule.retry, true
	}
	return false, false
}
