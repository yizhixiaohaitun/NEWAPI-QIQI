package model

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"sync"
	"testing"
)

func TestRemoveChannelModel(t *testing.T) {
	oldDB, oldCache := DB, common.MemoryCacheEnabled
	t.Cleanup(func() { DB = oldDB; common.MemoryCacheEnabled = oldCache })
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&Channel{}, &Ability{}))
	DB = db
	common.MemoryCacheEnabled = true
	channels := []Channel{{Id: 33, Group: "default", Models: "alias,other", Status: common.ChannelStatusEnabled}, {Id: 34, Group: "default", Models: "alias", Status: common.ChannelStatusEnabled}}
	for i := range channels {
		require.NoError(t, db.Create(&channels[i]).Error)
		require.NoError(t, channels[i].AddAbilities(nil))
	}
	InitChannelCache()
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, e := RemoveChannelModel(33, "alias")
			if e != nil {
				t.Error(e)
			}
		}()
	}
	wg.Wait()
	var c Channel
	require.NoError(t, db.First(&c, 33).Error)
	require.Equal(t, "other", c.Models)
	require.Equal(t, common.ChannelStatusEnabled, c.Status)
	var abilities []Ability
	require.NoError(t, db.Where("channel_id = ?", 33).Find(&abilities).Error)
	require.Len(t, abilities, 1)
	require.Equal(t, "other", abilities[0].Model)
	selected, err := GetRandomSatisfiedChannel("default", "alias", 0, "")
	require.NoError(t, err)
	require.Equal(t, 34, selected.Id)
	_, err = RemoveChannelModel(33, "other")
	require.NoError(t, err)
	require.NoError(t, db.First(&c, 33).Error)
	require.Empty(t, c.Models)
	require.NoError(t, c.UpdateAbilities(nil))
	require.NoError(t, db.Where("channel_id = ?", 33).Find(&abilities).Error)
	require.Empty(t, abilities)
	InitChannelCache()
	selected, err = GetRandomSatisfiedChannel("default", "", 0, "")
	require.NoError(t, err)
	require.Nil(t, selected)
}
