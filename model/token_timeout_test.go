package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidateTokenUpstreamTimeout(t *testing.T) {
	assert.Equal(t, 0, DefaultTokenUpstreamTimeout)
	assert.Equal(t, 0, DefaultTokenNonStreamUpstreamTimeout)
	assert.NoError(t, ValidateTokenUpstreamTimeout(DefaultTokenUpstreamTimeout))
	assert.NoError(t, ValidateTokenUpstreamTimeout(0))
	assert.Error(t, ValidateTokenUpstreamTimeout(-1))
	assert.NoError(t, ValidateTokenFirstByteTimeout(0))
	assert.NoError(t, ValidateTokenFirstByteTimeout(90))
	assert.Error(t, ValidateTokenFirstByteTimeout(-1))
}
