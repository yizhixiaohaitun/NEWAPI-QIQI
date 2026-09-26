package ratio_setting

import (
	"testing"

	"github.com/QuantumNous/new-api/setting/config"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFestivalDiscountInvalidStoredFactorIsNotApplied(t *testing.T) {
	saved := map[string]string{}
	require.NoError(t, config.GlobalConfig.SaveToDB(func(key, value string) error { saved[key] = value; return nil }))
	t.Cleanup(func() { require.NoError(t, config.GlobalConfig.LoadFromDB(saved)) })
	for _, value := range []string{"NaN", "+Inf", "-Inf", "0", "1.2"} {
		require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
			"group_ratio_setting.festival_discount_enabled": "true",
			"group_ratio_setting.festival_discount_factor":  value,
		}))
		enabled, factor := GetFestivalDiscount()
		assert.False(t, enabled)
		assert.Equal(t, 1.0, factor)
	}
}

func TestValidateFestivalDiscountFactor(t *testing.T) {
	for _, value := range []string{"0.8", "1", "0.000001"} {
		require.NoError(t, ValidateFestivalDiscountFactor(value))
	}
	for _, value := range []string{"", "0", "-0.1", "1.01", "NaN", "+Inf", "-Inf"} {
		assert.Error(t, ValidateFestivalDiscountFactor(value), value)
	}
}
