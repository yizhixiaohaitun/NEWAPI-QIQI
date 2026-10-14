package dto

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/gin-gonic/gin"
)

// RequestCompatibilitySetting carries token-owned policy without process-global
// state. Missing/null remains enabled for legacy tokens and conversion callers.
func RequestCompatibilitySetting(c *gin.Context) *bool {
	if c == nil {
		return nil
	}
	value, _ := common.GetContextKeyType[*bool](c, constant.ContextKeyTokenRequestCompatibilityEnabled)
	return value
}

func RequestCompatibilityEnabled(c *gin.Context) bool {
	value := RequestCompatibilitySetting(c)
	return value == nil || *value
}
