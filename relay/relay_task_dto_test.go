package relay

import (
	"encoding/json"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTaskModel2DtoExposesDetailsWithoutPrivateSecrets(t *testing.T) {
	task := &model.Task{
		TaskID:     "task_public",
		Platform:   "seedance",
		ChannelId:  12,
		Action:     "GENERATE",
		Status:     model.TaskStatusSuccess,
		SubmitTime: 100,
		StartTime:  101,
		FinishTime: 125,
		Progress:   "100%",
		Properties: model.Properties{
			Input:              "two characters running",
			ReferenceResources: []string{"https://cdn.example/ref.png"},
			OriginModelName:    "seedance-2.0",
		},
		PrivateData: model.TaskPrivateData{
			Key:       "channel-secret-key",
			ResultURL: "https://cdn.example/result.mp4",
		},
	}

	details := TaskModel2Dto(task)
	assert.Equal(t, "https://cdn.example/result.mp4", details.ResultURL)
	assert.Equal(t, task.Properties, details.Properties)

	encoded, err := json.Marshal(details)
	require.NoError(t, err)
	assert.NotContains(t, string(encoded), "channel-secret-key")
	assert.NotContains(t, string(encoded), "private_data")
}

func TestTaskModel2DtoUsesStableContentURLForCompletedOpenAIVideo(t *testing.T) {
	originalServerAddress := system_setting.ServerAddress
	system_setting.ServerAddress = "https://newapi.example"
	t.Cleanup(func() { system_setting.ServerAddress = originalServerAddress })

	task := &model.Task{
		TaskID:   "task_public",
		Platform: constant.TaskPlatformOpenAIVideo,
		Status:   model.TaskStatusSuccess,
		PrivateData: model.TaskPrivateData{
			ResultURL: "https://provider.example/expired.mp4?signature=short-lived",
		},
	}

	details := TaskModel2Dto(task)

	expectedSignature := common.GenerateHMAC("video-content:task_public")
	assert.Equal(t, "https://newapi.example/v1/videos/task_public/content?sig="+expectedSignature, details.ResultURL)
}
