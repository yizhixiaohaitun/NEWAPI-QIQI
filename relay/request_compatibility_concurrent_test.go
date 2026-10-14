package relay

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestRequestCompatibilityConcurrentSKHTTP(t *testing.T) {
	service.InitHttpClient() // Initialize mutable global clients only once, before workers.
	type result struct {
		enabled, stream bool
		channel         int
		body            []byte
		err             error
	}
	results := make(chan result, 16)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for id := 1; id <= 16; id++ {
		id := id
		enabled := id%2 == 0
		stream := id%4 < 2
		channel := constant.ChannelTypeAnthropic
		if id%8 < 4 {
			channel = constant.ChannelTypeOpenAI
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			out := result{enabled: enabled, stream: stream, channel: channel}
			defer func() { results <- out }()
			sent := make(chan []byte, 1)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, _ := io.ReadAll(r.Body)
				sent <- body
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(400)
				_, _ = w.Write([]byte(`{"error":{"type":"invalid_request_error","message":"local capture only"}}`))
			}))
			defer server.Close()
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest("POST", "/v1/chat/completions", bytes.NewReader([]byte(compatibilityUserJSON)))
			if err := middleware.SetupContextForToken(c, &model.Token{Id: id, UserId: 1, Key: fmt.Sprintf("test-sk-%d", id), RequestCompatibilityEnabled: &enabled}); err != nil {
				out.err = err
				return
			}
			common.SetContextKey(c, constant.ContextKeyChannelType, channel)
			common.SetContextKey(c, constant.ContextKeyChannelBaseUrl, server.URL)
			common.SetContextKey(c, constant.ContextKeyOriginalModel, "claude-opus-4-7")
			var request dto.GeneralOpenAIRequest
			if err := json.Unmarshal([]byte(compatibilityUserJSON), &request); err != nil {
				out.err = err
				return
			}
			request.Stream = &stream
			info, err := relaycommon.GenRelayInfo(c, types.RelayFormatOpenAI, &request, nil)
			if err != nil {
				out.err = err
				return
			}
			info.RequestURLPath = "/v1/chat/completions"
			info.StartTime = time.Now()
			<-start
			if apiErr := TextHelper(c, info); apiErr == nil {
				out.err = fmt.Errorf("expected capture HTTP400")
				return
			}
			select {
			case out.body = <-sent:
			default:
				out.err = fmt.Errorf("request did not reach capture server")
			}
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	count := 0
	for r := range results {
		count++
		require.NoError(t, r.err)
		var fields map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(r.body, &fields))
		if r.enabled {
			require.NotContains(t, fields, "temperature")
			require.NotContains(t, fields, "top_p")
		} else {
			require.Equal(t, "0.96", string(fields["temperature"]))
			require.Equal(t, "0", string(fields["top_p"]))
		}
		require.NotContains(t, fields, "thinking")
		require.NotContains(t, fields, "output_config")
		require.Equal(t, "30000", string(fields["max_tokens"]))
	}
	require.Equal(t, 16, count)
}
