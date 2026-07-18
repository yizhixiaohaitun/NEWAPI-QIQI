package model

import (
	"sync"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupInvitationSettlementTest(t *testing.T) {
	t.Helper()
	require.NoError(t, DB.Exec("DELETE FROM invitation_settlements").Error)
	require.NoError(t, DB.Exec("DELETE FROM users").Error)
	require.NoError(t, DB.Exec("DELETE FROM logs").Error)

	oldRedisEnabled := common.RedisEnabled
	oldBatchUpdateEnabled := common.BatchUpdateEnabled
	common.RedisEnabled = false
	common.BatchUpdateEnabled = false
	t.Cleanup(func() {
		common.RedisEnabled = oldRedisEnabled
		common.BatchUpdateEnabled = oldBatchUpdateEnabled
		DB.Exec("DELETE FROM invitation_settlements")
		DB.Exec("DELETE FROM users")
		DB.Exec("DELETE FROM logs")
	})
}

func createInvitationSettlementUsers(t *testing.T) (*User, *User) {
	t.Helper()
	inviter := &User{
		Username:        "settlement-inviter",
		Password:        "password",
		Status:          common.UserStatusEnabled,
		AffCode:         "settlement-inviter-code",
		AffCount:        3,
		AffQuota:        100,
		AffHistoryQuota: 400,
	}
	invitee := &User{
		Username:  "settlement-invitee",
		Password:  "password",
		Status:    common.UserStatusEnabled,
		AffCode:   "settlement-invitee-code",
		Quota:     700,
		InviterId: 0,
	}
	require.NoError(t, DB.Create(inviter).Error)
	invitee.InviterId = inviter.Id
	require.NoError(t, DB.Create(invitee).Error)
	return inviter, invitee
}

func createPendingInvitationSettlement(t *testing.T, inviterId int, inviteeId int, inviterReward int, inviteeReward int) InvitationSettlement {
	t.Helper()
	settlement := InvitationSettlement{
		InviterId:     inviterId,
		InviteeId:     inviteeId,
		InviterReward: inviterReward,
		InviteeReward: inviteeReward,
		Status:        InvitationSettlementStatusPending,
	}
	require.NoError(t, DB.Create(&settlement).Error)
	return settlement
}

func TestInvitationSettlementInviteeIDUniqueAndSnapshotImmutable(t *testing.T) {
	setupInvitationSettlementTest(t)
	inviter, invitee := createInvitationSettlementUsers(t)
	otherInviter := &User{Username: "other-inviter", Password: "password", Status: common.UserStatusEnabled, AffCode: "other-inviter-code"}
	require.NoError(t, DB.Create(otherInviter).Error)

	createPendingInvitationSettlement(t, inviter.Id, invitee.Id, 50, 20)
	duplicate := InvitationSettlement{
		InviterId:     otherInviter.Id,
		InviteeId:     invitee.Id,
		InviterReward: 999,
		InviteeReward: 888,
		Status:        InvitationSettlementStatusPending,
	}
	require.Error(t, DB.Create(&duplicate).Error)

	var rows []InvitationSettlement
	require.NoError(t, DB.Where("invitee_id = ?", invitee.Id).Find(&rows).Error)
	require.Len(t, rows, 1)
	assert.Equal(t, inviter.Id, rows[0].InviterId)
	assert.Equal(t, 50, rows[0].InviterReward)
	assert.Equal(t, 20, rows[0].InviteeReward)
}

func TestInvitationSettlementCreditsBothSidesExactlyOnce(t *testing.T) {
	setupInvitationSettlementTest(t)
	inviter, invitee := createInvitationSettlementUsers(t)
	createPendingInvitationSettlement(t, inviter.Id, invitee.Id, 50, 20)

	settledNow, err := RetryInvitationSettlement(invitee.Id)
	require.NoError(t, err)
	assert.True(t, settledNow)

	settledNow, err = RetryInvitationSettlement(invitee.Id)
	require.NoError(t, err)
	assert.False(t, settledNow)

	var storedInviter User
	require.NoError(t, DB.First(&storedInviter, inviter.Id).Error)
	assert.Equal(t, 4, storedInviter.AffCount)
	assert.Equal(t, 150, storedInviter.AffQuota)
	assert.Equal(t, 450, storedInviter.AffHistoryQuota)

	var storedInvitee User
	require.NoError(t, DB.First(&storedInvitee, invitee.Id).Error)
	assert.Equal(t, 720, storedInvitee.Quota)

	var settlement InvitationSettlement
	require.NoError(t, DB.Where("invitee_id = ?", invitee.Id).First(&settlement).Error)
	assert.Equal(t, InvitationSettlementStatusSettled, settlement.Status)
	assert.Equal(t, 1, settlement.Attempts)
	assert.Positive(t, settlement.SettledAt)
	assert.Zero(t, settlement.NextRetryAt)
	assert.Empty(t, settlement.LastError)
}

func TestInvitationSettlementConcurrentRetriesHaveSingleEffect(t *testing.T) {
	setupInvitationSettlementTest(t)
	inviter, invitee := createInvitationSettlementUsers(t)
	createPendingInvitationSettlement(t, inviter.Id, invitee.Id, 50, 20)

	const workers = 8
	start := make(chan struct{})
	errs := make([]error, workers)
	var wg sync.WaitGroup
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func(index int) {
			defer wg.Done()
			<-start
			_, errs[index] = RetryInvitationSettlement(invitee.Id)
		}(i)
	}
	close(start)
	wg.Wait()
	for _, err := range errs {
		require.NoError(t, err)
	}

	var storedInviter User
	require.NoError(t, DB.First(&storedInviter, inviter.Id).Error)
	assert.Equal(t, 4, storedInviter.AffCount)
	assert.Equal(t, 150, storedInviter.AffQuota)
	assert.Equal(t, 450, storedInviter.AffHistoryQuota)

	var storedInvitee User
	require.NoError(t, DB.First(&storedInvitee, invitee.Id).Error)
	assert.Equal(t, 720, storedInvitee.Quota)

	var settlement InvitationSettlement
	require.NoError(t, DB.Where("invitee_id = ?", invitee.Id).First(&settlement).Error)
	assert.Equal(t, InvitationSettlementStatusSettled, settlement.Status)
	assert.Equal(t, 1, settlement.Attempts)
}

func TestInvitationSettlementFailureRollsBackAndCanRetry(t *testing.T) {
	setupInvitationSettlementTest(t)
	inviter, invitee := createInvitationSettlementUsers(t)
	createPendingInvitationSettlement(t, inviter.Id, invitee.Id, 50, 20)
	require.NoError(t, DB.Unscoped().Delete(&User{}, invitee.Id).Error)

	settledNow, err := RetryInvitationSettlement(invitee.Id)
	require.Error(t, err)
	assert.False(t, settledNow)

	var storedInviter User
	require.NoError(t, DB.First(&storedInviter, inviter.Id).Error)
	assert.Equal(t, 3, storedInviter.AffCount)
	assert.Equal(t, 100, storedInviter.AffQuota)
	assert.Equal(t, 400, storedInviter.AffHistoryQuota)

	var failed InvitationSettlement
	require.NoError(t, DB.Where("invitee_id = ?", invitee.Id).First(&failed).Error)
	assert.Equal(t, InvitationSettlementStatusFailed, failed.Status)
	assert.Equal(t, 1, failed.Attempts)
	assert.NotEmpty(t, failed.LastError)
	assert.Positive(t, failed.NextRetryAt)

	restoredInvitee := &User{
		Id:        invitee.Id,
		Username:  invitee.Username,
		Password:  "password",
		Status:    common.UserStatusEnabled,
		AffCode:   invitee.AffCode,
		Quota:     700,
		InviterId: inviter.Id,
	}
	require.NoError(t, DB.Create(restoredInvitee).Error)

	settledNow, err = RetryInvitationSettlement(invitee.Id)
	require.NoError(t, err)
	assert.True(t, settledNow)
	settledNow, err = RetryInvitationSettlement(invitee.Id)
	require.NoError(t, err)
	assert.False(t, settledNow)

	require.NoError(t, DB.First(&storedInviter, inviter.Id).Error)
	assert.Equal(t, 4, storedInviter.AffCount)
	assert.Equal(t, 150, storedInviter.AffQuota)
	assert.Equal(t, 450, storedInviter.AffHistoryQuota)
	require.NoError(t, DB.First(restoredInvitee, invitee.Id).Error)
	assert.Equal(t, 720, restoredInvitee.Quota)
}

func TestInsertWithTxPersistsSnapshotBeforeOAuthFinalize(t *testing.T) {
	setupInvitationSettlementTest(t)
	inviter := &User{Username: "oauth-inviter", Password: "password", Status: common.UserStatusEnabled}
	require.NoError(t, DB.Create(inviter).Error)

	oldInviterReward := common.QuotaForInviter
	oldInviteeReward := common.QuotaForInvitee
	paymentSetting := operation_setting.GetPaymentSetting()
	oldPaymentSetting := *paymentSetting
	common.QuotaForInviter = 120
	common.QuotaForInvitee = 80
	paymentSetting.ComplianceConfirmed = true
	paymentSetting.ComplianceTermsVersion = operation_setting.CurrentComplianceTermsVersion
	t.Cleanup(func() {
		common.QuotaForInviter = oldInviterReward
		common.QuotaForInvitee = oldInviteeReward
		*paymentSetting = oldPaymentSetting
	})

	invitee := &User{Username: "oauth-invitee", Status: common.UserStatusEnabled}
	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
		return invitee.InsertWithTx(tx, inviter.Id)
	}))

	var pending InvitationSettlement
	require.NoError(t, DB.Where("invitee_id = ?", invitee.Id).First(&pending).Error)
	assert.Equal(t, InvitationSettlementStatusPending, pending.Status)
	assert.Equal(t, 120, pending.InviterReward)
	assert.Equal(t, 80, pending.InviteeReward)

	common.QuotaForInviter = 999
	common.QuotaForInvitee = 888
	invitee.FinalizeOAuthUserCreation(inviter.Id)
	invitee.FinalizeOAuthUserCreation(inviter.Id)

	var storedInviter User
	require.NoError(t, DB.First(&storedInviter, inviter.Id).Error)
	assert.Equal(t, 1, storedInviter.AffCount)
	assert.Equal(t, 120, storedInviter.AffQuota)
	assert.Equal(t, 120, storedInviter.AffHistoryQuota)
	var storedInvitee User
	require.NoError(t, DB.First(&storedInvitee, invitee.Id).Error)
	assert.Equal(t, 80, storedInvitee.Quota)
}

func TestInsertRollsBackUserWhenSettlementCannotBeCreated(t *testing.T) {
	setupInvitationSettlementTest(t)
	invitee := &User{Username: "orphan-invitee", Password: "password", Status: common.UserStatusEnabled}

	err := invitee.Insert(999999)
	require.Error(t, err)

	var userCount int64
	require.NoError(t, DB.Model(&User{}).Where("username = ?", invitee.Username).Count(&userCount).Error)
	assert.Zero(t, userCount)
	var settlementCount int64
	require.NoError(t, DB.Model(&InvitationSettlement{}).Count(&settlementCount).Error)
	assert.Zero(t, settlementCount)
}
