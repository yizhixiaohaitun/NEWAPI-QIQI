package relay

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNormalizeSeedanceAssetType(t *testing.T) {
	tests := []struct {
		input      string
		upstream   string
		normalized string
		valid      bool
	}{
		{input: "image", upstream: "Image", normalized: "image", valid: true},
		{input: "Video", upstream: "Video", normalized: "video", valid: true},
		{input: " audio ", upstream: "Audio", normalized: "audio", valid: true},
		{input: "document", valid: false},
	}
	for _, test := range tests {
		t.Run(test.input, func(t *testing.T) {
			upstream, normalized, err := normalizeSeedanceAssetType(test.input)
			if !test.valid {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, test.upstream, upstream)
			assert.Equal(t, test.normalized, normalized)
		})
	}
}

func TestValidateSeedanceAssetURL(t *testing.T) {
	require.NoError(t, validateSeedanceAssetURL("https://example.com/person.png"))
	assert.Error(t, validateSeedanceAssetURL("data:image/png;base64,AAAA"))
	assert.Error(t, validateSeedanceAssetURL("/tmp/person.png"))
	assert.Error(t, validateSeedanceAssetURL("ftp://example.com/person.png"))
}

func TestRelaySeedanceAssetRequestUsesProviderContract(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, seedanceAssetUploadPath, r.URL.Path)
		assert.Equal(t, "Bearer upstream-key", r.Header.Get("Authorization"))
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

		var request seedanceAssetUpstreamRequest
		require.NoError(t, common.DecodeJson(r.Body, &request))
		assert.Equal(t, seedanceAssetUpstreamRequest{
			AssetType: "Image",
			URL:       "https://example.com/a.png",
			Name:      "reference",
		}, request)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"assetId":"asset-123","assetType":"Image","url":"https://example.com/a.png","status":"PROCESSING","name":"reference","errorMessage":null}`))
	}))
	defer server.Close()

	info := &relaycommon.RelayInfo{
		UpstreamContext: context.Background(),
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelBaseUrl: "http://provider.example",
			ApiKey:         "upstream-key",
			ChannelSetting: dto.ChannelSettings{Proxy: server.URL},
		},
	}
	resp, err := relaySeedanceAssetRequest(info, seedanceAssetUploadPath, seedanceAssetUpstreamRequest{
		AssetType: "Image",
		URL:       "https://example.com/a.png",
		Name:      "reference",
	})
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestParseSeedanceAssetResponseUsesOpenAIStyleEnvelope(t *testing.T) {
	upstream := `{"assetId":"asset-123","assetType":"Image","url":"https://example.com/a.png","status":"PROCESSING","name":"reference","errorMessage":null}`
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(upstream)),
	}

	result, taskErr := parseSeedanceAssetResponse(resp)
	require.Nil(t, taskErr)
	require.NotNil(t, result)
	assert.Equal(t, "asset-123", result.ID)
	assert.Equal(t, "video.asset", result.Object)
	assert.Equal(t, "assetId://asset-123", result.URI)
	assert.Equal(t, "image", result.AssetType)
	assert.Equal(t, "processing", result.Status)
	assert.Nil(t, result.Error)

	encoded, err := common.Marshal(result)
	require.NoError(t, err)
	assert.JSONEq(t, `{"id":"asset-123","object":"video.asset","uri":"assetId://asset-123","asset_type":"image","url":"https://example.com/a.png","status":"processing","name":"reference","error":null}`, string(encoded))
}
