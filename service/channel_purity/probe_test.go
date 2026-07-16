package channel_purity

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunQuickProbeSuccessfulStructure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/chat/completions", r.URL.Path)
		assert.Equal(t, "Bearer test-secret", r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		_, err := w.Write([]byte(`{"id":"chatcmpl-test","object":"chat.completion","model":"gpt-test","choices":[{"message":{"role":"assistant","content":"OK"}}],"usage":{"prompt_tokens":8,"completion_tokens":1,"total_tokens":9}}`))
		require.NoError(t, err)
	}))
	defer server.Close()

	channel := testChannel(server.URL)
	outcome := RunQuickProbe(context.Background(), channel, "gpt-test")

	assert.Equal(t, model.ChannelPurityConclusionNoObviousRisk, outcome.Conclusion)
	assert.Equal(t, model.ChannelPurityRiskLow, outcome.Risk)
	require.NotNil(t, outcome.Result)
	assert.Equal(t, 200, outcome.Result.HTTPStatus)
	assert.Equal(t, "gpt-test", outcome.Result.DeclaredModel)
	assert.True(t, outcome.Result.HasUsage)
	assert.True(t, outcome.Result.HasOutput)
	assert.NotContains(t, outcome.Result.EvidenceJSON, "test-secret")
}

func TestRunQuickProbeOperationalErrorRemainsUnknown(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":{"type":"rate_limit"}}`))
	}))
	defer server.Close()

	outcome := RunQuickProbe(context.Background(), testChannel(server.URL), "gpt-test")

	assert.Equal(t, "rate_limit", outcome.ErrorClass)
	assert.Equal(t, model.ChannelPurityConclusionUnknown, outcome.Conclusion)
	assert.Equal(t, model.ChannelPurityRiskUnknown, outcome.Risk)
}

func TestRunQuickProbeModelMismatchIsWeakEvidence(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"id":"chatcmpl-test","model":"different-model","choices":[{}],"usage":{"prompt_tokens":8,"completion_tokens":1,"total_tokens":9}}`))
	}))
	defer server.Close()

	outcome := RunQuickProbe(context.Background(), testChannel(server.URL), "gpt-test")

	assert.Equal(t, model.ChannelPurityConclusionUnknown, outcome.Conclusion)
	assert.Equal(t, model.ChannelPurityRiskUnknown, outcome.Risk)
	assert.Contains(t, outcome.Summary, "weak evidence")
}

func TestRunQuickProbeBlocksCrossHostRedirect(t *testing.T) {
	receivedAuthorization := ""
	target := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		receivedAuthorization = r.Header.Get("Authorization")
	}))
	defer target.Close()
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
	}))
	defer source.Close()

	outcome := RunQuickProbe(context.Background(), testChannel(source.URL), "gpt-test")

	assert.Equal(t, "redirect_blocked", outcome.ErrorClass)
	assert.Empty(t, receivedAuthorization)
	assert.Equal(t, model.ChannelPurityConclusionUnknown, outcome.Conclusion)
}

func testChannel(baseURL string) *model.Channel {
	return &model.Channel{
		Id: 1, Type: constant.ChannelTypeOpenAI, Name: "test-channel", Key: "test-secret",
		BaseURL: &baseURL, Models: "gpt-test",
	}
}
