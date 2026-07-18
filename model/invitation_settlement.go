package model

import (
	"errors"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type InvitationSettlementStatus string

const (
	InvitationSettlementStatusPending    InvitationSettlementStatus = "pending"
	InvitationSettlementStatusProcessing InvitationSettlementStatus = "processing"
	InvitationSettlementStatusSettled    InvitationSettlementStatus = "settled"
	InvitationSettlementStatusFailed     InvitationSettlementStatus = "failed"
)

// InvitationSettlement is the durable accounting record for one invited user.
// InviteeId is the idempotency key: registration callbacks and retries may run
// more than once, but the inviter and invitee can only be credited once.
type InvitationSettlement struct {
	Id            int64                      `json:"id" gorm:"primaryKey"`
	InviterId     int                        `json:"inviter_id" gorm:"not null;index"`
	InviteeId     int                        `json:"invitee_id" gorm:"not null;uniqueIndex"`
	InviterReward int                        `json:"inviter_reward" gorm:"not null;default:0"`
	InviteeReward int                        `json:"invitee_reward" gorm:"not null;default:0"`
	Status        InvitationSettlementStatus `json:"status" gorm:"type:varchar(16);not null;default:'pending';index"`
	Attempts      int                        `json:"attempts" gorm:"not null;default:0"`
	NextRetryAt   int64                      `json:"next_retry_at" gorm:"not null;default:0;index"`
	LastError     string                     `json:"last_error" gorm:"type:text"`
	CreatedAt     int64                      `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt     int64                      `json:"updated_at" gorm:"autoUpdateTime"`
	SettledAt     int64                      `json:"settled_at" gorm:"not null;default:0;index"`
}

type InvitationRetryResult struct {
	Scanned int `json:"scanned"`
	Settled int `json:"settled"`
	Failed  int `json:"failed"`
}

func invitationRewardSnapshot() (int, int) {
	if !operation_setting.IsPaymentComplianceConfirmed() {
		return 0, 0
	}
	return common.QuotaForInviter, common.QuotaForInvitee
}

func validateInvitationSettlement(inviterId int, inviteeId int, inviterReward int, inviteeReward int) error {
	if inviterId <= 0 || inviteeId <= 0 {
		return errors.New("inviter_id and invitee_id must be positive")
	}
	if inviterId == inviteeId {
		return errors.New("inviter_id and invitee_id must differ")
	}
	if inviterReward < 0 || inviteeReward < 0 {
		return errors.New("invitation rewards cannot be negative")
	}
	return nil
}

// createInvitationSettlementWithTx persists the immutable reward snapshot in
// the same transaction that creates the invitee. Existing records are never
// overwritten with a later reward configuration.
func createInvitationSettlementWithTx(tx *gorm.DB, inviterId int, inviteeId int, inviterReward int, inviteeReward int) error {
	if err := validateInvitationSettlement(inviterId, inviteeId, inviterReward, inviteeReward); err != nil {
		return err
	}

	var inviter User
	if err := tx.Select("id").First(&inviter, "id = ?", inviterId).Error; err != nil {
		return fmt.Errorf("load inviter %d: %w", inviterId, err)
	}

	settlement := InvitationSettlement{
		InviterId:     inviterId,
		InviteeId:     inviteeId,
		InviterReward: inviterReward,
		InviteeReward: inviteeReward,
		Status:        InvitationSettlementStatusPending,
	}
	result := tx.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "invitee_id"}},
		DoNothing: true,
	}).Create(&settlement)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 1 {
		return nil
	}

	var stored InvitationSettlement
	if err := tx.Where("invitee_id = ?", inviteeId).First(&stored).Error; err != nil {
		return err
	}
	if stored.InviterId != inviterId {
		return fmt.Errorf("invitee %d already belongs to inviter %d", inviteeId, stored.InviterId)
	}
	return nil
}

// EnsureInvitationSettlement is a compatibility/recovery entry point. New
// registrations create the record in their user transaction; this function is
// safe for old callers and preserves the first persisted reward snapshot.
func EnsureInvitationSettlement(inviterId int, inviteeId int, inviterReward int, inviteeReward int) (bool, error) {
	if err := DB.Transaction(func(tx *gorm.DB) error {
		return createInvitationSettlementWithTx(tx, inviterId, inviteeId, inviterReward, inviteeReward)
	}); err != nil {
		return false, err
	}
	return RetryInvitationSettlement(inviteeId)
}

func retryInvitationAfterRegistration(inviterId int, inviteeId int) {
	if _, err := RetryInvitationSettlement(inviteeId); err != nil {
		common.SysError(fmt.Sprintf("邀请结算失败 inviter_id=%d invitee_id=%d: %s", inviterId, inviteeId, err.Error()))
	}
}

// RetryInvitationSettlement retries one durable settlement. A settled row is a
// successful no-op, so duplicate callbacks and manual retries are safe.
func RetryInvitationSettlement(inviteeId int) (bool, error) {
	if inviteeId <= 0 {
		return false, errors.New("invitee_id must be positive")
	}
	return settleInvitationByInviteeId(inviteeId)
}

func settleInvitationByInviteeId(inviteeId int) (bool, error) {
	var settled InvitationSettlement
	settledNow := false

	err := DB.Transaction(func(tx *gorm.DB) error {
		if err := lockForUpdate(tx).Where("invitee_id = ?", inviteeId).First(&settled).Error; err != nil {
			return err
		}
		if settled.Status == InvitationSettlementStatusSettled {
			return nil
		}

		// The processing state is written and consumed in this transaction. It is
		// never committed on failure, but makes the winner explicit on SQLite too.
		claim := tx.Model(&InvitationSettlement{}).
			Where("id = ? AND status IN ?", settled.Id, []InvitationSettlementStatus{
				InvitationSettlementStatusPending,
				InvitationSettlementStatusFailed,
			}).
			Update("status", InvitationSettlementStatusProcessing)
		if claim.Error != nil {
			return claim.Error
		}
		if claim.RowsAffected != 1 {
			return errors.New("invitation settlement is already being processed")
		}

		inviterUpdates := map[string]interface{}{
			"aff_count": gorm.Expr("aff_count + ?", 1),
		}
		if settled.InviterReward > 0 {
			inviterUpdates["aff_quota"] = gorm.Expr("aff_quota + ?", settled.InviterReward)
			inviterUpdates["aff_history"] = gorm.Expr("aff_history + ?", settled.InviterReward)
		}
		result := tx.Model(&User{}).Where("id = ?", settled.InviterId).Updates(inviterUpdates)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return fmt.Errorf("inviter %d not found", settled.InviterId)
		}

		if settled.InviteeReward > 0 {
			result = tx.Model(&User{}).Where("id = ?", settled.InviteeId).
				Update("quota", gorm.Expr("quota + ?", settled.InviteeReward))
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected != 1 {
				return fmt.Errorf("invitee %d not found", settled.InviteeId)
			}
		} else {
			var count int64
			if err := tx.Model(&User{}).Where("id = ?", settled.InviteeId).Count(&count).Error; err != nil {
				return err
			}
			if count != 1 {
				return fmt.Errorf("invitee %d not found", settled.InviteeId)
			}
		}

		now := common.GetTimestamp()
		result = tx.Model(&InvitationSettlement{}).
			Where("id = ? AND status = ?", settled.Id, InvitationSettlementStatusProcessing).
			Updates(map[string]interface{}{
				"status":        InvitationSettlementStatusSettled,
				"attempts":      gorm.Expr("attempts + ?", 1),
				"next_retry_at": 0,
				"last_error":    "",
				"settled_at":    now,
				"updated_at":    now,
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return errors.New("invitation settlement was completed concurrently")
		}
		settledNow = true
		settled.Status = InvitationSettlementStatusSettled
		settled.SettledAt = now
		return nil
	})
	if err != nil {
		markInvitationSettlementFailed(inviteeId, err)
		return false, err
	}
	if !settledNow {
		return false, nil
	}

	if err := invalidateUserCache(settled.InviteeId); err != nil {
		common.SysLog(fmt.Sprintf("failed to invalidate invitee cache %d: %s", settled.InviteeId, err.Error()))
	}
	if err := invalidateUserCache(settled.InviterId); err != nil {
		common.SysLog(fmt.Sprintf("failed to invalidate inviter cache %d: %s", settled.InviterId, err.Error()))
	}
	if settled.InviteeReward > 0 {
		RecordLog(settled.InviteeId, LogTypeSystem, fmt.Sprintf("使用邀请码赠送 %s", logger.LogQuota(settled.InviteeReward)))
	}
	if settled.InviterReward > 0 {
		RecordLog(settled.InviterId, LogTypeSystem, fmt.Sprintf("邀请用户赠送 %s", logger.LogQuota(settled.InviterReward)))
	}
	return true, nil
}

func markInvitationSettlementFailed(inviteeId int, settleErr error) {
	message := strings.TrimSpace(settleErr.Error())
	const maxErrorLength = 2000
	if len(message) > maxErrorLength {
		message = message[:maxErrorLength]
	}
	if err := DB.Model(&InvitationSettlement{}).
		Where("invitee_id = ? AND status <> ?", inviteeId, InvitationSettlementStatusSettled).
		Updates(map[string]interface{}{
			"status":        InvitationSettlementStatusFailed,
			"attempts":      gorm.Expr("attempts + ?", 1),
			"next_retry_at": common.GetTimestamp() + 60,
			"last_error":    message,
			"updated_at":    common.GetTimestamp(),
		}).Error; err != nil {
		common.SysError(fmt.Sprintf("记录邀请结算失败状态时出错 invitee_id=%d: %s", inviteeId, err.Error()))
	}
}

// RetryInvitationSettlements retries due pending/failed rows oldest first.
// Each row is independently transactional, so one bad record cannot block the
// rest of the batch.
func RetryInvitationSettlements(limit int) InvitationRetryResult {
	if limit <= 0 {
		limit = 100
	}
	if limit > 1000 {
		limit = 1000
	}

	var inviteeIds []int
	if err := DB.Model(&InvitationSettlement{}).
		Where("status IN ? AND next_retry_at <= ?", []InvitationSettlementStatus{
			InvitationSettlementStatusPending,
			InvitationSettlementStatusFailed,
		}, common.GetTimestamp()).
		Order("id asc").
		Limit(limit).
		Pluck("invitee_id", &inviteeIds).Error; err != nil {
		common.SysError("查询待重试邀请结算失败: " + err.Error())
		return InvitationRetryResult{Failed: 1}
	}

	result := InvitationRetryResult{Scanned: len(inviteeIds)}
	for _, inviteeId := range inviteeIds {
		settled, err := RetryInvitationSettlement(inviteeId)
		if err != nil {
			result.Failed++
			continue
		}
		if settled {
			result.Settled++
		}
	}
	return result
}
