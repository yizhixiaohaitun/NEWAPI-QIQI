package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidateTokenUpstreamTimeout(t *testing.T) {
	assert.Equal(t, 0, DefaultTokenUpstreamTimeout)
	assert.NoError(t, ValidateTokenUpstreamTimeout(DefaultTokenUpstreamTimeout))
	assert.NoError(t, ValidateTokenUpstreamTimeout(0))
	assert.Error(t, ValidateTokenUpstreamTimeout(-1))
}
