package sora

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func newJSONTaskContext(t *testing.T, body string) (*gin.Context, *relaycommon.RelayInfo) {
	return newJSONTaskContextForModel(t, body, "MiniMax-H3")
}

func newJSONTaskContextForModel(t *testing.T, body, upstreamModel string) (*gin.Context, *relaycommon.RelayInfo) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	request := httptest.NewRequest(http.MethodPost, "/v1/videos", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	context.Request = request
	info := &relaycommon.RelayInfo{
		TaskRelayInfo: &relaycommon.TaskRelayInfo{},
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: upstreamModel,
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

func TestMiniMaxH3ResolutionFamilyKeepsTopLevelProtocol(t *testing.T) {
	models := []string{
		"MiniMaxH3-480p",
		"MiniMaxH3-720p",
		"MiniMaxH3-2k",
		"MiniMaxH3-720p-sec",
		"MiniMaxH3-720p-pro",
		"MiniMaxH3-720p-nf",
	}
	for _, modelName := range models {
		t.Run(modelName, func(t *testing.T) {
			body := `{"model":"client-alias","prompt":"a lighthouse","resolution":"720p","duration":6}`
			context, info := newJSONTaskContextForModel(t, body, modelName)
			adaptor := &TaskAdaptor{}

			require.Nil(t, adaptor.ValidateRequestAndSetAction(context, info))
			assert.Equal(t, float64(6), adaptor.EstimateBilling(context, info)["seconds"])
			reader, err := adaptor.BuildRequestBody(context, info)
			require.NoError(t, err)
			forwarded, err := io.ReadAll(reader)
			require.NoError(t, err)

			var payload map[string]any
			require.NoError(t, common.Unmarshal(forwarded, &payload))
			assert.Equal(t, modelName, payload["model"])
			assert.Equal(t, "a lighthouse", payload["prompt"])
			assert.Equal(t, "720p", payload["resolution"])
			assert.Equal(t, float64(6), payload["duration"])
			assert.NotContains(t, payload, "input")
		})
	}
}

func TestMiniMaxH3ResolutionFamilyFlattensCompatibleInput(t *testing.T) {
	body := `{"model":"client-alias","input":{"prompt":"a comet","resolution":"2k","duration":8}}`
	context, info := newJSONTaskContextForModel(t, body, "MiniMaxH3-2k-pro")
	adaptor := &TaskAdaptor{}

	require.Nil(t, adaptor.ValidateRequestAndSetAction(context, info))
	reader, err := adaptor.BuildRequestBody(context, info)
	require.NoError(t, err)
	forwarded, err := io.ReadAll(reader)
	require.NoError(t, err)

	assert.JSONEq(t, `{"model":"MiniMaxH3-2k-pro","prompt":"a comet","resolution":"2k","duration":8}`, string(forwarded))
}

func TestXinshujuProtocolUsesMultimodalContent(t *testing.T) {
	body := `{"model":"seedance-2.0","prompt":"@图1，@图2，@图3，@图4 在广场奔跑","duration":5,"aspect_ratio":"16:9","resolution":"720p","audio":true,"n":1,"images":["https://example.com/one.png","https://example.com/two.jpg","https://example.com/three.webp","https://example.com/four.png"]}`
	context, info := newJSONTaskContextForModel(t, body, "48:seedance-2.0-fast")
	info.ChannelSetting.VideoUpstreamProtocol = dto.VideoUpstreamProtocolXinshujuContent
	adaptor := &TaskAdaptor{}

	require.Nil(t, adaptor.ValidateRequestAndSetAction(context, info))
	reader, err := adaptor.BuildRequestBody(context, info)
	require.NoError(t, err)
	forwarded, err := io.ReadAll(reader)
	require.NoError(t, err)

	assert.JSONEq(t, `{
		"model":"48:seedance-2.0-fast",
		"content":[
			{"type":"text","text":"@图1，@图2，@图3，@图4 在广场奔跑"},
			{"type":"image_url","image_url":{"url":"https://example.com/one.png"},"role":"reference_image"},
			{"type":"image_url","image_url":{"url":"https://example.com/two.jpg"},"role":"reference_image"},
			{"type":"image_url","image_url":{"url":"https://example.com/three.webp"},"role":"reference_image"},
			{"type":"image_url","image_url":{"url":"https://example.com/four.png"},"role":"reference_image"}
		],
		"generate_audio":true,
		"ratio":"16:9",
		"duration":5,
		"watermark":false,
		"resolution":"720p"
	}`, string(forwarded))
	assert.JSONEq(t, string(forwarded), string(info.TaskRequestSnapshot))

	var payload map[string]any
	require.NoError(t, common.Unmarshal(forwarded, &payload))
	assert.NotContains(t, payload, "prompt")
	assert.NotContains(t, payload, "audio")
	assert.NotContains(t, payload, "aspect_ratio")
	assert.NotContains(t, payload, "image")
	assert.NotContains(t, payload, "images")
	assert.NotContains(t, payload, "image_urls")
	assert.NotContains(t, payload, "metadata")
	assert.NotContains(t, payload, "n")
}

func TestGenericOpenAIVideoProtocolDoesNotUseXinshujuFormatForSeedanceModel(t *testing.T) {
	body := `{"model":"seedance-2.0","prompt":"a lighthouse","images":["https://example.com/one.png","https://example.com/two.jpg"]}`
	context, info := newJSONTaskContextForModel(t, body, "48:seedance-2.0-fast")
	info.ChannelSetting.VideoUpstreamProtocol = dto.VideoUpstreamProtocolOpenAI
	adaptor := &TaskAdaptor{}

	require.Nil(t, adaptor.ValidateRequestAndSetAction(context, info))
	reader, err := adaptor.BuildRequestBody(context, info)
	require.NoError(t, err)
	forwarded, err := io.ReadAll(reader)
	require.NoError(t, err)

	var payload map[string]any
	require.NoError(t, common.Unmarshal(forwarded, &payload))
	assert.Equal(t, "48:seedance-2.0-fast", payload["model"])
	assert.Equal(t, "a lighthouse", payload["prompt"])
	assert.Len(t, payload["images"], 2)
	assert.NotContains(t, payload, "content")
}

func TestXinshujuProtocolSelectionIsProviderScoped(t *testing.T) {
	tests := []struct {
		name     string
		protocol dto.VideoUpstreamProtocol
		baseURL  string
		expected bool
	}{
		{"explicit protocol", dto.VideoUpstreamProtocolXinshujuContent, "https://proxy.example.com", true},
		{"legacy exact host", dto.VideoUpstreamProtocolOpenAI, "https://xinshuju.net", true},
		{"legacy www host", dto.VideoUpstreamProtocolOpenAI, "https://www.xinshuju.net/v1", true},
		{"other Seedance provider", dto.VideoUpstreamProtocolOpenAI, "https://video.example.com", false},
		{"lookalike host", dto.VideoUpstreamProtocolOpenAI, "https://xinshuju.net.evil.example", false},
		{"channel default", dto.VideoUpstreamProtocolDefault, "https://www.xinshuju.net", false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{
				ChannelBaseUrl: test.baseURL,
				ChannelSetting: dto.ChannelSettings{VideoUpstreamProtocol: test.protocol},
			}}
			assert.Equal(t, test.expected, usesXinshujuContentProtocol(info))
		})
	}
}

func TestNonSeedanceOpenAIVideoBodyIsNotExpanded(t *testing.T) {
	body := `{"model":"sora-2","prompt":"a lighthouse","images":["https://example.com/one.png","https://example.com/two.jpg"]}`
	context, info := newJSONTaskContextForModel(t, body, "sora-2")
	adaptor := &TaskAdaptor{}

	require.Nil(t, adaptor.ValidateRequestAndSetAction(context, info))
	reader, err := adaptor.BuildRequestBody(context, info)
	require.NoError(t, err)
	forwarded, err := io.ReadAll(reader)
	require.NoError(t, err)

	var payload map[string]any
	require.NoError(t, common.Unmarshal(forwarded, &payload))
	assert.NotContains(t, payload, "image_urls")
	assert.NotContains(t, payload, "metadata")
}

func TestMiniMaxH3ProtocolFamiliesStayDistinct(t *testing.T) {
	assert.True(t, isMiniMaxH3Model("MiniMax-H3"))
	assert.False(t, isMiniMaxH3ResolutionModel("MiniMax-H3"))
	assert.True(t, isMiniMaxH3ResolutionModel("MiniMaxH3-720p"))
	assert.True(t, isMiniMaxH3ResolutionModel(" minimaxh3-2K-NF "))
	assert.False(t, isMiniMaxH3ResolutionModel("MiniMaxH3"))
	assert.False(t, isMiniMaxH3ResolutionModel("MiniMaxH30-720p"))
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

func TestMiniMaxH3ResolutionCreateAndQueryResponses(t *testing.T) {
	t.Run("create response", func(t *testing.T) {
		gin.SetMode(gin.TestMode)
		recorder := httptest.NewRecorder()
		context, _ := gin.CreateTestContext(recorder)
		adaptor := &TaskAdaptor{}
		response := &http.Response{
			Body: io.NopCloser(strings.NewReader(`{"id":"upstream-720p","model":"MiniMaxH3-720p","status":"queued"}`)),
		}
		info := &relaycommon.RelayInfo{TaskRelayInfo: &relaycommon.TaskRelayInfo{PublicTaskID: "task_public"}}

		taskID, taskData, taskErr := adaptor.DoResponse(context, response, info)

		require.Nil(t, taskErr)
		assert.Equal(t, "upstream-720p", taskID)
		assert.JSONEq(t, `{"id":"upstream-720p","model":"MiniMaxH3-720p","status":"queued"}`, string(taskData))
		assert.JSONEq(t, `{"id":"task_public","task_id":"task_public","object":"","model":"MiniMaxH3-720p","status":"queued","progress":0,"created_at":0}`, recorder.Body.String())
	})

	t.Run("upstream query", func(t *testing.T) {
		service.InitHttpClient()
		var gotPath, gotAuthorization string
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotPath = r.URL.Path
			gotAuthorization = r.Header.Get("Authorization")
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"id":"upstream-720p","status":"completed","progress":100}`)
		}))
		defer server.Close()

		adaptor := &TaskAdaptor{}
		response, err := adaptor.FetchTask(server.URL, "provider-token", map[string]any{"task_id": "upstream-720p"}, "")
		require.NoError(t, err)
		defer response.Body.Close()
		body, err := io.ReadAll(response.Body)
		require.NoError(t, err)
		result, err := adaptor.ParseTaskResult(body)
		require.NoError(t, err)

		assert.Equal(t, "/v1/videos/upstream-720p", gotPath)
		assert.Equal(t, "Bearer provider-token", gotAuthorization)
		assert.Equal(t, model.TaskStatusSuccess, result.Status)
	})
}

func TestConvertToOpenAIVideoPreservesNestedQueryResponse(t *testing.T) {
	originalServerAddress := system_setting.ServerAddress
	system_setting.ServerAddress = "https://newapi.example"
	t.Cleanup(func() { system_setting.ServerAddress = originalServerAddress })
	adaptor := &TaskAdaptor{}
	task := &model.Task{
		TaskID: "task_public",
		Status: model.TaskStatusSuccess,
		Data:   []byte(`{"success":true,"data":{"task_id":"upstream-123","status":"SUCCESS","progress":100,"fail_reason":"","content":[{"type":"video_url","video_url":{"url":"https://cdn.example/reference.mp4"}}],"data":{"outputs":[{"url":"https://cdn.example/expired.mp4"}]}}}`),
	}

	converted, err := adaptor.ConvertToOpenAIVideo(task)

	require.NoError(t, err)
	payload := gjson.ParseBytes(converted)
	assert.Equal(t, "task_public", payload.Get("data.task_id").String())
	assert.Equal(t, "https://cdn.example/reference.mp4", payload.Get("data.content.0.video_url.url").String())
	expectedSignature := common.GenerateHMAC("video-content:task_public")
	assert.Equal(t, "https://newapi.example/v1/videos/task_public/content?sig="+expectedSignature, payload.Get("data.data.outputs.0.url").String())
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
