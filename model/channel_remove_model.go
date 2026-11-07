package model

import (
	"fmt"
	"github.com/QuantumNous/new-api/common"
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
			result := tx.Model(&Channel{}).Where("id = ? AND models = ?", id, ch.Models).UpdateColumn("models", strings.Join(kept, ","))
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
