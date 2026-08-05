package model

import (
	"errors"
	"strconv"
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

func useRegistrationRewardSettings(t *testing.T, newUser int, invitee int, inviter int) {
	t.Helper()
	originalNewUser, originalInvitee, originalInviter := common.QuotaForNewUser, common.QuotaForInvitee, common.QuotaForInviter
	payment := operation_setting.GetPaymentSetting()
	originalCompliance, originalVersion := payment.ComplianceConfirmed, payment.ComplianceTermsVersion
	common.QuotaForNewUser, common.QuotaForInvitee, common.QuotaForInviter = newUser, invitee, inviter
	payment.ComplianceConfirmed = true
	payment.ComplianceTermsVersion = operation_setting.CurrentComplianceTermsVersion
	t.Cleanup(func() {
		common.QuotaForNewUser, common.QuotaForInvitee, common.QuotaForInviter = originalNewUser, originalInvitee, originalInviter
		payment.ComplianceConfirmed, payment.ComplianceTermsVersion = originalCompliance, originalVersion
	})
}

func TestRegistrationRewardsMatchCreditedBalances(t *testing.T) {
	setupUserUpdateTestState(t)
	useRegistrationRewardSettings(t, 100, 20, 30)

	inviter := User{Username: "registration-inviter", Status: common.UserStatusEnabled, AffCode: "registration-inviter-code"}
	require.NoError(t, DB.Create(&inviter).Error)
	invitee := User{Username: "registration-invitee", Status: common.UserStatusEnabled, InviterId: inviter.Id}
	require.NoError(t, invitee.Insert(inviter.Id))

	var storedInvitee User
	require.NoError(t, DB.First(&storedInvitee, invitee.Id).Error)
	assert.Equal(t, 120, storedInvitee.Quota)
	var storedInviter User
	require.NoError(t, DB.First(&storedInviter, inviter.Id).Error)
	assert.Equal(t, 1, storedInviter.AffCount)
	assert.Equal(t, 30, storedInviter.AffQuota)
	assert.Equal(t, 30, storedInviter.AffHistoryQuota)

	var inviteeLogs []Log
	require.NoError(t, LOG_DB.Where("user_id = ? AND type = ?", invitee.Id, LogTypeSystem).Order("id asc").Find(&inviteeLogs).Error)
	require.Len(t, inviteeLogs, 2)
	assert.Equal(t, 100, inviteeLogs[0].Quota)
	assert.Equal(t, 20, inviteeLogs[1].Quota)
	var inviterLogs []Log
	require.NoError(t, LOG_DB.Where("user_id = ? AND type = ?", inviter.Id, LogTypeSystem).Find(&inviterLogs).Error)
	require.Len(t, inviterLogs, 1)
	assert.Equal(t, 30, inviterLogs[0].Quota)
}

func TestRegistrationDoesNotLogFailedInviterReward(t *testing.T) {
	setupUserUpdateTestState(t)
	useRegistrationRewardSettings(t, 0, 0, 10)

	inviter := User{
		Username: "full-registration-inviter", Status: common.UserStatusEnabled, AffCode: "full-registration-inviter-code",
		AffQuota: common.MaxQuota,
	}
	require.NoError(t, DB.Create(&inviter).Error)
	invitee := User{Username: "failed-registration-reward", Status: common.UserStatusEnabled, InviterId: inviter.Id}
	require.NoError(t, invitee.Insert(inviter.Id))

	var storedInviter User
	require.NoError(t, DB.First(&storedInviter, inviter.Id).Error)
	assert.Zero(t, storedInviter.AffCount)
	assert.Equal(t, common.MaxQuota, storedInviter.AffQuota)
	var rewardLogs int64
	require.NoError(t, LOG_DB.Model(&Log{}).Where("user_id = ? AND type = ?", inviter.Id, LogTypeSystem).Count(&rewardLogs).Error)
	assert.Zero(t, rewardLogs)
}

func TestTransferAffQuotaSupportsBalanceAboveChargeLimit(t *testing.T) {
	if strconv.IntSize < 64 {
		t.Skip("large account balances require a 64-bit server")
	}
	setupUserUpdateTestState(t)
	originalQuotaPerUnit := common.QuotaPerUnit
	common.QuotaPerUnit = 1
	t.Cleanup(func() { common.QuotaPerUnit = originalQuotaPerUnit })

	user := User{Username: "transfer-limit-user", Status: common.UserStatusEnabled, Quota: common.MaxQuota, AffQuota: 10}
	require.NoError(t, DB.Create(&user).Error)
	require.NoError(t, user.TransferAffQuotaToQuota(10))

	var stored User
	require.NoError(t, DB.First(&stored, user.Id).Error)
	assert.EqualValues(t, int64(common.MaxQuota)+10, stored.Quota)
	assert.Zero(t, stored.AffQuota)
}

func TestTransferAffQuotaRejectsAccountBalanceOverflow(t *testing.T) {
	setupUserUpdateTestState(t)
	originalQuotaPerUnit := common.QuotaPerUnit
	common.QuotaPerUnit = 1
	t.Cleanup(func() { common.QuotaPerUnit = originalQuotaPerUnit })

	limit := int(userBalanceLimit())
	user := User{Username: "transfer-overflow-user", Status: common.UserStatusEnabled, Quota: limit, AffQuota: 10}
	require.NoError(t, DB.Create(&user).Error)
	require.ErrorIs(t, user.TransferAffQuotaToQuota(10), ErrAffQuotaTransferOutOfRange)

	var stored User
	require.NoError(t, DB.First(&stored, user.Id).Error)
	assert.Equal(t, limit, stored.Quota)
	assert.Equal(t, 10, stored.AffQuota)
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

func TestGmailDotVariantsShareEmailIdentity(t *testing.T) {
	setupUserUpdateTestState(t)

	require.NoError(t, DB.Create(&User{
		Username: "gmail-owner",
		Password: "old-password",
		Email:    "coderwar.021@gmail.com",
		Status:   common.UserStatusEnabled,
	}).Error)

	variants := []string{
		"coderwar.021@gmail.com",
		"coderwar021@gmail.com",
		"coderwar021+promo@gmail.com",
		"code.rwar021@gmail.com",
		"code.rwar021+promo@gmail.com",
		"code.rwar.021@GMAIL.COM",
		"code.rwar.021+promo@GMAIL.COM",
	}
	for _, email := range variants {
		assert.Equal(t, "coderwar021@gmail.com", NormalizeEmail(email), email)
		assert.ErrorIs(t, EnsureEmailAvailable(email, 0), ErrEmailAlreadyTaken, email)

		exists, err := CheckUserExistOrDeleted("gmail-probe", email)
		require.NoError(t, err)
		assert.True(t, exists, email)
	}

	user, err := GetUniqueUserByEmail("code.rwar.021@gmail.com")
	require.NoError(t, err)
	assert.Equal(t, "gmail-owner", user.Username)

	assert.Equal(t, "code.rwar021@example.com", NormalizeEmail("code.rwar021@example.com"))
	require.NoError(t, EnsureEmailAvailable("code.rwar021@example.com", 0))
	assert.Equal(t, "code.rwar021+promo@example.com", NormalizeEmail("code.rwar021+promo@example.com"))
	require.NoError(t, EnsureEmailAvailable("code.rwar021+promo@example.com", 0))
	assert.Equal(t, "codewar021@gmail.com", NormalizeEmail("codewar021@gmail.com"))
	require.NoError(t, EnsureEmailAvailable("codewar021@gmail.com", 0))
}

func TestGmailPlusTagLegacyRowsShareEmailIdentity(t *testing.T) {
	setupUserUpdateTestState(t)

	require.NoError(t, DB.Create(&User{
		Username: "legacy-gmail-plus",
		Password: "old-password",
		Email:    "code.rwar021+promo@gmail.com",
		Status:   common.UserStatusEnabled,
	}).Error)

	assert.ErrorIs(t, EnsureEmailAvailable("coderwar021@gmail.com", 0), ErrEmailAlreadyTaken)
	user, err := GetUniqueUserByEmail("code.rwar.021+other@gmail.com")
	require.NoError(t, err)
	assert.Equal(t, "legacy-gmail-plus", user.Username)
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
	assert.Len(t, stored.AffCode, 4)
}

func TestGetOrCreateAffCodeGeneratesOnceAndKeepsExistingCode(t *testing.T) {
	setupUserUpdateTestState(t)

	generatedUser := User{Username: "generated-aff-code", Status: common.UserStatusEnabled}
	existingUser := User{Username: "existing-aff-code", Status: common.UserStatusEnabled, AffCode: "KEEP"}
	require.NoError(t, DB.Create(&generatedUser).Error)
	require.NoError(t, DB.Create(&existingUser).Error)

	generated, err := GetOrCreateAffCode(generatedUser.Id)
	require.NoError(t, err)
	assert.Len(t, generated, 4)
	assert.Regexp(t, `^[A-Za-z0-9]{4}$`, generated)
	generatedAgain, err := GetOrCreateAffCode(generatedUser.Id)
	require.NoError(t, err)
	assert.Equal(t, generated, generatedAgain)

	existing, err := GetOrCreateAffCode(existingUser.Id)
	require.NoError(t, err)
	assert.Equal(t, "KEEP", existing)
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

func TestInsertPersistsInviterRelationship(t *testing.T) {
	setupUserUpdateTestState(t)

	passwordUser := &User{Username: "invited-password-user", Status: common.UserStatusEnabled}
	require.NoError(t, passwordUser.Insert(41))
	oauthUser := &User{Username: "invited-oauth-user", Status: common.UserStatusEnabled}
	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
		return oauthUser.InsertWithTx(tx, 42)
	}))

	var users []User
	require.NoError(t, DB.Where("id IN ?", []int{passwordUser.Id, oauthUser.Id}).Order("id asc").Find(&users).Error)
	require.Len(t, users, 2)
	assert.Equal(t, 41, users[0].InviterId)
	assert.Equal(t, 42, users[1].InviterId)
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

func TestBatchUpdateInviteRebateRatioByCurrentRatio(t *testing.T) {
	setupUserUpdateTestState(t)

	require.NoError(t, DB.Create(&[]User{
		{Id: 20, Username: "batch-zero", Status: common.UserStatusEnabled, AffCode: "batch-zero", InviteRebateRatio: 0},
		{Id: 21, Username: "batch-standard", Status: common.UserStatusEnabled, AffCode: "batch-standard", InviteRebateRatio: 100},
		{Id: 22, Username: "batch-custom-a", Status: common.UserStatusEnabled, AffCode: "batch-custom-a", InviteRebateRatio: 250},
		{Id: 23, Username: "batch-custom-b", Status: common.UserStatusEnabled, AffCode: "batch-custom-b", InviteRebateRatio: 250},
	}).Error)

	result, err := BatchUpdateInviteRebateRatio(250, 500, true)
	require.NoError(t, err)
	assert.Equal(t, int64(2), result.Matched)
	assert.Equal(t, int64(0), result.Updated)

	var custom User
	require.NoError(t, DB.First(&custom, 22).Error)
	assert.Equal(t, 250, custom.InviteRebateRatio)

	result, err = BatchUpdateInviteRebateRatio(250, 500, false)
	require.NoError(t, err)
	assert.Equal(t, int64(2), result.Matched)
	assert.Equal(t, int64(2), result.Updated)

	var users []User
	require.NoError(t, DB.Order("id asc").Find(&users).Error)
	assert.Equal(t, []int{0, 100, 500, 500}, []int{
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

func TestValidateAndFillMatchesGmailDotVariantEmail(t *testing.T) {
	setupUserUpdateTestState(t)

	hashedPassword, err := common.Password2Hash("NewPassword123")
	require.NoError(t, err)
	require.NoError(t, DB.Create(&User{
		Username: "gmail-login",
		Password: hashedPassword,
		Email:    "coderwar021@gmail.com",
		Status:   common.UserStatusEnabled,
	}).Error)

	loginUser := User{
		Username: "code.rwar.021+promo@gmail.com",
		Password: "NewPassword123",
	}
	require.NoError(t, loginUser.ValidateAndFill())
	assert.Equal(t, "gmail-login", loginUser.Username)
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
