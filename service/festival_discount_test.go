package service

import (
	"context"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func festivalContext() *gin.Context {
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	return ctx
}

func TestFestivalTextSettlementUsesSnapshotAndSpecialGroup(t *testing.T) {
	ctx := festivalContext()
	info := &relaycommon.RelayInfo{OriginModelName: "festival-text", StartTime: time.Now(), PriceData: types.PriceData{
		ModelRatio: 2, CompletionRatio: 1, CacheRatio: 1, ImageRatio: 1, GroupRatioInfo: types.GroupRatioInfo{GroupRatio: 1.5, HasSpecialRatio: true, GroupSpecialRatio: 1.5},
		FestivalDiscountEnabled: true, FestivalDiscountFactor: 0.8,
	}}
	usage := &dto.Usage{PromptTokens: 100, CompletionTokens: 100, TotalTokens: 200}
	assert.Equal(t, 480, calculateTextQuotaSummary(ctx, info, usage).Quota)
	info.PriceData.FestivalDiscountEnabled = false
	assert.Equal(t, 600, calculateTextQuotaSummary(ctx, info, usage).Quota)
}

func TestFestivalAudioSettlementUsesSnapshot(t *testing.T) {
	info := QuotaInfo{InputDetails: TokenDetails{TextTokens: 100}, UsePrice: true, ModelPrice: 0.01, GroupRatio: 1.5, DiscountFactor: 0.8}
	quota, clamp := calculateAudioQuota(info)
	assert.Nil(t, clamp)
	assert.Equal(t, 6000, quota)
	info.DiscountFactor = math.NaN()
	quota, _ = calculateAudioQuota(info)
	assert.Equal(t, 7500, quota)
}

func TestFestivalTaskPersistedSnapshotSettlementAndRefund(t *testing.T) {
	truncate(t)
	saved := map[string]string{}
	require.NoError(t, config.GlobalConfig.SaveToDB(func(key, value string) error { saved[key] = value; return nil }))
	t.Cleanup(func() { require.NoError(t, config.GlobalConfig.LoadFromDB(saved)) })
	const userID, tokenID, channelID = 6101, 6102, 6103
	seedUser(t, userID, 20000)
	seedToken(t, tokenID, userID, "sk-festival-task", 20000)
	seedChannel(t, channelID)
	task := makeTask(userID, channelID, 2400, tokenID, BillingSourceWallet, 0)
	task.PrivateData.BillingContext = &model.TaskBillingContext{BillingSnapshotCaptured: true, OriginModelName: "festival-task", ModelRatio: 2, GroupRatio: 1.5,
		OtherRatios: map[string]float64{"seconds": 2}, FestivalDiscountEnabled: true, FestivalDiscountFactor: 0.8}
	// Simulate the submit pre-consume using the exact request price snapshot.
	price := types.PriceData{ModelRatio: 2, GroupRatioInfo: types.GroupRatioInfo{GroupRatio: 1.5}, FestivalDiscountEnabled: true, FestivalDiscountFactor: 0.8}
	price.AddOtherRatio("seconds", 2)
	require.Equal(t, 2400, common.QuotaFromFloat(float64(500)*price.ModelRatio*price.GroupRatioInfo.GroupRatio*price.OtherRatioMultiplier()))
	require.NoError(t, model.DB.Create(task).Error)
	require.NoError(t, model.DecreaseUserQuota(userID, task.Quota, false))
	require.NoError(t, model.DecreaseTokenQuota(tokenID, "sk-festival-task", task.Quota))
	var persisted model.Task
	require.NoError(t, model.DB.First(&persisted, task.ID).Error)
	require.NotNil(t, persisted.PrivateData.BillingContext)
	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{"group_ratio_setting.festival_discount_enabled": "false", "group_ratio_setting.festival_discount_factor": "1"}))
	// Settlement uses the persisted 0.8 snapshot, not the activity now disabled.
	RecalculateTaskQuotaByTokens(context.Background(), &persisted, 1000)
	assert.Equal(t, 4800, persisted.Quota)
	assert.Equal(t, 15200, getUserQuota(t, userID))
	assert.Equal(t, 15200, getTokenRemainQuota(t, tokenID))
	var stored model.Task
	require.NoError(t, model.DB.First(&stored, task.ID).Error)
	assert.Equal(t, 4800, stored.Quota)
	log := getLastLog(t)
	require.NotNil(t, log)
	assert.Equal(t, 2400, log.Quota)
	assert.Contains(t, log.Other, "festival_discount_factor")
	RefundTaskQuota(context.Background(), &persisted, "upstream failed", TaskFailureUpstreamConfirmed)
	assert.Equal(t, 20000, getUserQuota(t, userID))
	assert.Equal(t, 20000, getTokenRemainQuota(t, tokenID))
	assert.Equal(t, 4800, getLastLog(t).Quota)
}

func TestFestivalAdaptorActualQuotaIsAlreadyDiscounted(t *testing.T) {
	truncate(t)
	const userID, tokenID, channelID = 6301, 6302, 6303
	seedUser(t, userID, 10000)
	seedToken(t, tokenID, userID, "sk-festival-adaptor", 10000)
	seedChannel(t, channelID)
	task := makeTask(userID, channelID, 1600, tokenID, BillingSourceWallet, 0)
	task.PrivateData.BillingContext.ModelRatio = 2
	task.PrivateData.BillingContext.FestivalDiscountEnabled = true
	task.PrivateData.BillingContext.FestivalDiscountFactor = 0.8
	require.NoError(t, model.DB.Create(task).Error)
	require.NoError(t, model.DecreaseUserQuota(userID, task.Quota, false))
	require.NoError(t, model.DecreaseTokenQuota(tokenID, "sk-festival-adaptor", task.Quota))
	// The adapter contract returns a FINAL quota (3000 * 0.8), not an undiscounted cost.
	settleTaskBillingOnComplete(context.Background(), &mockAdaptor{adjustReturn: 2400}, task, &relaycommon.TaskInfo{TotalTokens: 9999})
	assert.Equal(t, 2400, task.Quota)
	assert.Equal(t, 7600, getUserQuota(t, userID))
	assert.Equal(t, 7600, getTokenRemainQuota(t, tokenID))
	assert.Contains(t, getLastLog(t).Other, "festival_discount_factor")
}

func TestFestivalTaskZeroSnapshotNeverFallsBackToNewRatios(t *testing.T) {
	truncate(t)
	seedUser(t, 6201, 10000)
	seedToken(t, 6202, 6201, "sk-free-festival", 10000)
	task := makeTask(6201, 0, 0, 6202, BillingSourceWallet, 0)
	task.PrivateData.BillingContext = &model.TaskBillingContext{BillingSnapshotCaptured: true, OriginModelName: "gpt-4o", ModelRatio: 0, GroupRatio: 1}
	RecalculateTaskQuotaByTokens(context.Background(), task, 1000)
	assert.Zero(t, task.Quota)
	assert.Equal(t, 10000, getUserQuota(t, 6201))
	task.PrivateData.BillingContext.ModelRatio = 2
	task.PrivateData.BillingContext.GroupRatio = 0
	RecalculateTaskQuotaByTokens(context.Background(), task, 1000)
	assert.Zero(t, task.Quota)
	assert.Equal(t, 10000, getUserQuota(t, 6201))
	// Before the captured marker existed, incomplete contexts resolved live ratios.
	oldRatios := ratio_setting.GetModelRatioCopy()
	previous, err := common.Marshal(oldRatios)
	require.NoError(t, err)
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"festival-legacy":2}`))
	t.Cleanup(func() { require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(string(previous))) })
	legacy := makeTask(6201, 0, 1000, 0, BillingSourceWallet, 0)
	legacy.PrivateData.BillingContext.OriginModelName = "festival-legacy"
	legacy.PrivateData.BillingContext.ModelRatio = 0
	legacy.PrivateData.BillingContext.GroupRatio = 0
	legacy.Quota = 1000
	RecalculateTaskQuotaByTokens(context.Background(), legacy, 1000)
	assert.Equal(t, 2000, legacy.Quota)
}
