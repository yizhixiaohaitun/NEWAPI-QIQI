package controller

import (
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

func writeRelayHTTPError(c *gin.Context, format types.RelayFormat, public *types.NewAPIError) {
	if format == types.RelayFormatClaude {
		c.JSON(public.StatusCode, gin.H{"type": "error", "error": public.ToClaudeError()})
		return
	}
	c.JSON(public.StatusCode, gin.H{"error": public.ToOpenAIError()})
}
