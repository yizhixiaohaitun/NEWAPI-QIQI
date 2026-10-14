package dto

import (
	"encoding/json"
	"regexp"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/reasoning"
)

var adaptiveClaudeModel = regexp.MustCompile(`^(?:us\.|eu\.|apac\.|jp\.|au\.|global\.)?(?:anthropic\.)?claude-opus-(4-7|4-8|5-5)(?:-(?:[0-9]{8}|v[0-9]+))?(?:@(?:[0-9]{8}|[0-9]+)|:[0-9]+)?$`)

// ClaudeRequiresAdaptiveThinking is an explicit compatibility list, not a
// prediction about future model versions. Opus 5.5 enabled AND disabled are
// rejected by the supplied Bedrock ValidationExceptions; 4.7/4.8 retain
// their existing disabled support.
func ClaudeRequiresAdaptiveThinking(model string) bool {
	return claudeAdaptiveVersion(model) != ""
}

func claudeAdaptiveVersion(model string) string {
	model = strings.TrimSuffix(model, "-thinking")
	model, _, _ = reasoning.TrimEffortSuffix(model)
	if match := adaptiveClaudeModel.FindStringSubmatch(model); len(match) > 1 {
		// jp/au inference-profile aliases are confirmed only for 4.7.
		if (strings.HasPrefix(model, "jp.") || strings.HasPrefix(model, "au.")) && match[1] != "4-7" {
			return ""
		}
		return match[1]
	}
	return ""
}

// ClaudeOmitsSamplingParameters is deliberately limited to Opus 4.7, including
// dated/Bedrock/Vertex aliases and known effort suffixes. Official capability:
// https://docs.aws.amazon.com/bedrock/latest/userguide/model-card-anthropic-claude-opus-4-7.html
// No inference is made about other or future models.
func ClaudeOmitsSamplingParameters(model string) bool {
	return claudeAdaptiveVersion(model) == "4-7"
}

// NormalizeClaudeAdaptiveThinking also removes deprecated sampling on 4.7, even
// without thinking or with disabled thinking. It is called only when the SK
// compatibility collection is enabled. It preserves adaptive and 4.7/4.8 disabled, plus
// existing output_config extensions. Effort is a soft hint, NOT an equivalent
// token budget; thresholds reuse this project's legacy effort budgets.
func NormalizeClaudeAdaptiveThinking(req *ClaudeRequest, effort string) error {
	if req == nil || !ClaudeRequiresAdaptiveThinking(req.Model) {
		return nil
	}
	if ClaudeOmitsSamplingParameters(req.Model) {
		req.Temperature, req.TopP, req.TopK = nil, nil, nil
	}
	// Disabled has no equivalent on 5.5. Low reuses this project's effort
	// vocabulary to best preserve the less-thinking intent, NOT to turn it off.
	// Choose it before suffix defaults; explicit effort hints remain authoritative.
	migrateDisabled := req.Thinking != nil && req.Thinking.Type == "disabled" && claudeAdaptiveVersion(req.Model) == "5-5"
	if migrateDisabled && (effort == "" || effort == "none") {
		effort = "low"
	}
	base, suffixEffort, suffix := reasoning.TrimEffortSuffix(req.Model)
	thinkingSuffix := strings.HasSuffix(req.Model, "-thinking")
	if suffix || thinkingSuffix {
		if thinkingSuffix {
			base = strings.TrimSuffix(req.Model, "-thinking")
		}
		req.Model = base
		if req.Thinking == nil {
			req.Thinking = &Thinking{Type: "adaptive"}
		}
		if effort == "" {
			effort = suffixEffort
			if effort == "" {
				effort = "high"
			}
		}
		// Preserve existing 4.7/4.8 suffix behavior only. No assumptions about
		// Opus 5.5 sampling, display or forced tool choice support.
		if strings.Contains(base, "claude-opus-4-7") || strings.Contains(base, "claude-opus-4-8") {
			if req.Thinking.Type != "disabled" {
				req.Thinking.Display = "summarized"
				req.Temperature = nil
				req.TopP = nil
				req.TopK = nil
			}
		}
	}
	if req.Thinking == nil {
		return nil
	}
	if req.Thinking.Type == "disabled" && !migrateDisabled {
		return nil
	}
	if req.Thinking.Type != "enabled" && req.Thinking.Type != "adaptive" && !migrateDisabled {
		return nil
	}
	migrated := req.Thinking.Type == "enabled"
	if effort == "" && migrated {
		effort = "high"
		if budget := req.Thinking.GetBudgetTokens(); budget > 0 {
			if budget <= 1280 {
				effort = "low"
			} else if budget <= 2048 {
				effort = "medium"
			}
		}
	}
	var output map[string]json.RawMessage
	if len(req.OutputConfig) > 0 && string(req.OutputConfig) != "null" {
		if err := common.Unmarshal(req.OutputConfig, &output); err != nil {
			return err
		}
	}
	if effort != "" {
		if output == nil {
			output = map[string]json.RawMessage{}
		}
		if _, exists := output["effort"]; !exists {
			value, err := common.Marshal(effort)
			if err != nil {
				return err
			}
			output["effort"] = value
			encoded, err := common.Marshal(output)
			if err != nil {
				return err
			}
			req.OutputConfig = encoded
		}
	}
	thinking := *req.Thinking
	thinking.Type = "adaptive"
	thinking.BudgetTokens = nil
	req.Thinking = &thinking
	return nil
}

// NormalizeClaudeAdaptiveJSON protects the final non-passthrough body AFTER
// parameter overrides. Only known thinking/output_config and deprecated 4.7
// sampling fields are patched: provider
// extensions, message/tool content and exact numeric values stay intact.
func NormalizeClaudeAdaptiveJSON(body []byte, fallbackModel string) ([]byte, error) {
	var fields map[string]json.RawMessage
	if err := common.Unmarshal(body, &fields); err != nil {
		return nil, err
	}
	model := fallbackModel
	if raw, ok := fields["model"]; ok {
		if err := common.Unmarshal(raw, &model); err != nil {
			return nil, err
		}
	}
	if !ClaudeRequiresAdaptiveThinking(model) {
		return body, nil
	}
	changed := false
	if ClaudeOmitsSamplingParameters(model) {
		for _, name := range []string{"temperature", "top_p", "top_k"} {
			if _, exists := fields[name]; exists {
				delete(fields, name)
				changed = true
			}
		}
	}
	unchangedThinking := func() ([]byte, error) {
		if changed {
			return common.Marshal(fields)
		}
		return body, nil
	}
	raw, ok := fields["thinking"]
	if !ok || string(raw) == "null" {
		return unchangedThinking()
	}
	var thinking Thinking
	if err := common.Unmarshal(raw, &thinking); err != nil {
		return nil, err
	}
	if thinking.Type != "enabled" && thinking.Type != "adaptive" &&
		!(thinking.Type == "disabled" && claudeAdaptiveVersion(model) == "5-5") {
		return unchangedThinking()
	}
	req := &ClaudeRequest{Model: model, Thinking: &thinking, OutputConfig: fields["output_config"]}
	if err := NormalizeClaudeAdaptiveThinking(req, ""); err != nil {
		return nil, err
	}
	var tf map[string]json.RawMessage
	if err := common.Unmarshal(raw, &tf); err != nil {
		return nil, err
	}
	delete(tf, "budget_tokens")
	tf["type"] = json.RawMessage(`"adaptive"`)
	value, err := common.Marshal(tf)
	if err != nil {
		return nil, err
	}
	fields["thinking"] = value
	if len(req.OutputConfig) > 0 {
		fields["output_config"] = req.OutputConfig
	}
	if req.Model != model {
		if _, hasModel := fields["model"]; hasModel {
			fields["model"], err = common.Marshal(req.Model)
			if err != nil {
				return nil, err
			}
		}
	}
	return common.Marshal(fields)
}
