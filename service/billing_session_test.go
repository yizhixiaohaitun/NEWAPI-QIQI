package service

import (
	"errors"
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLocalQuotaExhaustedErrorsUse429AndSkipRetry(t *testing.T) {
	tests := []struct {
		name string
		code types.ErrorCode
	}{
		{name: "user quota", code: types.ErrorCodeInsufficientUserQuota},
		{name: "token pre-consume", code: types.ErrorCodePreConsumeTokenQuotaFailed},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := newLocalQuotaExhaustedError(errors.New("quota exhausted"), tt.code)

			require.NotNil(t, err)
			assert.Equal(t, http.StatusTooManyRequests, err.StatusCode)
			assert.Equal(t, tt.code, err.GetErrorCode())
			assert.True(t, types.IsSkipRetryError(err))
			assert.False(t, types.IsRecordErrorLog(err))
		})
	}
}
