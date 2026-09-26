package types

import (
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
)

func TestPriceDataAppliesFestivalDiscountExactlyOnceWithOtherRatios(t *testing.T) {
	price := PriceData{FestivalDiscountEnabled: true, FestivalDiscountFactor: 0.8}
	price.AddOtherRatio("duration", 1.5)
	require.InDelta(t, 120, price.ApplyOtherRatiosToFloat(100), 1e-9)
	require.True(t, price.ApplyOtherRatiosToDecimal(decimal.NewFromInt(100)).Equal(decimal.NewFromInt(120)))
	require.InDelta(t, 100, price.RemoveOtherRatiosFromFloat(120), 1e-9)
}

func TestPriceDataDisabledFestivalDiscountPreservesLegacyMultiplier(t *testing.T) {
	price := PriceData{FestivalDiscountEnabled: false, FestivalDiscountFactor: 0.8}
	price.AddOtherRatio("duration", 1.5)
	require.InDelta(t, 150, price.ApplyOtherRatiosToFloat(100), 1e-9)
}
