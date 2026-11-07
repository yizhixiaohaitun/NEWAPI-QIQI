package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestRemoveModelSurvivesStaleInternalMaintenance(t *testing.T) {
	for _, kind := range []string{"status", "setting", "other_settings", "save", "save_without_key"} {
		t.Run(kind, func(t *testing.T) {
			oldDB, oldCache := DB, common.MemoryCacheEnabled
			t.Cleanup(func() { DB = oldDB; common.MemoryCacheEnabled = oldCache })
			db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
			require.NoError(t, err)
			require.NoError(t, db.AutoMigrate(&Channel{}, &Ability{}))
			DB = db
			common.MemoryCacheEnabled = false
			invalid := "{"
			ch := Channel{Id: 33, Group: "default", Models: "alias,other", Status: common.ChannelStatusEnabled, Setting: &invalid, OtherSettings: invalid}
			require.NoError(t, db.Create(&ch).Error)
			require.NoError(t, ch.AddAbilities(nil))
			var stale Channel
			require.NoError(t, db.First(&stale, 33).Error)
			_, err = RemoveChannelModel(33, "alias")
			require.NoError(t, err)
			switch kind {
			case "status":
				stale.Status = common.ChannelStatusAutoDisabled
				require.NoError(t, stale.saveStatusMaintenance())
			case "setting":
				stale.GetSetting()
			case "other_settings":
				stale.GetOtherSettings()
			case "save":
				require.NoError(t, stale.Save())
			case "save_without_key":
				require.NoError(t, stale.SaveWithoutKey())
			}
			require.NoError(t, db.First(&ch, 33).Error)
			require.Equal(t, "other", ch.Models)
			require.Contains(t, ch.GetOtherSettings().UpstreamModelUpdateIgnoredModels, "alias")
			var n int64
			require.NoError(t, db.Model(&Ability{}).Where("channel_id = ? AND model = ?", 33, "alias").Count(&n).Error)
			require.Zero(t, n)
			common.MemoryCacheEnabled = true
			InitChannelCache()
			selected, err := GetRandomSatisfiedChannel("default", "alias", 0, "")
			require.NoError(t, err)
			require.Nil(t, selected)
		})
	}
}
