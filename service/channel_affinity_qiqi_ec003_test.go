package service

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func setupQiqiEC003Rule(t *testing.T) operation_setting.ChannelAffinityRule {
	t.Helper()
	rule := operation_setting.ChannelAffinityRule{
		Name:       "qiqi-ec-003-test",
		ModelRegex: []string{"^gpt-5$"},
		PathRegex:  []string{"/v1/responses"},
		KeySources: []operation_setting.ChannelAffinityKeySource{
			{Type: "gjson", Path: "prompt_cache_key"},
		},
		IncludeRuleName:   true,
		IncludeUsingGroup: true,
	}
	affinitySetting := operation_setting.GetChannelAffinitySetting()
	originalRules := affinitySetting.Rules
	affinitySetting.Rules = []operation_setting.ChannelAffinityRule{rule}
	qiqiSetting := operation_setting.GetQiqiSetting()
	originalQiqi := *qiqiSetting
	qiqiSetting.AzureResponsesResourceAffinityEnabled = true
	t.Cleanup(func() {
		affinitySetting.Rules = originalRules
		*qiqiSetting = originalQiqi
	})
	return rule
}

func responsesAffinityContext(body string) *gin.Context {
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	return ctx
}

func TestQiqiEC003ResponsesStateWinsPromptCacheKeyConflict(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rule := setupQiqiEC003Rule(t)
	stateID := fmt.Sprintf("resp_state_%d", time.Now().UnixNano())
	promptKey := fmt.Sprintf("prompt_cache_%d", time.Now().UnixNano())
	stateCacheKey := buildChannelAffinityCacheKeySuffix(rule, "gpt-5", "default", stateID)
	promptCacheKey := buildChannelAffinityCacheKeySuffix(rule, "gpt-5", "default", promptKey)
	cache := getChannelAffinityCache()
	require.NoError(t, cache.SetWithTTL(stateCacheKey, 3003, time.Minute))
	require.NoError(t, cache.SetWithTTL(promptCacheKey, 3004, time.Minute))
	t.Cleanup(func() { _, _ = cache.DeleteMany([]string{stateCacheKey, promptCacheKey}) })

	ctx := responsesAffinityContext(fmt.Sprintf(`{"previous_response_id":%q,"prompt_cache_key":%q}`, stateID, promptKey))
	channelID, found := GetPreferredChannelByAffinity(ctx, "gpt-5", "default")
	require.True(t, found)
	require.Equal(t, 3003, channelID)
	meta, ok := getChannelAffinityMeta(ctx)
	require.True(t, ok)
	require.Equal(t, channelAffinityKeySourceResponsesState, meta.KeySourceType)
}

func TestQiqiEC003ItemReferenceWinsPromptCacheKeyConflict(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rule := setupQiqiEC003Rule(t)
	stateID := fmt.Sprintf("rs_state_%d", time.Now().UnixNano())
	promptKey := fmt.Sprintf("prompt_cache_%d", time.Now().UnixNano())
	stateCacheKey := buildChannelAffinityCacheKeySuffix(rule, "gpt-5", "default", stateID)
	promptCacheKey := buildChannelAffinityCacheKeySuffix(rule, "gpt-5", "default", promptKey)
	cache := getChannelAffinityCache()
	require.NoError(t, cache.SetWithTTL(stateCacheKey, 3007, time.Minute))
	require.NoError(t, cache.SetWithTTL(promptCacheKey, 3008, time.Minute))
	t.Cleanup(func() { _, _ = cache.DeleteMany([]string{stateCacheKey, promptCacheKey}) })

	ctx := responsesAffinityContext(fmt.Sprintf(`{"prompt_cache_key":%q,"input":[{"type":"item_reference","id":%q}]}`, promptKey, stateID))
	channelID, found := GetPreferredChannelByAffinity(ctx, "gpt-5", "default")
	require.True(t, found)
	require.Equal(t, 3007, channelID)
	meta, ok := getChannelAffinityMeta(ctx)
	require.True(t, ok)
	require.Equal(t, channelAffinityKeySourceResponsesState, meta.KeySourceType)
	require.Equal(t, "input.item_reference.id", meta.KeySourcePath)
}

func TestQiqiEC003StateMissFallsBackToPromptAndRecordsPreferredState(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rule := setupQiqiEC003Rule(t)
	stateID := fmt.Sprintf("resp_state_miss_%d", time.Now().UnixNano())
	promptKey := fmt.Sprintf("prompt_cache_hit_%d", time.Now().UnixNano())
	stateCacheKey := buildChannelAffinityCacheKeySuffix(rule, "gpt-5", "default", stateID)
	promptCacheKey := buildChannelAffinityCacheKeySuffix(rule, "gpt-5", "default", promptKey)
	cache := getChannelAffinityCache()
	require.NoError(t, cache.SetWithTTL(promptCacheKey, 3011, time.Minute))
	t.Cleanup(func() { _, _ = cache.DeleteMany([]string{stateCacheKey, promptCacheKey}) })

	ctx := responsesAffinityContext(fmt.Sprintf(`{"previous_response_id":%q,"prompt_cache_key":%q}`, stateID, promptKey))
	channelID, found := GetPreferredChannelByAffinity(ctx, "gpt-5", "default")
	require.True(t, found)
	require.Equal(t, 3011, channelID)

	meta, ok := getChannelAffinityMeta(ctx)
	require.True(t, ok)
	require.Equal(t, channelAffinityKeySourceResponsesState, meta.KeySourceType)
	require.Equal(t, cache.FullKey(stateCacheKey), meta.CacheKey)

	RecordChannelAffinity(ctx, channelID)
	storedID, stored, err := cache.Get(stateCacheKey)
	require.NoError(t, err)
	require.True(t, stored)
	require.Equal(t, 3011, storedID)
}

func TestQiqiEC003UnresolvedStateIsRejectedBeforeRandomRouting(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupQiqiEC003Rule(t)
	stateID := fmt.Sprintf("resp_unresolved_%d", time.Now().UnixNano())

	ctx := responsesAffinityContext(fmt.Sprintf(`{"previous_response_id":%q}`, stateID))
	channelID, found := GetPreferredChannelByAffinity(ctx, "gpt-5", "default")
	require.False(t, found)
	require.Zero(t, channelID)
	require.True(t, ShouldProtectResponsesStateAffinity(ctx))
	require.True(t, ShouldRejectUnresolvedResponsesStateAffinity(ctx))
}

func TestQiqiEC003DisabledPreservesLegacyPromptPriority(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rule := setupQiqiEC003Rule(t)
	operation_setting.GetQiqiSetting().AzureResponsesResourceAffinityEnabled = false
	stateID := fmt.Sprintf("resp_legacy_state_%d", time.Now().UnixNano())
	promptKey := fmt.Sprintf("prompt_legacy_%d", time.Now().UnixNano())
	stateCacheKey := buildChannelAffinityCacheKeySuffix(rule, "gpt-5", "default", stateID)
	promptCacheKey := buildChannelAffinityCacheKeySuffix(rule, "gpt-5", "default", promptKey)
	cache := getChannelAffinityCache()
	require.NoError(t, cache.SetWithTTL(stateCacheKey, 3012, time.Minute))
	require.NoError(t, cache.SetWithTTL(promptCacheKey, 3013, time.Minute))
	t.Cleanup(func() { _, _ = cache.DeleteMany([]string{stateCacheKey, promptCacheKey}) })

	ctx := responsesAffinityContext(fmt.Sprintf(`{"previous_response_id":%q,"prompt_cache_key":%q}`, stateID, promptKey))
	channelID, found := GetPreferredChannelByAffinity(ctx, "gpt-5", "default")
	require.True(t, found)
	require.Equal(t, 3013, channelID)
	meta, ok := getChannelAffinityMeta(ctx)
	require.True(t, ok)
	require.Equal(t, "gjson", meta.KeySourceType)
	require.False(t, ShouldProtectResponsesStateAffinity(ctx))
}

func TestQiqiEC003AutoGroupStateAliasCanBeQueriedByAuto(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rule := setupQiqiEC003Rule(t)
	stateID := fmt.Sprintf("resp_auto_%d", time.Now().UnixNano())
	autoKey := buildChannelAffinityCacheKeySuffix(rule, "gpt-5", "auto", stateID)
	selectedKey := buildChannelAffinityCacheKeySuffix(rule, "gpt-5", "azure-a", stateID)
	cache := getChannelAffinityCache()
	t.Cleanup(func() { _, _ = cache.DeleteMany([]string{autoKey, selectedKey}) })

	recordCtx := responsesAffinityContext(`{"input":"hi"}`)
	recordCtx.Set(string(constant.ContextKeyUsingGroup), "auto")
	recordCtx.Set(string(constant.ContextKeyAutoGroup), "azure-a")
	RecordResponsesStateChannelAffinity(recordCtx, 3005, "gpt-5", "azure-a", []string{stateID})

	lookupCtx := responsesAffinityContext(fmt.Sprintf(`{"previous_response_id":%q}`, stateID))
	channelID, found := GetPreferredChannelByAffinity(lookupCtx, "gpt-5", "auto")
	require.True(t, found)
	require.Equal(t, 3005, channelID)
	for _, key := range []string{autoKey, selectedKey} {
		storedID, stored, err := cache.Get(key)
		require.NoError(t, err)
		require.True(t, stored)
		require.Equal(t, 3005, storedID)
	}
}

func TestQiqiEC003UnavailableStateChannelRemainsProtected(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rule := setupQiqiEC003Rule(t)
	stateID := fmt.Sprintf("resp_unavailable_%d", time.Now().UnixNano())
	cacheKey := buildChannelAffinityCacheKeySuffix(rule, "gpt-5", "default", stateID)
	cache := getChannelAffinityCache()
	require.NoError(t, cache.SetWithTTL(cacheKey, 3006, time.Minute))
	t.Cleanup(func() { _, _ = cache.DeleteMany([]string{cacheKey}) })

	ctx := responsesAffinityContext(fmt.Sprintf(`{"previous_response_id":%q}`, stateID))
	channelID, found := GetPreferredChannelByAffinity(ctx, "gpt-5", "default")
	require.True(t, found)
	require.Equal(t, 3006, channelID)
	require.True(t, ShouldProtectResponsesStateAffinity(ctx))

	mismatch := types.NewErrorWithStatusCode(
		fmt.Errorf("The requested item was created under a different Azure OpenAI resource. Use the same resource that created the item to access it."),
		types.ErrorCodeInvalidRequest,
		http.StatusBadRequest,
	)
	require.True(t, IsResponsesStateResourceMismatchError(mismatch))
	require.False(t, ClearChannelAffinityOnResponsesStateMismatch(ctx, mismatch))
	storedID, stored, err := cache.Get(cacheKey)
	require.NoError(t, err)
	require.True(t, stored)
	require.Equal(t, 3006, storedID)
}
