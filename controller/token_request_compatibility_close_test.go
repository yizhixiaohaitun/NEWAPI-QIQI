package controller

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// Barrier in the actual controller read, not the model's SelectUpdate path:
// status-only reads true -> normal controller saves false -> status write.
func TestUpdateTokenStatusOnlyDoesNotRestoreConcurrentCompatibility(t *testing.T) {
	db, r := setupRequestCompatibilityAPI(t)
	token := model.Token{UserId: 1, Key: "test-only-controller-barrier-" + strconv.FormatInt(time.Now().UnixNano(), 10), Name: "barrier", UnlimitedQuota: true, ExpiredTime: -1, Status: common.TokenStatusEnabled, RequestCompatibilityEnabled: common.GetPointer(true)}
	require.NoError(t, db.Create(&token).Error)
	addr := os.Getenv("TEST_REDIS_ADDR")
	if addr == "" {
		t.Skip("set TEST_REDIS_ADDR for controller cache barrier integration")
	}
	client := redis.NewClient(&redis.Options{Addr: addr})
	require.NoError(t, client.Ping(context.Background()).Err())
	oldRDB := common.RDB
	common.RDB, common.RedisEnabled = client, true
	cacheKey := "token:" + common.GenerateHMAC(token.Key)
	defer func() {
		client.Del(context.Background(), cacheKey)
		common.RDB, common.RedisEnabled = oldRDB, false
		client.Close()
	}()
	_, err := model.GetTokenByKey(token.Key, false)
	require.NoError(t, err)
	oldRead := make(chan struct{})
	release := make(chan struct{})
	var paused atomic.Bool
	require.NoError(t, db.Callback().Query().After("gorm:query").Register("test:status-controller-barrier", func(tx *gorm.DB) {
		if row, ok := tx.Statement.Dest.(*model.Token); ok && row.Id == token.Id && row.RequestCompatibilityEnabled != nil && *row.RequestCompatibilityEnabled && paused.CompareAndSwap(false, true) {
			close(oldRead)
			<-release
		}
	}))
	defer db.Callback().Query().Remove("test:status-controller-barrier")
	payload, _ := common.Marshal(map[string]any{"id": token.Id, "status": common.TokenStatusDisabled, "unlimited_quota": true})
	w := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		defer close(done)
		req := httptest.NewRequest(http.MethodPut, "/api/token/?status_only=true", bytes.NewReader(payload))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)
	}()
	select {
	case <-oldRead:
	case <-time.After(10 * time.Second):
		t.Fatal("controller read barrier not reached")
	}
	response := requestCompatibilityAPI(t, r, http.MethodPut, "/api/token/", map[string]any{"id": token.Id, "name": token.Name, "unlimited_quota": true, "expired_time": -1, "request_compatibility_enabled": false})
	close(release)
	require.True(t, response.Success, response.Message)
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("status controller did not complete")
	}
	require.Equal(t, http.StatusOK, w.Code)
	var result tokenAPIResponse
	require.NoError(t, common.Unmarshal(w.Body.Bytes(), &result))
	require.True(t, result.Success, result.Message)
	var stored model.Token
	require.NoError(t, db.First(&stored, token.Id).Error)
	require.NotNil(t, stored.RequestCompatibilityEnabled)
	require.False(t, *stored.RequestCompatibilityEnabled)
	require.Equal(t, common.TokenStatusDisabled, stored.Status)
	var public model.Token
	require.NoError(t, common.Unmarshal(result.Data, &public))
	require.False(t, *public.RequestCompatibilityEnabled, "status response must reload committed settings")
	// Prove next auth lookup is from the refreshed cache, not a lucky SQL read.
	require.NoError(t, db.Callback().Query().Before("gorm:query").Register("test:barrier-no-sql", func(tx *gorm.DB) { tx.AddError(gorm.ErrInvalidDB) }))
	defer db.Callback().Query().Remove("test:barrier-no-sql")
	cached, err := model.GetTokenByKey(token.Key, false)
	require.NoError(t, err)
	require.False(t, *cached.RequestCompatibilityEnabled)
	require.Equal(t, common.TokenStatusDisabled, cached.Status)
}
