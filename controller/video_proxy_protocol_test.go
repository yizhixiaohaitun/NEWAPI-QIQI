package controller

import (
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
)

func TestPersistedVideoProtocolControlsContentSource(t *testing.T) {
	channel := &model.Channel{Key: "secret", Type: constant.ChannelTypeDoubaoVideo}
	openAI := &model.Task{Platform: constant.TaskPlatformOpenAIVideo, PrivateData: model.TaskPrivateData{UpstreamTaskID: "upstream/id", ResultURL: "https://cdn.example/wrong.mp4"}}
	videoURL, auth, handled := persistedVideoDownloadURL(openAI, channel, "https://provider.example/")
	assert.True(t, handled)
	assert.True(t, auth)
	assert.Equal(t, "https://provider.example/v1/videos/upstream%2Fid/content", videoURL)

	for _, platform := range []constant.TaskPlatform{constant.TaskPlatformSeedance, constant.TaskPlatformSeedanceDiscount} {
		task := &model.Task{Platform: platform, PrivateData: model.TaskPrivateData{UpstreamTaskID: "upstream", ResultURL: "https://cdn.example/result.mp4"}}
		videoURL, auth, handled = persistedVideoDownloadURL(task, channel, "https://provider.example")
		assert.True(t, handled)
		assert.False(t, auth)
		assert.Equal(t, "https://cdn.example/result.mp4", videoURL)
	}

	legacy := &model.Task{Platform: constant.TaskPlatform("40")}
	_, _, handled = persistedVideoDownloadURL(legacy, channel, "https://provider.example")
	assert.False(t, handled)
}
