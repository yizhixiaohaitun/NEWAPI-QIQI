package common

import (
	"testing"

	rootcommon "github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
)

func TestRemoveMissingResponsesReasoningItemRemovesOnlyMatchingEmptyItem(t *testing.T) {
	body := []byte(`{
		"model":"gpt-5.5",
		"input":[
			{"role":"user","content":"keep user content"},
			{"type":"reasoning","id":"rs_missing","summary":[],"content":[]},
			{"type":"reasoning","id":"rs_other","summary":[],"content":[]},
			{"type":"function_call","id":"fc_1","name":"lookup","arguments":"{}"}
		]
	}`)

	normalized, removed, err := RemoveMissingResponsesReasoningItem(body, "rs_missing")
	require.NoError(t, err)
	require.Equal(t, 1, removed)
	require.NotContains(t, string(normalized), "rs_missing")
	require.Contains(t, string(normalized), "rs_other")
	require.Contains(t, string(normalized), "keep user content")
	require.Contains(t, string(normalized), "fc_1")
}

func TestRemoveMissingResponsesReasoningItemPreservesRecoverableReasoning(t *testing.T) {
	testCases := []struct {
		name string
		item string
	}{
		{
			name: "encrypted content",
			item: `{"type":"reasoning","id":"rs_missing","summary":[],"content":[],"encrypted_content":"ciphertext"}`,
		},
		{
			name: "summary content",
			item: `{"type":"reasoning","id":"rs_missing","summary":[{"type":"summary_text","text":"keep"}],"content":[]}`,
		},
		{
			name: "reasoning content",
			item: `{"type":"reasoning","id":"rs_missing","summary":[],"content":[{"type":"reasoning_text","text":"keep"}]}`,
		},
		{
			name: "different item type",
			item: `{"type":"item_reference","id":"rs_missing"}`,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			body := []byte(`{"input":[` + testCase.item + `]}`)
			normalized, removed, err := RemoveMissingResponsesReasoningItem(body, "rs_missing")
			require.NoError(t, err)
			require.Zero(t, removed)
			require.JSONEq(t, string(body), string(normalized))
		})
	}
}

func TestRemoveMissingResponsesReasoningItemRejectsInvalidJSON(t *testing.T) {
	body := []byte(`{"input":`)
	normalized, removed, err := RemoveMissingResponsesReasoningItem(body, "rs_missing")
	require.Error(t, err)
	require.Zero(t, removed)
	require.Equal(t, body, normalized)

	var payload map[string]any
	require.Error(t, rootcommon.Unmarshal(normalized, &payload))
}
