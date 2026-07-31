package model

import (
	"errors"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupUserUpdateTestState(t *testing.T) {
	t.Helper()
	truncateTables(t)
	require.NoError(t, DB.Exec("DELETE FROM users").Error)

	oldRedisEnabled := common.RedisEnabled
	oldBatchUpdateEnabled := common.BatchUpdateEnabled
	common.RedisEnabled = false
	common.BatchUpdateEnabled = false
	t.Cleanup(func() {
		common.RedisEnabled = oldRedisEnabled
		common.BatchUpdateEnabled = oldBatchUpdateEnabled
	})
}

func TestUserUpdateDoesNotOverwriteAccountingFields(t *testing.T) {
	setupUserUpdateTestState(t)

	user := User{
		Id:           1,
		Username:     "quota-race-user",
		Password:     "password",
		DisplayName:  "before",
		Status:       common.UserStatusEnabled,
		Quota:        1000,
		UsedQuota:    20,
		RequestCount: 3,
	}
	require.NoError(t, DB.Create(&user).Error)

	staleUser, err := GetUserById(user.Id, true)
	require.NoError(t, err)

	require.NoError(t, DB.Model(&User{}).Where("id = ?", user.Id).Updates(map[string]interface{}{
		"quota":         gorm.Expr("quota - ?", 400),
		"used_quota":    gorm.Expr("used_quota + ?", 400),
		"request_count": gorm.Expr("request_count + ?", 1),
	}).Error)

	staleUser.DisplayName = "after"
	require.NoError(t, staleUser.Update(false))

	var got User
	require.NoError(t, DB.First(&got, user.Id).Error)
	assert.Equal(t, "after", got.DisplayName)
	assert.Equal(t, 600, got.Quota)
	assert.Equal(t, 420, got.UsedQuota)
	assert.Equal(t, 4, got.RequestCount)
}

func TestUpdateUserSettingOnlyUpdatesSetting(t *testing.T) {
	setupUserUpdateTestState(t)

	user := User{
		Id:           2,
		Username:     "setting-user",
		Password:     "password",
		Status:       common.UserStatusEnabled,
		Quota:        1000,
		UsedQuota:    20,
		RequestCount: 3,
	}
	require.NoError(t, DB.Create(&user).Error)

	require.NoError(t, DB.Model(&User{}).Where("id = ?", user.Id).Updates(map[string]interface{}{
		"quota":         gorm.Expr("quota - ?", 250),
		"used_quota":    gorm.Expr("used_quota + ?", 250),
		"request_count": gorm.Expr("request_count + ?", 1),
	}).Error)

	require.NoError(t, UpdateUserSetting(user.Id, dto.UserSetting{Language: "zh"}))

	var got User
	require.NoError(t, DB.First(&got, user.Id).Error)
	assert.Equal(t, 750, got.Quota)
	assert.Equal(t, 270, got.UsedQuota)
	assert.Equal(t, 4, got.RequestCount)
	assert.Equal(t, "zh", got.GetSetting().Language)
}

func TestEnsureEmailAvailableRejectsExistingEmailCaseInsensitive(t *testing.T) {
	setupUserUpdateTestState(t)

	require.NoError(t, DB.Create(&User{
		Username: "existing",
		Password: "old-password",
		Email:    "Taken@Example.com",
		Status:   common.UserStatusEnabled,
	}).Error)

	err := EnsureEmailAvailable(" taken@example.COM ", 0)
	require.ErrorIs(t, err, ErrEmailAlreadyTaken)

	user, err := GetUniqueUserByEmail("TAKEN@example.com")
	require.NoError(t, err)
	assert.Equal(t, "existing", user.Username)

	require.NoError(t, EnsureEmailAvailable("taken@example.com", user.Id))
}

func TestInsertRejectsDuplicateEmailWithoutUniqueIndex(t *testing.T) {
	setupUserUpdateTestState(t)

	require.NoError(t, DB.Create(&User{
		Username: "existing",
		Password: "old-password",
		Email:    "taken@example.com",
		Status:   common.UserStatusEnabled,
	}).Error)

	user := &User{
		Username: "oauth-user",
		Email:    "TAKEN@example.com",
		Role:     common.RoleCommonUser,
		Status:   common.UserStatusEnabled,
	}

	err := user.Insert(0)
	require.ErrorIs(t, err, ErrEmailAlreadyTaken)

	var count int64
	require.NoError(t, DB.Model(&User{}).Where("username = ?", "oauth-user").Count(&count).Error)
	assert.Zero(t, count)
}

func TestInsertKeepsBlankPasswordForPasswordlessUser(t *testing.T) {
	setupUserUpdateTestState(t)

	user := &User{
		Username: "passwordless-user",
		Role:     common.RoleCommonUser,
		Status:   common.UserStatusEnabled,
	}

	require.NoError(t, user.Insert(0))

	var stored User
	require.NoError(t, DB.Where("username = ?", user.Username).First(&stored).Error)
	assert.Empty(t, stored.Password)
}

func TestInsertUsesDefaultInviteRebateRatio(t *testing.T) {
	setupUserUpdateTestState(t)

	quotaSetting := operation_setting.GetQuotaSetting()
	originalRatio := quotaSetting.DefaultInviteRebateRatio
	quotaSetting.DefaultInviteRebateRatio = 125
	t.Cleanup(func() {
		quotaSetting.DefaultInviteRebateRatio = originalRatio
	})

	user := &User{
		Username: "default-rebate-user",
		Role:     common.RoleCommonUser,
		Status:   common.UserStatusEnabled,
	}
	require.NoError(t, user.Insert(0))

	oauthUser := &User{
		Username: "default-rebate-oauth-user",
		Role:     common.RoleCommonUser,
		Status:   common.UserStatusEnabled,
	}
	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
		return oauthUser.InsertWithTx(tx, 0)
	}))

	var users []User
	require.NoError(t, DB.Where("username IN ?", []string{user.Username, oauthUser.Username}).Find(&users).Error)
	require.Len(t, users, 2)
	for _, stored := range users {
		assert.Equal(t, 125, stored.InviteRebateRatio)
	}
}

func TestUpdateZeroInviteRebateRatioOnlyTouchesZeroUsers(t *testing.T) {
	setupUserUpdateTestState(t)

	require.NoError(t, DB.Create(&[]User{
		{Id: 10, Username: "zero-a", Status: common.UserStatusEnabled, AffCode: "zero-a", InviteRebateRatio: 0},
		{Id: 11, Username: "custom", Status: common.UserStatusEnabled, AffCode: "custom", InviteRebateRatio: 250},
		{Id: 12, Username: "zero-b", Status: common.UserStatusEnabled, AffCode: "zero-b", InviteRebateRatio: 0},
	}).Error)

	updated, err := UpdateZeroInviteRebateRatio(100)
	require.NoError(t, err)
	assert.Equal(t, int64(2), updated)

	var users []User
	require.NoError(t, DB.Order("id asc").Find(&users).Error)
	assert.Equal(t, 100, users[0].InviteRebateRatio)
	assert.Equal(t, 250, users[1].InviteRebateRatio)
	assert.Equal(t, 100, users[2].InviteRebateRatio)
}

func TestBatchUpdateInviteRebateRatioScopes(t *testing.T) {
	setupUserUpdateTestState(t)

	quotaSetting := operation_setting.GetQuotaSetting()
	originalRatio := quotaSetting.DefaultInviteRebateRatio
	quotaSetting.DefaultInviteRebateRatio = 100
	t.Cleanup(func() {
		quotaSetting.DefaultInviteRebateRatio = originalRatio
	})

	require.NoError(t, DB.Create(&[]User{
		{Id: 20, Username: "batch-zero", Status: common.UserStatusEnabled, AffCode: "batch-zero", InviteRebateRatio: 0},
		{Id: 21, Username: "batch-standard", Status: common.UserStatusEnabled, AffCode: "batch-standard", InviteRebateRatio: 100},
		{Id: 22, Username: "batch-custom-a", Status: common.UserStatusEnabled, AffCode: "batch-custom-a", InviteRebateRatio: 250},
		{Id: 23, Username: "batch-custom-b", Status: common.UserStatusEnabled, AffCode: "batch-custom-b", InviteRebateRatio: 300},
	}).Error)

	result, err := BatchUpdateInviteRebateRatio(InviteRebateBatchScopeNonStandard, nil, 500, true)
	require.NoError(t, err)
	assert.Equal(t, int64(2), result.Matched)
	assert.Equal(t, int64(0), result.Updated)

	var custom User
	require.NoError(t, DB.First(&custom, 22).Error)
	assert.Equal(t, 250, custom.InviteRebateRatio)

	result, err = BatchUpdateInviteRebateRatio(InviteRebateBatchScopeNonStandard, nil, 500, false)
	require.NoError(t, err)
	assert.Equal(t, int64(2), result.Matched)
	assert.Equal(t, int64(2), result.Updated)

	currentRatio := 500
	result, err = BatchUpdateInviteRebateRatio(InviteRebateBatchScopeCurrentRatio, &currentRatio, 250, false)
	require.NoError(t, err)
	assert.Equal(t, int64(2), result.Matched)
	assert.Equal(t, int64(2), result.Updated)

	result, err = BatchUpdateInviteRebateRatio(InviteRebateBatchScopeStandard, nil, 150, false)
	require.NoError(t, err)
	assert.Equal(t, int64(1), result.Matched)
	assert.Equal(t, int64(1), result.Updated)

	var users []User
	require.NoError(t, DB.Order("id asc").Find(&users).Error)
	assert.Equal(t, []int{0, 150, 250, 250}, []int{
		users[0].InviteRebateRatio,
		users[1].InviteRebateRatio,
		users[2].InviteRebateRatio,
		users[3].InviteRebateRatio,
	})
}

func TestListInviteRebateRatioSummaries(t *testing.T) {
	setupUserUpdateTestState(t)

	require.NoError(t, DB.Create(&[]User{
		{Id: 20, Username: "ratio-zero", Status: common.UserStatusEnabled, AffCode: "ratio-zero", InviteRebateRatio: 0},
		{Id: 21, Username: "ratio-one-a", Status: common.UserStatusEnabled, AffCode: "ratio-one-a", InviteRebateRatio: 100},
		{Id: 22, Username: "ratio-one-b", Status: common.UserStatusEnabled, AffCode: "ratio-one-b", InviteRebateRatio: 100},
		{Id: 23, Username: "ratio-two", Status: common.UserStatusEnabled, AffCode: "ratio-two", InviteRebateRatio: 250},
	}).Error)

	summaries, err := ListInviteRebateRatioSummaries()
	require.NoError(t, err)
	assert.Equal(t, []InviteRebateRatioSummary{
		{Ratio: 0, Count: 1},
		{Ratio: 100, Count: 2},
		{Ratio: 250, Count: 1},
	}, summaries)
}

func TestValidateAndFillRejectsPasswordlessUser(t *testing.T) {
	setupUserUpdateTestState(t)

	require.NoError(t, DB.Create(&User{
		Username: "passwordless-user",
		Password: "",
		Status:   common.UserStatusEnabled,
	}).Error)

	loginUser := User{
		Username: "passwordless-user",
		Password: "NewPassword123",
	}
	err := loginUser.ValidateAndFill()
	require.ErrorIs(t, err, ErrInvalidCredentials)

	var stored User
	require.NoError(t, DB.Where("username = ?", "passwordless-user").First(&stored).Error)
	assert.Empty(t, stored.Password)
}

func TestResetUserPasswordByEmailRequiresSingleActiveMatch(t *testing.T) {
	setupUserUpdateTestState(t)

	require.NoError(t, DB.Create(&User{
		Username: "duplicate-1",
		Password: "old-1",
		Email:    "legacy@example.com",
		AffCode:  "dupe1",
		Status:   common.UserStatusEnabled,
	}).Error)
	require.NoError(t, DB.Create(&User{
		Username: "duplicate-2",
		Password: "old-2",
		Email:    "LEGACY@example.com",
		AffCode:  "dupe2",
		Status:   common.UserStatusEnabled,
	}).Error)

	err := ResetUserPasswordByEmail("legacy@example.com", "NewPassword123")
	require.ErrorIs(t, err, ErrEmailAmbiguous)

	var duplicates []User
	require.NoError(t, DB.Where("LOWER(email) = ?", "legacy@example.com").Order("username asc").Find(&duplicates).Error)
	require.Len(t, duplicates, 2)
	assert.Equal(t, "old-1", duplicates[0].Password)
	assert.Equal(t, "old-2", duplicates[1].Password)

	require.NoError(t, DB.Create(&User{
		Username: "unique",
		Password: "old",
		Email:    "unique@example.com",
		AffCode:  "unique",
		Status:   common.UserStatusEnabled,
	}).Error)

	require.NoError(t, ResetUserPasswordByEmail("UNIQUE@example.com", "NewPassword123"))

	var unique User
	require.NoError(t, DB.Where("username = ?", "unique").First(&unique).Error)
	assert.True(t, common.ValidatePasswordAndHash("NewPassword123", unique.Password))

	err = ResetUserPasswordByEmail("missing@example.com", "NewPassword123")
	require.True(t, errors.Is(err, ErrEmailNotFound))
}
