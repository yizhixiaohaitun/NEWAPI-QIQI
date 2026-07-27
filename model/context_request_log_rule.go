package model

import (
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/setting/operation_setting"
)

const (
	ContextLogDecisionCapture = "capture"
	ContextLogDecisionSkip    = "skip"
)

type ContextRequestLogRule struct {
	Id           int    `json:"id"`
	Name         string `json:"name" gorm:"type:varchar(128);not null"`
	UserId       *int   `json:"user_id" gorm:"index"`
	ModelPattern string `json:"model_pattern" gorm:"type:varchar(191);default:'';index"`
	Decision     string `json:"decision" gorm:"type:varchar(16);not null"`
	Enabled      bool   `json:"enabled" gorm:"index;not null;default:true"`
	Priority     int    `json:"priority" gorm:"not null;default:0"`
	CreatedAt    int64  `json:"created_at" gorm:"bigint;not null"`
	UpdatedAt    int64  `json:"updated_at" gorm:"bigint;not null"`
}

func (ContextRequestLogRule) TableName() string { return "qiqi_context_request_log_rules" }

type ContextLogDecision struct {
	Capture  bool   `json:"capture"`
	RuleId   int    `json:"rule_id"`
	RuleName string `json:"rule_name"`
	Source   string `json:"source"`
}

var contextLogRuleCache = struct {
	sync.RWMutex
	rules    []ContextRequestLogRule
	loadedAt time.Time
}{}

// MatchContextLogRules is deterministic across all SQL engines. Patterns are
// case-insensitive and use '*' as the only wildcard; all other characters are literal.
func MatchContextLogRules(rules []ContextRequestLogRule, userId int, modelName string, defaultCapture bool) ContextLogDecision {
	matches := make([]ContextRequestLogRule, 0)
	bestSpecificity := -1
	for _, rule := range rules {
		if !rule.Enabled || (rule.Decision != ContextLogDecisionCapture && rule.Decision != ContextLogDecisionSkip) {
			continue
		}
		userMatch := rule.UserId == nil || *rule.UserId == userId
		modelMatch := rule.ModelPattern == "" || wildcardModelMatch(rule.ModelPattern, modelName)
		if !userMatch || !modelMatch {
			continue
		}
		specificity := 0
		if rule.UserId != nil {
			specificity += 2
		}
		if rule.ModelPattern != "" {
			specificity++
		}
		if specificity > bestSpecificity {
			matches = matches[:0]
			bestSpecificity = specificity
		}
		if specificity == bestSpecificity {
			matches = append(matches, rule)
		}
	}
	if len(matches) == 0 {
		return ContextLogDecision{Capture: defaultCapture, Source: "global_default"}
	}
	sort.Slice(matches, func(i, j int) bool {
		if matches[i].Priority != matches[j].Priority {
			return matches[i].Priority > matches[j].Priority
		}
		return matches[i].Id < matches[j].Id
	})
	winner := matches[0]
	return ContextLogDecision{Capture: winner.Decision == ContextLogDecisionCapture, RuleId: winner.Id, RuleName: winner.Name, Source: "rule"}
}

func wildcardModelMatch(pattern, value string) bool {
	pattern = strings.ToLower(pattern)
	value = strings.ToLower(value)
	parts := strings.Split(pattern, "*")
	if len(parts) == 1 {
		return pattern == value
	}
	if parts[0] != "" {
		if !strings.HasPrefix(value, parts[0]) {
			return false
		}
		value = value[len(parts[0]):]
	}
	for _, part := range parts[1 : len(parts)-1] {
		if part == "" {
			continue
		}
		index := strings.Index(value, part)
		if index < 0 {
			return false
		}
		value = value[index+len(part):]
	}
	last := parts[len(parts)-1]
	return last == "" || strings.HasSuffix(value, last)
}

func cachedContextLogRules() []ContextRequestLogRule {
	contextLogRuleCache.RLock()
	stale := time.Since(contextLogRuleCache.loadedAt) > 5*time.Second
	rules := append([]ContextRequestLogRule(nil), contextLogRuleCache.rules...)
	contextLogRuleCache.RUnlock()
	if stale && DB != nil {
		var fresh []ContextRequestLogRule
		if err := DB.Where("enabled = ?", true).Find(&fresh).Error; err == nil {
			contextLogRuleCache.Lock()
			contextLogRuleCache.rules = fresh
			contextLogRuleCache.loadedAt = time.Now()
			contextLogRuleCache.Unlock()
			rules = fresh
		}
	}
	return rules
}

func ContextLogRulesNeedModel(userId int) bool {
	for _, rule := range cachedContextLogRules() {
		if rule.Enabled && rule.ModelPattern != "" && (rule.UserId == nil || *rule.UserId == userId) {
			return true
		}
	}
	return false
}

func GetContextLogDecision(userId int, modelName string) ContextLogDecision {
	return MatchContextLogRules(cachedContextLogRules(), userId, modelName, operation_setting.IsContextRequestLoggingEnabled())
}

func InvalidateContextLogRuleCache() {
	contextLogRuleCache.Lock()
	contextLogRuleCache.loadedAt = time.Time{}
	contextLogRuleCache.Unlock()
}

func ListContextLogRules() ([]ContextRequestLogRule, error) {
	var rules []ContextRequestLogRule
	err := DB.Order("priority desc, id asc").Find(&rules).Error
	return rules, err
}

func SaveContextLogRule(rule *ContextRequestLogRule) error {
	now := time.Now().Unix()
	if rule.Id == 0 {
		rule.CreatedAt = now
	}
	rule.UpdatedAt = now
	err := DB.Save(rule).Error
	if err == nil {
		InvalidateContextLogRuleCache()
	}
	return err
}

func DeleteContextLogRule(id int) (bool, error) {
	result := DB.Delete(&ContextRequestLogRule{}, id)
	if result.Error == nil {
		InvalidateContextLogRuleCache()
	}
	return result.RowsAffected > 0, result.Error
}
