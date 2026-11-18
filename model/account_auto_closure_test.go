package model

import (
	"context"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupAccountClosureTest(t *testing.T) {
	t.Helper()
	truncateTables(t)
	require.NoError(t, DB.Exec("DELETE FROM users").Error)
	oldQuotaPerUnit := common.QuotaPerUnit
	oldBatchUpdateEnabled := common.BatchUpdateEnabled
	common.QuotaPerUnit = 100
	common.BatchUpdateEnabled = false
	t.Cleanup(func() {
		common.QuotaPerUnit = oldQuotaPerUnit
		common.BatchUpdateEnabled = oldBatchUpdateEnabled
	})
}

func newTrackedUser(t *testing.T, id int, createdAt int64, quota int) *User {
	t.Helper()
	user := &User{
		Id:                   id,
		Username:             "account-closure-" + string(rune('a'+id)),
		Password:             "password",
		Role:                 common.RoleCommonUser,
		Status:               common.UserStatusEnabled,
		CreatedAt:            createdAt,
		AffCode:              "tracked-code-" + string(rune('a'+id)),
		Quota:                quota,
		AccountReviewTracked: true,
	}
	require.NoError(t, DB.Create(user).Error)
	return user
}

func TestAccountClosureUsesHistoricalThresholdNotCurrentBalance(t *testing.T) {
	setupAccountClosureTest(t)
	now := time.Now().Unix()
	user := newTrackedUser(t, 301, now-1, 0)

	require.NoError(t, IncreaseUserQuota(user.Id, 101, true))
	require.NoError(t, DecreaseUserQuota(user.Id, 101, true))

	closed, err := CloseExpiredAccounts(context.Background(), time.Unix(now+int64((72*time.Hour)/time.Second)+1, 0), 20)
	require.NoError(t, err)
	assert.Empty(t, closed)

	var stored User
	require.NoError(t, DB.First(&stored, user.Id).Error)
	assert.NotZero(t, stored.AccountQualifiedAt)
	assert.Equal(t, 0, stored.Quota)
}

func TestAccountClosureRemovesExpiredUnqualifiedUser(t *testing.T) {
	setupAccountClosureTest(t)
	now := time.Now().Unix()
	user := newTrackedUser(t, 302, now-int64((72*time.Hour)/time.Second)-1, 0)

	closed, err := CloseExpiredAccounts(context.Background(), time.Unix(now, 0), 20)
	require.NoError(t, err)
	assert.Equal(t, []int{user.Id}, closed)

	var stored User
	assert.ErrorIs(t, DB.First(&stored, user.Id).Error, gorm.ErrRecordNotFound)
}

func TestAccountClosureKeepsAdminCreatedAndAdminAdjustedAccounts(t *testing.T) {
	setupAccountClosureTest(t)
	now := time.Now().Unix()
	adminCreated := &User{
		Id:             303,
		Username:       "admin-created-account",
		Password:       "password",
		Role:           common.RoleCommonUser,
		Status:         common.UserStatusEnabled,
		CreatedAt:      now - int64(72*time.Hour) - 1,
		AffCode:        "admin-created-code",
		CreatedByAdmin: true,
	}
	adminAdjusted := newTrackedUser(t, 304, now-int64((72*time.Hour)/time.Second)-1, 0)
	require.NoError(t, DB.Create(adminCreated).Error)
	require.NoError(t, SetUserQuota(adminAdjusted.Id, 0))

	closed, err := CloseExpiredAccounts(context.Background(), time.Unix(now, 0), 20)
	require.NoError(t, err)
	assert.Equal(t, []int{adminAdjusted.Id}, closed)
	assert.NoError(t, DB.First(&adminCreated, adminCreated.Id).Error)
}
