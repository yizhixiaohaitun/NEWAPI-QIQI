package openai

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newFishAudioTestContext() *gin.Context {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/audio/speech", nil)
	c.Request.Header.Set("Content-Type", "application/json")
	return c
}

func newFishAudioRelayInfo() *relaycommon.RelayInfo {
	return &relaycommon.RelayInfo{
		RelayMode: relayconstant.RelayModeAudioSpeech,
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType:       constant.ChannelTypeCustom,
			ChannelBaseUrl:    "https://api.fish.audio/v1/tts",
			UpstreamModelName: "s2.1-pro",
			ApiKey:            "fish-test-key",
		},
	}
}

func decodeFishAudioRequest(t *testing.T, reader io.Reader) map[string]any {
	t.Helper()
	data, err := io.ReadAll(reader)
	require.NoError(t, err)
	var body map[string]any
	require.NoError(t, common.Unmarshal(data, &body))
	return body
}

func TestFishAudioCustomChannelConvertsSpeechContract(t *testing.T) {
	adaptor := &Adaptor{}
	info := newFishAudioRelayInfo()
	c := newFishAudioTestContext()

	reader, err := adaptor.ConvertAudioRequest(c, info, dto.AudioRequest{
		Model:          "s2.1-pro",
		Input:          "你好",
		Voice:          "reference-id",
		ResponseFormat: "wav",
	})
	require.NoError(t, err)
	assert.Equal(t, map[string]any{
		"text":         "你好",
		"reference_id": "reference-id",
		"format":       "wav",
	}, decodeFishAudioRequest(t, reader))

	header := http.Header{}
	require.NoError(t, adaptor.SetupRequestHeader(c, &header, info))
	assert.Equal(t, "Bearer fish-test-key", header.Get("Authorization"))
	assert.Equal(t, "application/json", header.Get("Content-Type"))
	assert.Equal(t, "s2.1-pro", header.Get("model"))
}

func TestFishAudioCustomChannelOmitsEmptyReferenceID(t *testing.T) {
	adaptor := &Adaptor{}
	info := newFishAudioRelayInfo()
	info.ChannelBaseUrl += "/"

	reader, err := adaptor.ConvertAudioRequest(newFishAudioTestContext(), info, dto.AudioRequest{
		Model: "s2.1-pro",
		Input: "使用默认音色",
	})
	require.NoError(t, err)
	body := decodeFishAudioRequest(t, reader)
	assert.Equal(t, "使用默认音色", body["text"])
	assert.Equal(t, "mp3", body["format"])
	assert.NotContains(t, body, "reference_id")
}

func TestFishAudioCustomChannelRejectsInvalidSpeechInput(t *testing.T) {
	adaptor := &Adaptor{}
	info := newFishAudioRelayInfo()
	c := newFishAudioTestContext()

	_, err := adaptor.ConvertAudioRequest(c, info, dto.AudioRequest{Model: "s2.1-pro", Input: "  "})
	require.EqualError(t, err, "input is required")

	_, err = adaptor.ConvertAudioRequest(c, info, dto.AudioRequest{
		Model:          "s2.1-pro",
		Input:          "你好",
		ResponseFormat: "flac",
	})
	require.EqualError(t, err, "unsupported Fish Audio response format: flac")
}

func TestOtherCustomAudioChannelKeepsOpenAIContract(t *testing.T) {
	adaptor := &Adaptor{}
	info := newFishAudioRelayInfo()
	info.ChannelBaseUrl = "https://voice.example/v1/audio/speech"
	c := newFishAudioTestContext()

	reader, err := adaptor.ConvertAudioRequest(c, info, dto.AudioRequest{
		Model:          "tts-1",
		Input:          "hello",
		Voice:          "alloy",
		ResponseFormat: "mp3",
	})
	require.NoError(t, err)
	data, err := io.ReadAll(reader)
	require.NoError(t, err)
	assert.Contains(t, string(data), `"input":"hello"`)
	assert.NotContains(t, string(data), `"text"`)

	header := http.Header{}
	require.NoError(t, adaptor.SetupRequestHeader(c, &header, info))
	assert.Empty(t, header.Get("model"))
	assert.True(t, strings.HasPrefix(header.Get("Authorization"), "Bearer "))
}
