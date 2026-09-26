package helper

import (
	"net/http"
	"net/http/httptest"
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFestivalPricingRetryKeepsFirstSnapshot(t *testing.T) {
	saved := map[string]string{}
	require.NoError(t, config.GlobalConfig.SaveToDB(func(key, value string) error { saved[key] = value; return nil }))
	t.Cleanup(func() { require.NoError(t, config.GlobalConfig.LoadFromDB(saved)) })
	previousPrices := ratio_setting.ModelPrice2JSONString()
	require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(`{"festival-retry-test":0.01}`))
	t.Cleanup(func() { require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(previousPrices)) })
	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
		"group_ratio_setting.festival_discount_enabled": "true",
		"group_ratio_setting.festival_discount_factor":  "0.8",
		"group_ratio_setting.group_group_ratio":         `{"vip":{"default":1.5}}`,
		"group_ratio_setting.group_ratio":               `{"default":2}`,
		"model_ratio_setting.model_price":               `{}`,
	}))
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", nil)
	info := &relaycommon.RelayInfo{OriginModelName: "festival-retry-test", UserGroup: "vip", UsingGroup: "default"}
	first, err := ModelPriceHelperPerCall(ctx, info)
	require.NoError(t, err)
	require.True(t, first.FestivalDiscountEnabled)
	assert.Equal(t, 1.5, first.GroupRatioInfo.GroupRatio)
	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
		"group_ratio_setting.festival_discount_enabled": "false",
		"group_ratio_setting.festival_discount_factor":  "1",
	}))
	second, err := ModelPriceHelperPerCall(ctx, info)
	require.NoError(t, err)
	assert.Equal(t, first.FestivalDiscountFactor, second.FestivalDiscountFactor)
	assert.Equal(t, first.FestivalDiscountEnabled, second.FestivalDiscountEnabled)
	assert.Equal(t, first.ApplyOtherRatiosToFloat(100), second.ApplyOtherRatiosToFloat(100))
	newInfo := &relaycommon.RelayInfo{OriginModelName: "festival-retry-test", UserGroup: "vip", UsingGroup: "default"}
	fresh, err := ModelPriceHelperPerCall(ctx, newInfo)
	require.NoError(t, err)
	assert.False(t, fresh.FestivalDiscountEnabled)
	assert.Equal(t, 100.0, fresh.ApplyOtherRatiosToFloat(100))
	// The synchronous pricing entry shares the captured snapshot when invoked again.
	_, err = ModelPriceHelper(ctx, info, 100, &types.TokenCountMeta{})
	require.NoError(t, err)
	assert.True(t, info.PriceData.FestivalDiscountEnabled)
}
