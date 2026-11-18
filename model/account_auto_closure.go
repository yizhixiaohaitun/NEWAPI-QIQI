package model

import (
	"context"
	"errors"
	"time"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

const accountReviewWindow = 72 * time.Hour

// Enroll only registrations whose balance history can be observed from the start.
func (user *User) enrollAccountReview(tx *gorm.DB) {
	user.AccountReviewTracked = !user.CreatedByAdmin
	user.AccountQualifiedAt = 0
	if user.CreatedAt == 0 {
		user.CreatedAt = tx.NowFunc().Unix()
	}
	if user.AccountReviewTracked && float64(user.Quota) > common.QuotaPerUnit {
		user.AccountQualifiedAt = user.CreatedAt
	}
}

// Update the balance and record the first threshold crossing while the user
// row is locked. This keeps a quick top-up followed by spending from losing
// the qualification marker between two independent statements.
func updateUserBalance(tx *gorm.DB, userID int, balance any, other map[string]any) *gorm.DB {
	updates := make(map[string]any, len(other)+1)
	for key, value := range other {
		updates[key] = value
	}
	updates["quota"] = balance

	result := &gorm.DB{}
	err := tx.Transaction(func(inner *gorm.DB) error {
		var user User
		if err := lockForUpdate(inner).First(&user, "id = ?", userID).Error; err != nil {
			return err
		}
		write := inner.Model(&User{}).Where("id = ?", userID).Updates(updates)
		result = write
		if write.Error != nil {
			return write.Error
		}

		now := inner.NowFunc().Unix()
		if user.AccountReviewTracked && user.AccountQualifiedAt == 0 &&
			user.CreatedAt > now-int64(accountReviewWindow/time.Second) && user.CreatedAt <= now {
			var current User
			if err := inner.Select("quota").First(&current, "id = ?", userID).Error; err != nil {
				return err
			}
			if float64(current.Quota) > common.QuotaPerUnit {
				if err := inner.Model(&User{}).Where("id = ? AND account_qualified_at = 0", userID).
					Update("account_qualified_at", now).Error; err != nil {
					return err
				}
			}
		}
		return nil
	})
	if err != nil {
		if result.Error == nil {
			result.Error = err
		}
		return result
	}
	return result
}

func isAccountReviewTracked(userID int) bool {
	var count int64
	if err := DB.Model(&User{}).Where("id = ? AND account_review_tracked = ?", userID, true).Count(&count).Error; err != nil {
		common.SysLog("failed to check account review state: " + err.Error())
		return true
	}
	return count > 0
}

// GetActiveUserStatus does not consult Redis or a session's old status. A
// soft-deleted account must be rejected even if a stale cache entry survives.
func GetActiveUserStatus(ctx context.Context, id int) (int, error) {
	var user User
	err := DB.WithContext(ctx).Select("status").Take(&user, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return common.UserStatusDisabled, nil
	}
	return user.Status, err
}

func accountClosureCandidates(tx *gorm.DB, now time.Time) *gorm.DB {
	topUps := tx.Session(&gorm.Session{NewDB: true}).Model(&TopUp{}).Select("1").
		Where("top_ups.user_id = users.id AND top_ups.status = ?", common.TopUpStatusPending)
	orders := tx.Session(&gorm.Session{NewDB: true}).Model(&SubscriptionOrder{}).Select("1").
		Where("subscription_orders.user_id = users.id AND subscription_orders.status = ?", common.TopUpStatusPending)
	return tx.Model(&User{}).
		Where("account_review_tracked = ? AND created_by_admin = ? AND role = ?", true, false, common.RoleCommonUser).
		Where("created_at > 0 AND created_at <= ? AND account_qualified_at = 0", now.Add(-accountReviewWindow).Unix()).
		Where("NOT EXISTS (?) AND NOT EXISTS (?)", topUps, orders)
}

func PreviewAccountClosures(ctx context.Context, now time.Time) (int64, error) {
	var count int64
	err := accountClosureCandidates(DB.WithContext(ctx), now).Count(&count).Error
	return count, err
}

func CloseExpiredAccounts(ctx context.Context, now time.Time, limit int) ([]int, error) {
	if limit <= 0 {
		return nil, nil
	}
	var ids, closed []int
	if err := accountClosureCandidates(DB.WithContext(ctx), now).Order("id").Limit(limit).Pluck("id", &ids).Error; err != nil {
		return nil, err
	}
	for _, id := range ids {
		var changed bool
		err := DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			var user User
			if err := lockForUpdate(tx).Select("id").Take(&user, id).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return nil
				}
				return err
			}
			result := accountClosureCandidates(tx, now).Where("id = ?", id).Delete(&User{})
			changed = result.RowsAffected == 1
			return result.Error
		})
		if err != nil {
			return closed, err
		}
		if changed {
			closed = append(closed, id)
			if err := invalidateUserCache(id); err != nil {
				common.SysError("automatic account closure: could not clear user cache: " + err.Error())
			}
		}
	}
	return closed, nil
}

func creditUserBalance(tx *gorm.DB, userID int, balance any, other map[string]any) error {
	result := updateUserBalance(tx, userID, balance, other)
	if result.Error == nil && result.RowsAffected == 0 {
		var user User
		return tx.Select("id").Take(&user, userID).Error
	}
	return result.Error
}

func SetUserQuota(id int, quota int) error {
	if err := creditUserBalance(DB, id, quota, nil); err != nil {
		return err
	}
	if err := invalidateUserCache(id); err != nil {
		common.SysError("failed to clear user cache after setting balance: " + err.Error())
	}
	return nil
}
