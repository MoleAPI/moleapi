package model

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// One durable counter per user; restarts and multiple application instances
// must not reset the daily export allowance.
type LogExportAllowance struct {
	UserID   int   `gorm:"primaryKey;autoIncrement:false"`
	DayStart int64 `gorm:"bigint"`
	Attempts int
}

var ErrLogExportLimit = errors.New("Daily export limit reached (3 per UTC day)")

func ReserveLogExport(ctx context.Context, userID int, now time.Time) error {
	day := now.UTC().Truncate(24 * time.Hour).Unix()
	return DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&LogExportAllowance{UserID: userID}).Error; err != nil {
			return err
		}
		// This conditional UPDATE also serializes concurrent requests on SQLite.
		result := tx.Model(&LogExportAllowance{}).Where("user_id = ? AND (day_start < ? OR attempts < ?)", userID, day, 3).
			Updates(map[string]any{"attempts": gorm.Expr("CASE WHEN day_start < ? THEN 1 ELSE attempts + 1 END", day), "day_start": day})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrLogExportLimit
		}
		return nil
	})
}
