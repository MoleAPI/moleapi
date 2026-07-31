package model

import (
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestUserAutoMigrationAddsInviteRewardFields(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&User{}))
	require.NoError(t, db.Create(&User{Id: 1, Username: "legacy_reward_user", Password: "password123"}).Error)
	require.NoError(t, db.Migrator().DropColumn(&User{}, "InviterTopupRewarded"))
	require.NoError(t, db.Migrator().DropColumn(&User{}, "InviteRebateRatio"))
	assert.False(t, db.Migrator().HasColumn(&User{}, "InviterTopupRewarded"))
	assert.False(t, db.Migrator().HasColumn(&User{}, "InviteRebateRatio"))

	require.NoError(t, db.AutoMigrate(&User{}))
	require.NoError(t, db.AutoMigrate(&User{}))
	assert.True(t, db.Migrator().HasColumn(&User{}, "InviterTopupRewarded"))
	assert.True(t, db.Migrator().HasColumn(&User{}, "InviteRebateRatio"))

	var legacy User
	require.NoError(t, db.First(&legacy, 1).Error)
	assert.False(t, legacy.InviterTopupRewarded)
	assert.Zero(t, legacy.InviteRebateRatio)
}
