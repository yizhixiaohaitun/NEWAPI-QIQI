package controller

import (
	"net/http"

	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

func prepareFinalRelayError(raw *types.NewAPIError, requestID string) *types.NewAPIError {
	public := service.SanitizeFinalRelayError(raw)
	if public.GetErrorCode() != types.ErrorCodeUpstreamResourceInsufficient {
		return types.PreparePublicError(public, requestID)
	}
	return public
}

const statusRelayUpstreamTimeout = 498

func writeRelayHTTPError(c *gin.Context, format types.RelayFormat, public *types.NewAPIError) {
	status := public.StatusCode
	// Convert the final downstream status only; internal retries and logs keep 504.
	if status == http.StatusGatewayTimeout {
		status = statusRelayUpstreamTimeout
	}
	if format == types.RelayFormatClaude {
		c.JSON(status, gin.H{"type": "error", "error": public.ToClaudeError()})
		return
	}
	c.JSON(status, gin.H{"error": public.ToOpenAIError()})
}
