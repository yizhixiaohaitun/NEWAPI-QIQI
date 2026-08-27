package controller

import (
	"encoding/json"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTaskToDetailDtoReturnsSanitizedNormalizedSnapshot(t *testing.T) {
	task := &model.Task{
		TaskID:    "task_public",
		Platform:  constant.TaskPlatform("seedance"),
		ChannelId: 12,
		Status:    model.TaskStatusInProgress,
		Progress:  "50%",
		Properties: model.Properties{
			Input:              "city run",
			ReferenceResources: []string{"https://cdn.example/ref.png?api_key=secret"},
			RequestSnapshot:    json.RawMessage(`{"model":"seedance-2.0","input":{"prompt":"city run","duration":5,"authorization":"Bearer hidden"}}`),
			OriginModelName:    "seedance-2.0",
		},
		Data: json.RawMessage(`{"status":"processing","cookie":"private"}`),
	}

	detail := taskToDetailDto(task, false)
	require.NotNil(t, detail)
	assert.Equal(t, "normalized_upstream_request", detail.DetailSource)
	assert.Equal(t, 0, detail.ChannelId)
	assert.Equal(t, "50%", detail.Progress)
	assert.NotContains(t, string(detail.RequestSnapshot), "hidden")
	assert.NotContains(t, string(detail.Data), "private")
	assert.Contains(t, string(detail.RequestSnapshot), "[REDACTED]")
	assert.Contains(t, detail.Properties.(model.Properties).ReferenceResources[0], "%5BREDACTED%5D")
	assert.Contains(t, task.Properties.ReferenceResources[0], "secret")
}

func TestTaskToDetailDtoBuildsLegacyPartialSnapshot(t *testing.T) {
	task := &model.Task{
		TaskID:   "task_legacy",
		Platform: constant.TaskPlatform("seedance"),
		Properties: model.Properties{
			Input:              "legacy prompt",
			ReferenceResources: []string{"https://cdn.example/ref.png"},
			OriginModelName:    "seedance-2.0",
		},
	}

	detail := taskToDetailDto(task, true)
	require.NotNil(t, detail)
	assert.Equal(t, "legacy_partial", detail.DetailSource)
	assert.Equal(t, []string{"normalized_request_snapshot"}, detail.MissingFields)
	assert.JSONEq(t, `{"model":"seedance-2.0","prompt":"legacy prompt","reference_resources":["https://cdn.example/ref.png"]}`, string(detail.RequestSnapshot))
}
