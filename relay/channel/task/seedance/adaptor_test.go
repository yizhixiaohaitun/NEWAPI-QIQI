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

func TestBuildRequestBodyTranslatesNestedCompatibilityShapeToSeedance(t *testing.T) {
	context, info := newTaskContext(t, validRequest)
	adaptor := &TaskAdaptor{}
	require.Nil(t, adaptor.ValidateRequestAndSetAction(context, info))

	reader, err := adaptor.BuildRequestBody(context, info)
	require.NoError(t, err)
	body, err := io.ReadAll(reader)
	require.NoError(t, err)

	var payload map[string]any
	require.NoError(t, common.Unmarshal(body, &payload))
	assert.Equal(t, "seedance-2.0", payload["model"])
	input := payload["input"].(map[string]any)
	assert.Equal(t, "a paper boat on a river", input["prompt"])
	assert.Equal(t, "16:9", input["aspect_ratio"])
	assert.Equal(t, "720p", input["resolution"])
	assert.Equal(t, float64(5), input["duration"])
	assert.Equal(t, false, input["audio"])
	assert.Equal(t, []any{"https://cdn.example/image.png", "https://cdn.example/ref.png"}, input["image_references"])
	assert.Equal(t, []any{"https://cdn.example/video.mp4"}, input["video_references"])
	assert.NotContains(t, payload, "prompt")
	assert.NotContains(t, payload, "reference_images")
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
		"model":"seedance-2.0",
		"input":{"prompt":"city lights","image_references":["assetId://image-1"],"duration":6,"aspect_ratio":"9:16","resolution":"480p","audio":true,"n":1}
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
		"model":"seedance-2.0-fast",
		"input":{"prompt":"a lighthouse at dusk","duration":8,"aspect_ratio":"16:9","resolution":"720p","start_frames":["assetId://asset-123"],"n":1}
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
	input := payload["input"].(map[string]any)
	assert.Equal(t, []any{
		"https://h.m66x.cn/s/anita/mgw/asset/1787063516138_5c421eee.png",
		"https://h.m66x.cn/s/anita/mgw/asset/1787051387831_1ce33457.png",
	}, input["image_references"])
	assert.NotContains(t, input, "start_frames")
	assert.Equal(t, "@图1 和 @图2 在雨夜霓虹巷奔跑，电影感", input["prompt"])
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
		"model":"seedance-2.0",
		"input":{"prompt":"legacy river scene","duration":5,"aspect_ratio":"16:9","resolution":"720p","n":1}
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
		"model":"seedance-2.0",
		"input":{"prompt":"a car crossing a bridge","image_references":["assetId://image-1"],"video_references":["https://cdn.example/ref.mp4"],"audio_references":["assetId://audio-1"],"duration":6,"aspect_ratio":"16:9","resolution":"720p","n":1}
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
		"model":"seedance-2.0",
		"input":{"prompt":"smooth transition","duration":5,"aspect_ratio":"16:9","resolution":"720p","start_frames":["assetId://first"],"end_frames":["assetId://last"],"n":1}
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
		"model":"seedance-2.0-fast",
		"input":{"prompt":"coastline","duration":6,"aspect_ratio":"16:9","resolution":"720p","start_frames":["assetId://asset-456"],"n":1}
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
		{name: "too many images", body: `{"model":"seedance-2.0","prompt":"x","input":{"reference_images":["https://e.example/1","https://e.example/2","https://e.example/3","https://e.example/4","https://e.example/5"]}}`, message: "at most 4"},
		{name: "too many videos", body: `{"model":"seedance-2.0","prompt":"x","input":{"video_references":["https://e.example/1.mp4","https://e.example/2.mp4","https://e.example/3.mp4","https://e.example/4.mp4"]}}`, message: "at most 3"},
		{name: "too many audios", body: `{"model":"seedance-2.0","prompt":"x","input":{"audio_references":["https://e.example/1.mp3","https://e.example/2.mp3"]}}`, message: "at most 1"},
		{name: "too many start frames", body: `{"model":"seedance-2.0","prompt":"x","input":{"start_frames":["https://e.example/1.png","https://e.example/2.png"]}}`, message: "at most 1"},
		{name: "too many end frames", body: `{"model":"seedance-2.0","prompt":"x","input":{"start_frames":["https://e.example/start.png"],"end_frames":["https://e.example/1.png","https://e.example/2.png"]}}`, message: "at most 1"},
		{name: "last frame without first", body: `{"model":"seedance-2.0","prompt":"x","input":{"last_image":"https://e.example/end.png"}}`, message: "requires start_frames"},
		{name: "video strength", body: `{"model":"seedance-2.0","prompt":"x","input":{"video_references":[{"video_url":"https://e.example/ref.mp4","strength":0.8}]}}`, message: "do not support strength"},
		{name: "audio strength", body: `{"model":"seedance-2.0","prompt":"x","input":{"audio_references":[{"audio_url":"https://e.example/ref.mp3","strength":0.8}]}}`, message: "do not support strength"},
		{name: "local path", body: `{"model":"seedance-2.0","prompt":"x","input":{"reference_videos":["C:\\\\secret.mp4"]}}`, message: "media reference"},
		{name: "empty reference object", body: `{"model":"seedance-2.0","prompt":"x","input":{"reference_videos":[{"strength":0.8}]}}`, message: "must contain"},
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
		"https://provider.example":                                                "https://provider.example/async/tasks/task%2Fupstream",
		"https://provider.example/v1":                                             "https://provider.example/async/tasks/task%2Fupstream",
		"https://provider.example/async/tasks":                                    "https://provider.example/async/tasks/task%2Fupstream",
		"https://provider.example/prefix/kyyReactApiServer/v2/model-center/tasks": "https://provider.example/prefix/async/tasks/task%2Fupstream",
	}
	for baseURL, want := range tests {
		assert.Equal(t, want, endpointURL(baseURL, "task/upstream"))
		assert.NotContains(t, endpointURL(baseURL, "task/upstream"), "kyyReactApiServer")
		assert.NotContains(t, endpointURL(baseURL, "task/upstream"), "model-center")
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

func TestFetchTaskUsesSeedanceAsyncEndpointAndUpstreamID(t *testing.T) {
	service.InitHttpClient()
	var requestedPath string
	var authorization string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestedPath = r.URL.EscapedPath()
		authorization = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"success":true,"message":"ok","data":{"task_id":"video_seedance_1","status":"processing"}}`)
	}))
	defer server.Close()

	response, err := (&TaskAdaptor{}).FetchTask(server.URL, "channel-key", map[string]any{"task_id": "video_seedance_1"}, "")
	require.NoError(t, err)
	defer response.Body.Close()
	assert.Equal(t, "/async/tasks/video_seedance_1", requestedPath)
	assert.Equal(t, "Bearer channel-key", authorization)
}

func TestValidateRequestStoresSafeTaskDetailsSummary(t *testing.T) {
	context, info := newTaskContext(t, `{
		"model":"seedance-2.0",
		"prompt":"two characters running in neon rain",
		"duration":5,
		"resolution":"720p",
		"images":[
			"https://cdn.example/ref-1.png?api_key=secret&sign=public-sign",
			"https://cdn.example/ref-2.png"
		]
	}`)

	taskErr := (&TaskAdaptor{}).ValidateRequestAndSetAction(context, info)
	require.Nil(t, taskErr)
	assert.Equal(t, "two characters running in neon rain", info.TaskInput)
	require.Len(t, info.ReferenceResources, 2)
	assert.NotContains(t, info.ReferenceResources[0], "secret")
	assert.Contains(t, info.ReferenceResources[0], "api_key=%5BREDACTED%5D")
	assert.Contains(t, info.ReferenceResources[0], "sign=public-sign")

	task := model.InitTask("seedance", info)
	assert.Equal(t, info.TaskInput, task.Properties.Input)
	assert.Equal(t, info.ReferenceResources, task.Properties.ReferenceResources)
}

func TestOpenAIVideosEndToEndUsesSeedanceAsyncContract(t *testing.T) {
	service.InitHttpClient()
	var upstreamMethod, upstreamPath, upstreamAuthorization string
	var upstreamBody []byte
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamMethod = r.Method
		upstreamPath = r.URL.EscapedPath()
		upstreamAuthorization = r.Header.Get("Authorization")
		upstreamBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"success":true,"message":"created","data":{"task_id":"upstream-private","status":"queued","progress":0}}`)
	}))
	defer upstream.Close()

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/v1/videos", func(c *gin.Context) {
		info := &relaycommon.RelayInfo{
			TaskRelayInfo: &relaycommon.TaskRelayInfo{PublicTaskID: "task_public"},
			ChannelMeta:   &relaycommon.ChannelMeta{ChannelBaseUrl: upstream.URL + "/v1", ApiKey: "channel-key", UpstreamModelName: "seedance-2.0"},
		}
		info.OriginModelName = "seedance-2.0"
		adaptor := &TaskAdaptor{}
		adaptor.Init(info)
		if taskErr := adaptor.ValidateRequestAndSetAction(c, info); taskErr != nil {
			c.JSON(taskErr.StatusCode, taskErr)
			return
		}
		body, err := adaptor.BuildRequestBody(c, info)
		if err != nil {
			c.String(http.StatusInternalServerError, err.Error())
			return
		}
		requestURL, _ := adaptor.BuildRequestURL(info)
		request, err := http.NewRequest(http.MethodPost, requestURL, body)
		if err != nil {
			c.String(http.StatusInternalServerError, err.Error())
			return
		}
		_ = adaptor.BuildRequestHeader(c, request, info)
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			c.String(http.StatusBadGateway, err.Error())
			return
		}
		_, _, taskErr := adaptor.DoResponse(c, response, info)
		if taskErr != nil {
			c.JSON(taskErr.StatusCode, taskErr)
		}
	})

	requestBody := `{"model":"seedance-2.0","prompt":"camera orbit","duration":8,"size":"1280x720","resolution":"720p","generate_audio":true,"input_reference":[{"type":"image","image_url":"https://cdn.example/ref.png","strength":0.7},{"type":"video","video_url":"data:video/mp4;base64,YQ=="}]}`
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/videos", strings.NewReader(requestBody))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)

	assert.Equal(t, http.StatusCreated, recorder.Code)
	assert.Equal(t, http.MethodPost, upstreamMethod)
	assert.Equal(t, "/async/tasks", upstreamPath)
	assert.Equal(t, "Bearer channel-key", upstreamAuthorization)
	assert.NotContains(t, upstreamPath, "kyyReactApiServer")
	assert.NotContains(t, upstreamPath, "model-center")
	assert.JSONEq(t, `{"model":"seedance-2.0","input":{"prompt":"camera orbit","duration":8,"aspect_ratio":"16:9","resolution":"720p","audio":true,"image_references":[{"url":"https://cdn.example/ref.png","strength":0.7}],"video_references":["data:video/mp4;base64,YQ=="],"n":1}}`, string(upstreamBody))
	var downstream map[string]any
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &downstream))
	assert.Equal(t, "task_public", downstream["id"])
	assert.Equal(t, "task_public", downstream["task_id"])
	assert.Equal(t, "video", downstream["object"])
	assert.Equal(t, "seedance-2.0", downstream["model"])
	assert.Equal(t, "queued", downstream["status"])
}

func TestParseSeedanceEnvelopeUsesNestedOutputs(t *testing.T) {
	body := []byte(`{"success":true,"message":"ok","data":{"task_id":"upstream-123","status":"SUCCESS","progress":100,"data":{"outputs":[{"url":"https://cdn.example/video.mp4"}]}}}`)
	result, err := (&TaskAdaptor{}).ParseTaskResult(body)
	require.NoError(t, err)
	assert.Equal(t, "upstream-123", result.TaskID)
	assert.Equal(t, model.TaskStatusSuccess, result.Status)
	assert.Equal(t, "https://cdn.example/video.mp4", result.Url)

	task := &model.Task{TaskID: "task_public", Status: model.TaskStatusSuccess, Progress: "100%", Properties: model.Properties{OriginModelName: "seedance-2.0"}, Data: body}
	converted, err := (&TaskAdaptor{}).ConvertToOpenAIVideo(task)
	require.NoError(t, err)
	assert.Contains(t, string(converted), `"status":"completed"`)
	assert.Contains(t, string(converted), `"url":"https://cdn.example/video.mp4"`)
	assert.NotContains(t, string(converted), "upstream-123")
}
