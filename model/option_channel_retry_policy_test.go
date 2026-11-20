package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestChannelRetryPolicyDatabaseReload(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	require.NoError(t, db.AutoMigrate(&Option{}))
	oldDB := DB
	DB = db
	common.OptionMapRWMutex.Lock()
	oldOptions := common.OptionMap
	common.OptionMap = make(map[string]string)
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		DB = oldDB
		common.OptionMapRWMutex.Lock()
		common.OptionMap = oldOptions
		common.OptionMapRWMutex.Unlock()
		require.NoError(t, operation_setting.SetChannelRetryPolicy(operation_setting.DefaultChannelRetryPolicy))
	})
	const key = operation_setting.ChannelRetryPolicyOption
	const retryRule = `[{"channel_id":8,"status":400,"action":"retry"}]`
	const stopRule = `[{"channel_id":8,"status":400,"action":"stop"}]`
	require.NoError(t, UpdateOptionsBulk(map[string]string{key: retryRule}))
	assertPolicy := func(wantText string, wantRetry, wantMatched bool) {
		t.Helper()
		common.OptionMapRWMutex.RLock()
		text := common.OptionMap[key]
		common.OptionMapRWMutex.RUnlock()
		require.Equal(t, wantText, text)
		retry, matched := operation_setting.MatchChannelRetryPolicy(8, 400, "", "")
		require.Equal(t, wantRetry, retry)
		require.Equal(t, wantMatched, matched)
	}
	assertPolicy(retryRule, true, true)
	require.Error(t, UpdateOptionsBulk(map[string]string{key: `null`, "UnrelatedTestOption": "untouched"}))
	var count int64
	require.NoError(t, db.Model(&Option{}).Where("key = ?", "UnrelatedTestOption").Count(&count).Error)
	require.Zero(t, count)
	loadOptionsFromDatabase()
	assertPolicy(retryRule, true, true)

	// Simulate a setting written by a different process sharing the database.
	require.NoError(t, db.Save(&Option{Key: key, Value: stopRule}).Error)
	loadOptionsFromDatabase()
	assertPolicy(stopRule, false, true)

	// A bad database value must not replace the last valid snapshot.
	require.NoError(t, db.Save(&Option{Key: key, Value: `null`}).Error)
	loadOptionsFromDatabase()
	assertPolicy(stopRule, false, true)

	require.NoError(t, db.Delete(&Option{Key: key}).Error)
	loadOptionsFromDatabase()
	assertPolicy(operation_setting.DefaultChannelRetryPolicy, false, false)
}
