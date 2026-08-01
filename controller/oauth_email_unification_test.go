package controller

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/oauth"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupOAuthEmailUnificationTest(t *testing.T) *gorm.DB {
	t.Helper()
	previousDB := model.DB
	previousType := common.MainDatabaseType()
	previousRegisterEnabled := common.RegisterEnabled
	previousRedisEnabled := common.RedisEnabled
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.UserOAuthBinding{}))
	model.DB = db
	common.SetMainDatabaseType(common.DatabaseTypeSQLite)
	common.RegisterEnabled = false
	common.RedisEnabled = false
	t.Cleanup(func() {
		model.DB = previousDB
		common.SetMainDatabaseType(previousType)
		common.RegisterEnabled = previousRegisterEnabled
		common.RedisEnabled = previousRedisEnabled
	})
	return db
}

func TestOAuthLoginBindsVerifiedEmailToExistingUser(t *testing.T) {
	db := setupOAuthEmailUnificationTest(t)
	existing := model.User{
		Username: "existing",
		Password: "password",
		Email:    "owner@example.com",
		Role:     common.RoleCommonUser,
		Status:   common.UserStatusEnabled,
		Group:    "default",
	}
	require.NoError(t, db.Create(&existing).Error)

	user, err := findOrCreateOAuthUser(nil, &oauth.GitHubProvider{}, &oauth.OAuthUser{
		ProviderUserID: "12345",
		Email:          "OWNER@example.com",
		EmailVerified:  true,
	}, "")

	require.NoError(t, err)
	assert.Equal(t, existing.Id, user.Id)

	var updated model.User
	require.NoError(t, db.First(&updated, existing.Id).Error)
	assert.Equal(t, "12345", updated.GitHubId)
	assert.Equal(t, "owner@example.com", updated.Email)
}

func TestOAuthLoginDoesNotBindUnverifiedEmail(t *testing.T) {
	db := setupOAuthEmailUnificationTest(t)
	existing := model.User{
		Username: "existing",
		Password: "password",
		Email:    "owner@example.com",
		Role:     common.RoleCommonUser,
		Status:   common.UserStatusEnabled,
		Group:    "default",
	}
	require.NoError(t, db.Create(&existing).Error)

	_, err := findOrCreateOAuthUser(nil, &oauth.GitHubProvider{}, &oauth.OAuthUser{
		ProviderUserID: "12345",
		Email:          "owner@example.com",
	}, "")

	var registrationErr *OAuthRegistrationDisabledError
	require.ErrorAs(t, err, &registrationErr)

	var updated model.User
	require.NoError(t, db.First(&updated, existing.Id).Error)
	assert.Empty(t, updated.GitHubId)
}

func TestOAuthLoginBackfillsVerifiedEmailForExistingGitHubUser(t *testing.T) {
	db := setupOAuthEmailUnificationTest(t)
	existing := model.User{
		Username: "existing",
		Password: "password",
		GitHubId: "12345",
		Role:     common.RoleCommonUser,
		Status:   common.UserStatusEnabled,
		Group:    "default",
	}
	require.NoError(t, db.Create(&existing).Error)

	user, err := findOrCreateOAuthUser(nil, &oauth.GitHubProvider{}, &oauth.OAuthUser{
		ProviderUserID: "12345",
		Email:          "OWNER@example.com",
		EmailVerified:  true,
	}, "")

	require.NoError(t, err)
	assert.Equal(t, existing.Id, user.Id)

	var updated model.User
	require.NoError(t, db.First(&updated, existing.Id).Error)
	assert.Equal(t, "owner@example.com", updated.Email)
}

func TestOAuthLoginDoesNotOverwriteExistingEmailForGitHubUser(t *testing.T) {
	db := setupOAuthEmailUnificationTest(t)
	existing := model.User{
		Username: "existing",
		Password: "password",
		Email:    "manual@example.com",
		GitHubId: "12345",
		Role:     common.RoleCommonUser,
		Status:   common.UserStatusEnabled,
		Group:    "default",
	}
	require.NoError(t, db.Create(&existing).Error)

	user, err := findOrCreateOAuthUser(nil, &oauth.GitHubProvider{}, &oauth.OAuthUser{
		ProviderUserID: "12345",
		Email:          "github@example.com",
		EmailVerified:  true,
	}, "")

	require.NoError(t, err)
	assert.Equal(t, existing.Id, user.Id)

	var updated model.User
	require.NoError(t, db.First(&updated, existing.Id).Error)
	assert.Equal(t, "manual@example.com", updated.Email)
}
