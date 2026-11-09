package relayconvert

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestCanConvertChatToResponses(t *testing.T) {
	cases := []struct {
		body string
		want bool
	}{
		{`{"model":"m","messages":[{"role":"user","content":"hello"}]}`, true},
		{`{"model":"m","messages":[{"role":"assistant","content":null,"tool_calls":[{"id":"c","type":"function","function":{"name":"f","arguments":"{}"}}]},{"role":"tool","tool_call_id":"c","content":"ok"}]}`, true},
		{`{"model":"m","messages":[{"role":"user","content":"hello"}],"stop":"x"}`, false},
		{`{"model":"m","messages":[{"role":"user","content":"hello","name":"x"}]}`, false},
		{`{"model":"m","messages":[{"role":"user","content":[{"type":"image_url","image_url":{"url":"x","detail":"high"}}]}]}`, false},
		{`{"model":"m","messages":[{"role":"user","content":"hi"}],"tools":[{"type":"function","function":{"name":"f","strict":true}}]}`, false},
		{`{"model":"m","messages":[{"role":"user","content":"hi"}],"n":2}`, false},
		{`{"model":"m","messages":[{"role":"user","content":"hi"}],"unknown":false}`, false},
		{`{"model":"m","messages":[{"role":"user","content":"hi"},{"role":"system","content":"late"}]}`, false},
	}
	for _, tc := range cases {
		assert.Equal(t, tc.want, CanConvertChatToResponses([]byte(tc.body)), tc.body)
	}
}
