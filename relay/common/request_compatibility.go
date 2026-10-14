package common

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/gin-gonic/gin"
)

func (info *RelayInfo) IsRequestCompatibilityEnabled() bool {
	return info == nil || info.RequestCompatibilityEnabled == nil || *info.RequestCompatibilityEnabled
}

// BindRequestCompatibility keeps legacy converter APIs (which receive only c)
// aligned with RelayInfo. It is request-local; no model or global setting changes.
func (info *RelayInfo) BindRequestCompatibility(c *gin.Context) {
	if info != nil && c != nil && info.RequestCompatibilityEnabled != nil {
		common.SetContextKey(c, constant.ContextKeyTokenRequestCompatibilityEnabled, info.RequestCompatibilityEnabled)
	}
}
