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
