package controller

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestRelayHTTPErrorStatusAndEnvelope(t *testing.T) {
	for _, tc := range []struct {
		name       string
		format     types.RelayFormat
		upstream   *types.NewAPIError
		wantStatus int
		wantBody   string
	}{
		{
			name: "OpenAI timeout", format: types.RelayFormatOpenAI,
			upstream:   types.NewOpenAIError(errors.New("upstream timed out"), types.ErrorCodeBadResponse, http.StatusGatewayTimeout),
			wantStatus: 498,
			wantBody:   `{"error":{"message":"upstream timed out","type":"bad_response","param":"","code":"bad_response"}}`,
		},
		{
			name: "Claude timeout", format: types.RelayFormatClaude,
			upstream:   types.WithClaudeError(types.ClaudeError{Type: "api_error", Message: "upstream timed out"}, http.StatusGatewayTimeout),
			wantStatus: 498,
			wantBody:   `{"type":"error","error":{"type":"api_error","message":"upstream timed out"}}`,
		},
		{
			name: "other gateway failure", format: types.RelayFormatOpenAI,
			upstream:   types.NewOpenAIError(errors.New("upstream connection failed"), types.ErrorCodeBadResponse, http.StatusBadGateway),
			wantStatus: http.StatusBadGateway,
			wantBody:   `{"error":{"message":"upstream connection failed","type":"bad_response","param":"","code":"bad_response"}}`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(out)
			internalStatus := tc.upstream.StatusCode

			writeRelayHTTPError(c, tc.format, tc.upstream)

			require.Equal(t, tc.wantStatus, out.Code)
			require.JSONEq(t, tc.wantBody, out.Body.String())
			require.Equal(t, internalStatus, tc.upstream.StatusCode)
		})
	}
}
