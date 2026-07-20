package channel_purity

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestObserveResponseDoesNotRetainContent(t *testing.T) {
	body := `{"model":"gpt-4o-2024","choices":[{"message":{"content":"secret@example.com"},"finish_reason":"stop"}],"usage":{"prompt_tokens":4,"completion_tokens":2,"total_tokens":6}}`
	resp := &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(body))}
	var got AnonymousFeatures
	ObserveResponse(resp, FeatureSinkFunc(func(f AnonymousFeatures) { got = f }), 1<<20)
	_, err := io.Copy(io.Discard, resp.Body)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	encoded, err := json.Marshal(got)
	require.NoError(t, err)
	assert.NotContains(t, string(encoded), "secret@example.com")
	assert.Equal(t, 6, got.ProviderUsage.Total)
	assert.Equal(t, "gpt-4o", got.ModelFamily)
}

func TestExtractAnonymousFeaturesRetainsOnlySanitizedPathAndType(t *testing.T) {
	body := []byte(`{"response":{"items":[{"content":"secret-value","score":1,"enabled":true}],"optional":null}}`)
	got := ExtractAnonymousFeatures(200, http.Header{"Content-Type": []string{"application/json"}}, body, false)
	encoded, err := json.Marshal(got.FieldProfiles)
	require.NoError(t, err)
	assert.Contains(t, string(encoded), `"path":"response.items[]","type":"object"`)
	assert.Contains(t, string(encoded), `"path":"response.items[].score","type":"number"`)
	assert.Contains(t, string(encoded), `"path":"response.items[].enabled","type":"boolean"`)
	assert.Contains(t, string(encoded), `"path":"response.optional","type":"null"`)
	assert.NotContains(t, string(encoded), "secret-value")
}

func TestSensitiveLeavesRetainSafePathAndTypeWithoutValues(t *testing.T) {
	body := []byte(`{"choices":[{"message":{"content":"private","reasoning_content":"secret"},"delta":12}]}`)
	got := ExtractAnonymousFeatures(200, http.Header{"Content-Type": []string{"application/json"}}, body, false)
	profiles := map[string]string{}
	for _, profile := range got.FieldProfiles {
		profiles[profile.Path] = profile.Type
	}
	assert.Equal(t, "string", profiles["choices[].message.content"])
	assert.Equal(t, "string", profiles["choices[].message.reasoning_content"])
	assert.Equal(t, "number", profiles["choices[].delta"])
	encoded, err := json.Marshal(got)
	require.NoError(t, err)
	assert.NotContains(t, string(encoded), "private")
	assert.NotContains(t, string(encoded), "secret")
}

func TestExtractSSECodexEvidence(t *testing.T) {
	header := http.Header{"Content-Type": []string{"text/event-stream"}, "X-Reasoning-Included": []string{"true"}}
	body := []byte("event: response.completed\ndata: {\"type\":\"response.completed\",\"response\":{\"model\":\"gpt-5-codex\",\"signature_id\":\"never-retain\",\"usage\":{\"input_tokens\":4,\"output_tokens\":2,\"total_tokens\":6}}}\n\ndata: [DONE]\n")
	got := ExtractAnonymousFeatures(200, header, body, false)
	encoded, _ := json.Marshal(got)
	assert.Equal(t, "sse", got.Protocol)
	assert.True(t, got.HasSignatureID)
	assert.True(t, got.HeaderPresence["x-reasoning-included"])
	assert.Contains(t, got.EventSequence, "response.completed")
	assert.NotContains(t, string(encoded), "never-retain")
}

func TestStructureMetadataPreservesModelAndEventOrder(t *testing.T) {
	encoded := EncodeStructureMetadata(AnonymousFeatures{
		Protocol: "sse", ModelFamily: "gpt-5", EventSequence: []string{"response.created", "response.completed", "done"},
		FinishReasons: []string{"stop"}, HeaderPresence: map[string]bool{"x-request-id": true}, HasSignatureID: true,
	})
	var metadata StructureMetadata
	require.NoError(t, json.Unmarshal([]byte(encoded), &metadata))
	assert.Equal(t, "gpt-5", metadata.ModelFamily)
	assert.Equal(t, []string{"response.created", "response.completed", "done"}, metadata.EventSequence)
	assert.NotContains(t, encoded, "header-value")
}

func TestCaptureRejectsUnsafeAndPairIsOneShotPerRole(t *testing.T) {
	store, err := NewPairStore(time.Minute)
	require.NoError(t, err)
	policy := CapturePolicy{SampleRate: 1, Store: store}
	unsafe := policy.Capture("g", "responses", []byte(`{"model":"gpt-4o","input":[{"type":"input_image","image_url":"x"}]}`), false)
	assert.Equal(t, "sensitive_or_multimedia", unsafe.Reason)
	tool := policy.Capture("g", "chat", []byte(`{"model":"gpt-4o","tools":[{"type":"function","function":{"name":"send_email"}}]}`), false)
	assert.Equal(t, "side_effect_tool", tool.Reason)
	decision := policy.Capture("g", "chat", []byte(`{"model":"gpt-4o","messages":[{"role":"user","content":"hello"}]}`), false)
	require.True(t, decision.Sampled)
	base, err := store.Consume(decision.PairID, PairRoleBaseline)
	require.NoError(t, err)
	assert.Contains(t, string(base.Body), "hello")
	_, err = store.Consume(decision.PairID, PairRoleBaseline)
	assert.Error(t, err)
	_, err = store.Consume(decision.PairID, PairRoleTarget)
	require.NoError(t, err)
	_, err = store.Consume(decision.PairID, PairRoleTarget)
	assert.Error(t, err)
}

func TestDetectionNeverSamples(t *testing.T) {
	store, _ := NewPairStore(time.Minute)
	decision := (CapturePolicy{SampleRate: 1, Store: store}).Capture("g", "chat", []byte(`{"model":"gpt-4o"}`), true)
	assert.False(t, decision.Eligible)
	assert.Equal(t, "detection_request", decision.Reason)
}
