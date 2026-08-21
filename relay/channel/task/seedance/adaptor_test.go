package seedance

import (
	"bytes"
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
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const validRequest = `{
  "model":"seedance-2.0",
  "input":{
    "prompt":"a paper boat on a river",
    "duration":"5",
    "aspect_ratio":"16:9",
    "resolution":"720P",
    "audio":false,
    "image_references":["https://cdn.example/image.png",{"url":"data:image/png;base64,YQ==","type":"reference"}],
    "video_references":["YQ=="],
    "audio_references":[],
    "start_frames":[],
    "end_frames":[],
    "n":1
  }
}`

func newTaskContext(t *testing.T, body string) (*gin.Context, *relaycommon.RelayInfo) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	request := httptest.NewRequest(http.MethodPost, "/async/tasks", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	context.Request = request
	return context, &relaycommon.RelayInfo{
		TaskRelayInfo: &relaycommon.TaskRelayInfo{},
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "seedance-2.0",
		},
	}
}

func TestNestedRequestBillingAndForwarding(t *testing.T) {
	context, info := newTaskContext(t, validRequest)
	adaptor := &TaskAdaptor{}

	require.Nil(t, adaptor.ValidateRequestAndSetAction(context, info))
	ratios := adaptor.EstimateBilling(context, info)
	assert.Equal(t, float64(5), ratios["seconds"])
	assert.Equal(t, float64(1), ratios["resolution"])

	reader, err := adaptor.BuildRequestBody(context, info)
	require.NoError(t, err)
	body, err := io.ReadAll(reader)
	require.NoError(t, err)
	var payload map[string]any
	require.NoError(t, common.Unmarshal(body, &payload))
	assert.Len(t, payload, 2)
	assert.Equal(t, "seedance-2.0", payload["model"])
	input := payload["input"].(map[string]any)
	assert.Equal(t, float64(5), input["duration"])
	assert.Equal(t, "720p", input["resolution"])
	assert.Equal(t, false, input["audio"])
	assert.Len(t, input["image_references"], 2)
	assert.Len(t, input["video_references"], 1)
	assert.Contains(t, input, "audio_references")
	assert.NotContains(t, payload, "duration")
}

func TestOptionalDefaultsAreForwarded(t *testing.T) {
	body := strings.Replace(validRequest, `,
    "audio":false`, "", 1)
	body = strings.Replace(body, `,
    "n":1`, "", 1)
	context, info := newTaskContext(t, body)
	adaptor := &TaskAdaptor{}
	require.Nil(t, adaptor.ValidateRequestAndSetAction(context, info))
	reader, err := adaptor.BuildRequestBody(context, info)
	require.NoError(t, err)
	payloadBody, err := io.ReadAll(reader)
	require.NoError(t, err)
	var payload map[string]any
	require.NoError(t, common.Unmarshal(payloadBody, &payload))
	input := payload["input"].(map[string]any)
	assert.Equal(t, float64(1), input["n"])
	assert.Equal(t, true, input["audio"])
}

func TestExplicitNOneIsAccepted(t *testing.T) {
	context, info := newTaskContext(t, validRequest)
	require.Nil(t, (&TaskAdaptor{}).ValidateRequestAndSetAction(context, info))
}

func TestResolutionBillingRatios(t *testing.T) {
	tests := []struct {
		resolution string
		wantRatio  float64
		wantQuota  float64
	}{
		{resolution: "480p", wantRatio: 0.5, wantQuota: 25},
		{resolution: "720p", wantRatio: 1, wantQuota: 50},
		{resolution: "1080p", wantRatio: 2.5, wantQuota: 125},
	}

	for _, test := range tests {
		t.Run(test.resolution, func(t *testing.T) {
			body := strings.Replace(validRequest, "720P", test.resolution, 1)
			context, info := newTaskContext(t, body)
			adaptor := &TaskAdaptor{}
			require.Nil(t, adaptor.ValidateRequestAndSetAction(context, info))

			ratios := adaptor.EstimateBilling(context, info)
			assert.Equal(t, test.wantRatio, ratios["resolution"])
			priceData := types.PriceData{}
			for name, ratio := range ratios {
				priceData.AddOtherRatio(name, ratio)
			}
			assert.Equal(t, test.wantQuota, priceData.ApplyOtherRatiosToFloat(10))
		})
	}
}

func Test1080pBillingAndDurationBoundary(t *testing.T) {
	body := strings.ReplaceAll(validRequest, `"duration":"5"`, `"duration":12`)
	body = strings.ReplaceAll(body, `"resolution":"720P"`, `"resolution":"1080p"`)
	context, info := newTaskContext(t, body)
	adaptor := &TaskAdaptor{}
	require.Nil(t, adaptor.ValidateRequestAndSetAction(context, info))
	assert.Equal(t, float64(12), adaptor.EstimateBilling(context, info)["seconds"])
	assert.Equal(t, float64(2.5), adaptor.EstimateBilling(context, info)["resolution"])

	invalidContext, invalidInfo := newTaskContext(t, strings.ReplaceAll(body, `"duration":12`, `"duration":13`))
	assert.Contains(t, adaptor.ValidateRequestAndSetAction(invalidContext, invalidInfo).Message, "between 4 and 12")
}

func TestSeedanceValidationBoundaries(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(string) string
		message string
	}{
		{"unsupported model", func(s string) string { return strings.Replace(s, "seedance-2.0", "seedance-1.0", 1) }, "model must"},
		{"fast 1080p", func(s string) string {
			s = strings.Replace(s, "seedance-2.0", "seedance-2.0-fast", 1)
			return strings.Replace(s, "720P", "1080p", 1)
		}, "480p or 720p"},
		{"duration too short", func(s string) string { return strings.Replace(s, `"duration":"5"`, `"duration":3`, 1) }, "between 4 and 15"},
		{"fractional duration", func(s string) string { return strings.Replace(s, `"duration":"5"`, `"duration":5.5`, 1) }, "integer"},
		{"bad ratio", func(s string) string { return strings.Replace(s, "16:9", "4:3", 1) }, "aspect_ratio"},
		{"n fixed", func(s string) string { return strings.Replace(s, `"n":1`, `"n":2`, 1) }, "input.n must be 1"},
		{"audio type", func(s string) string { return strings.Replace(s, `"audio":false`, `"audio":"false"`, 1) }, "input.audio must be a boolean"},
		{"too many images", func(s string) string {
			return strings.Replace(s, `"image_references":["https://cdn.example/image.png",{"url":"data:image/png;base64,YQ==","type":"reference"}]`, `"image_references":["YQ==","YQ==","YQ==","YQ==","YQ=="]`, 1)
		}, "at most 4"},
		{"end requires start", func(s string) string { return strings.Replace(s, `"end_frames":[]`, `"end_frames":["YQ=="]`, 1) }, "requires input.start_frames"},
		{"local path", func(s string) string {
			return strings.Replace(s, `"video_references":["YQ=="]`, `"video_references":["C:\\\\secret.mp4"]`, 1)
		}, "local file paths"},
		{"empty reference object", func(s string) string {
			return strings.Replace(s, `"video_references":["YQ=="]`, `"video_references":[{"strength":0.8}]`, 1)
		}, "must contain"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			context, info := newTaskContext(t, test.mutate(validRequest))
			taskErr := (&TaskAdaptor{}).ValidateRequestAndSetAction(context, info)
			require.NotNil(t, taskErr)
			assert.Contains(t, taskErr.Message, test.message)
		})
	}
}

func TestCreateResponsePreservesEnvelopeAndHidesUpstreamID(t *testing.T) {
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodPost, "/async/tasks", nil)
	responseBody := `{"success":true,"message":"created","data":{"task_id":"upstream-private","id":"upstream-create-id","data":{"id":"upstream-create-nested"},"status":"PENDING","action":"generate","progress":0,"platform":"seedance","model":"seedance-2.0"}}`
	response := &http.Response{Body: io.NopCloser(strings.NewReader(responseBody))}
	info := &relaycommon.RelayInfo{TaskRelayInfo: &relaycommon.TaskRelayInfo{PublicTaskID: "task_public"}}

	upstreamID, stored, taskErr := (&TaskAdaptor{}).DoResponse(context, response, info)
	require.Nil(t, taskErr)
	assert.Equal(t, "upstream-private", upstreamID)
	assert.Contains(t, string(stored), "upstream-private")
	assert.NotContains(t, recorder.Body.String(), "upstream-private")
	assert.JSONEq(t, `{"success":true,"message":"created","data":{"task_id":"task_public","id":"task_public","data":{"id":"task_public"},"status":"PENDING","action":"generate","progress":0,"platform":"seedance","model":"seedance-2.0"}}`, recorder.Body.String())
}

func TestQuerySuccessFailureAndPublicIDReplacement(t *testing.T) {
	adaptor := &TaskAdaptor{}
	successBody := []byte(`{"success":true,"message":"ok","data":{"task_id":"upstream-private","status":"SUCCESS","progress":100,"fail_reason":"","data":{"id":"upstream-result-id","outputs":[{"url":"https://cdn.example/video.mp4","type":"video"}]}}}`)
	result, err := adaptor.ParseTaskResult(successBody)
	require.NoError(t, err)
	assert.Equal(t, model.TaskStatusSuccess, result.Status)
	assert.Equal(t, "100%", result.Progress)
	assert.Equal(t, "https://cdn.example/video.mp4", result.Url)

	converted, err := adaptor.ConvertTaskResponse(&model.Task{TaskID: "task_public", Data: successBody})
	require.NoError(t, err)
	assert.NotContains(t, string(converted), "upstream-private")
	assert.NotContains(t, string(converted), "upstream-result-id")
	assert.Contains(t, string(converted), "task_public")
	assert.Contains(t, string(converted), "outputs")

	failure, err := adaptor.ParseTaskResult([]byte(`{"success":true,"data":{"task_id":"upstream-failed","status":"FAILURE","progress":"100%","fail_reason":"生成参数校验失败，请检查模型是否支持当前参数","data":{"outputs":[]}}}`))
	require.NoError(t, err)
	assert.Equal(t, model.TaskStatusFailure, failure.Status)
	assert.Equal(t, "100%", failure.Progress)
	assert.Equal(t, "生成参数校验失败，请检查模型是否支持当前参数", failure.Reason)
}

func TestOpenAIVideosTranslatesToOfficialAsyncContract(t *testing.T) {
	service.InitHttpClient()
	var gotMethod, gotPath string
	var gotBody []byte
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.EscapedPath()
		gotBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"success":true,"message":"created","data":{"task_id":"upstream-1","status":"queued"}}`)
	}))
	defer upstream.Close()

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/v1/videos", func(c *gin.Context) {
		info := &relaycommon.RelayInfo{TaskRelayInfo: &relaycommon.TaskRelayInfo{PublicTaskID: "task_public"}, ChannelMeta: &relaycommon.ChannelMeta{ChannelBaseUrl: upstream.URL + "/v1", ApiKey: "key", UpstreamModelName: "seedance-2.0"}}
		info.OriginModelName = "seedance-2.0"
		adaptor := &TaskAdaptor{Protocol: "seedance_async"}
		adaptor.Init(info)
		if taskErr := adaptor.ValidateRequestAndSetAction(c, info); taskErr != nil {
			c.JSON(taskErr.StatusCode, taskErr)
			return
		}
		body, err := adaptor.BuildRequestBody(c, info)
		require.NoError(t, err)
		requestURL, err := adaptor.BuildRequestURL(info)
		require.NoError(t, err)
		req, err := http.NewRequest(http.MethodPost, requestURL, body)
		require.NoError(t, err)
		require.NoError(t, adaptor.BuildRequestHeader(c, req, info))
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		_, _, taskErr := adaptor.DoResponse(c, resp, info)
		require.Nil(t, taskErr)
	})

	body := `{"model":"seedance-2.0","prompt":"camera orbit","seconds":"8","size":"1280x720","generate_audio":false,"fps":24,"watermark":true,"input_reference":[{"type":"image","image_url":"https://cdn.example/first.png","strength":0.7},{"type":"video","video_url":"https://cdn.example/ref.mp4"}]}`
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/videos", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)

	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Equal(t, http.MethodPost, gotMethod)
	assert.Equal(t, "/async/tasks", gotPath)
	assert.NotContains(t, gotPath, "kyyReactApiServer")
	assert.JSONEq(t, `{"model":"seedance-2.0","input":{"prompt":"camera orbit","duration":8,"aspect_ratio":"16:9","resolution":"720p","audio":false,"start_frames":[{"url":"https://cdn.example/first.png","strength":0.7}],"video_references":["https://cdn.example/ref.mp4"],"n":1}}`, string(gotBody))
	assert.JSONEq(t, `{"id":"task_public","object":"video","model":"seedance-2.0","status":"queued","progress":20}`, recorder.Body.String())
}

func TestDiscountProtocolPathBodyAndResult(t *testing.T) {
	context, info := newTaskContext(t, `{"model":"seedance-2.0-fast","prompt":"ocean","duration":6,"size":"1280x720","input_reference":[{"type":"video","video_url":"https://cdn.example/ref.mp4"}]}`)
	context.Request.URL.Path = "/v1/videos"
	info.UpstreamModelName = "seedance-2.0-fast"
	info.ChannelBaseUrl = "https://provider.example/v1"
	adaptor := &TaskAdaptor{Protocol: "seedance_discount"}
	adaptor.Init(info)
	require.Nil(t, adaptor.ValidateRequestAndSetAction(context, info))
	requestURL, err := adaptor.BuildRequestURL(info)
	require.NoError(t, err)
	assert.Equal(t, "https://provider.example/kyyReactApiServer/v1/seedance-discount/videos", requestURL)
	reader, err := adaptor.BuildRequestBody(context, info)
	require.NoError(t, err)
	body, err := io.ReadAll(reader)
	require.NoError(t, err)
	assert.JSONEq(t, `{"model":"sd_2.0_fast_discount_720p_with_video_ref","ratio":"16:9","duration":6,"generate_audio":true,"content":[{"type":"text","text":"ocean"},{"type":"video_url","role":"reference_video","video_url":{"url":"https://cdn.example/ref.mp4"}}]}`, string(body))

	result, err := adaptor.ParseTaskResult([]byte(`{"id":"upstream-discount","status":"completed","video_url":"https://cdn.example/result.mp4","totalTokens":42}`))
	require.NoError(t, err)
	assert.Equal(t, model.TaskStatusSuccess, result.Status)
	assert.Equal(t, "https://cdn.example/result.mp4", result.Url)
	assert.Equal(t, 42, result.TotalTokens)

	var queryPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		queryPath = r.URL.EscapedPath()
		_, _ = io.WriteString(w, `{"id":"upstream-discount","status":"processing"}`)
	}))
	defer server.Close()
	resp, err := adaptor.FetchTask(server.URL+"/v1", "key", map[string]any{"task_id": "upstream/id"}, "")
	require.NoError(t, err)
	resp.Body.Close()
	assert.Equal(t, "/kyyReactApiServer/v1/result/upstream%2Fid", queryPath)
}

func TestVideoReferenceStrengthRejected(t *testing.T) {
	context, info := newTaskContext(t, `{"model":"seedance-2.0","prompt":"x","input_reference":[{"type":"video","video_url":"https://cdn.example/ref.mp4","strength":0.5}]}`)
	context.Request.URL.Path = "/v1/videos"
	taskErr := (&TaskAdaptor{}).ValidateRequestAndSetAction(context, info)
	require.NotNil(t, taskErr)
	assert.Contains(t, taskErr.Message, "does not support strength")
}

func TestMultipartInputReferenceBecomesStartFrameAndBooleanIsNormalized(t *testing.T) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for key, value := range map[string]string{"model": "seedance-2.0", "prompt": "sunrise", "seconds": "4", "size": "1280x720", "generate_audio": "false"} {
		require.NoError(t, writer.WriteField(key, value))
	}
	part, err := writer.CreateFormFile("input_reference", "first.png")
	require.NoError(t, err)
	_, err = part.Write([]byte("png-data"))
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	context.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", &body)
	context.Request.Header.Set("Content-Type", writer.FormDataContentType())
	info := &relaycommon.RelayInfo{TaskRelayInfo: &relaycommon.TaskRelayInfo{}, ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "seedance-2.0"}}
	adaptor := &TaskAdaptor{Protocol: "seedance_async"}
	require.Nil(t, adaptor.ValidateRequestAndSetAction(context, info))
	reader, err := adaptor.BuildRequestBody(context, info)
	require.NoError(t, err)
	encoded, err := io.ReadAll(reader)
	require.NoError(t, err)
	var payload map[string]any
	require.NoError(t, common.Unmarshal(encoded, &payload))
	input := payload["input"].(map[string]any)
	assert.Equal(t, false, input["audio"])
	assert.Len(t, input["start_frames"], 1)
	assert.Contains(t, input["start_frames"].([]any)[0], "data:")
	assert.NotContains(t, input, "image_references")
}

func TestHTMLUpstreamErrorIsReadableAndDoesNotLeakDocument(t *testing.T) {
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", nil)
	resp := &http.Response{StatusCode: 500, Header: http.Header{"Content-Type": []string{"text/html; charset=utf-8"}}, Body: io.NopCloser(strings.NewReader(`<!doctype html><meta name=generator content=new-api><div id=root></div>`))}
	_, _, taskErr := (&TaskAdaptor{Protocol: "seedance_async"}).DoResponse(context, resp, &relaycommon.RelayInfo{TaskRelayInfo: &relaycommon.TaskRelayInfo{}})
	require.NotNil(t, taskErr)
	assert.Contains(t, taskErr.Message, "status=500")
	assert.Contains(t, taskErr.Message, "text/html")
	assert.Contains(t, taskErr.Message, "HTML instead of JSON")
	assert.NotContains(t, taskErr.Message, "<!doctype")
	assert.NotContains(t, taskErr.Message, "invalid character '<'")
}

func TestFetchTaskUsesDocumentedPathAndUpstreamID(t *testing.T) {
	service.InitHttpClient()
	var requestedPath string
	var authorization string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestedPath = r.URL.EscapedPath()
		authorization = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"success":true,"data":{"task_id":"upstream/id","status":"RUNNING","progress":50}}`)
	}))
	defer server.Close()

	response, err := (&TaskAdaptor{}).FetchTask(server.URL, "channel-key", map[string]any{"task_id": "upstream/id"}, "")
	require.NoError(t, err)
	defer response.Body.Close()
	assert.Equal(t, "/async/tasks/upstream%2Fid", requestedPath)
	assert.Equal(t, "Bearer channel-key", authorization)
}
