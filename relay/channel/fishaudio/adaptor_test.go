package fishaudio

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestConvertAudioRequest(t *testing.T) {
	t.Parallel()
	adaptor := &Adaptor{}
	info := &relaycommon.RelayInfo{RelayMode: relayconstant.RelayModeAudioSpeech}

	reader, err := adaptor.ConvertAudioRequest(nil, info, dto.AudioRequest{
		Input: "hello", Voice: "voice-id", ResponseFormat: "mp3",
	})
	require.NoError(t, err)

	var got map[string]any
	require.NoError(t, json.NewDecoder(reader).Decode(&got))
	require.Equal(t, map[string]any{
		"text": "hello", "reference_id": "voice-id", "format": "mp3",
	}, got)

	reader, err = adaptor.ConvertAudioRequest(nil, info, dto.AudioRequest{Input: "hello"})
	require.NoError(t, err)
	body, err := io.ReadAll(reader)
	require.NoError(t, err)
	require.NotContains(t, string(body), "reference_id")
}

func TestDoRequestMapsHeadersAndBody(t *testing.T) {
	service.InitHttpClient()
	const fishKey = "test-fish-key"
	var received bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received = true
		require.Equal(t, "/v1/tts", r.URL.Path)
		require.Equal(t, "Bearer "+fishKey, r.Header.Get("Authorization"))
		require.Equal(t, "mapped-model", r.Header.Get("model"))
		require.Equal(t, "application/json", r.Header.Get("Content-Type"))
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		require.JSONEq(t, `{"text":"hello","reference_id":"voice-id","format":"wav"}`, string(body))
		w.Header().Set("Content-Type", "audio/wav")
		_, _ = w.Write([]byte("audio"))
	}))
	defer server.Close()

	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/audio/speech", nil)
	info := &relaycommon.RelayInfo{
		RelayMode: relayconstant.RelayModeAudioSpeech,
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelBaseUrl: server.URL, ApiKey: fishKey, UpstreamModelName: "mapped-model",
		},
	}
	adaptor := &Adaptor{}
	body, err := adaptor.ConvertAudioRequest(ctx, info, dto.AudioRequest{Input: "hello", Voice: "voice-id", ResponseFormat: "wav"})
	require.NoError(t, err)
	resp, err := adaptor.DoRequest(ctx, info, body)
	require.NoError(t, err)
	require.True(t, received)
	require.Equal(t, http.StatusOK, resp.(*http.Response).StatusCode)
	_ = resp.(*http.Response).Body.Close()
}

func TestDoRequestHonorsClientCancellation(t *testing.T) {
	service.InitHttpClient()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer server.Close()

	gin.SetMode(gin.TestMode)
	requestContext, cancel := context.WithCancel(context.Background())
	cancel()
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/audio/speech", nil).WithContext(requestContext)
	info := &relaycommon.RelayInfo{
		RelayMode:   relayconstant.RelayModeAudioSpeech,
		ChannelMeta: &relaycommon.ChannelMeta{ChannelBaseUrl: server.URL, ApiKey: "unused", UpstreamModelName: "model"},
	}

	_, err := (&Adaptor{}).DoRequest(ctx, info, strings.NewReader(`{"text":"hello"}`))
	require.Error(t, err)
	require.ErrorIs(t, ctx.Request.Context().Err(), context.Canceled)
}

func TestDoResponsePreservesAudioAndSanitizesContentType(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/audio/speech", nil)
	request := &dto.AudioRequest{Input: "hello", ResponseFormat: "pcm"}
	info := &relaycommon.RelayInfo{Request: request}
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/html; charset=utf-8"}},
		Body:       io.NopCloser(strings.NewReader("audio-bytes")),
	}

	usage, apiErr := (&Adaptor{}).DoResponse(ctx, resp, info)
	require.Nil(t, apiErr)
	require.NotNil(t, usage)
	require.Equal(t, "application/octet-stream", recorder.Header().Get("Content-Type"))
	require.Equal(t, "audio-bytes", recorder.Body.String())
}

func TestDoResponseRejectsEmptyAudio(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/audio/speech", nil)
	resp := &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(""))}

	usage, apiErr := (&Adaptor{}).DoResponse(ctx, resp, &relaycommon.RelayInfo{})
	require.Nil(t, usage)
	require.NotNil(t, apiErr)
	require.Equal(t, http.StatusBadGateway, apiErr.StatusCode)
}
