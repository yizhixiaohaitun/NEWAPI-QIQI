package model

import (
	"fmt"
	"gorm.io/gorm"
)

// CompareAndSwapUpstreamState commits the model list and settings with its abilities.
func CompareAndSwapUpstreamState(ch *Channel, oldModels, oldSettings string, modelsChanged bool) (bool, error) {
	channelCacheRefreshLock.Lock()
	defer channelCacheRefreshLock.Unlock()
	changed := false
	err := DB.Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&Channel{}).Where("id = ? AND models = ? AND settings = ?", ch.Id, oldModels, oldSettings).Updates(map[string]interface{}{"models": ch.Models, "settings": ch.OtherSettings})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return nil
		}
		changed = true
		if modelsChanged {
			return ch.UpdateAbilities(tx)
		}
		return nil
	})
	if err != nil || !changed {
		return changed, err
	}
	if err = initChannelCacheChecked(); err != nil {
		return true, fmt.Errorf("upstream update committed but cache refresh failed: %w", err)
	}
	return true, nil
}
