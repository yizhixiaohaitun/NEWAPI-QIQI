package openai

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/relay/channel"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// This exercises the real HTTP transport and the OpenAI adaptor response path.
// The upstream sends successful headers and then stalls its body, reproducing
// the case that previously committed a partial HTTP 200 before ReadAll failed.
func TestOpenaiTTSBodyDeadlineCancelsUpstreamAndReturnsStableTimeout(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service.InitHttpClient()

	upstreamCanceled := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "audio/mpeg")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "partial-audio")
		w.(http.Flusher).Flush()
		<-r.Context().Done()
		close(upstreamCanceled)
	}))
	defer upstream.Close()

	handlerDone := make(chan struct{})
	router := gin.New()
	router.POST("/v1/audio/speech", func(c *gin.Context) {
		defer close(handlerDone)
		upstreamCtx, cancel := context.WithTimeout(c.Request.Context(), 100*time.Millisecond)
		defer cancel()
		info := &relaycommon.RelayInfo{
			RelayMode:       relayconstant.RelayModeAudioSpeech,
			UpstreamContext: upstreamCtx,
			ChannelMeta:     &relaycommon.ChannelMeta{ChannelBaseUrl: upstream.URL},
			Request:         &dto.AudioRequest{ResponseFormat: "mp3"},
		}
		req, err := http.NewRequest(http.MethodPost, upstream.URL, strings.NewReader(`{}`))
		var relayErr *types.NewAPIError
		if err == nil {
			var resp *http.Response
			resp, err = channel.DoRequest(c, req, info)
			if err == nil {
				_, relayErr = (&Adaptor{}).DoResponse(c, resp, info)
			}
		}
		if c.Writer.Written() {
			t.Error("TTS response was committed before the complete body was read")
			return
		}
		if upstreamCtx.Err() == context.DeadlineExceeded {
			relayErr = service.NewUpstreamTimeoutError()
		} else if relayErr == nil && err != nil {
			relayErr = types.NewOpenAIError(err, types.ErrorCodeDoRequestFailed, http.StatusInternalServerError)
		}
		require.NotNil(t, relayErr)
		c.JSON(relayErr.StatusCode, gin.H{"error": gin.H{"code": relayErr.GetErrorCode()}})
	})
	downstream := httptest.NewServer(router)
	defer downstream.Close()

	resp, err := http.Post(downstream.URL+"/v1/audio/speech", "application/json", strings.NewReader(`{}`))
	require.NoError(t, err)
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusInternalServerError, resp.StatusCode)
	require.Contains(t, string(body), string(types.ErrorCodeUpstreamTimeout))

	select {
	case <-upstreamCanceled:
	case <-time.After(2 * time.Second):
		t.Fatal("TTS upstream did not observe request cancellation")
	}
	select {
	case <-handlerDone:
	case <-time.After(2 * time.Second):
		t.Fatal("TTS downstream handler did not finish after the deadline")
	}
}
