package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetModelRequestForPublicVideosDoesNotInferUpstreamProtocolFromModel(t *testing.T) {
	models := []string{
		"seedance-2.0",
		"seedance-2.0-fast",
		"sd_2.0_discount",
		"sd_2.0_fast_discount",
		"sd_2.0_mini_discount",
		"sd_2.0_special",
		"sd_2.0_fast_special",
		"sd_2.0_mini_special",
		"sora-2",
		"MiniMax-H3",
	}

	for _, model := range models {
		t.Run(model, func(t *testing.T) {
			body := `{"model":"` + model + `","prompt":"test"}`
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", strings.NewReader(body))
			c.Request.Header.Set("Content-Type", "application/json")
			defer common.CleanupBodyStorage(c)

			request, shouldSelectChannel, err := getModelRequest(c)
			require.NoError(t, err)
			require.NotNil(t, request)
			assert.True(t, shouldSelectChannel)
			assert.Equal(t, model, request.Model)
			assert.Empty(t, common.GetContextKeyString(c, "platform"))
			assert.Equal(t, relayconstant.RelayModeVideoSubmit, common.GetContextKeyInt(c, "relay_mode"))
		})
	}
}

func TestGetModelRequestForPublicVideoGenerationsDoesNotInferUpstreamProtocolFromModel(t *testing.T) {
	models := []string{"seedance-2.0", "sd_2.0_discount", "MiniMax-H3"}

	for _, model := range models {
		t.Run(model, func(t *testing.T) {
			body := `{"model":"` + model + `","prompt":"test"}`
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/video/generations", strings.NewReader(body))
			c.Request.Header.Set("Content-Type", "application/json")
			defer common.CleanupBodyStorage(c)

			request, shouldSelectChannel, err := getModelRequest(c)
			require.NoError(t, err)
			require.NotNil(t, request)
			assert.True(t, shouldSelectChannel)
			assert.Equal(t, model, request.Model)
			assert.Empty(t, common.GetContextKeyString(c, "platform"))
			assert.Equal(t, relayconstant.RelayModeVideoSubmit, common.GetContextKeyInt(c, "relay_mode"))
		})
	}
}

func TestGetModelRequestForSingularVideoAlias(t *testing.T) {
	body := `{"model":"seedance-2.0","prompt":"test"}`
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/video", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	defer common.CleanupBodyStorage(c)

	request, shouldSelectChannel, err := getModelRequest(c)
	require.NoError(t, err)
	require.NotNil(t, request)
	assert.True(t, shouldSelectChannel)
	assert.Equal(t, "seedance-2.0", request.Model)
	assert.Empty(t, common.GetContextKeyString(c, "platform"))
	assert.Equal(t, relayconstant.RelayModeVideoSubmit, common.GetContextKeyInt(c, "relay_mode"))
}

func TestGetModelRequestForSeedanceAssets(t *testing.T) {
	tests := []struct {
		name          string
		body          string
		expectedModel string
	}{
		{name: "default model", body: `{"asset_type":"image","url":"https://example.com/a.png"}`, expectedModel: "seedance-2.0"},
		{name: "explicit model", body: `{"model":"seedance-2.0-fast","asset_type":"image","url":"https://example.com/a.png"}`, expectedModel: "seedance-2.0-fast"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/video/assets", strings.NewReader(test.body))
			c.Request.Header.Set("Content-Type", "application/json")
			defer common.CleanupBodyStorage(c)

			request, shouldSelectChannel, err := getModelRequest(c)
			require.NoError(t, err)
			require.NotNil(t, request)
			assert.True(t, shouldSelectChannel)
			assert.Equal(t, test.expectedModel, request.Model)
			assert.Equal(t, string(constant.TaskPlatformSeedance), common.GetContextKeyString(c, "platform"))
			assert.Equal(t, relayconstant.RelayModeVideoSubmit, common.GetContextKeyInt(c, "relay_mode"))
		})
	}
}
