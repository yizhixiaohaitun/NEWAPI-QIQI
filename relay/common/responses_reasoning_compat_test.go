package common

import (
	"testing"

	rootcommon "github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
)

func TestRemoveMissingResponsesReasoningItemsRemovesAllEmptyReferences(t *testing.T) {
	body := []byte(`{
			"model":"gpt-5.5",
			"input":[
				{"role":"user","content":"keep user content"},
				{"type":"reasoning","id":"rs_missing","summary":[],"content":[]},
				{"type":"reasoning","id":"rs_other","summary":[],"content":[]},
				{"type":"reasoning","id":"legacy_reference","summary":[],"content":[]},
				{"type":"function_call","id":"fc_1","name":"lookup","arguments":"{}"}
			]
		}`)

	normalized, removed, err := RemoveMissingResponsesReasoningItems(body, "rs_missing")
	require.NoError(t, err)
	require.Equal(t, 2, removed)
	require.NotContains(t, string(normalized), "rs_missing")
	require.NotContains(t, string(normalized), "rs_other")
	require.Contains(t, string(normalized), "legacy_reference")
	require.Contains(t, string(normalized), "keep user content")
	require.Contains(t, string(normalized), "fc_1")
}

func TestRemoveMissingResponsesReasoningItemsRequiresReportedEmptyReference(t *testing.T) {
	body := []byte(`{"input":[{"type":"reasoning","id":"rs_other","summary":[],"content":[]}]}`)

	normalized, removed, err := RemoveMissingResponsesReasoningItems(body, "rs_missing")
	require.NoError(t, err)
	require.Zero(t, removed)
	require.JSONEq(t, string(body), string(normalized))
}

func TestRemoveMissingResponsesReasoningItemsPreservesRecoverableReasoning(t *testing.T) {
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
			normalized, removed, err := RemoveMissingResponsesReasoningItems(body, "rs_missing")
			require.NoError(t, err)
			require.Zero(t, removed)
			require.JSONEq(t, string(body), string(normalized))
		})
	}
}

func TestRemoveUndecryptableResponsesReasoningItemsRemovesOnlyEncryptedReasoning(t *testing.T) {
	body := []byte(`{
		"input":[
			{"type":"reasoning","id":"rs_bad","encrypted_content":"gAAAAA-secret","summary":[]},
			{"type":"reasoning","id":"rs_plain","content":[{"type":"reasoning_text","text":"keep"}]},
			{"type":"function_call","id":"fc_1","encrypted_content":"keep-tool-data"},
			{"role":"user","content":"keep user content"}
		]
	}`)

	normalized, removed, err := RemoveUndecryptableResponsesReasoningItems(body)
	require.NoError(t, err)
	require.Equal(t, 1, removed)
	require.NotContains(t, string(normalized), "gAAAAA-secret")
	require.Contains(t, string(normalized), "rs_plain")
	require.Contains(t, string(normalized), "keep-tool-data")
	require.Contains(t, string(normalized), "keep user content")
}

func TestRemoveUndecryptableResponsesReasoningItemsLeavesBodyWithoutEncryptedReasoningUnchanged(t *testing.T) {
	body := []byte(`{"input":[{"type":"reasoning","id":"rs_plain","content":[]}]}`)

	normalized, removed, err := RemoveUndecryptableResponsesReasoningItems(body)
	require.NoError(t, err)
	require.Zero(t, removed)
	require.Equal(t, body, normalized)
}

func TestRemoveMissingResponsesReasoningItemsRejectsInvalidJSON(t *testing.T) {
	body := []byte(`{"input":`)
	normalized, removed, err := RemoveMissingResponsesReasoningItems(body, "rs_missing")
	require.Error(t, err)
	require.Zero(t, removed)
	require.Equal(t, body, normalized)

	var payload map[string]any
	require.Error(t, rootcommon.Unmarshal(normalized, &payload))
}
