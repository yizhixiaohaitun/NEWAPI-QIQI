package sora

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newJSONTaskContext(t *testing.T, body string) (*gin.Context, *relaycommon.RelayInfo) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	request := httptest.NewRequest(http.MethodPost, "/v1/videos", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	context.Request = request
	info := &relaycommon.RelayInfo{
		TaskRelayInfo: &relaycommon.TaskRelayInfo{},
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "MiniMax-H3",
		},
	}
	return context, info
}

func TestMiniMaxH3NestedRequestIsNormalizedForBillingAndForwarding(t *testing.T) {
	body := `{"model":"MiniMax-H3","callback_url":"https://client.example/video-callback","input":{"prompt":"a comet","aspect_ratio":"16:9","resolution":"2K","duration":8,"audio":true,"n":1}}`
	context, info := newJSONTaskContext(t, body)
	adaptor := &TaskAdaptor{}

	require.Nil(t, adaptor.ValidateRequestAndSetAction(context, info))
	assert.Equal(t, float64(8), adaptor.EstimateBilling(context, info)["seconds"])

	reader, err := adaptor.BuildRequestBody(context, info)
	require.NoError(t, err)
	forwarded, err := io.ReadAll(reader)
	require.NoError(t, err)
	var payload map[string]any
	require.NoError(t, common.Unmarshal(forwarded, &payload))
	assert.Len(t, payload, 3)
	assert.Equal(t, "MiniMax-H3", payload["model"])
	assert.Equal(t, "https://client.example/video-callback", payload["callback_url"])
	assert.NotContains(t, payload, "prompt")
	assert.NotContains(t, payload, "duration")
	input, ok := payload["input"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "a comet", input["prompt"])
	assert.Equal(t, float64(8), input["duration"])
	assert.Equal(t, "2K", input["resolution"])
}

func TestMiniMaxH3TopLevelRequestIsForwardedAsDocumentedInput(t *testing.T) {
	body := `{"model":"MiniMax-H3","prompt":"a lighthouse","duration":6,"aspect_ratio":"9:16","resolution":"2K","audio":false,"n":1}`
	context, info := newJSONTaskContext(t, body)
	adaptor := &TaskAdaptor{}

	require.Nil(t, adaptor.ValidateRequestAndSetAction(context, info))
	reader, err := adaptor.BuildRequestBody(context, info)
	require.NoError(t, err)
	forwarded, err := io.ReadAll(reader)
	require.NoError(t, err)
	var payload map[string]any
	require.NoError(t, common.Unmarshal(forwarded, &payload))
	assert.Len(t, payload, 2)
	input := payload["input"].(map[string]any)
	assert.Equal(t, "a lighthouse", input["prompt"])
	assert.Equal(t, float64(6), input["duration"])
	assert.Equal(t, "9:16", input["aspect_ratio"])
}

func TestBuildRequestHeaderForwardsIdempotencyKeyOnlyForMiniMaxH3(t *testing.T) {
	context, info := newJSONTaskContext(t, `{}`)
	context.Request.Header.Set("Idempotency-Key", " create-video-123 ")
	adaptor := &TaskAdaptor{apiKey: "upstream-key"}

	t.Run("MiniMax-H3", func(t *testing.T) {
		upstream := httptest.NewRequest(http.MethodPost, "https://upstream.example/v1/videos", nil)
		require.NoError(t, adaptor.BuildRequestHeader(context, upstream, info))
		assert.Equal(t, "create-video-123", upstream.Header.Get("Idempotency-Key"))
	})

	t.Run("other Sora protocol", func(t *testing.T) {
		otherInfo := &relaycommon.RelayInfo{
			TaskRelayInfo: &relaycommon.TaskRelayInfo{},
			ChannelMeta:   &relaycommon.ChannelMeta{UpstreamModelName: "sora-2"},
		}
		upstream := httptest.NewRequest(http.MethodPost, "https://upstream.example/v1/videos", nil)
		require.NoError(t, adaptor.BuildRequestHeader(context, upstream, otherInfo))
		assert.Empty(t, upstream.Header.Get("Idempotency-Key"))
	})
}

func TestDoResponseAcceptsNestedTaskID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	adaptor := &TaskAdaptor{}
	response := &http.Response{
		Body: io.NopCloser(strings.NewReader(`{"success":true,"data":{"task_id":"upstream-123","status":"pending"}}`)),
	}
	info := &relaycommon.RelayInfo{TaskRelayInfo: &relaycommon.TaskRelayInfo{PublicTaskID: "task_public"}}

	taskID, taskData, taskErr := adaptor.DoResponse(context, response, info)

	require.Nil(t, taskErr)
	assert.Equal(t, "upstream-123", taskID)
	assert.JSONEq(t, `{"success":true,"data":{"task_id":"upstream-123","status":"pending"}}`, string(taskData))
	assert.JSONEq(t, `{"success":true,"data":{"task_id":"task_public","status":"pending"}}`, recorder.Body.String())
}

func TestConvertToOpenAIVideoPreservesNestedQueryResponse(t *testing.T) {
	adaptor := &TaskAdaptor{}
	task := &model.Task{
		TaskID: "task_public",
		Data:   []byte(`{"success":true,"data":{"task_id":"upstream-123","status":"SUCCESS","progress":100,"fail_reason":"","data":{"outputs":[{"url":"https://cdn.example/video.mp4"}]}}}`),
	}

	converted, err := adaptor.ConvertToOpenAIVideo(task)

	require.NoError(t, err)
	assert.JSONEq(t, `{"success":true,"data":{"task_id":"task_public","status":"SUCCESS","progress":100,"fail_reason":"","data":{"outputs":[{"url":"https://cdn.example/video.mp4"}]}}}`, string(converted))
}

func TestParseTaskResultSupportsNestedCompletionAndFailure(t *testing.T) {
	adaptor := &TaskAdaptor{}

	t.Run("nested completion with outputs", func(t *testing.T) {
		result, err := adaptor.ParseTaskResult([]byte(`{"success":true,"data":{"task_id":"upstream-123","status":"SUCCESS","progress":100,"fail_reason":"","data":{"outputs":[{"url":"https://cdn.example/video.mp4"}]}}}`))
		require.NoError(t, err)
		assert.Equal(t, model.TaskStatusSuccess, result.Status)
		assert.Equal(t, "https://cdn.example/video.mp4", result.Url)
	})

	t.Run("nested failure", func(t *testing.T) {
		result, err := adaptor.ParseTaskResult([]byte(`{"success":true,"data":{"task_id":"upstream-456","status":"FAILURE","progress":"100%","fail_reason":"provider rejected prompt","data":{"outputs":[]}}}`))
		require.NoError(t, err)
		assert.Equal(t, model.TaskStatusFailure, result.Status)
		assert.Equal(t, "provider rejected prompt", result.Reason)
	})

	t.Run("existing flat protocol", func(t *testing.T) {
		result, err := adaptor.ParseTaskResult([]byte(`{"id":"video-1","status":"completed","progress":100}`))
		require.NoError(t, err)
		assert.Equal(t, model.TaskStatusSuccess, result.Status)
	})
}
