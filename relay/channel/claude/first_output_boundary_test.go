package claude

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClaudeStreamOutputBoundary(t *testing.T) {
	var messageStart dto.ClaudeResponse
	require.NoError(t, common.UnmarshalJsonStr(`{"type":"message_start","message":{"id":"m"}}`, &messageStart))
	assert.False(t, claudeStreamHasVisibleOutput(&messageStart))
	assert.False(t, claudeStreamFrameUnsafe(&messageStart, `{"type":"message_start","message":{"id":"m"}}`))

	var textDelta dto.ClaudeResponse
	require.NoError(t, common.UnmarshalJsonStr(`{"type":"content_block_delta","delta":{"type":"text_delta","text":"hello"}}`, &textDelta))
	assert.True(t, claudeStreamHasVisibleOutput(&textDelta))

	var unknown dto.ClaudeResponse
	require.NoError(t, common.UnmarshalJsonStr(`{"type":"provider_metadata","value":{}}`, &unknown))
	assert.True(t, claudeStreamFrameUnsafe(&unknown, `{"type":"provider_metadata","value":{}}`))
	assert.True(t, claudeStreamFrameUnsafe(&messageStart, `{"type":"message_start","safeguard_results":{"blocked":false}}`))
}
