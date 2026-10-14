package middleware

import (
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTokenRequestCompatibilityContextUsesOptionalBool(t *testing.T) {
	for _, tc := range []struct {
		name   string
		stored *bool
		want   bool
	}{
		{"legacy-null-default-on", nil, true},
		{"explicit-false", common.GetPointer(false), false},
		{"explicit-true", common.GetPointer(true), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			token := &model.Token{Id: 7, UserId: 1, Key: "test-only-context-token", RequestCompatibilityEnabled: tc.stored}
			require.NoError(t, SetupContextForToken(c, token))
			value, exists := common.GetContextKey(c, constant.ContextKeyTokenRequestCompatibilityEnabled)
			require.True(t, exists)
			ptr, ok := value.(*bool)
			require.True(t, ok, "RelayInfo consumes an optional bool, not a bool zero value")
			require.NotNil(t, ptr)
			assert.Equal(t, tc.want, *ptr)
			assert.Equal(t, tc.stored, token.RequestCompatibilityEnabled, "normalization must not modify the stored token")
		})
	}
}
