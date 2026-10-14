package model

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func requestCompatibilityCacheFixture(t *testing.T) (*gorm.DB, *redis.Client, *Token, string) {
	t.Helper()
	addr := os.Getenv("TEST_REDIS_ADDR")
	if addr == "" {
		t.Skip("set TEST_REDIS_ADDR to a local Redis for cache integration")
	}
	oldDB, oldRDB, oldRedis, oldKeyCol := DB, common.RDB, common.RedisEnabled, commonKeyCol
	t.Cleanup(func() { DB, common.RDB, common.RedisEnabled, commonKeyCol = oldDB, oldRDB, oldRedis, oldKeyCol })
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { assert.NoError(t, sqlDB.Close()) })
	require.NoError(t, db.AutoMigrate(&Token{}))
	DB, commonKeyCol = db, "`key`"
	client := redis.NewClient(&redis.Options{Addr: addr, MaxRetries: -1, DialTimeout: time.Second, ReadTimeout: time.Second, WriteTimeout: time.Second})
	t.Cleanup(func() { assert.NoError(t, client.Close()) })
	require.NoError(t, client.Ping(context.Background()).Err())
	common.RDB, common.RedisEnabled = client, true
	key, err := common.GenerateKey()
	require.NoError(t, err)
	token := &Token{UserId: 1, Key: key, Name: "test-only-request-compatibility", UnlimitedQuota: true, ExpiredTime: -1}
	require.NoError(t, token.Insert())
	cacheKey := "token:" + common.GenerateHMAC(key)
	t.Cleanup(func() { assert.NoError(t, client.Del(context.Background(), cacheKey).Err()) })
	return db, client, token, cacheKey
}

func TestTokenRequestCompatibilityEffectiveDefault(t *testing.T) {
	for _, tc := range []struct {
		value *bool
		want  bool
	}{{nil, true}, {common.GetPointer(false), false}, {common.GetPointer(true), true}} {
		token := Token{RequestCompatibilityEnabled: tc.value}
		assert.Equal(t, tc.want, token.IsRequestCompatibilityEnabled())
	}
}

func TestTokenRequestCompatibilityCacheRoundTrip(t *testing.T) {
	db, client, token, cacheKey := requestCompatibilityCacheFixture(t)
	for _, tc := range []struct {
		name    string
		value   *bool
		want    bool
		encoded string
	}{
		{"legacy-null", nil, true, ""},
		{"explicit-off", common.GetPointer(false), false, "false"},
		{"explicit-on", common.GetPointer(true), true, "true"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			token.RequestCompatibilityEnabled = tc.value
			require.NoError(t, db.Model(token).UpdateColumn("request_compatibility_enabled", tc.value).Error)
			require.NoError(t, cacheSetToken(*token))
			encoded, err := client.HGet(context.Background(), cacheKey, "RequestCompatibilityEnabled").Result()
			require.NoError(t, err)
			assert.Equal(t, tc.encoded, encoded)
			require.NoError(t, db.Callback().Query().Before("gorm:query").Register("test:no-sql", func(tx *gorm.DB) { tx.AddError(assert.AnError) }))
			loaded, err := GetTokenByKey(token.Key, false)
			require.NoError(t, err)
			assert.Equal(t, tc.want, loaded.IsRequestCompatibilityEnabled())
			assert.Equal(t, tc.value, loaded.RequestCompatibilityEnabled)
			assert.Equal(t, token.Key, loaded.Key)
			require.NoError(t, db.Callback().Query().Remove("test:no-sql"))
		})
	}
}

func TestTokenRequestCompatibilityOldCacheMissingFieldReloadsOnce(t *testing.T) {
	db, client, token, cacheKey := requestCompatibilityCacheFixture(t)
	for _, tc := range []struct {
		name   string
		stored *bool
		want   bool
	}{
		{"legacy-null", nil, true},
		{"already-off-in-sql", common.GetPointer(false), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			token.RequestCompatibilityEnabled = tc.stored
			require.NoError(t, db.Model(token).UpdateColumn("request_compatibility_enabled", tc.stored).Error)
			require.NoError(t, cacheSetToken(*token))
			require.NoError(t, client.HSet(context.Background(), cacheKey, "CacheVersion", "1").Err())
			require.NoError(t, client.HDel(context.Background(), cacheKey, "RequestCompatibilityEnabled").Err())
			queries := 0
			require.NoError(t, db.Callback().Query().Before("gorm:query").Register("test:count-sql", func(tx *gorm.DB) { queries++ }))
			loaded, err := ValidateUserToken(token.Key)
			require.NoError(t, err)
			assert.Equal(t, tc.want, loaded.IsRequestCompatibilityEnabled())
			assert.Equal(t, 1, queries, "v1 cache must reload authoritative SQL")
			loaded, err = ValidateUserToken(token.Key)
			require.NoError(t, err)
			assert.Equal(t, tc.want, loaded.IsRequestCompatibilityEnabled())
			assert.Equal(t, 1, queries, "refreshed cache must not query SQL per request")
			require.NoError(t, db.Callback().Query().Remove("test:count-sql"))
		})
	}
}

// A go-redis hook faults only this test client; Redis itself is real and no
// global server configuration, shared keys, or user data are modified.
type requestCompatibilityRedisFault struct{ failWrite, failDelete bool }

func (h *requestCompatibilityRedisFault) BeforeProcess(ctx context.Context, cmd redis.Cmder) (context.Context, error) {
	if h.failDelete && cmd.Name() == "del" {
		return ctx, errors.New("test-only cache delete failure")
	}
	return ctx, nil
}
func (h *requestCompatibilityRedisFault) AfterProcess(context.Context, redis.Cmder) error { return nil }
func (h *requestCompatibilityRedisFault) BeforeProcessPipeline(ctx context.Context, cmds []redis.Cmder) (context.Context, error) {
	for _, cmd := range cmds {
		if h.failWrite && cmd.Name() == "hset" {
			return ctx, errors.New("test-only cache write failure")
		}
	}
	return ctx, nil
}
func (h *requestCompatibilityRedisFault) AfterProcessPipeline(context.Context, []redis.Cmder) error {
	return nil
}

func TestTokenRequestCompatibilityUpdateFailureContracts(t *testing.T) {
	for _, failure := range []string{"sql", "redis-refresh", "redis-refresh-and-invalidation"} {
		t.Run(failure, func(t *testing.T) {
			db, client, token, cacheKey := requestCompatibilityCacheFixture(t)
			require.NoError(t, cacheSetToken(*token))
			hook := &requestCompatibilityRedisFault{}
			client.AddHook(hook)
			t.Cleanup(func() { hook.failWrite, hook.failDelete = false, false })
			if failure == "sql" {
				require.NoError(t, db.Callback().Update().Before("gorm:update").Register("test:sql-update-failure", func(tx *gorm.DB) { tx.AddError(assert.AnError) }))
			} else {
				hook.failWrite = true
				hook.failDelete = failure == "redis-refresh-and-invalidation"
			}
			token.RequestCompatibilityEnabled = common.GetPointer(false)
			err := token.Update()
			require.Error(t, err, "an unsuccessful SQL+cache chain must not report success")
			var stored Token
			require.NoError(t, db.First(&stored, token.Id).Error)
			require.NotNil(t, stored.RequestCompatibilityEnabled)
			if failure == "sql" {
				assert.ErrorIs(t, err, assert.AnError)
				assert.True(t, *stored.RequestCompatibilityEnabled)
				cached, cacheErr := cacheGetTokenByKey(token.Key)
				require.NoError(t, cacheErr)
				assert.True(t, cached.IsRequestCompatibilityEnabled())
				return
			}
			assert.False(t, *stored.RequestCompatibilityEnabled, "SQL already committed; no false rollback claim")
			assert.Contains(t, err.Error(), "saved to database but cache refresh failed")
			if hook.failDelete {
				assert.Contains(t, err.Error(), "cache invalidation also failed")
				cached, cacheErr := cacheGetTokenByKey(token.Key)
				require.NoError(t, cacheErr)
				assert.True(t, cached.IsRequestCompatibilityEnabled(), "old hash may remain until recovery/TTL after both failures")
			} else {
				exists, existsErr := client.Exists(context.Background(), cacheKey).Result()
				require.NoError(t, existsErr)
				assert.Zero(t, exists, "invalidate old settings synchronously when refresh fails")
			}
			hook.failWrite, hook.failDelete = false, false
			loaded, loadErr := GetTokenByKey(token.Key, true)
			require.NoError(t, loadErr)
			assert.False(t, loaded.IsRequestCompatibilityEnabled())
			cached, cacheErr := cacheGetTokenByKey(token.Key)
			require.NoError(t, cacheErr)
			assert.False(t, cached.IsRequestCompatibilityEnabled())
		})
	}
}

func TestTokenRequestCompatibilitySelectUpdateDoesNotRestoreOldSettings(t *testing.T) {
	_, _, token, _ := requestCompatibilityCacheFixture(t)
	stale := *token
	token.RequestCompatibilityEnabled = common.GetPointer(false)
	require.NoError(t, token.Update())
	stale.AccessedTime = common.GetTimestamp()
	require.NoError(t, stale.SelectUpdate())
	loaded, err := GetTokenByKey(token.Key, false)
	require.NoError(t, err)
	assert.False(t, loaded.IsRequestCompatibilityEnabled(), "status/access updates must not republish stale compatibility")
}
