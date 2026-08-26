package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// mode is set to audit/apply at build time. These binaries are intentionally
// argument-free so they can be executed through a restricted container console.
var mode = "audit"

const cutoffUnix int64 = 1787587200 // 2026-08-25 00:00:00 Asia/Shanghai

type candidate struct {
	RefundID       int64  `gorm:"column:refund_id"`
	ConsumeID      int64  `gorm:"column:consume_id"`
	RequestID      string `gorm:"column:request_id"`
	UserID         int    `gorm:"column:user_id"`
	TokenID        int    `gorm:"column:token_id"`
	Quota          int64  `gorm:"column:quota"`
	BillingSource  string `gorm:"column:billing_source"`
	SubscriptionID int64  `gorm:"column:subscription_id"`
	ModelName      string `gorm:"column:model_name"`
	ChannelID      int    `gorm:"column:channel_id"`
	GroupName      string `gorm:"column:group_name"`
}

const candidateSQL = `
WITH refund_logs AS (
    SELECT l.*,
           CASE WHEN COALESCE(l.other, '') ~ '^\\s*\\{' THEN l.other::jsonb ELSE '{}'::jsonb END AS other_json
      FROM logs l
     WHERE l.type = 6
       AND l.created_at < ?
       AND COALESCE(l.other, '') LIKE '%zero_reply_auto_refund%'
), consume_logs AS (
    SELECT l.*,
           CASE WHEN COALESCE(l.other, '') ~ '^\\s*\\{' THEN l.other::jsonb ELSE '{}'::jsonb END AS other_json
      FROM logs l
     WHERE l.type = 2
       AND l.created_at < ?
       AND COALESCE(l.other, '') LIKE '%zero_reply_auto_refund%'
)
SELECT DISTINCT ON (r.id)
       r.id AS refund_id,
       c.id AS consume_id,
       r.request_id,
       r.user_id,
       c.token_id,
       r.quota::bigint AS quota,
       COALESCE(NULLIF(c.other_json->>'billing_source', ''), 'wallet') AS billing_source,
       CASE WHEN COALESCE(c.other_json->>'subscription_id', '') ~ '^[0-9]+$'
            THEN (c.other_json->>'subscription_id')::bigint ELSE 0 END AS subscription_id,
       c.model_name,
       c.channel_id,
       c."group" AS group_name
  FROM refund_logs r
  JOIN consume_logs c
    ON c.request_id = r.request_id
   AND c.user_id = r.user_id
 WHERE COALESCE(r.other_json->>'zero_reply_auto_refund', 'false') = 'true'
   AND COALESCE(c.other_json->>'zero_reply_auto_refund', 'false') = 'true'
   AND COALESCE(c.other_json->>'reject_reason', '') = 'claude_stop_reason=refusal'
   AND COALESCE(c.other_json->>'zero_reply_refusal_reversal', 'false') <> 'true'
   AND COALESCE(r.other_json->>'zero_reply_refusal_reversal', 'false') <> 'true'
   AND r.quota > 0
 ORDER BY r.id, c.id DESC`

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	dsn := strings.TrimSpace(os.Getenv("SQL_DSN"))
	if dsn == "" {
		return errors.New("SQL_DSN is empty")
	}
	if logDSN := strings.TrimSpace(os.Getenv("LOG_SQL_DSN")); logDSN != "" && logDSN != dsn {
		return errors.New("separate LOG_SQL_DSN is not supported by this one-off correction")
	}
	db, err := gorm.Open(postgres.New(postgres.Config{DSN: dsn, PreferSimpleProtocol: true}), &gorm.Config{
		PrepareStmt: false,
		Logger:      gormlogger.Default.LogMode(gormlogger.Silent),
	})
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}

	if mode != "apply" {
		return audit(db)
	}
	return apply(db)
}

func loadCandidates(db *gorm.DB) ([]candidate, error) {
	var rows []candidate
	if err := db.Raw(candidateSQL, cutoffUnix, cutoffUnix).Scan(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func audit(db *gorm.DB) error {
	rows, err := loadCandidates(db)
	if err != nil {
		return fmt.Errorf("load candidates: %w", err)
	}
	printSummary("AUDIT", rows)
	for _, row := range rows {
		fmt.Printf("candidate request=%s refund_log=%d consume_log=%d user=%d token=%d quota=%d source=%s subscription=%d model=%s\n",
			row.RequestID, row.RefundID, row.ConsumeID, row.UserID, row.TokenID, row.Quota,
			row.BillingSource, row.SubscriptionID, row.ModelName)
	}
	return validateTargets(db, rows)
}

func printSummary(prefix string, rows []candidate) {
	var total, wallet, subscription int64
	users := map[int]struct{}{}
	tokens := map[int]struct{}{}
	for _, row := range rows {
		total += row.Quota
		users[row.UserID] = struct{}{}
		if row.TokenID > 0 {
			tokens[row.TokenID] = struct{}{}
		}
		if row.BillingSource == "subscription" {
			subscription += row.Quota
		} else {
			wallet += row.Quota
		}
	}
	fmt.Printf("%s cutoff=%d count=%d quota_total=%d wallet=%d subscription=%d users=%d tokens=%d\n",
		prefix, cutoffUnix, len(rows), total, wallet, subscription, len(users), len(tokens))
}

func validateTargets(db *gorm.DB, rows []candidate) error {
	walletByUser := map[int]int64{}
	tokenTotals := map[int]int64{}
	subTotals := map[int64]int64{}
	for _, row := range rows {
		if row.BillingSource == "subscription" {
			if row.SubscriptionID <= 0 {
				return fmt.Errorf("request %s is subscription-billed but has no subscription_id", row.RequestID)
			}
			subTotals[row.SubscriptionID] += row.Quota
		} else {
			walletByUser[row.UserID] += row.Quota
		}
		if row.TokenID > 0 {
			tokenTotals[row.TokenID] += row.Quota
		}
	}
	for userID := range walletByUser {
		var count int64
		if err := db.Raw("SELECT count(*) FROM users WHERE id = ?", userID).Scan(&count).Error; err != nil || count != 1 {
			return fmt.Errorf("wallet user %d not found", userID)
		}
	}
	for tokenID := range tokenTotals {
		var count int64
		if err := db.Raw("SELECT count(*) FROM tokens WHERE id = ?", tokenID).Scan(&count).Error; err != nil || count != 1 {
			return fmt.Errorf("token %d not found", tokenID)
		}
	}
	for subID, delta := range subTotals {
		var state struct{ AmountUsed, AmountTotal int64 }
		if err := db.Raw("SELECT amount_used, amount_total FROM user_subscriptions WHERE id = ?", subID).Scan(&state).Error; err != nil {
			return fmt.Errorf("read subscription %d: %w", subID, err)
		}
		if state.AmountTotal > 0 && state.AmountUsed+delta > state.AmountTotal {
			return fmt.Errorf("subscription %d would exceed total: used=%d delta=%d total=%d", subID, state.AmountUsed, delta, state.AmountTotal)
		}
	}
	fmt.Println("VALIDATION OK")
	return nil
}

func apply(db *gorm.DB) error {
	// Initialize Redis before changing SQL. A failed cache connection must not leave
	// committed balances paired with stale quota caches.
	common.InitEnv()
	if err := common.InitRedisClient(); err != nil {
		return fmt.Errorf("initialize redis: %w", err)
	}

	var applied []candidate
	err := db.Transaction(func(tx *gorm.DB) error {
		rows, err := loadCandidates(tx)
		if err != nil {
			return err
		}
		if len(rows) == 0 {
			applied = rows
			return nil
		}
		if err := validateTargets(tx, rows); err != nil {
			return err
		}

		walletByUser := map[int]int64{}
		tokenTotals := map[int]int64{}
		subTotals := map[int64]int64{}
		for _, row := range rows {
			if row.BillingSource == "subscription" {
				subTotals[row.SubscriptionID] += row.Quota
			} else {
				walletByUser[row.UserID] += row.Quota
			}
			if row.TokenID > 0 {
				tokenTotals[row.TokenID] += row.Quota
			}
		}
		for userID, quota := range walletByUser {
			res := tx.Exec("UPDATE users SET quota = quota - ? WHERE id = ?", quota, userID)
			if res.Error != nil || res.RowsAffected != 1 {
				return fmt.Errorf("update user %d: %v rows=%d", userID, res.Error, res.RowsAffected)
			}
		}
		now := time.Now().Unix()
		for subID, quota := range subTotals {
			res := tx.Exec("UPDATE user_subscriptions SET amount_used = amount_used + ?, updated_at = ? WHERE id = ? AND (amount_total <= 0 OR amount_used + ? <= amount_total)", quota, now, subID, quota)
			if res.Error != nil || res.RowsAffected != 1 {
				return fmt.Errorf("update subscription %d: %v rows=%d", subID, res.Error, res.RowsAffected)
			}
		}
		for tokenID, quota := range tokenTotals {
			res := tx.Exec("UPDATE tokens SET remain_quota = remain_quota - ?, used_quota = used_quota + ?, accessed_time = ? WHERE id = ?", quota, quota, now, tokenID)
			if res.Error != nil || res.RowsAffected != 1 {
				return fmt.Errorf("update token %d: %v rows=%d", tokenID, res.Error, res.RowsAffected)
			}
		}

		for _, row := range rows {
			marker := map[string]any{
				"zero_reply_refusal_reversal":       true,
				"zero_reply_refusal_reversed_at":    now,
				"zero_reply_refusal_reversed_quota": row.Quota,
			}
			markerJSON, _ := json.Marshal(marker)
			for _, logID := range []int64{row.ConsumeID, row.RefundID} {
				res := tx.Exec(`UPDATE logs
					SET other = (CASE WHEN COALESCE(other, '') ~ '^\\s*\\{' THEN other::jsonb ELSE '{}'::jsonb END || ?::jsonb)::text
					WHERE id = ?`, string(markerJSON), logID)
				if res.Error != nil || res.RowsAffected != 1 {
					return fmt.Errorf("mark log %d: %v rows=%d", logID, res.Error, res.RowsAffected)
				}
			}
			res := tx.Exec("UPDATE logs SET quota = ?, content = content || ? WHERE id = ?", row.Quota, "，已扣回 Claude refusal 误回退额度", row.ConsumeID)
			if res.Error != nil || res.RowsAffected != 1 {
				return fmt.Errorf("restore consume log %d: %v rows=%d", row.ConsumeID, res.Error, res.RowsAffected)
			}
			auditOther, _ := json.Marshal(map[string]any{
				"zero_reply_refusal_reversal": true,
				"refund_log_id":               row.RefundID,
				"consume_log_id":              row.ConsumeID,
				"reversed_quota":              row.Quota,
				"billing_source":              row.BillingSource,
				"subscription_id":             row.SubscriptionID,
			})
			res = tx.Exec(`INSERT INTO logs
				(user_id, created_at, type, content, username, token_name, model_name, quota,
				 prompt_tokens, completion_tokens, use_time, is_stream, channel_id, token_id,
				 "group", ip, request_id, upstream_request_id, other)
				VALUES (?, ?, 3, ?, '', '', ?, 0, 0, 0, 0, false, ?, ?, ?, '', ?, '', ?)`,
				row.UserID, now, "扣回 Claude refusal 被误判为0回复的历史回退额度", row.ModelName,
				row.ChannelID, row.TokenID, row.GroupName, row.RequestID, string(auditOther))
			if res.Error != nil || res.RowsAffected != 1 {
				return fmt.Errorf("insert audit log for %s: %v", row.RequestID, res.Error)
			}
		}
		applied = rows
		return nil
	})
	if err != nil {
		return fmt.Errorf("transaction rolled back: %w", err)
	}

	// Drop affected Redis entries so the running service reloads corrected values.
	userIDs := map[int]struct{}{}
	tokenIDs := map[int]struct{}{}
	for _, row := range applied {
		if row.BillingSource != "subscription" {
			userIDs[row.UserID] = struct{}{}
		}
		if row.TokenID > 0 {
			tokenIDs[row.TokenID] = struct{}{}
		}
	}
	for userID := range userIDs {
		if err := common.RedisDelKey(fmt.Sprintf("user:%d", userID)); err != nil {
			return fmt.Errorf("invalidate user cache %d after commit: %w", userID, err)
		}
	}
	ids := make([]int, 0, len(tokenIDs))
	for id := range tokenIDs {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	for _, tokenID := range ids {
		var tokenKey string
		if err := db.Raw("SELECT key FROM tokens WHERE id = ?", tokenID).Scan(&tokenKey).Error; err != nil {
			return fmt.Errorf("read token %d for cache invalidation: %w", tokenID, err)
		}
		if tokenKey != "" {
			cacheKey := "token:" + common.GenerateHMAC(tokenKey)
			if err := common.RedisDelKey(cacheKey); err != nil {
				return fmt.Errorf("invalidate token cache %d after commit: %w", tokenID, err)
			}
		}
	}
	printSummary("APPLIED", applied)
	return nil
}
