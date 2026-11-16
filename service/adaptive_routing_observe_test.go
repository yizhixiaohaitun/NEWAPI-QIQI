package service

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAdaptiveRelayOutcome(t *testing.T) {
	upstream := func(status int, code, message string) *types.NewAPIError {
		return types.WithOpenAIError(types.OpenAIError{Code: code, Message: message}, status)
	}
	tests := []struct {
		name           string
		err            *types.NewAPIError
		stream         *relaycommon.StreamStatus
		client, budget error
		first          bool
		want           AdaptiveRoutingOutcome
	}{
		{name: "success", want: AdaptiveRoutingSuccess},
		{name: "rate limit", err: upstream(429, "rate_limit", "busy"), want: AdaptiveRoutingFailure},
		{name: "server", err: upstream(503, "server_error", "busy"), want: AdaptiveRoutingFailure},
		{name: "bad request", err: upstream(400, "invalid_request_error", "invalid parameter"), want: AdaptiveRoutingIgnore},
		{name: "refusal even on 500", err: upstream(500, "content_filter", "blocked"), want: AdaptiveRoutingIgnore},
		{name: "context window", err: upstream(500, "context_length_exceeded", "too long"), want: AdaptiveRoutingIgnore},
		{name: "client cancellation", err: upstream(503, "server_error", "busy"), client: context.Canceled, want: AdaptiveRoutingIgnore},
		{name: "total budget", err: upstream(503, "server_error", "busy"), budget: context.DeadlineExceeded, want: AdaptiveRoutingIgnore},
		{name: "first byte", first: true, want: AdaptiveRoutingFailure},
		{name: "first byte budget", first: true, budget: context.DeadlineExceeded, want: AdaptiveRoutingIgnore},
		{name: "local 500", err: types.NewError(errors.New("bad local setup"), types.ErrorCodeConvertRequestFailed), want: AdaptiveRoutingIgnore},
		{name: "network", err: types.NewError(errors.New("connection reset"), types.ErrorCodeDoRequestFailed), want: AdaptiveRoutingFailure},
		{name: "stream reset after output", stream: &relaycommon.StreamStatus{EndReason: relaycommon.StreamEndReasonScannerErr, EndError: errors.New("connection reset")}, want: AdaptiveRoutingFailure},
		{name: "stream client", stream: &relaycommon.StreamStatus{EndReason: relaycommon.StreamEndReasonClientGone}, want: AdaptiveRoutingIgnore},
		{name: "stream stop refusal", stream: &relaycommon.StreamStatus{EndReason: relaycommon.StreamEndReasonHandlerStop, EndError: errors.New("content filter")}, want: AdaptiveRoutingIgnore},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, AdaptiveRelayOutcome(tt.err, tt.stream, tt.client, tt.budget, tt.first))
		})
	}
}
func TestAdaptiveOriginalStatusAndTypedStreamFailure(t *testing.T) {
	for _, tc := range []struct {
		original int
		mapping  string
		want     AdaptiveRoutingOutcome
	}{
		{503, `{"503":"400"}`, AdaptiveRoutingFailure},
		{400, `{"400":"503"}`, AdaptiveRoutingIgnore},
	} {
		err := types.WithOpenAIError(types.OpenAIError{Type: "server_error", Message: "upstream error"}, tc.original)
		ResetStatusCode(err, tc.mapping)
		assert.Equal(t, tc.want, AdaptiveRelayOutcome(err, nil, nil, nil, false))
	}
	stream := relaycommon.NewStreamStatus()
	stream.RecordUpstreamError(types.WithOpenAIError(types.OpenAIError{Type: "server_error", Message: "busy"}, 503))
	assert.Equal(t, AdaptiveRoutingFailure, AdaptiveRelayOutcome(nil, stream, nil, nil, false))
	assert.Equal(t, AdaptiveRoutingIgnore, AdaptiveRelayOutcome(nil, stream, context.Canceled, nil, false))
}

func TestAdaptiveAssetOutcome(t *testing.T) {
	assert.Equal(t, AdaptiveRoutingSuccess, AdaptiveAssetOutcome(nil, nil, nil))
	assert.Equal(t, AdaptiveRoutingIgnore, AdaptiveAssetOutcome(&dto.TaskError{Code: "build_request_failed", StatusCode: 500}, nil, nil))
	assert.Equal(t, AdaptiveRoutingIgnore, AdaptiveAssetOutcome(&dto.TaskError{Code: "fail_to_fetch_task", StatusCode: http.StatusBadRequest}, nil, nil))
	assert.Equal(t, AdaptiveRoutingFailure, AdaptiveAssetOutcome(&dto.TaskError{Code: "fail_to_fetch_task", StatusCode: http.StatusServiceUnavailable}, nil, nil))
	assert.Equal(t, AdaptiveRoutingIgnore, AdaptiveAssetOutcome(&dto.TaskError{Code: "do_request_failed"}, context.Canceled, nil))
}

// Exercise real HTTP responses, including a canceled request, without any
// external provider or persistent routing state.
func TestAdaptiveRoutingMockUpstream(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/rate":
			w.WriteHeader(429)
			_, _ = w.Write([]byte(`{"error":{"code":"rate_limit","message":"busy"}}`))
		case "/bad":
			w.WriteHeader(400)
			_, _ = w.Write([]byte(`{"error":{"code":"invalid_request","message":"bad parameter"}}`))
		case "/refuse":
			w.WriteHeader(500)
			_, _ = w.Write([]byte(`{"error":{"code":"content_filter","message":"blocked"}}`))
		default:
			w.WriteHeader(200)
		}
	}))
	defer server.Close()
	for _, tc := range []struct {
		path string
		want AdaptiveRoutingOutcome
	}{{"/rate", AdaptiveRoutingFailure}, {"/bad", AdaptiveRoutingIgnore}, {"/refuse", AdaptiveRoutingIgnore}, {"/ok", AdaptiveRoutingSuccess}} {
		t.Run(tc.path, func(t *testing.T) {
			resp, err := server.Client().Get(server.URL + tc.path)
			require.NoError(t, err)
			defer resp.Body.Close()
			if resp.StatusCode == 200 {
				assert.Equal(t, tc.want, AdaptiveRelayOutcome(nil, nil, nil, nil, false))
				return
			}
			var body struct {
				Error types.OpenAIError `json:"error"`
			}
			require.NoError(t, common.DecodeJson(resp.Body, &body))
			assert.Equal(t, tc.want, AdaptiveRelayOutcome(types.WithOpenAIError(body.Error, resp.StatusCode), nil, nil, nil, false))
		})
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/ok", nil)
	require.NoError(t, err)
	_, err = server.Client().Do(req)
	require.Error(t, err)
	assert.Equal(t, AdaptiveRoutingIgnore, AdaptiveRelayOutcome(types.NewError(err, types.ErrorCodeDoRequestFailed), nil, ctx.Err(), nil, false))
}
