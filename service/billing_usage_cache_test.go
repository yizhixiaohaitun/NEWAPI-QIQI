package service

import (
	"github.com/QuantumNous/new-api/dto"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestResponsesBillingUsagePreservesInputDetails(t *testing.T) {
	source := &dto.Usage{InputTokens: 5000, OutputTokens: 10, InputTokensDetails: &dto.InputTokenDetails{CachedTokens: 4224}}
	result := effectiveBillingUsage(&dto.Usage{BillingUsage: dto.NewOpenAIResponsesBillingUsage(source)})
	require.Equal(t, 4224, result.PromptTokensDetails.CachedTokens)
	require.Equal(t, 5000, result.PromptTokens)
}
