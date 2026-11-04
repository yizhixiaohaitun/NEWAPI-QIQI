package relay

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStripHistoricalClaudeThinking(t *testing.T) {
	cases := []struct {
		name, input, want string
		changed           bool
	}{
		{"history and active tool turn", `{"unknown":123456789012345678901234567890,"messages":[{"role":"user","content":"old"},{"role":"assistant","content":[{"type":"thinking","thinking":"old"},{"type":"redacted_thinking","data":"old"},{"type":"text","text":"answer","extra":1e100}]},{"role":"user","content":[{"type":"text","text":"new"}]},{"role":"assistant","content":[{"type":"thinking","signature":"keep"},{"type":"tool_use","id":"x"}]},{"role":"user","content":[{"type":"tool_result","tool_use_id":"x"}]}]}`, `{"messages":[{"role":"user","content":"old"},{"content":[{"type":"text","text":"answer","extra":1e100}],"role":"assistant"},{"role":"user","content":[{"type":"text","text":"new"}]},{"role":"assistant","content":[{"type":"thinking","signature":"keep"},{"type":"tool_use","id":"x"}]},{"role":"user","content":[{"type":"tool_result","tool_use_id":"x"}]}],"unknown":123456789012345678901234567890}`, true},
		{"no genuine user", `{"messages":[{"role":"assistant","content":[{"type":"thinking"},{"type":"text","text":"x"}]},{"role":"user","content":[{"type":"tool_result"}]}]}`, "", false},
		{"thinking-only historical message", `{"messages":[{"role":"assistant","content":[{"type":"thinking"}]},{"role":"user","content":"new"}]}`, "", false},
		{"ambiguous user content", `{"messages":[{"role":"assistant","content":[{"type":"thinking"},{"type":"text","text":"x"}]},{"role":"user","content":[{}]}]}`, "", false},
		{"malformed", `{`, "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := []byte(tc.input)
			out, changed := stripHistoricalClaudeThinking(in)
			require.Equal(t, tc.changed, changed)
			if !changed {
				assert.Equal(t, in, out, "unchanged input modified")
				return
			}
			assert.Equal(t, tc.want, string(out))
		})
	}
}
