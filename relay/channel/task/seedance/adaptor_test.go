package seedance

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const validRequest = `{
  "model":"seedance-2.0",
  "prompt":"a paper boat on a river",
  "input":{
    "duration":5,
    "aspect_ratio":"16:9",
    "resolution":"720P",
    "generate_audio":false,
    "image_references":["https://cdn.example/image.png",{"image_url":{"url":"https://cdn.example/ref.png"}}],
    "video_references":["https://cdn.example/video.mp4"],
    "audio_references":[]
  }
}`

func newTaskContext(t *testing.T, body string) (*gin.Context, *relaycommon.RelayInfo) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	request := httptest.NewRequest(http.MethodPost, "/v1/videos", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	context.Request = request
	t.Cleanup(func() { common.CleanupBodyStorage(context) })
	return context, &relaycommon.RelayInfo{
		TaskRelayInfo: &relaycommon.TaskRelayInfo{},
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "seedance-2.0",
		},
	}
}

func TestValidationAndBillingRatios(t *testing.T) {
	context, info := newTaskContext(t, validRequest)
	adaptor := &TaskAdaptor{}

	require.Nil(t, adaptor.ValidateRequestAndSetAction(context, info))
	ratios := adaptor.EstimateBilling(context, info)
	assert.Equal(t, float64(5), ratios["seconds"])
	assert.Equal(t, float64(1), ratios["resolution"])
}

func TestBuildRequestBodyTranslatesNestedCompatibilityShapeToModelCenter(t *testing.T) {
	context, info := newTaskContext(t, validRequest)
	adaptor := &TaskAdaptor{}
	require.Nil(t, adaptor.ValidateRequestAndSetAction(context, info))

	reader, err := adaptor.BuildRequestBody(context, info)
	require.NoError(t, err)
	body, err := io.ReadAll(reader)
	require.NoError(t, err)

	var payload map[string]any
	require.NoError(t, common.Unmarshal(body, &payload))
	assert.Equal(t, "sd_2.0_discount", payload["model"])
	assert.Equal(t, "a paper boat on a river", payload["prompt"])
	assert.Equal(t, "16:9", payload["aspect_ratio"])
	assert.Equal(t, "720p", payload["resolution"])
	assert.Equal(t, float64(5), payload["duration"])
	assert.Equal(t, false, payload["generate_audio"])
	assert.Equal(t, []any{"https://cdn.example/image.png", "https://cdn.example/ref.png"}, payload["reference_images"])
	assert.Equal(t, []any{"https://cdn.example/video.mp4"}, payload["reference_videos"])
	assert.NotContains(t, payload, "input")
	assert.NotContains(t, payload, "content")
	assert.NotContains(t, payload, "ratio")
}

func TestBuildRequestBodyAcceptsTopLevelModelCenterFields(t *testing.T) {
	body := `{"model":"sd_2.0_discount","prompt":"city lights","resolution":"480p","aspect_ratio":"9:16","duration":6,"generate_audio":true,"reference_images":["assetId://image-1"]}`
	context, info := newTaskContext(t, body)
	info.UpstreamModelName = "sd_2.0_discount"
	adaptor := &TaskAdaptor{}
	require.Nil(t, adaptor.ValidateRequestAndSetAction(context, info))

	reader, err := adaptor.BuildRequestBody(context, info)
	require.NoError(t, err)
	forwarded, err := io.ReadAll(reader)
	require.NoError(t, err)
	assert.JSONEq(t, `{
		"model":"sd_2.0_discount",
		"prompt":"city lights",
		"reference_images":["assetId://image-1"],
		"duration":6,
		"aspect_ratio":"9:16",
		"resolution":"480p",
		"generate_audio":true
	}`, string(forwarded))
}

func TestBuildRequestBodyTranslatesStandardSoraFields(t *testing.T) {
	body := `{"model":"sd_2.0_fast_discount","prompt":"a lighthouse at dusk","size":"1280x720","seconds":"8","input_reference":"assetId://asset-123"}`
	context, info := newTaskContext(t, body)
	info.UpstreamModelName = "sd_2.0_fast_discount"
	adaptor := &TaskAdaptor{}
	require.Nil(t, adaptor.ValidateRequestAndSetAction(context, info))

	reader, err := adaptor.BuildRequestBody(context, info)
	require.NoError(t, err)
	forwarded, err := io.ReadAll(reader)
	require.NoError(t, err)
	assert.JSONEq(t, `{
		"model":"sd_2.0_fast_discount",
		"prompt":"a lighthouse at dusk",
		"duration":8,
		"aspect_ratio":"16:9",
		"resolution":"720p",
		"first_image":"assetId://asset-123"
	}`, string(forwarded))
}

func TestBuildRequestBodyTreatsTopLevelImagesAsOrderedReferences(t *testing.T) {
	body := `{
		"model":"seedance-2.0",
		"prompt":"@图1 和 @图2 在雨夜霓虹巷奔跑，电影感",
		"duration":5,
		"aspect_ratio":"16:9",
		"resolution":"720p",
		"audio":true,
		"images":[
			"https://h.m66x.cn/s/anita/mgw/asset/1787063516138_5c421eee.png",
			"https://h.m66x.cn/s/anita/mgw/asset/1787051387831_1ce33457.png"
		]
	}`
	context, info := newTaskContext(t, body)
	adaptor := &TaskAdaptor{}
	require.Nil(t, adaptor.ValidateRequestAndSetAction(context, info))

	reader, err := adaptor.BuildRequestBody(context, info)
	require.NoError(t, err)
	forwarded, err := io.ReadAll(reader)
	require.NoError(t, err)

	var payload map[string]any
	require.NoError(t, common.Unmarshal(forwarded, &payload))
	assert.Equal(t, []any{
		"https://h.m66x.cn/s/anita/mgw/asset/1787063516138_5c421eee.png",
		"https://h.m66x.cn/s/anita/mgw/asset/1787051387831_1ce33457.png",
	}, payload["reference_images"])
	assert.NotContains(t, payload, "first_image")
	assert.Equal(t, "@图1 和 @图2 在雨夜霓虹巷奔跑，电影感", payload["prompt"])
}

func TestBuildRequestBodyAcceptsLegacyNestedPrompt(t *testing.T) {
	body := `{"model":"seedance-2.0","input":{"prompt":"legacy river scene","duration":5,"aspect_ratio":"16:9","resolution":"720p"}}`
	context, info := newTaskContext(t, body)
	adaptor := &TaskAdaptor{}
	require.Nil(t, adaptor.ValidateRequestAndSetAction(context, info))

	reader, err := adaptor.BuildRequestBody(context, info)
	require.NoError(t, err)
	forwarded, err := io.ReadAll(reader)
	require.NoError(t, err)
	assert.JSONEq(t, `{
		"model":"sd_2.0_discount",
		"prompt":"legacy river scene",
		"duration":5,
		"aspect_ratio":"16:9",
		"resolution":"720p"
	}`, string(forwarded))
}

func TestBuildRequestBodyTranslatesContentResources(t *testing.T) {
	body := `{
		"model":"sd_2.0_discount",
		"resolution":"720p",
		"duration":6,
		"content":[
			{"type":"text","text":"a car crossing a bridge"},
			{"type":"image_url","role":"reference_image","image_url":{"url":"assetId://image-1"}},
			{"type":"video_url","role":"reference_video","video_url":{"url":"https://cdn.example/ref.mp4"}},
			{"type":"audio_url","role":"reference_audio","audio_url":{"url":"assetId://audio-1"}}
		]
	}`
	context, info := newTaskContext(t, body)
	info.UpstreamModelName = "sd_2.0_discount"
	adaptor := &TaskAdaptor{}
	require.Nil(t, adaptor.ValidateRequestAndSetAction(context, info))

	reader, err := adaptor.BuildRequestBody(context, info)
	require.NoError(t, err)
	forwarded, err := io.ReadAll(reader)
	require.NoError(t, err)
	assert.JSONEq(t, `{
		"model":"sd_2.0_discount",
		"prompt":"a car crossing a bridge",
		"reference_images":["assetId://image-1"],
		"reference_videos":["https://cdn.example/ref.mp4"],
		"reference_audios":["assetId://audio-1"],
		"duration":6,
		"aspect_ratio":"16:9",
		"resolution":"720p"
	}`, string(forwarded))
}

func TestBuildRequestBodyTranslatesContentFrameRoles(t *testing.T) {
	body := `{
		"model":"seedance-2.0",
		"prompt":"smooth transition",
		"size":"1280x720",
		"seconds":"5",
		"content":[
			{"type":"image_url","role":"first_frame","image_url":{"url":"assetId://first"}},
			{"type":"image_url","role":"last_frame","image_url":{"url":"assetId://last"}}
		]
	}`
	context, info := newTaskContext(t, body)
	adaptor := &TaskAdaptor{}
	require.Nil(t, adaptor.ValidateRequestAndSetAction(context, info))

	reader, err := adaptor.BuildRequestBody(context, info)
	require.NoError(t, err)
	forwarded, err := io.ReadAll(reader)
	require.NoError(t, err)
	assert.JSONEq(t, `{
		"model":"sd_2.0_discount",
		"prompt":"smooth transition",
		"duration":5,
		"aspect_ratio":"16:9",
		"resolution":"720p",
		"first_image":"assetId://first",
		"last_image":"assetId://last"
	}`, string(forwarded))
}

func TestBuildRequestBodyAcceptsMultipartRemoteInputReference(t *testing.T) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for key, value := range map[string]string{
		"model": "sd_2.0_fast_discount", "prompt": "coastline", "size": "1280x720",
		"seconds": "6", "input_reference": "assetId://asset-456",
	} {
		require.NoError(t, writer.WriteField(key, value))
	}
	require.NoError(t, writer.Close())

	gin.SetMode(gin.TestMode)
	request := httptest.NewRequest(http.MethodPost, "/v1/videos", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	context.Request = request
	t.Cleanup(func() { common.CleanupBodyStorage(context) })
	info := &relaycommon.RelayInfo{
		TaskRelayInfo: &relaycommon.TaskRelayInfo{},
		ChannelMeta:   &relaycommon.ChannelMeta{UpstreamModelName: "sd_2.0_fast_discount"},
	}
	adaptor := &TaskAdaptor{}
	require.Nil(t, adaptor.ValidateRequestAndSetAction(context, info))

	reader, err := adaptor.BuildRequestBody(context, info)
	require.NoError(t, err)
	forwarded, err := io.ReadAll(reader)
	require.NoError(t, err)
	assert.JSONEq(t, `{
		"model":"sd_2.0_fast_discount",
		"prompt":"coastline",
		"duration":6,
		"aspect_ratio":"16:9",
		"resolution":"720p",
		"first_image":"assetId://asset-456"
	}`, string(forwarded))
}

func TestCanonicalModelAliasesAndResolutionLimits(t *testing.T) {
	tests := []struct {
		model      string
		canonical  string
		resolution string
		valid      bool
	}{
		{model: "seedance-2.0", canonical: "sd_2.0_discount", resolution: "1080p", valid: true},
		{model: "seedance-2.0-fast", canonical: "sd_2.0_fast_discount", resolution: "720p", valid: true},
		{model: "sd_2.0_mini_discount", canonical: "sd_2.0_mini_discount", resolution: "480p", valid: true},
		{model: "sd_2.0_special", canonical: "sd_2.0_special", resolution: "1080p", valid: true},
		{model: "sd_2.0_fast_special", canonical: "sd_2.0_fast_special", resolution: "720p", valid: true},
		{model: "sd_2.0_mini_special", canonical: "sd_2.0_mini_special", resolution: "720p", valid: true},
		{model: "seedance-1.0", resolution: "720p", valid: false},
	}

	for _, test := range tests {
		t.Run(test.model, func(t *testing.T) {
			canonical, ok := canonicalModel(test.model)
			assert.Equal(t, test.valid, ok)
			if !test.valid {
				return
			}
			assert.Equal(t, test.canonical, canonical)
			_, supported := modelSpecs[canonical][test.resolution]
			assert.True(t, supported)
		})
	}
}

func TestRequestResolutionMapsSoraSize(t *testing.T) {
	tests := []struct {
		body    string
		want    string
		wantErr string
	}{
		{body: `{"model":"seedance-2.0","prompt":"x","size":"854x480"}`, want: "480p"},
		{body: `{"model":"seedance-2.0","prompt":"x","size":"1280x720"}`, want: "720p"},
		{body: `{"model":"seedance-2.0","prompt":"x","size":"1920x1080"}`, want: "1080p"},
		{body: `{"model":"seedance-2.0","prompt":"x","size":"4k"}`, wantErr: "must map"},
	}
	for _, test := range tests {
		var req relaycommon.TaskSubmitReq
		require.NoError(t, json.Unmarshal([]byte(test.body), &req))
		got, err := requestResolution(req)
		if test.wantErr != "" {
			require.Error(t, err)
			assert.Contains(t, err.Error(), test.wantErr)
			continue
		}
		require.NoError(t, err)
		assert.Equal(t, test.want, got)
	}
}

func TestSeedanceValidationBoundaries(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		message string
	}{
		{name: "unsupported model", body: `{"model":"seedance-1.0","prompt":"x"}`, message: "unsupported Seedance model"},
		{name: "fast 1080p", body: `{"model":"seedance-2.0-fast","prompt":"x","size":"1920x1080"}`, message: "does not support resolution"},
		{name: "duration too short", body: `{"model":"seedance-2.0","prompt":"x","seconds":"3"}`, message: "between 4 and 15"},
		{name: "fractional duration", body: `{"model":"seedance-2.0","prompt":"x","input":{"duration":5.5}}`, message: "integer between 4 and 15"},
		{name: "malformed duration", body: `{"model":"seedance-2.0","prompt":"x","input":{"duration":"five"}}`, message: "integer between 4 and 15"},
		{name: "bad ratio", body: `{"model":"seedance-2.0","prompt":"x","input":{"aspect_ratio":"bogus"}}`, message: "aspect_ratio"},
		{name: "generate audio type", body: `{"model":"seedance-2.0","prompt":"x","input":{"generate_audio":"false"}}`, message: "must be a boolean"},
		{name: "too many images", body: `{"model":"seedance-2.0","prompt":"x","input":{"reference_images":["https://e.example/1","https://e.example/2","https://e.example/3","https://e.example/4","https://e.example/5","https://e.example/6","https://e.example/7","https://e.example/8","https://e.example/9","https://e.example/10"]}}`, message: "at most 9"},
		{name: "last frame without first", body: `{"model":"seedance-2.0","prompt":"x","input":{"last_image":"https://e.example/end.png"}}`, message: "requires first_image"},
		{name: "local path", body: `{"model":"seedance-2.0","prompt":"x","input":{"reference_videos":["C:\\\\secret.mp4"]}}`, message: "media reference"},
		{name: "empty reference object", body: `{"model":"seedance-2.0","prompt":"x","input":{"reference_videos":[{"strength":0.8}]}}`, message: "must contain"},
		{name: "frames mixed with references", body: `{"model":"seedance-2.0","prompt":"x","input":{"first_image":"https://e.example/start.png","reference_images":["https://e.example/ref.png"]}}`, message: "cannot be mixed"},
		{name: "audio without image or video", body: `{"model":"seedance-2.0","prompt":"x","input":{"reference_audios":["https://e.example/audio.mp3"]}}`, message: "requires at least one"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			context, info := newTaskContext(t, test.body)
			taskErr := (&TaskAdaptor{}).ValidateRequestAndSetAction(context, info)
			require.NotNil(t, taskErr)
			assert.Contains(t, taskErr.Message, test.message)
		})
	}
}

func TestAssetReferenceAndFirstLastFramesAreAccepted(t *testing.T) {
	body := `{"model":"seedance-2.0","prompt":"transition","input":{"first_image":"assetId://first","last_image":"assetId://last"}}`
	context, info := newTaskContext(t, body)
	require.Nil(t, (&TaskAdaptor{}).ValidateRequestAndSetAction(context, info))
}

func TestEndpointURLNormalizesCommonChannelBaseURLs(t *testing.T) {
	tests := map[string]string{
		"https://provider.example":                                                         "https://provider.example/kyyReactApiServer/v2/model-center/tasks/task%2Fupstream",
		"https://provider.example/kyyReactApiServer":                                       "https://provider.example/kyyReactApiServer/v2/model-center/tasks/task%2Fupstream",
		"https://provider.example/kyyReactApiServer/v1/seedance-discount/videos":           "https://provider.example/kyyReactApiServer/v2/model-center/tasks/task%2Fupstream",
		"https://provider.example/prefix/kyyReactApiServer/v2/model-center/tasks/old-task": "https://provider.example/prefix/kyyReactApiServer/v2/model-center/tasks/task%2Fupstream",
	}
	for baseURL, want := range tests {
		assert.Equal(t, want, endpointURL(baseURL, "task/upstream"))
	}
}

func TestDoResponseTranslatesToOpenAIVideoStyle(t *testing.T) {
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	responseBody := `{"id":"video_seedance_upstream","object":"video","created":1783561234,"model":"sd_2.0_fast_discount","status":"queued"}`
	response := &http.Response{Body: io.NopCloser(strings.NewReader(responseBody))}
	info := &relaycommon.RelayInfo{TaskRelayInfo: &relaycommon.TaskRelayInfo{PublicTaskID: "task_public"}, ChannelMeta: &relaycommon.ChannelMeta{}}
	info.OriginModelName = "seedance-2.0-fast"

	upstreamID, stored, taskErr := (&TaskAdaptor{}).DoResponse(context, response, info)
	require.Nil(t, taskErr)
	assert.Equal(t, "video_seedance_upstream", upstreamID)
	assert.Contains(t, string(stored), "video_seedance_upstream")

	var out map[string]any
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &out))
	assert.Equal(t, "task_public", out["id"])
	assert.Equal(t, "queued", out["status"])
	assert.Equal(t, "seedance-2.0-fast", out["model"])
	assert.NotContains(t, recorder.Body.String(), "video_seedance_upstream")
}

func TestAsyncCreateAndFetchResponsesKeepLegacyEnvelope(t *testing.T) {
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodPost, "/async/tasks", nil)
	responseBody := `{"id":"video_seedance_upstream","object":"video","created":1783561234,"model":"sd_2.0_discount","status":"queued"}`
	response := &http.Response{Body: io.NopCloser(strings.NewReader(responseBody))}
	info := &relaycommon.RelayInfo{
		TaskRelayInfo: &relaycommon.TaskRelayInfo{PublicTaskID: "task_public"},
		ChannelMeta:   &relaycommon.ChannelMeta{},
	}
	info.OriginModelName = "seedance-2.0"

	upstreamID, _, taskErr := (&TaskAdaptor{}).DoResponse(context, response, info)
	require.Nil(t, taskErr)
	assert.Equal(t, "video_seedance_upstream", upstreamID)
	assert.JSONEq(t, `{
		"success":true,
		"message":"created",
		"data":{"id":"task_public","task_id":"task_public","object":"video","model":"seedance-2.0","status":"queued","progress":20,"created_at":1783561234,"metadata":{"upstream_model":"sd_2.0_discount"}}
	}`, recorder.Body.String())

	stored := &model.Task{
		TaskID: "task_public", Status: model.TaskStatusQueued, Progress: "20%", CreatedAt: 1783561234,
		Properties: model.Properties{OriginModelName: "seedance-2.0"}, Data: []byte(responseBody),
	}
	converted, err := (&TaskAdaptor{}).ConvertTaskResponse(stored)
	require.NoError(t, err)
	assert.Contains(t, string(converted), `"success":true`)
	assert.Contains(t, string(converted), `"task_id":"task_public"`)
	assert.NotContains(t, string(converted), "video_seedance_upstream")
}

func TestParseTaskResultSuccessFailureAndExplicitBillingSignal(t *testing.T) {
	adaptor := &TaskAdaptor{}
	successBody := []byte(`{"id":"video_seedance_1","status":"completed","video_url":"https://cdn.example/video.mp4","amount":0.76,"totalTokens":108900}`)
	result, err := adaptor.ParseTaskResult(successBody)
	require.NoError(t, err)
	assert.Equal(t, model.TaskStatusSuccess, result.Status)
	assert.Equal(t, "https://cdn.example/video.mp4", result.Url)
	assert.Equal(t, 108900, result.TotalTokens)

	amountOnly, err := adaptor.ParseTaskResult([]byte(`{"id":"video_seedance_2","status":"completed","video_url":"https://cdn.example/video-2.mp4","amount":0.76}`))
	require.NoError(t, err)
	assert.Zero(t, amountOnly.TotalTokens, "amount must not be guessed into internal token billing")

	failure, err := adaptor.ParseTaskResult([]byte(`{"id":"video_seedance_3","status":"failed","error":"provider rejected prompt"}`))
	require.NoError(t, err)
	assert.Equal(t, model.TaskStatusFailure, failure.Status)
	assert.Equal(t, "provider rejected prompt", failure.Reason)
}

func TestConvertToOpenAIVideoUsesStoredQueryEnvelope(t *testing.T) {
	data := []byte(`{"id":"video_seedance_1","created":1783561234,"model":"sd_2.0_discount","status":"completed","video_url":"https://cdn.example/video.mp4","last_frame_url":"https://cdn.example/last.png"}`)
	task := &model.Task{
		TaskID:    "task_public",
		Status:    model.TaskStatusSuccess,
		Progress:  "100%",
		CreatedAt: 1783561234,
		UpdatedAt: 1783561334,
		Properties: model.Properties{
			OriginModelName: "seedance-2.0",
		},
		Data: data,
	}
	converted, err := (&TaskAdaptor{}).ConvertToOpenAIVideo(task)
	require.NoError(t, err)
	assert.Contains(t, string(converted), "task_public")
	assert.Contains(t, string(converted), "completed")
	assert.Contains(t, string(converted), "https://cdn.example/video.mp4")
	assert.NotContains(t, string(converted), "video_seedance_1")
}

func TestFetchTaskUsesModelCenterEndpointAndUpstreamID(t *testing.T) {
	service.InitHttpClient()
	var requestedPath string
	var authorization string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestedPath = r.URL.EscapedPath()
		authorization = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"video_seedance_1","status":"processing"}`)
	}))
	defer server.Close()

	response, err := (&TaskAdaptor{}).FetchTask(server.URL, "channel-key", map[string]any{"task_id": "video_seedance_1"}, "")
	require.NoError(t, err)
	defer response.Body.Close()
	assert.Equal(t, "/kyyReactApiServer/v2/model-center/tasks/video_seedance_1", requestedPath)
	assert.Equal(t, "Bearer channel-key", authorization)
}
