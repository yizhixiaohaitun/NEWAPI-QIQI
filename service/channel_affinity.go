package service

import (
	"fmt"
	"hash/fnv"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/pkg/cachex"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/samber/hot"
	"github.com/tidwall/gjson"
)

const (
	ginKeyChannelAffinityCacheKey   = "channel_affinity_cache_key"
	ginKeyChannelAffinityTTLSeconds = "channel_affinity_ttl_seconds"
	ginKeyChannelAffinityMeta       = "channel_affinity_meta"
	ginKeyChannelAffinityLogInfo    = "channel_affinity_log_info"
	ginKeyChannelAffinitySkipRetry  = "channel_affinity_skip_retry_on_failure"
	ginKeyResponsesStateUnresolved  = "responses_state_affinity_unresolved"

	channelAffinityCacheNamespace           = "new-api:channel_affinity:v1"
	channelAffinityUsageCacheStatsNamespace = "new-api:channel_affinity_usage_cache_stats:v1"

	channelAffinityKeySourceResponsesState = "responses_state"
)

var (
	channelAffinityCacheOnce sync.Once
	channelAffinityCache     *cachex.HybridCache[int]

	channelAffinityUsageCacheStatsOnce  sync.Once
	channelAffinityUsageCacheStatsCache *cachex.HybridCache[ChannelAffinityUsageCacheCounters]

	channelAffinityRegexCache sync.Map // map[string]*regexp.Regexp
)

type channelAffinityMeta struct {
	CacheKey       string
	TTLSeconds     int
	RuleName       string
	SkipRetry      bool
	ParamTemplate  map[string]interface{}
	KeySourceType  string
	KeySourceKey   string
	KeySourcePath  string
	KeyHint        string
	KeyFingerprint string
	UsingGroup     string
	ModelName      string
	RequestPath    string
}

type ChannelAffinityStatsContext struct {
	RuleName       string
	UsingGroup     string
	KeyFingerprint string
	TTLSeconds     int64
}

const (
	cacheTokenRateModeCachedOverPrompt           = "cached_over_prompt"
	cacheTokenRateModeCachedOverPromptPlusCached = "cached_over_prompt_plus_cached"
	cacheTokenRateModeMixed                      = "mixed"
)

type ChannelAffinityCacheStats struct {
	Enabled       bool           `json:"enabled"`
	Total         int            `json:"total"`
	Unknown       int            `json:"unknown"`
	ByRuleName    map[string]int `json:"by_rule_name"`
	CacheCapacity int            `json:"cache_capacity"`
	CacheAlgo     string         `json:"cache_algo"`
}

func getChannelAffinityCache() *cachex.HybridCache[int] {
	channelAffinityCacheOnce.Do(func() {
		setting := operation_setting.GetChannelAffinitySetting()
		capacity := setting.MaxEntries
		if capacity <= 0 {
			capacity = 100_000
		}
		defaultTTLSeconds := setting.DefaultTTLSeconds
		if defaultTTLSeconds <= 0 {
			defaultTTLSeconds = 3600
		}

		channelAffinityCache = cachex.NewHybridCache[int](cachex.HybridCacheConfig[int]{
			Namespace: cachex.Namespace(channelAffinityCacheNamespace),
			Redis:     common.RDB,
			RedisEnabled: func() bool {
				return common.RedisEnabled && common.RDB != nil
			},
			RedisCodec: cachex.IntCodec{},
			Memory: func() *hot.HotCache[string, int] {
				return hot.NewHotCache[string, int](hot.LRU, capacity).
					WithTTL(time.Duration(defaultTTLSeconds) * time.Second).
					WithJanitor().
					Build()
			},
		})
	})
	return channelAffinityCache
}

func GetChannelAffinityCacheStats() ChannelAffinityCacheStats {
	setting := operation_setting.GetChannelAffinitySetting()
	if setting == nil {
		return ChannelAffinityCacheStats{
			Enabled:    false,
			Total:      0,
			Unknown:    0,
			ByRuleName: map[string]int{},
		}
	}

	cache := getChannelAffinityCache()
	mainCap, _ := cache.Capacity()
	mainAlgo, _ := cache.Algorithm()

	rules := setting.Rules
	ruleByName := make(map[string]operation_setting.ChannelAffinityRule, len(rules))
	for _, r := range rules {
		name := strings.TrimSpace(r.Name)
		if name == "" {
			continue
		}
		if !r.IncludeRuleName {
			continue
		}
		ruleByName[name] = r
	}

	byRuleName := make(map[string]int, len(ruleByName))
	for name := range ruleByName {
		byRuleName[name] = 0
	}

	keys, err := cache.Keys()
	if err != nil {
		common.SysError(fmt.Sprintf("channel affinity cache list keys failed: err=%v", err))
		keys = nil
	}
	total := len(keys)
	unknown := 0
	for _, k := range keys {
		prefix := channelAffinityCacheNamespace + ":"
		if !strings.HasPrefix(k, prefix) {
			unknown++
			continue
		}
		rest := strings.TrimPrefix(k, prefix)
		parts := strings.Split(rest, ":")
		if len(parts) < 2 {
			unknown++
			continue
		}
		ruleName := parts[0]
		rule, ok := ruleByName[ruleName]
		if !ok {
			unknown++
			continue
		}
		if rule.IncludeModelName {
			if len(parts) < 3 {
				unknown++
				continue
			}
		}
		if rule.IncludeUsingGroup {
			minParts := 3
			if rule.IncludeModelName {
				minParts = 4
			}
			if len(parts) < minParts {
				unknown++
				continue
			}
		}
		byRuleName[ruleName]++
	}

	return ChannelAffinityCacheStats{
		Enabled:       setting.Enabled,
		Total:         total,
		Unknown:       unknown,
		ByRuleName:    byRuleName,
		CacheCapacity: mainCap,
		CacheAlgo:     mainAlgo,
	}
}

func ClearChannelAffinityCacheAll() int {
	cache := getChannelAffinityCache()
	keys, err := cache.Keys()
	if err != nil {
		common.SysError(fmt.Sprintf("channel affinity cache list keys failed: err=%v", err))
		keys = nil
	}
	if len(keys) > 0 {
		if _, err := cache.DeleteMany(keys); err != nil {
			common.SysError(fmt.Sprintf("channel affinity cache delete many failed: err=%v", err))
		}
	}
	return len(keys)
}

func ClearChannelAffinityCacheByRuleName(ruleName string) (int, error) {
	ruleName = strings.TrimSpace(ruleName)
	if ruleName == "" {
		return 0, fmt.Errorf("rule_name 不能为空")
	}

	setting := operation_setting.GetChannelAffinitySetting()
	if setting == nil {
		return 0, fmt.Errorf("channel_affinity_setting 未初始化")
	}

	var matchedRule *operation_setting.ChannelAffinityRule
	for i := range setting.Rules {
		r := &setting.Rules[i]
		if strings.TrimSpace(r.Name) != ruleName {
			continue
		}
		matchedRule = r
		break
	}
	if matchedRule == nil {
		return 0, fmt.Errorf("未知规则名称")
	}
	if !matchedRule.IncludeRuleName {
		return 0, fmt.Errorf("该规则未启用 include_rule_name，无法按规则清空缓存")
	}

	cache := getChannelAffinityCache()
	deleted, err := cache.DeleteByPrefix(ruleName)
	if err != nil {
		return 0, err
	}
	return deleted, nil
}

func matchAnyRegexCached(patterns []string, s string) bool {
	if len(patterns) == 0 || s == "" {
		return false
	}
	for _, pattern := range patterns {
		if pattern == "" {
			continue
		}
		re, ok := channelAffinityRegexCache.Load(pattern)
		if !ok {
			compiled, err := regexp.Compile(pattern)
			if err != nil {
				continue
			}
			re = compiled
			channelAffinityRegexCache.Store(pattern, re)
		}
		if re.(*regexp.Regexp).MatchString(s) {
			return true
		}
	}
	return false
}

func matchAnyIncludeFold(patterns []string, s string) bool {
	if len(patterns) == 0 || s == "" {
		return false
	}
	sLower := strings.ToLower(s)
	for _, p := range patterns {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if strings.Contains(sLower, strings.ToLower(p)) {
			return true
		}
	}
	return false
}

func extractChannelAffinityValue(c *gin.Context, src operation_setting.ChannelAffinityKeySource) string {
	switch src.Type {
	case "context_int":
		if src.Key == "" {
			return ""
		}
		v := c.GetInt(src.Key)
		if v <= 0 {
			return ""
		}
		return strconv.Itoa(v)
	case "context_string":
		if src.Key == "" {
			return ""
		}
		return strings.TrimSpace(c.GetString(src.Key))
	case "request_header":
		if c == nil || c.Request == nil || src.Key == "" {
			return ""
		}
		return strings.TrimSpace(c.Request.Header.Get(src.Key))
	case "gjson":
		if src.Path == "" {
			return ""
		}
		storage, err := common.GetBodyStorage(c)
		if err != nil {
			return ""
		}
		body, err := storage.Bytes()
		if err != nil || len(body) == 0 {
			return ""
		}
		res := gjson.GetBytes(body, src.Path)
		if !res.Exists() {
			return ""
		}
		switch res.Type {
		case gjson.String, gjson.Number, gjson.True, gjson.False:
			return strings.TrimSpace(res.String())
		default:
			return strings.TrimSpace(res.Raw)
		}
	case channelAffinityKeySourceResponsesState:
		value, _ := extractResponsesStateAffinityValue(c)
		return value
	default:
		return ""
	}
}

func extractResponsesStateAffinityValue(c *gin.Context) (string, string) {
	if c == nil {
		return "", ""
	}
	storage, err := common.GetBodyStorage(c)
	if err != nil {
		return "", ""
	}
	body, err := storage.Bytes()
	if err != nil || len(body) == 0 {
		return "", ""
	}

	if value := gjsonScalarString(gjson.GetBytes(body, "previous_response_id")); value != "" {
		return value, "previous_response_id"
	}
	if value := extractResponsesConversationAffinityValue(body); value != "" {
		return value, "conversation"
	}
	if value := extractResponsesItemReferenceAffinityValue(body); value != "" {
		return value, "input.item_reference.id"
	}
	return "", ""
}

func gjsonScalarString(res gjson.Result) string {
	if !res.Exists() {
		return ""
	}
	switch res.Type {
	case gjson.String, gjson.Number, gjson.True, gjson.False:
		return strings.TrimSpace(res.String())
	default:
		return ""
	}
}

func extractResponsesConversationAffinityValue(body []byte) string {
	if value := gjsonScalarString(gjson.GetBytes(body, "conversation")); value != "" {
		return value
	}
	return gjsonScalarString(gjson.GetBytes(body, "conversation.id"))
}

func extractResponsesItemReferenceAffinityValue(body []byte) string {
	input := gjson.GetBytes(body, "input")
	if !input.Exists() || input.Raw == "" {
		return ""
	}
	var payload any
	if err := common.Unmarshal([]byte(input.Raw), &payload); err != nil {
		return ""
	}
	return findFirstResponsesItemReferenceID(payload)
}

func findFirstResponsesItemReferenceID(value any) string {
	switch v := value.(type) {
	case []any:
		for _, item := range v {
			if id := findFirstResponsesItemReferenceID(item); id != "" {
				return id
			}
		}
	case map[string]any:
		if strings.EqualFold(strings.TrimSpace(common.Interface2String(v["type"])), "item_reference") {
			if id := strings.TrimSpace(common.Interface2String(v["id"])); id != "" {
				return id
			}
		}
		for _, item := range v {
			if id := findFirstResponsesItemReferenceID(item); id != "" {
				return id
			}
		}
	}
	return ""
}

func shouldUseResponsesStateAffinityFallback(rule operation_setting.ChannelAffinityRule, path string) bool {
	if !strings.Contains(path, "/responses") {
		return false
	}
	for _, src := range rule.KeySources {
		if strings.EqualFold(strings.TrimSpace(src.Type), channelAffinityKeySourceResponsesState) {
			return true
		}
		if strings.EqualFold(strings.TrimSpace(src.Type), "gjson") &&
			strings.EqualFold(strings.TrimSpace(src.Path), "prompt_cache_key") {
			return true
		}
	}
	return strings.Contains(strings.ToLower(strings.TrimSpace(rule.Name)), "codex")
}

func buildChannelAffinityCacheKeySuffix(rule operation_setting.ChannelAffinityRule, modelName string, usingGroup string, affinityValue string) string {
	parts := make([]string, 0, 4)
	if rule.IncludeRuleName && rule.Name != "" {
		parts = append(parts, rule.Name)
	}
	if rule.IncludeModelName && modelName != "" {
		parts = append(parts, modelName)
	}
	if rule.IncludeUsingGroup && usingGroup != "" {
		parts = append(parts, usingGroup)
	}
	parts = append(parts, affinityValue)
	return strings.Join(parts, ":")
}

func setChannelAffinityContext(c *gin.Context, meta channelAffinityMeta) {
	c.Set(ginKeyChannelAffinityCacheKey, meta.CacheKey)
	c.Set(ginKeyChannelAffinityTTLSeconds, meta.TTLSeconds)
	c.Set(ginKeyChannelAffinityMeta, meta)
}

func getChannelAffinityContext(c *gin.Context) (string, int, bool) {
	keyAny, ok := c.Get(ginKeyChannelAffinityCacheKey)
	if !ok {
		return "", 0, false
	}
	key, ok := keyAny.(string)
	if !ok || key == "" {
		return "", 0, false
	}
	ttlAny, ok := c.Get(ginKeyChannelAffinityTTLSeconds)
	if !ok {
		return key, 0, true
	}
	ttlSeconds, _ := ttlAny.(int)
	return key, ttlSeconds, true
}

func getChannelAffinityMeta(c *gin.Context) (channelAffinityMeta, bool) {
	anyMeta, ok := c.Get(ginKeyChannelAffinityMeta)
	if !ok {
		return channelAffinityMeta{}, false
	}
	meta, ok := anyMeta.(channelAffinityMeta)
	if !ok {
		return channelAffinityMeta{}, false
	}
	return meta, true
}

func GetChannelAffinityStatsContext(c *gin.Context) (ChannelAffinityStatsContext, bool) {
	if c == nil {
		return ChannelAffinityStatsContext{}, false
	}
	meta, ok := getChannelAffinityMeta(c)
	if !ok {
		return ChannelAffinityStatsContext{}, false
	}
	ruleName := strings.TrimSpace(meta.RuleName)
	keyFp := strings.TrimSpace(meta.KeyFingerprint)
	usingGroup := strings.TrimSpace(meta.UsingGroup)
	if ruleName == "" || keyFp == "" {
		return ChannelAffinityStatsContext{}, false
	}
	ttlSeconds := int64(meta.TTLSeconds)
	if ttlSeconds <= 0 {
		return ChannelAffinityStatsContext{}, false
	}
	return ChannelAffinityStatsContext{
		RuleName:       ruleName,
		UsingGroup:     usingGroup,
		KeyFingerprint: keyFp,
		TTLSeconds:     ttlSeconds,
	}, true
}

func affinityFingerprint(s string) string {
	if s == "" {
		return ""
	}
	hex := common.Sha1([]byte(s))
	if len(hex) >= 8 {
		return hex[:8]
	}
	return hex
}

func buildChannelAffinityKeyHint(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", " ")
	if len(s) <= 12 {
		return s
	}
	return s[:4] + "..." + s[len(s)-4:]
}

func cloneStringAnyMap(src map[string]interface{}) map[string]interface{} {
	if len(src) == 0 {
		return map[string]interface{}{}
	}
	dst := make(map[string]interface{}, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

func mergeChannelOverride(base map[string]interface{}, tpl map[string]interface{}) map[string]interface{} {
	if len(base) == 0 && len(tpl) == 0 {
		return map[string]interface{}{}
	}
	if len(tpl) == 0 {
		return base
	}
	out := cloneStringAnyMap(base)
	for k, v := range tpl {
		if strings.EqualFold(strings.TrimSpace(k), "operations") {
			baseOps, hasBaseOps := extractParamOperations(out[k])
			tplOps, hasTplOps := extractParamOperations(v)
			if hasTplOps {
				if hasBaseOps {
					out[k] = append(tplOps, baseOps...)
				} else {
					out[k] = tplOps
				}
				continue
			}
		}
		if _, exists := out[k]; exists {
			continue
		}
		out[k] = v
	}
	return out
}

func extractParamOperations(value interface{}) ([]interface{}, bool) {
	switch ops := value.(type) {
	case []interface{}:
		if len(ops) == 0 {
			return []interface{}{}, true
		}
		cloned := make([]interface{}, 0, len(ops))
		cloned = append(cloned, ops...)
		return cloned, true
	case []map[string]interface{}:
		cloned := make([]interface{}, 0, len(ops))
		for _, op := range ops {
			cloned = append(cloned, op)
		}
		return cloned, true
	default:
		return nil, false
	}
}

func appendChannelAffinityTemplateAdminInfo(c *gin.Context, meta channelAffinityMeta) {
	if c == nil {
		return
	}
	if len(meta.ParamTemplate) == 0 {
		return
	}

	templateInfo := map[string]interface{}{
		"applied":             true,
		"rule_name":           meta.RuleName,
		"param_override_keys": len(meta.ParamTemplate),
	}
	if anyInfo, ok := c.Get(ginKeyChannelAffinityLogInfo); ok {
		if info, ok := anyInfo.(map[string]interface{}); ok {
			info["override_template"] = templateInfo
			c.Set(ginKeyChannelAffinityLogInfo, info)
			return
		}
	}
	c.Set(ginKeyChannelAffinityLogInfo, map[string]interface{}{
		"reason":            meta.RuleName,
		"rule_name":         meta.RuleName,
		"using_group":       meta.UsingGroup,
		"model":             meta.ModelName,
		"request_path":      meta.RequestPath,
		"key_source":        meta.KeySourceType,
		"key_key":           meta.KeySourceKey,
		"key_path":          meta.KeySourcePath,
		"key_hint":          meta.KeyHint,
		"key_fp":            meta.KeyFingerprint,
		"override_template": templateInfo,
	})
}

// ApplyChannelAffinityOverrideTemplate merges per-rule channel override templates onto the selected channel override config.
func ApplyChannelAffinityOverrideTemplate(c *gin.Context, paramOverride map[string]interface{}) (map[string]interface{}, bool) {
	if c == nil {
		return paramOverride, false
	}
	meta, ok := getChannelAffinityMeta(c)
	if !ok {
		return paramOverride, false
	}
	if len(meta.ParamTemplate) == 0 {
		return paramOverride, false
	}

	mergedParam := mergeChannelOverride(paramOverride, meta.ParamTemplate)
	appendChannelAffinityTemplateAdminInfo(c, meta)
	return mergedParam, true
}

func GetPreferredChannelByAffinity(c *gin.Context, modelName string, usingGroup string) (int, bool) {
	setting := operation_setting.GetChannelAffinitySetting()
	if setting == nil || !setting.Enabled {
		return 0, false
	}
	if c != nil {
		c.Set(ginKeyResponsesStateUnresolved, false)
	}
	path := ""
	if c != nil && c.Request != nil && c.Request.URL != nil {
		path = c.Request.URL.Path
	}
	userAgent := ""
	if c != nil && c.Request != nil {
		userAgent = c.Request.UserAgent()
	}

	for _, rule := range setting.Rules {
		if !matchAnyRegexCached(rule.ModelRegex, modelName) {
			continue
		}
		if len(rule.PathRegex) > 0 && !matchAnyRegexCached(rule.PathRegex, path) {
			continue
		}
		if len(rule.UserAgentInclude) > 0 && !matchAnyIncludeFold(rule.UserAgentInclude, userAgent) {
			continue
		}
		type affinityCandidate struct {
			value  string
			source operation_setting.ChannelAffinityKeySource
		}
		candidates := make([]affinityCandidate, 0, len(rule.KeySources)+1)
		appendCandidate := func(value string, source operation_setting.ChannelAffinityKeySource) {
			if value == "" {
				return
			}
			for _, candidate := range candidates {
				if candidate.value == value && strings.EqualFold(candidate.source.Type, source.Type) {
					return
				}
			}
			candidates = append(candidates, affinityCandidate{value: value, source: source})
		}

		useResponsesState := shouldUseResponsesStateAffinityFallback(rule, path)
		stateValue, statePath := "", ""
		if useResponsesState {
			stateValue, statePath = extractResponsesStateAffinityValue(c)
		}
		protectResponsesState := operation_setting.IsAzureResponsesResourceAffinityEnabled() && stateValue != ""
		if protectResponsesState {
			// Persistent Responses state is bound to the Azure resource that created it.
			// Always try that state before weaker hints such as prompt_cache_key.
			appendCandidate(stateValue, operation_setting.ChannelAffinityKeySource{
				Type: channelAffinityKeySourceResponsesState,
				Path: statePath,
			})
			for _, src := range rule.KeySources {
				appendCandidate(extractChannelAffinityValue(c, src), src)
			}
		} else {
			// Preserve the legacy first-non-empty key semantics when the optimization
			// is disabled or the request does not carry persistent Responses state.
			for _, src := range rule.KeySources {
				value := extractChannelAffinityValue(c, src)
				if value == "" {
					continue
				}
				appendCandidate(value, src)
				break
			}
			if len(candidates) == 0 && stateValue != "" {
				appendCandidate(stateValue, operation_setting.ChannelAffinityKeySource{
					Type: channelAffinityKeySourceResponsesState,
					Path: statePath,
				})
			}
		}
		if len(candidates) == 0 {
			continue
		}

		ttlSeconds := rule.TTLSeconds
		if ttlSeconds <= 0 {
			ttlSeconds = setting.DefaultTTLSeconds
		}
		cache := getChannelAffinityCache()
		preferredContextSet := false
		for _, candidate := range candidates {
			if rule.ValueRegex != "" && !matchAnyRegexCached([]string{rule.ValueRegex}, candidate.value) {
				continue
			}
			lookupGroups := channelAffinityLookupGroups(c, usingGroup, candidate.source.Type)
			for _, lookupGroup := range lookupGroups {
				cacheKeySuffix := buildChannelAffinityCacheKeySuffix(rule, modelName, lookupGroup, candidate.value)
				cacheKeyFull := channelAffinityCacheNamespace + ":" + cacheKeySuffix
				if !preferredContextSet {
					setChannelAffinityContext(c, channelAffinityMeta{
						CacheKey:       cacheKeyFull,
						TTLSeconds:     ttlSeconds,
						RuleName:       rule.Name,
						SkipRetry:      rule.SkipRetryOnFailure,
						ParamTemplate:  cloneStringAnyMap(rule.ParamOverrideTemplate),
						KeySourceType:  strings.TrimSpace(candidate.source.Type),
						KeySourceKey:   strings.TrimSpace(candidate.source.Key),
						KeySourcePath:  strings.TrimSpace(candidate.source.Path),
						KeyHint:        buildChannelAffinityKeyHint(candidate.value),
						KeyFingerprint: affinityFingerprint(candidate.value),
						UsingGroup:     usingGroup,
						ModelName:      modelName,
						RequestPath:    path,
					})
					preferredContextSet = true
				}

				channelID, found, err := cache.Get(cacheKeySuffix)
				if err != nil {
					common.SysError(fmt.Sprintf("channel affinity cache get failed: key=%s, err=%v", cacheKeyFull, err))
					if protectResponsesState && c != nil {
						c.Set(ginKeyResponsesStateUnresolved, true)
					}
					return 0, false
				}
				if found {
					return channelID, true
				}
			}
		}
		if preferredContextSet {
			if protectResponsesState && c != nil {
				c.Set(ginKeyResponsesStateUnresolved, true)
			}
			return 0, false
		}
	}
	return 0, false
}

func channelAffinityLookupGroups(c *gin.Context, usingGroup string, sourceType string) []string {
	groups := []string{usingGroup}
	if !operation_setting.IsAzureResponsesResourceAffinityEnabled() ||
		!strings.EqualFold(strings.TrimSpace(sourceType), channelAffinityKeySourceResponsesState) ||
		!strings.EqualFold(strings.TrimSpace(usingGroup), "auto") || c == nil {
		return groups
	}
	for _, group := range GetUserAutoGroup(common.GetContextKeyString(c, constant.ContextKeyUserGroup)) {
		group = strings.TrimSpace(group)
		if group != "" && !slicesContainsFold(groups, group) {
			groups = append(groups, group)
		}
	}
	return groups
}

func slicesContainsFold(values []string, target string) bool {
	for _, value := range values {
		if strings.EqualFold(value, target) {
			return true
		}
	}
	return false
}

// ShouldProtectResponsesStateAffinity reports whether a matched persistent Responses state
// must stay on its original Azure resource instead of falling back to random routing.
func ShouldProtectResponsesStateAffinity(c *gin.Context) bool {
	if c == nil || !operation_setting.IsAzureResponsesResourceAffinityEnabled() {
		return false
	}
	meta, ok := getChannelAffinityMeta(c)
	return ok && strings.EqualFold(strings.TrimSpace(meta.KeySourceType), channelAffinityKeySourceResponsesState)
}

// ShouldRejectUnresolvedResponsesStateAffinity reports that the request carries
// persistent Responses state but no safe original-resource route could be found.
func ShouldRejectUnresolvedResponsesStateAffinity(c *gin.Context) bool {
	if !ShouldProtectResponsesStateAffinity(c) {
		return false
	}
	unresolved, ok := c.Get(ginKeyResponsesStateUnresolved)
	return ok && unresolved == true
}

func ShouldSkipRetryAfterChannelAffinityFailure(c *gin.Context) bool {
	if c == nil {
		return false
	}
	v, ok := c.Get(ginKeyChannelAffinitySkipRetry)
	if ok {
		b, ok := v.(bool)
		if ok {
			return b
		}
	}
	meta, ok := getChannelAffinityMeta(c)
	if !ok {
		return false
	}
	return meta.SkipRetry
}

func ClearCurrentChannelAffinityCache(c *gin.Context) bool {
	if c == nil {
		return false
	}
	cacheKey, _, ok := getChannelAffinityContext(c)
	if !ok || cacheKey == "" {
		return false
	}

	cache := getChannelAffinityCache()
	deleted, err := cache.DeleteMany([]string{cacheKey})
	if err != nil {
		common.SysError(fmt.Sprintf("channel affinity cache delete current failed: err=%v", err))
		return false
	}
	c.Set(ginKeyChannelAffinitySkipRetry, false)
	for _, ok := range deleted {
		if ok {
			return true
		}
	}
	return false
}

func ShouldKeepChannelAffinityOnChannelDisabled() bool {
	setting := operation_setting.GetChannelAffinitySetting()
	if setting == nil {
		return false
	}
	return setting.KeepOnChannelDisabled
}

func MarkChannelAffinityUsed(c *gin.Context, selectedGroup string, channelID int) {
	if c == nil || channelID <= 0 {
		return
	}
	meta, ok := getChannelAffinityMeta(c)
	if !ok {
		return
	}
	c.Set(ginKeyChannelAffinitySkipRetry, meta.SkipRetry)
	info := map[string]interface{}{
		"reason":         meta.RuleName,
		"rule_name":      meta.RuleName,
		"using_group":    meta.UsingGroup,
		"selected_group": selectedGroup,
		"model":          meta.ModelName,
		"request_path":   meta.RequestPath,
		"channel_id":     channelID,
		"key_source":     meta.KeySourceType,
		"key_key":        meta.KeySourceKey,
		"key_path":       meta.KeySourcePath,
		"key_hint":       meta.KeyHint,
		"key_fp":         meta.KeyFingerprint,
	}
	c.Set(ginKeyChannelAffinityLogInfo, info)
}

func AppendChannelAffinityAdminInfo(c *gin.Context, adminInfo map[string]interface{}) {
	if c == nil || adminInfo == nil {
		return
	}
	anyInfo, ok := c.Get(ginKeyChannelAffinityLogInfo)
	if !ok || anyInfo == nil {
		return
	}
	adminInfo["channel_affinity"] = anyInfo
}

func RecordChannelAffinity(c *gin.Context, channelID int) {
	if channelID <= 0 {
		return
	}
	setting := operation_setting.GetChannelAffinitySetting()
	if setting == nil || !setting.Enabled {
		return
	}
	if setting.SwitchOnSuccess && c != nil {
		if successChannelID := c.GetInt("channel_id"); successChannelID > 0 {
			channelID = successChannelID
		}
	}
	cacheKey, ttlSeconds, ok := getChannelAffinityContext(c)
	if !ok {
		return
	}
	if ttlSeconds <= 0 {
		ttlSeconds = setting.DefaultTTLSeconds
	}
	if ttlSeconds <= 0 {
		ttlSeconds = 3600
	}
	cache := getChannelAffinityCache()
	if err := cache.SetWithTTL(cacheKey, channelID, time.Duration(ttlSeconds)*time.Second); err != nil {
		common.SysError(fmt.Sprintf("channel affinity cache set failed: key=%s, err=%v", cacheKey, err))
	}
}

func RecordResponsesStateChannelAffinity(c *gin.Context, channelID int, modelName string, usingGroup string, values []string) {
	if channelID <= 0 {
		return
	}
	setting := operation_setting.GetChannelAffinitySetting()
	if setting == nil || !setting.Enabled {
		return
	}
	values = normalizeChannelAffinityValues(values)
	if len(values) == 0 {
		return
	}
	if setting.SwitchOnSuccess && c != nil {
		if successChannelID := c.GetInt("channel_id"); successChannelID > 0 {
			channelID = successChannelID
		}
	}

	path := ""
	userAgent := ""
	if c != nil && c.Request != nil {
		userAgent = c.Request.UserAgent()
		if c.Request.URL != nil {
			path = c.Request.URL.Path
		}
	}
	modelName = strings.TrimSpace(modelName)
	if modelName == "" && c != nil {
		modelName = strings.TrimSpace(c.GetString(string(constant.ContextKeyOriginalModel)))
	}
	usingGroups := channelAffinityRecordGroups(c, usingGroup)

	cache := getChannelAffinityCache()
	for _, rule := range setting.Rules {
		if !matchAnyRegexCached(rule.ModelRegex, modelName) {
			continue
		}
		if len(rule.PathRegex) > 0 && !matchAnyRegexCached(rule.PathRegex, path) {
			continue
		}
		if len(rule.UserAgentInclude) > 0 && !matchAnyIncludeFold(rule.UserAgentInclude, userAgent) {
			continue
		}
		ttlSeconds := rule.TTLSeconds
		if ttlSeconds <= 0 {
			ttlSeconds = setting.DefaultTTLSeconds
		}
		if ttlSeconds <= 0 {
			ttlSeconds = 3600
		}
		ttl := time.Duration(ttlSeconds) * time.Second
		for _, value := range values {
			if rule.ValueRegex != "" && !matchAnyRegexCached([]string{rule.ValueRegex}, value) {
				continue
			}
			for _, recordGroup := range usingGroups {
				cacheKey := buildChannelAffinityCacheKeySuffix(rule, modelName, recordGroup, value)
				if err := cache.SetWithTTL(cacheKey, channelID, ttl); err != nil {
					common.SysError(fmt.Sprintf("responses state channel affinity cache set failed: key=%s, err=%v", cache.FullKey(cacheKey), err))
				}
			}
		}
	}
}

func channelAffinityRecordGroups(c *gin.Context, usingGroup string) []string {
	usingGroup = strings.TrimSpace(usingGroup)
	if !operation_setting.IsAzureResponsesResourceAffinityEnabled() {
		return []string{normalizeChannelAffinityUsingGroup(c, usingGroup)}
	}
	groups := make([]string, 0, 2)
	if usingGroup != "" {
		groups = append(groups, usingGroup)
	}
	if c != nil {
		requestGroup := strings.TrimSpace(c.GetString(string(constant.ContextKeyUsingGroup)))
		if requestGroup != "" && !slicesContainsFold(groups, requestGroup) {
			groups = append(groups, requestGroup)
		}
		selectedGroup := strings.TrimSpace(c.GetString(string(constant.ContextKeyAutoGroup)))
		if selectedGroup != "" && !slicesContainsFold(groups, selectedGroup) {
			groups = append(groups, selectedGroup)
		}
	}
	if len(groups) == 0 {
		groups = append(groups, usingGroup)
	}
	return groups
}

func normalizeChannelAffinityUsingGroup(c *gin.Context, usingGroup string) string {
	usingGroup = strings.TrimSpace(usingGroup)
	if usingGroup != "" && !strings.EqualFold(usingGroup, "auto") {
		return usingGroup
	}
	if c == nil {
		return usingGroup
	}
	if autoGroup := strings.TrimSpace(c.GetString(string(constant.ContextKeyAutoGroup))); autoGroup != "" {
		return autoGroup
	}
	if usingGroup != "" {
		return usingGroup
	}
	return strings.TrimSpace(c.GetString(string(constant.ContextKeyUsingGroup)))
}

func normalizeChannelAffinityValues(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func CollectOpenAIResponsesAffinityAliases(resp *dto.OpenAIResponsesResponse) []string {
	if resp == nil {
		return nil
	}
	values := make([]string, 0, 1+len(resp.Output))
	values = append(values, resp.ID)
	values = append(values, collectResponsesOutputAffinityAliases(resp.Output)...)
	return normalizeChannelAffinityValues(values)
}

func CollectResponsesCompactionAffinityAliases(resp *dto.OpenAIResponsesCompactionResponse) []string {
	if resp == nil {
		return nil
	}
	values := []string{resp.ID}
	values = append(values, collectResponsesOutputRawAffinityAliases(resp.Output)...)
	return normalizeChannelAffinityValues(values)
}

func collectResponsesOutputAffinityAliases(outputs []dto.ResponsesOutput) []string {
	if len(outputs) == 0 {
		return nil
	}
	values := make([]string, 0, len(outputs))
	for _, output := range outputs {
		if output.ID != "" {
			values = append(values, output.ID)
		}
	}
	return values
}

func collectResponsesOutputRawAffinityAliases(raw []byte) []string {
	if len(raw) == 0 {
		return nil
	}
	var outputs []dto.ResponsesOutput
	if err := common.Unmarshal(raw, &outputs); err == nil {
		return collectResponsesOutputAffinityAliases(outputs)
	}
	var output dto.ResponsesOutput
	if err := common.Unmarshal(raw, &output); err == nil {
		return collectResponsesOutputAffinityAliases([]dto.ResponsesOutput{output})
	}
	return nil
}

func IsResponsesStateResourceMismatchError(err *types.NewAPIError) bool {
	if err == nil || err.StatusCode != 400 {
		return false
	}
	msg := strings.ToLower(err.Error())
	if !strings.Contains(msg, "different azure openai resource") {
		return false
	}
	if !strings.Contains(msg, "created under") || (!strings.Contains(msg, "requested item") && !strings.Contains(msg, "requested response")) {
		return false
	}
	return true
}

func ClearChannelAffinityOnResponsesStateMismatch(c *gin.Context, err *types.NewAPIError) bool {
	if !IsResponsesStateResourceMismatchError(err) || ShouldProtectResponsesStateAffinity(c) {
		return false
	}
	return ClearCurrentChannelAffinityCache(c)
}

type ChannelAffinityUsageCacheStats struct {
	RuleName            string `json:"rule_name"`
	UsingGroup          string `json:"using_group"`
	KeyFingerprint      string `json:"key_fp"`
	CachedTokenRateMode string `json:"cached_token_rate_mode"`

	Hit           int64 `json:"hit"`
	Total         int64 `json:"total"`
	WindowSeconds int64 `json:"window_seconds"`

	PromptTokens         int64 `json:"prompt_tokens"`
	CompletionTokens     int64 `json:"completion_tokens"`
	TotalTokens          int64 `json:"total_tokens"`
	CachedTokens         int64 `json:"cached_tokens"`
	PromptCacheHitTokens int64 `json:"prompt_cache_hit_tokens"`
	LastSeenAt           int64 `json:"last_seen_at"`
}

type ChannelAffinityUsageCacheCounters struct {
	CachedTokenRateMode string `json:"cached_token_rate_mode"`

	Hit           int64 `json:"hit"`
	Total         int64 `json:"total"`
	WindowSeconds int64 `json:"window_seconds"`

	PromptTokens         int64 `json:"prompt_tokens"`
	CompletionTokens     int64 `json:"completion_tokens"`
	TotalTokens          int64 `json:"total_tokens"`
	CachedTokens         int64 `json:"cached_tokens"`
	PromptCacheHitTokens int64 `json:"prompt_cache_hit_tokens"`
	LastSeenAt           int64 `json:"last_seen_at"`
}

var channelAffinityUsageCacheStatsLocks [64]sync.Mutex

// ObserveChannelAffinityUsageCacheByRelayFormat records usage cache stats with a stable rate mode derived from relay format.
func ObserveChannelAffinityUsageCacheByRelayFormat(c *gin.Context, usage *dto.Usage, relayFormat types.RelayFormat) {
	ObserveChannelAffinityUsageCacheFromContext(c, usage, cachedTokenRateModeByRelayFormat(relayFormat))
}

func ObserveChannelAffinityUsageCacheFromContext(c *gin.Context, usage *dto.Usage, cachedTokenRateMode string) {
	statsCtx, ok := GetChannelAffinityStatsContext(c)
	if !ok {
		return
	}
	observeChannelAffinityUsageCache(statsCtx, usage, cachedTokenRateMode)
}

func GetChannelAffinityUsageCacheStats(ruleName, usingGroup, keyFp string) ChannelAffinityUsageCacheStats {
	ruleName = strings.TrimSpace(ruleName)
	usingGroup = strings.TrimSpace(usingGroup)
	keyFp = strings.TrimSpace(keyFp)

	entryKey := channelAffinityUsageCacheEntryKey(ruleName, usingGroup, keyFp)
	if entryKey == "" {
		return ChannelAffinityUsageCacheStats{
			RuleName:       ruleName,
			UsingGroup:     usingGroup,
			KeyFingerprint: keyFp,
		}
	}

	cache := getChannelAffinityUsageCacheStatsCache()
	v, found, err := cache.Get(entryKey)
	if err != nil || !found {
		return ChannelAffinityUsageCacheStats{
			RuleName:       ruleName,
			UsingGroup:     usingGroup,
			KeyFingerprint: keyFp,
		}
	}
	return ChannelAffinityUsageCacheStats{
		CachedTokenRateMode:  v.CachedTokenRateMode,
		RuleName:             ruleName,
		UsingGroup:           usingGroup,
		KeyFingerprint:       keyFp,
		Hit:                  v.Hit,
		Total:                v.Total,
		WindowSeconds:        v.WindowSeconds,
		PromptTokens:         v.PromptTokens,
		CompletionTokens:     v.CompletionTokens,
		TotalTokens:          v.TotalTokens,
		CachedTokens:         v.CachedTokens,
		PromptCacheHitTokens: v.PromptCacheHitTokens,
		LastSeenAt:           v.LastSeenAt,
	}
}

func observeChannelAffinityUsageCache(statsCtx ChannelAffinityStatsContext, usage *dto.Usage, cachedTokenRateMode string) {
	entryKey := channelAffinityUsageCacheEntryKey(statsCtx.RuleName, statsCtx.UsingGroup, statsCtx.KeyFingerprint)
	if entryKey == "" {
		return
	}

	windowSeconds := statsCtx.TTLSeconds
	if windowSeconds <= 0 {
		return
	}

	cache := getChannelAffinityUsageCacheStatsCache()
	ttl := time.Duration(windowSeconds) * time.Second

	lock := channelAffinityUsageCacheStatsLock(entryKey)
	lock.Lock()
	defer lock.Unlock()

	prev, found, err := cache.Get(entryKey)
	if err != nil {
		return
	}
	next := prev
	if !found {
		next = ChannelAffinityUsageCacheCounters{}
	}
	currentMode := normalizeCachedTokenRateMode(cachedTokenRateMode)
	if currentMode != "" {
		if next.CachedTokenRateMode == "" {
			next.CachedTokenRateMode = currentMode
		} else if next.CachedTokenRateMode != currentMode && next.CachedTokenRateMode != cacheTokenRateModeMixed {
			next.CachedTokenRateMode = cacheTokenRateModeMixed
		}
	}
	next.Total++
	hit, cachedTokens, promptCacheHitTokens := usageCacheSignals(usage)
	if hit {
		next.Hit++
	}
	next.WindowSeconds = windowSeconds
	next.LastSeenAt = time.Now().Unix()
	next.CachedTokens += cachedTokens
	next.PromptCacheHitTokens += promptCacheHitTokens
	next.PromptTokens += int64(usagePromptTokens(usage))
	next.CompletionTokens += int64(usageCompletionTokens(usage))
	next.TotalTokens += int64(usageTotalTokens(usage))
	_ = cache.SetWithTTL(entryKey, next, ttl)
}

func normalizeCachedTokenRateMode(mode string) string {
	switch mode {
	case cacheTokenRateModeCachedOverPrompt:
		return cacheTokenRateModeCachedOverPrompt
	case cacheTokenRateModeCachedOverPromptPlusCached:
		return cacheTokenRateModeCachedOverPromptPlusCached
	case cacheTokenRateModeMixed:
		return cacheTokenRateModeMixed
	default:
		return ""
	}
}

func cachedTokenRateModeByRelayFormat(relayFormat types.RelayFormat) string {
	switch relayFormat {
	case types.RelayFormatOpenAI, types.RelayFormatOpenAIResponses, types.RelayFormatOpenAIResponsesCompaction:
		return cacheTokenRateModeCachedOverPrompt
	case types.RelayFormatClaude:
		return cacheTokenRateModeCachedOverPromptPlusCached
	default:
		return ""
	}
}

func channelAffinityUsageCacheEntryKey(ruleName, usingGroup, keyFp string) string {
	ruleName = strings.TrimSpace(ruleName)
	usingGroup = strings.TrimSpace(usingGroup)
	keyFp = strings.TrimSpace(keyFp)
	if ruleName == "" || keyFp == "" {
		return ""
	}
	return ruleName + "\n" + usingGroup + "\n" + keyFp
}

func usageCacheSignals(usage *dto.Usage) (hit bool, cachedTokens int64, promptCacheHitTokens int64) {
	if usage == nil {
		return false, 0, 0
	}

	cached := int64(0)
	if usage.PromptTokensDetails.CachedTokens > 0 {
		cached = int64(usage.PromptTokensDetails.CachedTokens)
	} else if usage.InputTokensDetails != nil && usage.InputTokensDetails.CachedTokens > 0 {
		cached = int64(usage.InputTokensDetails.CachedTokens)
	}
	pcht := int64(0)
	if usage.PromptCacheHitTokens > 0 {
		pcht = int64(usage.PromptCacheHitTokens)
	}
	return cached > 0 || pcht > 0, cached, pcht
}

func usagePromptTokens(usage *dto.Usage) int {
	if usage == nil {
		return 0
	}
	if usage.PromptTokens > 0 {
		return usage.PromptTokens
	}
	return usage.InputTokens
}

func usageCompletionTokens(usage *dto.Usage) int {
	if usage == nil {
		return 0
	}
	if usage.CompletionTokens > 0 {
		return usage.CompletionTokens
	}
	return usage.OutputTokens
}

func usageTotalTokens(usage *dto.Usage) int {
	if usage == nil {
		return 0
	}
	if usage.TotalTokens > 0 {
		return usage.TotalTokens
	}
	pt := usagePromptTokens(usage)
	ct := usageCompletionTokens(usage)
	if pt > 0 || ct > 0 {
		return pt + ct
	}
	return 0
}

func getChannelAffinityUsageCacheStatsCache() *cachex.HybridCache[ChannelAffinityUsageCacheCounters] {
	channelAffinityUsageCacheStatsOnce.Do(func() {
		setting := operation_setting.GetChannelAffinitySetting()
		capacity := 100_000
		defaultTTLSeconds := 3600
		if setting != nil {
			if setting.MaxEntries > 0 {
				capacity = setting.MaxEntries
			}
			if setting.DefaultTTLSeconds > 0 {
				defaultTTLSeconds = setting.DefaultTTLSeconds
			}
		}

		channelAffinityUsageCacheStatsCache = cachex.NewHybridCache[ChannelAffinityUsageCacheCounters](cachex.HybridCacheConfig[ChannelAffinityUsageCacheCounters]{
			Namespace: cachex.Namespace(channelAffinityUsageCacheStatsNamespace),
			Redis:     common.RDB,
			RedisEnabled: func() bool {
				return common.RedisEnabled && common.RDB != nil
			},
			RedisCodec: cachex.JSONCodec[ChannelAffinityUsageCacheCounters]{},
			Memory: func() *hot.HotCache[string, ChannelAffinityUsageCacheCounters] {
				return hot.NewHotCache[string, ChannelAffinityUsageCacheCounters](hot.LRU, capacity).
					WithTTL(time.Duration(defaultTTLSeconds) * time.Second).
					WithJanitor().
					Build()
			},
		})
	})
	return channelAffinityUsageCacheStatsCache
}

func channelAffinityUsageCacheStatsLock(key string) *sync.Mutex {
	h := fnv.New32a()
	_, _ = h.Write([]byte(key))
	idx := h.Sum32() % uint32(len(channelAffinityUsageCacheStatsLocks))
	return &channelAffinityUsageCacheStatsLocks[idx]
}
