package controller

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildChannelPurityInspectionItems(t *testing.T) {
	channels := []*model.Channel{
		{Id: 2, Models: " model-b,model-a,model-b, "},
		{Id: 3, Models: ""},
	}

	items, skipped := buildChannelPurityInspectionItems(channels)

	require.Len(t, items, 2)
	assert.Equal(t, 1, skipped)
	assert.Equal(t, 2, items[0].channel.Id)
	assert.Equal(t, "model-a", items[0].model)
	assert.Equal(t, "model-b", items[1].model)
}

func TestPurityEndpointTypeCoversProtocolFamilies(t *testing.T) {
	assert.Equal(t, string(constant.EndpointTypeAnthropic), purityEndpointType(&model.Channel{Type: constant.ChannelTypeAnthropic}, "claude-sonnet-4"))
	assert.Equal(t, string(constant.EndpointTypeGemini), purityEndpointType(&model.Channel{Type: constant.ChannelTypeGemini}, "gemini-2.5-pro"))
	assert.Equal(t, string(constant.EndpointTypeEmbeddings), purityEndpointType(&model.Channel{}, "text-embedding-3-small"))
	assert.Equal(t, string(constant.EndpointTypeJinaRerank), purityEndpointType(&model.Channel{}, "bge-reranker-v2"))
}

func TestApplyChannelTestProbeKeepsOperationalFailureUnknown(t *testing.T) {
	scan := &model.ChannelPurityScan{}
	applyChannelTestProbe(scan, testResult{localErr: errors.New("unauthorized"), httpStatus: http.StatusUnauthorized})

	assert.Equal(t, model.ChannelPurityStatusFailed, scan.Status)
	assert.Equal(t, model.ChannelPurityRiskUnknown, scan.Risk)
	assert.Equal(t, "authentication_error", scan.ErrorClass)

	scan = &model.ChannelPurityScan{}
	applyChannelTestProbe(scan, testResult{localErr: context.DeadlineExceeded, httpStatus: http.StatusGatewayTimeout})
	assert.Equal(t, model.ChannelPurityStatusFailed, scan.Status)
	assert.Equal(t, model.ChannelPurityRiskUnknown, scan.Risk)
	assert.Equal(t, "timeout", scan.ErrorClass)
}

func TestApplyChannelTestProbeDetectsOutputAndIdentityEvidence(t *testing.T) {
	scan := &model.ChannelPurityScan{}
	applyChannelTestProbe(scan, testResult{
		responseBody: []byte(`{"model":"gpt-4o","choices":[{"message":{"content":"OK"}}]}`),
		httpStatus:   http.StatusOK,
		protocol:     types.RelayFormatOpenAI,
		mappedModel:  "gpt-4o",
		usagePresent: true,
	})

	assert.Equal(t, model.ChannelPurityStatusCompleted, scan.Status)
	assert.Equal(t, model.ChannelPurityRiskLow, scan.Risk)
	assert.Equal(t, 100, scan.Coverage)
}
