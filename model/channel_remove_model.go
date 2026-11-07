package model

import (
	"fmt"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"gorm.io/gorm"
	"strings"
)

func RemoveChannelModel(id int, name string) (bool, error) {
	if strings.TrimSpace(name) == "" {
		return false, fmt.Errorf("empty request model")
	}
	channelCacheRefreshLock.Lock()
	defer channelCacheRefreshLock.Unlock()
	changed := false
	for attempt := 0; attempt < 10; attempt++ {
		conflict := false
		err := DB.Transaction(func(tx *gorm.DB) error {
			var ch Channel
			if err := tx.First(&ch, id).Error; err != nil {
				return err
			}
			parts := strings.Split(ch.Models, ",")
			kept := make([]string, 0, len(parts))
			found := false
			for _, part := range parts {
				if part == name {
					found = true
				} else {
					kept = append(kept, part)
				}
			}
			if !found {
				return nil
			}
			settings := dto.ChannelOtherSettings{}
			if ch.OtherSettings != "" {
				_ = common.UnmarshalJsonStr(ch.OtherSettings, &settings)
			}
			if !containsModelName(settings.UpstreamModelUpdateIgnoredModels, name) {
				settings.UpstreamModelUpdateIgnoredModels = append(settings.UpstreamModelUpdateIgnoredModels, name)
			}
			oldSettings := ch.OtherSettings
			ch.SetOtherSettings(settings)
			result := tx.Model(&Channel{}).Where("id = ? AND models = ? AND settings = ?", id, ch.Models, oldSettings).Updates(map[string]interface{}{"models": strings.Join(kept, ","), "settings": ch.OtherSettings})
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected != 1 {
				conflict = true
				return nil
			}
			if err := tx.Where("channel_id = ? AND model = ?", id, name).Delete(&Ability{}).Error; err != nil {
				return err
			}
			changed = true
			return nil
		})
		if err != nil {
			return false, err
		}
		if !conflict {
			break
		}
		if attempt == 9 {
			return false, fmt.Errorf("concurrent channel update")
		}
	}
	if err := initChannelCacheChecked(); err != nil {
		return false, fmt.Errorf("model update committed but cache refresh failed: %w", err)
	}
	if changed {
		common.SysLog(fmt.Sprintf("channel #%d removed model %q after upstream keyword match", id, name))
	}
	return changed, nil
}

func containsModelName(names []string, name string) bool {
	for _, n := range names {
		if n == name {
			return true
		}
	}
	return false
}
