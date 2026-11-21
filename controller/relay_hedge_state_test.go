package controller

import (
	"testing"

	"github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/require"
)

func TestStreamRaceKeepsStatefulRequestsOnOrdinaryPath(t *testing.T) {
	for _, tc := range []struct {
		name     string
		format   types.RelayFormat
		request  string
		eligible bool
	}{
		{"responses stateless", types.RelayFormatOpenAIResponses, `{"store":false,"input":"hello","tools":[{"type":"function"}]}`, true},
		{"responses implicit store", types.RelayFormatOpenAIResponses, `{"input":"hello"}`, false},
		{"responses previous response", types.RelayFormatOpenAIResponses, `{"store":false,"previous_response_id":"resp_prior"}`, false},
		{"responses reference item", types.RelayFormatOpenAIResponses, `{"store":false,"input":[{"type":"item_reference","id":"msg_prior"}]}`, false},
		{"responses provider tool", types.RelayFormatOpenAIResponses, `{"store":false,"tools":[{"type":"mcp"}]}`, false},
		{"chat function tool", types.RelayFormatOpenAI, `{"tools":[{"type":"function"}]}`, true},
		{"chat provider tool", types.RelayFormatOpenAI, `{"tools":[{"type":"web_search"}]}`, false},
		{"chat stored", types.RelayFormatOpenAI, `{"store":true}`, false},
		{"gemini function tool", types.RelayFormatGemini, `{"tools":[{"functionDeclarations":[]}]}`, true},
		{"gemini provider tool", types.RelayFormatGemini, `{"tools":[{"codeExecution":{}}]}`, false},
		{"gemini cached state", types.RelayFormatGemini, `{"cachedContent":"cachedContents/state"}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) { require.Equal(t, tc.eligible, statelessStreamRace(tc.format, []byte(tc.request))) })
	}
}
