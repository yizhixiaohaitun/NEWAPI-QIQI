package gemini

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGeminiStreamOutputBoundary(t *testing.T) {
	var usageOnly dto.GeminiChatResponse
	require.NoError(t, common.UnmarshalJsonStr(`{"usageMetadata":{"promptTokenCount":1}}`, &usageOnly))
	assert.False(t, geminiStreamHasVisibleOutput(&usageOnly))
	assert.False(t, geminiStreamFrameUnsafe(`{"usageMetadata":{"promptTokenCount":1}}`))

	var text dto.GeminiChatResponse
	require.NoError(t, common.UnmarshalJsonStr(`{"candidates":[{"content":{"parts":[{"text":"hello"}]}}]}`, &text))
	assert.True(t, geminiStreamHasVisibleOutput(&text))

	assert.True(t, geminiStreamFrameUnsafe(`{"promptFeedback":{"blockReason":"SAFETY"}}`))
	assert.True(t, geminiStreamFrameUnsafe(`{"candidates":[{"safetyRatings":[{"category":"HARM"}]}]}`))
}
