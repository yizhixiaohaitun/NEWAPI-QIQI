package model

import "gorm.io/gorm"

// createPayableOrder serializes order creation with automatic account closure.
// The closure worker locks the same user row before deleting, then rechecks
// pending orders, so a newly started payment cannot race into a deleted user.
func createPayableOrder(userID int, order any) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		var user User
		if err := lockForUpdate(tx).Select("id").Take(&user, userID).Error; err != nil {
			return err
		}
		return tx.Create(order).Error
	})
}
