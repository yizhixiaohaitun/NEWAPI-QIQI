package channel

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
)

type timeoutWebSocketAdaptor struct {
	url string
}

func (a *timeoutWebSocketAdaptor) Init(*relaycommon.RelayInfo) {}
func (a *timeoutWebSocketAdaptor) GetRequestURL(*relaycommon.RelayInfo) (string, error) {
	return a.url, nil
}
func (a *timeoutWebSocketAdaptor) SetupRequestHeader(*gin.Context, *http.Header, *relaycommon.RelayInfo) error {
	return nil
}
func (a *timeoutWebSocketAdaptor) ConvertOpenAIRequest(*gin.Context, *relaycommon.RelayInfo, *dto.GeneralOpenAIRequest) (any, error) {
	return nil, nil
}
func (a *timeoutWebSocketAdaptor) ConvertRerankRequest(*gin.Context, int, dto.RerankRequest) (any, error) {
	return nil, nil
}
func (a *timeoutWebSocketAdaptor) ConvertEmbeddingRequest(*gin.Context, *relaycommon.RelayInfo, dto.EmbeddingRequest) (any, error) {
	return nil, nil
}
func (a *timeoutWebSocketAdaptor) ConvertAudioRequest(*gin.Context, *relaycommon.RelayInfo, dto.AudioRequest) (io.Reader, error) {
	return nil, nil
}
func (a *timeoutWebSocketAdaptor) ConvertImageRequest(*gin.Context, *relaycommon.RelayInfo, dto.ImageRequest) (any, error) {
	return nil, nil
}
func (a *timeoutWebSocketAdaptor) ConvertOpenAIResponsesRequest(*gin.Context, *relaycommon.RelayInfo, dto.OpenAIResponsesRequest) (any, error) {
	return nil, nil
}
func (a *timeoutWebSocketAdaptor) DoRequest(*gin.Context, *relaycommon.RelayInfo, io.Reader) (any, error) {
	return nil, nil
}
func (a *timeoutWebSocketAdaptor) DoResponse(*gin.Context, *http.Response, *relaycommon.RelayInfo) (any, *types.NewAPIError) {
	return nil, nil
}
func (a *timeoutWebSocketAdaptor) GetModelList() []string { return nil }
func (a *timeoutWebSocketAdaptor) GetChannelName() string { return "timeout-test" }
func (a *timeoutWebSocketAdaptor) ConvertClaudeRequest(*gin.Context, *relaycommon.RelayInfo, *dto.ClaudeRequest) (any, error) {
	return nil, nil
}
func (a *timeoutWebSocketAdaptor) ConvertGeminiRequest(*gin.Context, *relaycommon.RelayInfo, *dto.GeminiChatRequest) (any, error) {
	return nil, nil
}

func TestDoRequestCancelsNonStreamHTTPUpstreamAtDeadline(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service.InitHttpClient()
	requestStarted := make(chan struct{})
	upstreamCanceled := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		_ = r.Body.Close()
		close(requestStarted)
		<-r.Context().Done()
		close(upstreamCanceled)
	}))
	defer server.Close()

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	upstreamCtx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	info := &relaycommon.RelayInfo{
		UpstreamContext: upstreamCtx,
		ChannelMeta:     &relaycommon.ChannelMeta{},
	}
	req, err := http.NewRequest(http.MethodPost, server.URL, strings.NewReader("{}"))
	require.NoError(t, err)

	_, err = DoRequest(c, req, info)
	require.Error(t, err)

	select {
	case <-requestStarted:
	default:
		t.Fatal("upstream request never started")
	}
	select {
	case <-upstreamCanceled:
	case <-time.After(2 * time.Second):
		t.Fatal("upstream server did not observe request cancellation")
	}
	require.ErrorIs(t, upstreamCtx.Err(), context.DeadlineExceeded)
}

func TestDoRequestCancelsUpstreamWhileReadingNonStreamBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service.InitHttpClient()
	upstreamCanceled := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `{"partial":`)
		w.(http.Flusher).Flush()
		<-r.Context().Done()
		close(upstreamCanceled)
	}))
	defer server.Close()

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	upstreamCtx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	info := &relaycommon.RelayInfo{UpstreamContext: upstreamCtx, ChannelMeta: &relaycommon.ChannelMeta{}}
	req, err := http.NewRequest(http.MethodPost, server.URL, strings.NewReader("{}"))
	require.NoError(t, err)

	resp, err := DoRequest(c, req, info)
	require.NoError(t, err, "headers should make client.Do return before the deadline")
	defer resp.Body.Close()
	_, err = io.ReadAll(resp.Body)
	require.Error(t, err, "the body read must be interrupted at the deadline")
	require.ErrorIs(t, upstreamCtx.Err(), context.DeadlineExceeded)
	select {
	case <-upstreamCanceled:
	case <-time.After(2 * time.Second):
		t.Fatal("upstream server did not observe cancellation during body read")
	}
}

func TestNonStreamBodyTimeoutEndsHandlerAndReturnsErrorBeforeDownstreamCommit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service.InitHttpClient()
	upstreamCanceled := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `{"partial":`)
		w.(http.Flusher).Flush()
		<-r.Context().Done()
		close(upstreamCanceled)
	}))
	defer upstream.Close()

	handlerDone := make(chan struct{})
	router := gin.New()
	router.POST("/relay", func(c *gin.Context) {
		defer close(handlerDone)
		upstreamCtx, cancel := context.WithTimeout(c.Request.Context(), 100*time.Millisecond)
		defer cancel()
		info := &relaycommon.RelayInfo{UpstreamContext: upstreamCtx, ChannelMeta: &relaycommon.ChannelMeta{}}
		req, err := http.NewRequest(http.MethodPost, upstream.URL, strings.NewReader("{}"))
		if err == nil {
			var resp *http.Response
			resp, err = DoRequest(c, req, info)
			if err == nil {
				defer resp.Body.Close()
				_, err = io.ReadAll(resp.Body)
			}
		}
		if err != nil && !c.Writer.Written() {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "upstream_timeout"})
		}
	})
	relayServer := httptest.NewServer(router)
	defer relayServer.Close()

	resp, err := http.Post(relayServer.URL+"/relay", "application/json", strings.NewReader("{}"))
	require.NoError(t, err)
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusInternalServerError, resp.StatusCode)
	require.Contains(t, string(body), "upstream_timeout")
	select {
	case <-handlerDone:
	case <-time.After(2 * time.Second):
		t.Fatal("non-stream relay handler did not end after body timeout")
	}
	select {
	case <-upstreamCanceled:
	case <-time.After(2 * time.Second):
		t.Fatal("upstream server did not observe cancellation before headers")
	}
}

func TestDoWssRequestClosesEstablishedConnectionWhenUpstreamContextEnds(t *testing.T) {
	gin.SetMode(gin.TestMode)
	upgrader := websocket.Upgrader{}
	serverClosed := make(chan error, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			serverClosed <- err
			return
		}
		defer conn.Close()
		_, _, err = conn.ReadMessage()
		serverClosed <- err
	}))
	defer server.Close()

	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodGet, "/v1/realtime", nil)
	upstreamCtx, cancel := context.WithCancel(context.Background())
	info := &relaycommon.RelayInfo{
		UpstreamContext: upstreamCtx,
		ChannelMeta:     &relaycommon.ChannelMeta{},
	}
	adaptor := &timeoutWebSocketAdaptor{url: "ws" + strings.TrimPrefix(server.URL, "http")}

	conn, err := DoWssRequest(adaptor, ctx, info, nil)
	require.NoError(t, err)
	require.NotNil(t, conn)
	cancel()

	select {
	case err := <-serverClosed:
		require.Error(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("upstream websocket remained open after its context was canceled")
	}
}
