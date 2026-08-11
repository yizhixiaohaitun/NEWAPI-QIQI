package relay

import (
	"strconv"
	"testing"

	"github.com/QuantumNous/new-api/constant"
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
