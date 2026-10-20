package dto

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLunaTemperatureJSON(t *testing.T) {
	for _, tc := range []struct {
		name, body, fallback, want string
	}{
		{"non-default", `{"model":"gpt-6-luna","temperature":0.3,"top_p":0,"top_k":7,"messages":[{"role":"system","content":"translator instructions"},{"role":"user","content":"text"}],"tools":[{"type":"function"}]}`, "", `{"model":"gpt-6-luna","top_p":0,"top_k":7,"messages":[{"role":"system","content":"translator instructions"},{"role":"user","content":"text"}],"tools":[{"type":"function"}]}`},
		{"zero", `{"model":"gpt-6-luna","temperature":0}`, "", `{"model":"gpt-6-luna"}`},
		{"fallback", `{"temperature":0.3}`, "gpt-6-luna", `{}`},
		{"default", `{ "model":"gpt-6-luna", "temperature":1.0 }`, "", ""},
		{"absent", `{ "model":"gpt-6-luna" }`, "", ""},
		{"null", `{ "model":"gpt-6-luna", "temperature":null }`, "", ""},
		{"mapped-away", `{ "model":"gpt-6-other", "temperature":0.3 }`, "gpt-6-luna", ""},
		{"no-prefix", `{ "model":"gpt-6-luna-preview", "temperature":0.3 }`, "", ""},
		{"no-case-folding", `{ "model":"GPT-6-LUNA", "temperature":0.3 }`, "", ""},
		{"no-suffix", `{ "model":"gpt-6-luna-high", "temperature":0.3 }`, "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := NormalizeLunaTemperatureJSON([]byte(tc.body), tc.fallback)
			require.NoError(t, err)
			if tc.want == "" {
				assert.Equal(t, tc.body, string(got), "no-op preserves exact bytes")
			} else {
				assert.JSONEq(t, tc.want, string(got))
			}
		})
	}
}
