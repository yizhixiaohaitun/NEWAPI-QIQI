package controller

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func openRequestCompatibilitySQLite(t *testing.T, path string) *gorm.DB {
	t.Helper()
	oldPath, oldMaster := common.SQLitePath, common.IsMasterNode
	common.SQLitePath, common.IsMasterNode = path, false
	t.Cleanup(func() { common.SQLitePath, common.IsMasterNode = oldPath, oldMaster })
	t.Setenv("SQL_DSN", "local")
	t.Setenv("LOG_SQL_DSN", "")
	// Use the real initializer so SQL quoting metadata is not left empty in a
	// single-test run. Master=false avoids unrelated schema migrations.
	require.NoError(t, model.InitDB())
	db := model.DB
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	return db
}

func setupRequestCompatibilityAPI(t *testing.T) (*gorm.DB, *gin.Engine) {
	t.Helper()
	oldDB, oldLogDB, oldRedis := model.DB, model.LOG_DB, common.RedisEnabled
	oldMain, oldLog := common.MainDatabaseType(), common.LogDatabaseType()
	t.Cleanup(func() {
		model.DB, model.LOG_DB, common.RedisEnabled = oldDB, oldLogDB, oldRedis
		common.SetDatabaseTypes(oldMain, oldLog)
	})
	common.RedisEnabled = false
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	db := openRequestCompatibilitySQLite(t, ":memory:")
	require.NoError(t, db.AutoMigrate(&model.Token{}))
	model.DB, model.LOG_DB = db, db
	gin.SetMode(gin.TestMode)
	r := gin.New()
	// Isolated authenticated user fixture: real token handlers and JSON binding,
	// without using any installed app, existing session, or external auth service.
	r.Use(func(c *gin.Context) { c.Set("id", 1) })
	r.POST("/api/token/", AddToken)
	r.PUT("/api/token/", UpdateToken)
	r.GET("/api/token/", GetAllTokens)
	r.GET("/api/token/search", SearchTokens)
	r.GET("/api/token/:id", GetToken)
	return db, r
}

func requestCompatibilityAPI(t *testing.T, r *gin.Engine, method, path string, body any) tokenAPIResponse {
	t.Helper()
	payload, err := common.Marshal(body)
	require.NoError(t, err)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(method, path, bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var response tokenAPIResponse
	require.NoError(t, common.Unmarshal(w.Body.Bytes(), &response))
	return response
}

func TestTokenRequestCompatibilityAPICreateAndUpdate(t *testing.T) {
	db, r := setupRequestCompatibilityAPI(t)
	for _, tc := range []struct {
		name  string
		value any
		omit  bool
		want  bool
	}{
		{"default", nil, true, true},
		{"null", nil, false, true},
		{"explicit-false", false, false, false},
		{"explicit-true", true, false, true},
		// The UI creates a batch with multiple POSTs, not an unsupported batch-create API.
		{"batch-1", false, false, false},
		{"batch-2", false, false, false},
		{"batch-3", false, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := map[string]any{"name": tc.name, "unlimited_quota": true, "expired_time": -1, "upstream_timeout": 0}
			if !tc.omit {
				body["request_compatibility_enabled"] = tc.value
			}
			response := requestCompatibilityAPI(t, r, http.MethodPost, "/api/token/", body)
			require.True(t, response.Success, response.Message)
			var token model.Token
			require.NoError(t, db.Where("name = ?", tc.name).First(&token).Error)
			require.NotNil(t, token.RequestCompatibilityEnabled)
			assert.Equal(t, tc.want, *token.RequestCompatibilityEnabled)
			assert.Equal(t, tc.want, token.IsRequestCompatibilityEnabled())
		})
	}
	var count int64
	require.NoError(t, db.Model(&model.Token{}).Count(&count).Error)
	assert.EqualValues(t, 7, count)

	var token model.Token
	require.NoError(t, db.Where("name = ?", "default").First(&token).Error)
	for _, tc := range []struct {
		name       string
		value      any
		omit       bool
		statusOnly bool
		want       bool
	}{
		{"update-false", false, false, false, false},
		{"old-client-omitted", nil, true, false, false},
		{"old-client-null", nil, false, false, false},
		{"status-only-ignores-true", true, false, true, false},
		{"update-true", true, false, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := map[string]any{"id": token.Id, "name": tc.name, "expired_time": -1, "unlimited_quota": true, "status": common.TokenStatusDisabled}
			if !tc.omit {
				body["request_compatibility_enabled"] = tc.value
			}
			path := "/api/token/"
			if tc.statusOnly {
				path += "?status_only=true"
			}
			response := requestCompatibilityAPI(t, r, http.MethodPut, path, body)
			require.True(t, response.Success, response.Message)
			var result model.Token
			require.NoError(t, common.Unmarshal(response.Data, &result))
			require.NotNil(t, result.RequestCompatibilityEnabled)
			assert.Equal(t, tc.want, *result.RequestCompatibilityEnabled)
			assert.Equal(t, token.GetMaskedKey(), result.Key)
			assert.NotContains(t, string(response.Data), token.Key)
			var stored model.Token
			require.NoError(t, db.First(&stored, token.Id).Error)
			require.NotNil(t, stored.RequestCompatibilityEnabled)
			assert.Equal(t, tc.want, *stored.RequestCompatibilityEnabled)
			if tc.statusOnly {
				assert.Equal(t, common.TokenStatusDisabled, stored.Status)
				assert.NotEqual(t, tc.name, stored.Name)
			}
		})
	}
}

func TestTokenRequestCompatibilityAPILegacyMaskedResponses(t *testing.T) {
	db, r := setupRequestCompatibilityAPI(t)
	token := model.Token{UserId: 1, Name: "legacy", Key: "test-only-legacy-request-compatibility", UnlimitedQuota: true, ExpiredTime: -1}
	require.NoError(t, db.Create(&token).Error)
	// Simulate a row predating the new nullable column, not a newly-created row.
	require.NoError(t, db.Model(&token).UpdateColumn("request_compatibility_enabled", nil).Error)
	response := requestCompatibilityAPI(t, r, http.MethodGet, "/api/token/"+strconv.Itoa(token.Id), nil)
	require.True(t, response.Success, response.Message)
	var detail model.Token
	require.NoError(t, common.Unmarshal(response.Data, &detail))
	require.NotNil(t, detail.RequestCompatibilityEnabled)
	assert.True(t, *detail.RequestCompatibilityEnabled)
	assert.Equal(t, token.GetMaskedKey(), detail.Key)
	for _, path := range []string{"/api/token/", "/api/token/search?keyword=legacy"} {
		response = requestCompatibilityAPI(t, r, http.MethodGet, path, nil)
		require.True(t, response.Success, response.Message)
		var page struct {
			Items []*model.Token `json:"items"`
		}
		require.NoError(t, common.Unmarshal(response.Data, &page))
		require.Len(t, page.Items, 1)
		require.NotNil(t, page.Items[0].RequestCompatibilityEnabled)
		assert.True(t, *page.Items[0].RequestCompatibilityEnabled)
		assert.NotContains(t, string(response.Data), token.Key)
	}
	var stored model.Token
	require.NoError(t, db.First(&stored, token.Id).Error)
	assert.Nil(t, stored.RequestCompatibilityEnabled, "public normalization must not mutate storage")
}

func TestTokenRequestCompatibilityAPISynchronousCache(t *testing.T) {
	db, r := setupRequestCompatibilityAPI(t)
	addr := os.Getenv("TEST_REDIS_ADDR")
	if addr == "" {
		t.Skip("set TEST_REDIS_ADDR to a local Redis for cache integration")
	}
	client := redis.NewClient(&redis.Options{Addr: addr, MaxRetries: -1, DialTimeout: time.Second, ReadTimeout: time.Second, WriteTimeout: time.Second})
	require.NoError(t, client.Ping(context.Background()).Err())
	oldRDB := common.RDB
	common.RDB, common.RedisEnabled = client, true
	t.Cleanup(func() { common.RDB, common.RedisEnabled = oldRDB, false; assert.NoError(t, client.Close()) })
	response := requestCompatibilityAPI(t, r, http.MethodPost, "/api/token/", map[string]any{"name": "cache-api", "unlimited_quota": true, "expired_time": -1})
	require.True(t, response.Success, response.Message)
	var token model.Token
	require.NoError(t, db.Where("name = ?", "cache-api").First(&token).Error)
	cacheKey := "token:" + common.GenerateHMAC(token.Key)
	t.Cleanup(func() { assert.NoError(t, client.Del(context.Background(), cacheKey).Err()) })
	warm, err := model.GetTokenByKey(token.Key, false)
	require.NoError(t, err)
	assert.True(t, warm.IsRequestCompatibilityEnabled())

	// A failed Redis client makes both refresh and invalidation fail. SQL is
	// already committed, but the API must not claim success while Redis is old.
	deadClient := redis.NewClient(&redis.Options{Addr: addr, MaxRetries: -1})
	require.NoError(t, deadClient.Close())
	common.RDB = deadClient
	response = requestCompatibilityAPI(t, r, http.MethodPut, "/api/token/", map[string]any{
		"id": token.Id, "name": token.Name, "unlimited_quota": true, "expired_time": -1,
		"request_compatibility_enabled": false,
	})
	common.RDB = client
	require.False(t, response.Success)
	assert.Contains(t, response.Message, "saved to database but cache refresh failed")
	assert.Contains(t, response.Message, "cache invalidation also failed")
	var committed model.Token
	require.NoError(t, db.First(&committed, token.Id).Error)
	require.NotNil(t, committed.RequestCompatibilityEnabled)
	assert.False(t, *committed.RequestCompatibilityEnabled)
	oldValue, err := client.HGet(context.Background(), cacheKey, "RequestCompatibilityEnabled").Result()
	require.NoError(t, err)
	assert.Equal(t, "true", oldValue)

	// Retrying after Redis recovers repairs the hash synchronously.
	for _, tc := range []struct {
		value            any
		omit, statusOnly bool
	}{{false, false, false}, {nil, true, false}, {nil, false, false}, {true, false, true}} {
		body := map[string]any{"id": token.Id, "name": token.Name, "unlimited_quota": true, "expired_time": -1, "status": common.TokenStatusDisabled}
		if !tc.omit {
			body["request_compatibility_enabled"] = tc.value
		}
		path := "/api/token/"
		if tc.statusOnly {
			path += "?status_only=true"
		}
		response = requestCompatibilityAPI(t, r, http.MethodPut, path, body)
		require.True(t, response.Success, response.Message)
		// No wait/poll: the very next lookup must use the new Redis value without SQL.
		require.NoError(t, db.Callback().Query().Before("gorm:query").Register("test:compat-no-sql", func(tx *gorm.DB) { tx.AddError(assert.AnError) }))
		cached, err := model.GetTokenByKey(token.Key, false)
		require.NoError(t, err)
		require.NotNil(t, cached.RequestCompatibilityEnabled)
		assert.False(t, *cached.RequestCompatibilityEnabled)
		require.NoError(t, db.Callback().Query().Remove("test:compat-no-sql"))
	}
}

func TestTokenRequestCompatibilityMigrationAndRestart(t *testing.T) {
	oldDB, oldRedis := model.DB, common.RedisEnabled
	t.Cleanup(func() { model.DB, common.RedisEnabled = oldDB, oldRedis })
	common.RedisEnabled = false
	require.NoError(t, os.MkdirAll("../tmp/backend-request-compatibility", 0755))
	dir, err := os.MkdirTemp("../tmp/backend-request-compatibility", "migration-test-")
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, os.RemoveAll(dir)) })
	path := filepath.Join(dir, "tokens.db")
	db := openRequestCompatibilitySQLite(t, path)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&legacyToken{}))
	require.NoError(t, db.Create(&legacyToken{UserId: 1, Key: "test-only-legacy-migration", Name: "legacy", UnlimitedQuota: true, ExpiredTime: -1}).Error)
	require.NoError(t, db.AutoMigrate(&model.Token{}))
	var legacy model.Token
	require.NoError(t, db.Where("name = ?", "legacy").First(&legacy).Error)
	assert.Nil(t, legacy.RequestCompatibilityEnabled)
	assert.True(t, legacy.IsRequestCompatibilityEnabled())
	token := model.Token{UserId: 1, Name: "off", Key: "test-only-false-migration", UnlimitedQuota: true, ExpiredTime: -1, RequestCompatibilityEnabled: common.GetPointer(false)}
	require.NoError(t, db.Create(&token).Error)
	for i := 0; i < 2; i++ {
		require.NoError(t, db.AutoMigrate(&model.Token{}))
	}
	var columns []struct {
		Name    string  `gorm:"column:name"`
		NotNull int     `gorm:"column:notnull"`
		Default *string `gorm:"column:dflt_value"`
	}
	require.NoError(t, db.Raw("PRAGMA table_info(tokens)").Scan(&columns).Error)
	found := false
	for _, column := range columns {
		if column.Name == "request_compatibility_enabled" {
			found = true
			assert.Nil(t, column.Default)
			assert.Zero(t, column.NotNull)
		}
	}
	require.True(t, found)
	require.NoError(t, sqlDB.Close())
	// A fresh pool/connection reloads the file, rather than reusing GORM state.
	reopened := openRequestCompatibilitySQLite(t, path)
	require.NoError(t, reopened.AutoMigrate(&model.Token{}))
	model.DB = reopened
	loaded, err := model.GetTokenByKey(token.Key, true)
	require.NoError(t, err)
	require.NotNil(t, loaded.RequestCompatibilityEnabled)
	assert.False(t, *loaded.RequestCompatibilityEnabled)
	loaded, err = model.GetTokenByKey(legacy.Key, true)
	require.NoError(t, err)
	assert.Nil(t, loaded.RequestCompatibilityEnabled)
	assert.True(t, loaded.IsRequestCompatibilityEnabled())
}
