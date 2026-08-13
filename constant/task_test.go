package constant

import (
	"strconv"
	"testing"
)

func TestIsVideoTaskPlatform(t *testing.T) {
	videoChannelTypes := []int{
		ChannelTypeAli,
		ChannelTypeKling,
		ChannelTypeJimeng,
		ChannelTypeVertexAi,
		ChannelTypeVidu,
		ChannelTypeDoubaoVideo,
		ChannelTypeVolcEngine,
		ChannelTypeSora,
		ChannelTypeOpenAI,
		ChannelTypeGemini,
		ChannelTypeMiniMax,
	}

	if !IsVideoTaskPlatform(TaskPlatformSeedance) {
		t.Fatal("Seedance must be classified as a video task platform")
	}
	for _, channelType := range videoChannelTypes {
		platform := TaskPlatform(strconv.Itoa(channelType))
		if !IsVideoTaskPlatform(platform) {
			t.Errorf("channel type %d must be classified as a video task platform", channelType)
		}
	}

	for _, platform := range []TaskPlatform{TaskPlatformSuno, TaskPlatformMidjourney, "", "unknown", "999999"} {
		if IsVideoTaskPlatform(platform) {
			t.Errorf("platform %q must not be classified as video", platform)
		}
	}
}
