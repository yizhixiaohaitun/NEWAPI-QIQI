package controller

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestFishAudioAutomaticChannelTest(t *testing.T) {
	oldDB, oldRedis, oldMemory := model.DB, common.RedisEnabled, common.MemoryCacheEnabled
	t.Cleanup(func() { model.DB = oldDB; common.RedisEnabled = oldRedis; common.MemoryCacheEnabled = oldMemory })
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.User{}))
	model.DB = db
	common.RedisEnabled, common.MemoryCacheEnabled = false, false
	require.NoError(t, db.Create(&model.User{Id: 1, Username: "fish-test", Status: common.UserStatusEnabled, Group: "default"}).Error)
	service.InitHttpClient()
	var gotModel, gotPath, gotBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotModel, gotPath = r.Header.Get("model"), r.URL.Path
		body, readErr := io.ReadAll(r.Body)
		require.NoError(t, readErr)
		gotBody = string(body)
		w.Header().Set("Content-Type", "audio/mpeg")
		_, _ = w.Write([]byte("mock audio"))
	}))
	defer server.Close()
	channel := &model.Channel{Type: constant.ChannelTypeFishAudio, Key: "test-key", BaseURL: &server.URL}
	for _, name := range []string{"s2.1-pro", "s2-pro"} {
		assert.Equal(t, []constant.EndpointType{constant.EndpointTypeAudioSpeech}, common.GetEndpointTypesByChannelType(constant.ChannelTypeFishAudio, name))
		t.Run(name, func(t *testing.T) {
			gotModel, gotPath, gotBody = "", "", ""
			result := testChannelWithOptions(context.Background(), channel, 1, name, "", false, channelTestOptions{})
			require.NoError(t, result.localErr)
			assert.Equal(t, name, gotModel)
			assert.Equal(t, "/v1/tts", gotPath)
			assert.JSONEq(t, `{"text":"hello world","format":"mp3"}`, gotBody)
		})
	}
	assert.Empty(t, common.GetEndpointTypesByChannelType(constant.ChannelTypeFishAudio, "transcribe-stream-1"))
	endpoint, ok := common.GetDefaultEndpointInfo(constant.EndpointTypeAudioSpeech)
	require.True(t, ok)
	assert.Equal(t, "/v1/audio/speech", endpoint.Path)
	result := testChannelWithOptions(context.Background(), channel, 1, "s2-pro", "", true, channelTestOptions{})
	assert.ErrorContains(t, result.localErr, "non-streaming")
	result = testChannelWithOptions(context.Background(), channel, 1, "s2-pro", string(constant.EndpointTypeAudioSpeech), false, channelTestOptions{})
	require.NoError(t, result.localErr)
	result = testChannelWithOptions(context.Background(), channel, 1, "s2-pro", string(constant.EndpointTypeOpenAI), false, channelTestOptions{})
	assert.ErrorContains(t, result.localErr, "only supports speech synthesis")
	result = testChannelWithOptions(context.Background(), channel, 1, "transcribe-stream-1", "", false, channelTestOptions{})
	assert.ErrorContains(t, result.localErr, "only supports speech synthesis")
}
