package operation_setting

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync/atomic"

	"github.com/QuantumNous/new-api/common"
)

const (
	ChannelRetryPolicyOption  = "ChannelRetryPolicy"
	DefaultChannelRetryPolicy = "[]"
	maxChannelRetryRules      = 128
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
var channelRetryPolicy atomic.Pointer[[]channelRetryRule]

func parseChannelRetryPolicy(value string) ([]channelRetryRule, error) {
	if len(value) > 64*1024 {
		return nil, fmt.Errorf("channel retry policy exceeds 64 KiB")
	}
	var items []map[string]json.RawMessage
	if common.GetJsonType([]byte(value)) != "array" || common.UnmarshalJsonStr(value, &items) != nil {
		return nil, fmt.Errorf("channel retry policy must be a JSON array of rules")
	}
	if len(items) > maxChannelRetryRules {
		return nil, fmt.Errorf("channel retry policy accepts at most %d rules", maxChannelRetryRules)
	}
	rules := make([]channelRetryRule, 0, len(items))
	for i, item := range items {
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
		rules = append(rules, rule)
	}
	return rules, nil
}

func ValidateChannelRetryPolicy(value string) error {
	_, err := parseChannelRetryPolicy(value)
	return err
}

func SetChannelRetryPolicy(value string) error {
	rules, err := parseChannelRetryPolicy(value)
	if err != nil {
		return err
	}
	channelRetryPolicy.Store(&rules)
	return nil
}

// MatchChannelRetryPolicy returns the first matching action. All fields on a
// rule must match. A miss leaves the original keyword/status policy in charge.
func MatchChannelRetryPolicy(channelID, status int, errorCode, message string) (retry, matched bool) {
	rules := channelRetryPolicy.Load()
	if rules == nil || len(*rules) == 0 || status < 400 || status > 599 {
		return false, false
	}
	errorCode = strings.ToLower(strings.TrimSpace(errorCode))
	message = strings.ToLower(message)
	for _, rule := range *rules {
		if rule.channelID != 0 && rule.channelID != channelID || rule.status != 0 && rule.status != status ||
			rule.errorCode != "" && rule.errorCode != errorCode || rule.messageContains != "" && !strings.Contains(message, rule.messageContains) {
			continue
		}
		return rule.retry, true
	}
	return false, false
}
