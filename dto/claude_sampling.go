package dto

import (
	"encoding/json"
	"github.com/QuantumNous/new-api/common"
)

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
