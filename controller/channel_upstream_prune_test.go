package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/stretchr/testify/require"
)

func TestUpstreamSyncDoesNotRestoreAutomaticallyRemovedModel(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.Ability{}))
	oldCache := common.MemoryCacheEnabled
	common.MemoryCacheEnabled = false
	t.Cleanup(func() { common.MemoryCacheEnabled = oldCache })
	entered, release := make(chan struct{}, 1), make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		entered <- struct{}{}
		<-release
		_, _ = w.Write([]byte(`{"data":[{"id":"alias"},{"id":"other"},{"id":"new"}]}`))
	}))
	defer server.Close()
	service.InitHttpClient()
	base := server.URL
	ch := model.Channel{Id: 33, Key: "test", Group: "default", Models: "alias,other", Status: common.ChannelStatusEnabled, BaseURL: &base}
	settings := dto.ChannelOtherSettings{UpstreamModelUpdateAutoSyncEnabled: true}
	ch.SetOtherSettings(settings)
	require.NoError(t, db.Create(&ch).Error)
	require.NoError(t, ch.AddAbilities(nil))
	done := make(chan error, 1)
	go func() {
		_, _, err := checkAndPersistChannelUpstreamModelUpdates(&ch, &settings, true, true)
		done <- err
	}()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("fetch did not start")
	}
	changed, err := model.RemoveChannelModel(33, "alias")
	require.NoError(t, err)
	require.True(t, changed)
	close(release)
	require.NoError(t, <-done)
	var current model.Channel
	require.NoError(t, db.First(&current, 33).Error)
	require.Equal(t, "other,new", current.Models)
	require.Contains(t, current.GetOtherSettings().UpstreamModelUpdateIgnoredModels, "alias")
	var abilities []model.Ability
	require.NoError(t, db.Where("channel_id = ?", 33).Find(&abilities).Error)
	require.Len(t, abilities, 2)
	for _, a := range abilities {
		require.NotEqual(t, "alias", a.Model)
	}
	// The next fetch sees alias again but the persistent ignore list prevents resurrection.
	_, _, err = checkAndPersistChannelUpstreamModelUpdates(&current, &settings, true, true)
	require.NoError(t, err)
	require.NoError(t, db.First(&current, 33).Error)
	require.Equal(t, "other,new", current.Models)
}
