package dto

import (
	"encoding/json"

	"github.com/QuantumNous/new-api/common"
)

// NormalizeLunaTemperatureJSON applies only the confirmed gpt-6-luna temperature
// restriction. Call after model mapping and parameter overrides, outside raw
// passthrough, and only when the SK request compatibility collection is enabled.
// Other sampling parameters and the default temperature (1) remain untouched.
func NormalizeLunaTemperatureJSON(body []byte, fallbackModel string) ([]byte, error) {
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
	if model != "gpt-6-luna" {
		return body, nil
	}
	raw, ok := fields["temperature"]
	if !ok {
		return body, nil
	}
	var temperature *float64
	if err := common.Unmarshal(raw, &temperature); err != nil {
		return nil, err
	}
	if temperature == nil || *temperature == 1 {
		return body, nil
	}
	delete(fields, "temperature")
	return common.Marshal(fields)
}
