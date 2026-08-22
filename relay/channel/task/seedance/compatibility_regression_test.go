package seedance

import (
	"bytes"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPublicRequestRequiresDuration(t *testing.T) {
	context, info := newTaskContext(t, `{"model":"seedance-2.0","prompt":"missing duration"}`)
	taskErr := (&TaskAdaptor{}).ValidateRequestAndSetAction(context, info)
	require.NotNil(t, taskErr)
	assert.Equal(t, http.StatusBadRequest, taskErr.StatusCode)
	assert.Contains(t, taskErr.Message, "duration is required")
}

func TestPublicRequestDoesNotDefaultAudio(t *testing.T) {
	context, info := newTaskContext(t, `{"model":"seedance-2.0","prompt":"no default audio","duration":4}`)
	adaptor := &TaskAdaptor{}
	require.Nil(t, adaptor.ValidateRequestAndSetAction(context, info))

	reader, err := adaptor.BuildRequestBody(context, info)
	require.NoError(t, err)
	body, err := io.ReadAll(reader)
	require.NoError(t, err)
	assert.JSONEq(t, `{"model":"seedance-2.0","input":{"prompt":"no default audio","duration":4,"resolution":"720p","aspect_ratio":"16:9","n":1}}`, string(body))
}

func TestPublicRequestRejectsTopLevelInputConflicts(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		message string
	}{
		{name: "duration", body: `{"model":"seedance-2.0","prompt":"x","duration":5,"input":{"duration":6}}`, message: "conflicts"},
		{name: "duration alias", body: `{"model":"seedance-2.0","prompt":"x","seconds":"5","input":{"duration":6}}`, message: "conflicts"},
		{name: "resolution", body: `{"model":"seedance-2.0","prompt":"x","duration":5,"resolution":"720p","input":{"resolution":"1080p"}}`, message: "conflicts"},
		{name: "aspect ratio", body: `{"model":"seedance-2.0","prompt":"x","duration":5,"aspect_ratio":"16:9","input":{"aspect_ratio":"9:16"}}`, message: "conflicts"},
		{name: "generate audio", body: `{"model":"seedance-2.0","prompt":"x","duration":5,"generate_audio":true,"input":{"audio":false}}`, message: "conflicts"},
		{name: "size resolution", body: `{"model":"seedance-2.0","prompt":"x","duration":5,"size":"1280x720","input":{"resolution":"1080p"}}`, message: "conflicts"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			context, info := newTaskContext(t, test.body)
			taskErr := (&TaskAdaptor{}).ValidateRequestAndSetAction(context, info)
			require.NotNil(t, taskErr)
			assert.Equal(t, http.StatusBadRequest, taskErr.StatusCode)
			assert.Contains(t, taskErr.Message, test.message)
		})
	}
}

func TestPublicRequestRejectsMalformedTopLevelScalars(t *testing.T) {
	for _, test := range []struct {
		name    string
		body    string
		message string
	}{
		{name: "duration string", body: `{"model":"seedance-2.0","prompt":"x","duration":"five"}`, message: "duration must be an integer"},
		{name: "duration fraction", body: `{"model":"seedance-2.0","prompt":"x","duration":4.5}`, message: "duration must be an integer"},
		{name: "duration zero", body: `{"model":"seedance-2.0","prompt":"x","duration":0}`, message: "between 4 and 15"},
		{name: "duration zero conflict", body: `{"model":"seedance-2.0","prompt":"x","duration":0,"input":{"duration":6}}`, message: "conflicts"},
		{name: "generate audio", body: `{"model":"seedance-2.0","prompt":"x","generate_audio":"yes"}`, message: "generate_audio must be a boolean"},
	} {
		t.Run(test.name, func(t *testing.T) {
			context, info := newTaskContext(t, test.body)
			taskErr := (&TaskAdaptor{}).ValidateRequestAndSetAction(context, info)
			require.NotNil(t, taskErr)
			assert.Equal(t, http.StatusBadRequest, taskErr.StatusCode)
			assert.Contains(t, taskErr.Message, test.message)
		})
	}
}

func TestMappedUpstreamModelMustBeSupported(t *testing.T) {
	context, info := newTaskContext(t, `{"model":"seedance-2.0","prompt":"mapped model","duration":5}`)
	adaptor := &TaskAdaptor{}
	require.Nil(t, adaptor.ValidateRequestAndSetAction(context, info))
	info.UpstreamModelName = "sd_2.0_unknown"

	_, err := adaptor.BuildRequestBody(context, info)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "mapped upstream model")
}

func TestMultipartAudioIsParsedAndConflictsAreRejected(t *testing.T) {
	buildContext := func(t *testing.T, generateAudio, audio string) (*gin.Context, *relaycommon.RelayInfo) {
		t.Helper()
		var body bytes.Buffer
		writer := multipart.NewWriter(&body)
		fields := map[string]string{
			"model": "seedance-2.0", "prompt": "coastline", "seconds": "4", "size": "1280x720", "audio": audio,
		}
		if generateAudio != "" {
			fields["generate_audio"] = generateAudio
		}
		for key, value := range fields {
			require.NoError(t, writer.WriteField(key, value))
		}
		require.NoError(t, writer.Close())

		context, _ := gin.CreateTestContext(httptest.NewRecorder())
		context.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", &body)
		context.Request.Header.Set("Content-Type", writer.FormDataContentType())
		t.Cleanup(func() { common.CleanupBodyStorage(context) })
		return context, &relaycommon.RelayInfo{TaskRelayInfo: &relaycommon.TaskRelayInfo{}, ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "seedance-2.0"}}
	}

	context, info := buildContext(t, "", "false")
	adaptor := &TaskAdaptor{}
	require.Nil(t, adaptor.ValidateRequestAndSetAction(context, info))
	reader, err := adaptor.BuildRequestBody(context, info)
	require.NoError(t, err)
	body, err := io.ReadAll(reader)
	require.NoError(t, err)
	assert.Contains(t, string(body), `"audio":false`)

	conflictContext, conflictInfo := buildContext(t, "true", "false")
	taskErr := adaptor.ValidateRequestAndSetAction(conflictContext, conflictInfo)
	require.NotNil(t, taskErr)
	assert.Contains(t, taskErr.Message, "generate_audio conflicts with audio")
}
