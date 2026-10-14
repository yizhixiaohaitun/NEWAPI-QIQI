package dto

import (
	"encoding/json"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/reasoning"
)

// ClaudeSamplingBaseModel strips collection-only suffixes for confirmed 4.7
// proxies without changing the OpenAI protocol or inserting Claude thinking.
func ClaudeSamplingBaseModel(model string) string {
	if !ClaudeOmitsSamplingParameters(model) {
		return model
	}
	model = strings.TrimSuffix(model, "-thinking")
	model, _, _ = reasoning.TrimEffortSuffix(model)
	return model
}

// NormalizeClaudeSamplingJSON handles OpenAI-protocol proxy channels for the
// same known 4.7 capability. Do not introduce Claude-specific protocol fields.
func NormalizeClaudeSamplingJSON(body []byte, fallbackModel string) ([]byte, error) {
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
	if !ClaudeOmitsSamplingParameters(model) {
		return body, nil
	}
	changed := false
	if base := ClaudeSamplingBaseModel(model); base != model {
		if _, hasModel := fields["model"]; hasModel {
			fields["model"], _ = common.Marshal(base)
			changed = true
		}
	}
	for _, key := range []string{"temperature", "top_p", "top_k"} {
		if _, ok := fields[key]; ok {
			delete(fields, key)
			changed = true
		}
	}
	if !changed {
		return body, nil
	}
	return common.Marshal(fields)
}
