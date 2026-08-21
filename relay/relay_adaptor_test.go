package relay

import (
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	taskdoubao "github.com/QuantumNous/new-api/relay/channel/task/doubao"
	taskseedance "github.com/QuantumNous/new-api/relay/channel/task/seedance"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestGetTaskAdaptorPrefersExplicitRoutePlatform(t *testing.T) {
	gin.SetMode(gin.TestMode)
	context, _ := gin.CreateTestContext(nil)
	context.Set("platform", string(constant.TaskPlatformSeedance))
	context.Set("channel_type", constant.ChannelTypeDoubaoVideo)

	platform := GetTaskPlatform(context)
	assert.Equal(t, constant.TaskPlatform(constant.TaskPlatformSeedance), platform)
	assert.IsType(t, &taskseedance.TaskAdaptor{}, GetTaskAdaptor(platform))
}

func TestGetTaskAdaptorKeepsChannelTypeDispatchWithoutRoutePlatform(t *testing.T) {
	gin.SetMode(gin.TestMode)
	context, _ := gin.CreateTestContext(nil)
	context.Set("channel_type", constant.ChannelTypeDoubaoVideo)

	platform := GetTaskPlatform(context)
	assert.Equal(t, constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeDoubaoVideo)), platform)
	assert.IsType(t, &taskdoubao.TaskAdaptor{}, GetTaskAdaptor(platform))
}

func TestExplicitVideoProtocolOverridesChannelTypeWithoutModelInference(t *testing.T) {
	tests := []struct {
		name     string
		setting  dto.VideoUpstreamProtocol
		platform constant.TaskPlatform
	}{
		{"openai", dto.VideoUpstreamProtocolOpenAI, constant.TaskPlatformOpenAIVideo},
		{"official", dto.VideoUpstreamProtocolSeedanceAsync, constant.TaskPlatformSeedance},
		{"discount", dto.VideoUpstreamProtocolSeedanceDiscount, constant.TaskPlatformSeedanceDiscount},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			context, _ := gin.CreateTestContext(nil)
			context.Request = httptest.NewRequest("POST", "/v1/videos", nil)
			context.Set("channel_type", constant.ChannelTypeDoubaoVideo)
			context.Set("original_model", "not-a-seedance-model")
			common.SetContextKey(context, constant.ContextKeyChannelSetting, dto.ChannelSettings{VideoUpstreamProtocol: test.setting})
			assert.Equal(t, test.platform, GetTaskPlatform(context))
		})
	}
}

func TestChannelDefaultPreservesSoraDispatchEvenForSeedanceModelName(t *testing.T) {
	context, _ := gin.CreateTestContext(nil)
	context.Request = httptest.NewRequest("POST", "/v1/videos", nil)
	context.Set("channel_type", constant.ChannelTypeSora)
	context.Set("original_model", "seedance-2.0")
	common.SetContextKey(context, constant.ContextKeyChannelSetting, dto.ChannelSettings{VideoUpstreamProtocol: dto.VideoUpstreamProtocolDefault})
	platform := GetTaskPlatform(context)
	assert.Equal(t, constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeSora)), platform)
}
