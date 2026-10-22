package model

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm/schema"
)

func useTopupRewardSettingsForTest(t *testing.T, _ int, discount map[int]float64, bonus map[int]float64) {
	t.Helper()
	originalDiscount := operation_setting.GetPaymentSetting().AmountDiscount
	originalBonus := operation_setting.GetPaymentSetting().AmountBonus
	operation_setting.GetPaymentSetting().AmountDiscount = discount
	operation_setting.GetPaymentSetting().AmountBonus = bonus
	t.Cleanup(func() {
		operation_setting.GetPaymentSetting().AmountDiscount = originalDiscount
		operation_setting.GetPaymentSetting().AmountBonus = originalBonus
	})
}

func insertInviterPairForTopupTest(t *testing.T, inviterID int, inviteeID int, rebateRatio int) {
	t.Helper()
	require.NoError(t, DB.Create(&User{
		Id:                inviterID,
		Username:          fmt.Sprintf("inviter_%d", inviterID),
		AffCode:           fmt.Sprintf("inviter-aff-%d", inviterID),
		Status:            common.UserStatusEnabled,
		AffQuota:          10,
		AffHistoryQuota:   20,
		InviteRebateRatio: rebateRatio,
	}).Error)
	require.NoError(t, DB.Create(&User{
		Id:        inviteeID,
		Username:  fmt.Sprintf("invitee_%d", inviteeID),
		AffCode:   fmt.Sprintf("invitee-aff-%d", inviteeID),
		Status:    common.UserStatusEnabled,
		Quota:     5,
		InviterId: inviterID,
	}).Error)
}

func TestEveryRealTopupPathRebatesInviterExactlyOnce(t *testing.T) {
	useQuotaPerUnitForTopUpTest(t, 100)
	useTopupRewardSettingsForTest(t, 40, map[int]float64{}, map[int]float64{})

	testCases := []struct {
		name     string
		provider string
		settle   func(string) error
	}{
		{name: "epay", provider: PaymentProviderEpay, settle: func(tradeNo string) error {
			return RechargeEpay(tradeNo, "alipay", "gateway", "203.0.113.1")
		}},
		{name: "stripe", provider: PaymentProviderStripe, settle: func(tradeNo string) error {
			return RechargeStripeWithPaymentDetails(tradeNo, "", "gateway", "USD", "203.0.113.1")
		}},
		{name: "creem", provider: PaymentProviderCreem, settle: func(tradeNo string) error {
			return RechargeCreemWithPaymentDetails(tradeNo, "", "", "gateway", "USD", "203.0.113.1")
		}},
		{name: "lantu", provider: PaymentProviderLanTu, settle: func(tradeNo string) error {
			return RechargeLanTuWithPaymentDetails(tradeNo, "gateway", "CNY", "203.0.113.1")
		}},
		{name: "waffo", provider: PaymentProviderWaffo, settle: func(tradeNo string) error {
			return RechargeWaffoWithPaymentDetails(tradeNo, "gateway", "USD", "203.0.113.1")
		}},
		{name: "waffo pancake", provider: PaymentProviderWaffoPancake, settle: func(tradeNo string) error {
			return RechargeWaffoPancakeWithPaymentDetails(tradeNo, "gateway", "USD", "", "203.0.113.1")
		}},
		{name: "manual completion", provider: PaymentProviderEpay, settle: func(tradeNo string) error {
			return ManualCompleteTopUp(tradeNo, "203.0.113.1")
		}},
		{name: "nowpayments", provider: PaymentProviderNowPayments, settle: func(tradeNo string) error {
			return RechargeNowPaymentsWithPaymentDetails(tradeNo, "gateway", "USD", "203.0.113.1")
		}},
	}

	for index, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			truncateTables(t)
			inviterID := 700 + index*2
			inviteeID := inviterID + 1
			insertInviterPairForTopupTest(t, inviterID, inviteeID, MaxInviteRebateRatio)
			tradeNo := fmt.Sprintf("reward-%d", index)
			require.NoError(t, DB.Model(&User{}).Where("id = ?", inviteeID).Update("exclusive_bonus_ratio", 2000).Error)
			topUp := &TopUp{UserId: inviteeID, Amount: 100, Money: 1, TradeNo: tradeNo, PaymentCurrency: "USD", PaymentMethod: testCase.provider, PaymentProvider: testCase.provider, Status: common.TopUpStatusPending}
			if testCase.provider == PaymentProviderCreem {
				topUp.Amount = 10000
			}
			require.NoError(t, topUp.Insert())

			require.NoError(t, testCase.settle(tradeNo))
			require.NoError(t, testCase.settle(tradeNo))

			topUp = GetTopUpByTradeNo(tradeNo)
			require.NotNil(t, topUp)
			assert.Equal(t, 12000, topUp.CreditedQuota)
			assert.Equal(t, 2000, topUp.ExclusiveBonusQuota)
			assert.Equal(t, 12005, getUserQuotaForPaymentGuardTest(t, inviteeID))
			expectedRebate := 10000

			var inviter User
			require.NoError(t, DB.Select("aff_quota", "aff_history").First(&inviter, inviterID).Error)
			assert.Equal(t, 10+expectedRebate, inviter.AffQuota)
			assert.Equal(t, 20+expectedRebate, inviter.AffHistoryQuota)

			assert.Equal(t, inviterID, topUp.InviteRebateInviterId)
			assert.Equal(t, MaxInviteRebateRatio, topUp.InviteRebateRatio)
			assert.Equal(t, expectedRebate, topUp.InviteRebateQuota)

			var rewardLog Log
			require.NoError(t, LOG_DB.Where("user_id = ? AND type = ?", inviterID, LogTypeSystem).First(&rewardLog).Error)
			maskedInvitee := maskedIdentifier(fmt.Sprintf("invitee_%d", inviteeID))
			assert.Contains(t, rewardLog.Content, maskedInvitee)
			assert.NotContains(t, rewardLog.Content, tradeNo)
			assert.NotContains(t, rewardLog.Content, fmt.Sprintf("invitee_%d", inviteeID))
			assert.NotContains(t, rewardLog.Content, fmt.Sprintf("#%d", inviteeID))
			var rewardLogs int64
			require.NoError(t, LOG_DB.Model(&Log{}).Where("user_id = ? AND type = ?", inviterID, LogTypeSystem).Count(&rewardLogs).Error)
			assert.EqualValues(t, 1, rewardLogs)
		})
	}
}

func TestExclusiveTopupBonusAddsToPublicAndKeepsOrderSnapshot(t *testing.T) {
	truncateTables(t)
	useQuotaPerUnitForTopUpTest(t, 100)
	useTopupRewardSettingsForTest(t, 0, map[int]float64{}, map[int]float64{100: 0.1})
	insertInviterPairForTopupTest(t, 1200, 1201, 1000)
	user, err := GetUserById(1201, true)
	require.NoError(t, err)
	user.ExclusiveBonusRatio = 2000
	require.NoError(t, user.Edit(false))

	order := &TopUp{UserId: 1201, Amount: 100, Money: 100, TradeNo: "exclusive-snapshot", PaymentMethod: PaymentMethodWaffo, PaymentProvider: PaymentProviderWaffo, Status: common.TopUpStatusPending}
	require.NoError(t, order.Insert())
	assert.Equal(t, 13000, order.PromisedQuota)
	assert.Equal(t, 2000, order.ExclusiveBonusQuota)
	assert.Equal(t, 0.1, order.PublicBonusRate)

	user.ExclusiveBonusRatio = 0
	require.NoError(t, user.Edit(false))
	operation_setting.GetPaymentSetting().AmountBonus = map[int]float64{100: 0.5}
	require.NoError(t, ManualCompleteTopUp(order.TradeNo, "127.0.0.1"))
	require.NoError(t, RechargeWaffo(order.TradeNo, "127.0.0.1"))
	stored := GetTopUpByTradeNo(order.TradeNo)
	require.NotNil(t, stored)
	assert.Equal(t, 13000, stored.CreditedQuota)
	assert.Equal(t, 1100, stored.InviteRebateQuota)
	assert.Equal(t, 2000, stored.ExclusiveBonusRatio)
	assert.Equal(t, 13005, getUserQuotaForPaymentGuardTest(t, 1201))
}

func TestExclusiveTopupBonusRoundingLimitsAndLegacyOrders(t *testing.T) {
	truncateTables(t)
	useQuotaPerUnitForTopUpTest(t, 1)
	useTopupRewardSettingsForTest(t, 0, map[int]float64{}, map[int]float64{1: 0.1})
	insertInviterPairForTopupTest(t, 1210, 1211, MaxInviteRebateRatio)
	require.NoError(t, DB.Model(&User{}).Where("id = ?", 1211).Update("exclusive_bonus_ratio", 1250).Error)
	order := &TopUp{UserId: 1211, Amount: 9, Money: 9, TradeNo: "exclusive-rounding", PaymentMethod: PaymentMethodWaffo, PaymentProvider: PaymentProviderWaffo, Status: common.TopUpStatusPending}
	require.NoError(t, order.Insert())
	assert.Equal(t, 11, order.PromisedQuota)
	assert.Equal(t, 1, order.ExclusiveBonusQuota)
	require.NoError(t, RechargeWaffo(order.TradeNo, "127.0.0.1"))
	assert.Equal(t, 10, GetTopUpByTradeNo(order.TradeNo).InviteRebateQuota)

	insertTopUpForSettlementTest(t, "legacy-exclusive-not-retroactive", 1211, 10, 10, PaymentProviderWaffo)
	require.NoError(t, RechargeWaffo("legacy-exclusive-not-retroactive", "127.0.0.1"))
	legacy := GetTopUpByTradeNo("legacy-exclusive-not-retroactive")
	require.NotNil(t, legacy)
	assert.Equal(t, 11, legacy.CreditedQuota)
	assert.Zero(t, legacy.ExclusiveBonusQuota)

	for _, ratio := range []int{-1, MaxExclusiveBonusRatio + 1} {
		require.NoError(t, DB.Model(&User{}).Where("id = ?", 1211).Update("exclusive_bonus_ratio", ratio).Error)
		assert.ErrorIs(t, order.PrepareQuota(), ErrTopUpQuotaInvalid)
	}
	require.NoError(t, DB.Model(&User{}).Where("id = ?", 1211).Update("exclusive_bonus_ratio", MaxExclusiveBonusRatio).Error)
	order.Amount = common.MaxWalletQuota / 2
	assert.ErrorIs(t, order.PrepareQuota(), ErrTopUpQuotaInvalid)
	order.Amount = 100
	require.NoError(t, DB.Model(&User{}).Where("id = ?", 1211).Update("quota", common.MaxWalletQuota-200).Error)
	order.PromisedQuota = 0
	order.TradeNo = "exclusive-wallet-overflow"
	assert.ErrorIs(t, order.Insert(), ErrTopUpQuotaLimitExceeded)

	operation_setting.GetPaymentSetting().AmountBonus = map[int]float64{}
	require.NoError(t, DB.Model(&User{}).Where("id = ?", 1211).Update("quota", 5).Error)
	order.Amount = int64(common.MaxWalletQuota/2 - 100)
	order.Id = 0
	order.TradeNo = "exclusive-large-referral"
	order.PromisedQuota = 0
	require.NoError(t, order.Insert())
	require.NoError(t, RechargeWaffo(order.TradeNo, "127.0.0.1"))
	assert.Equal(t, int(order.Amount), GetTopUpByTradeNo(order.TradeNo).InviteRebateQuota)
}

func TestExclusiveBonusDatabaseMatrix(t *testing.T) {
	useQuotaPerUnitForTopUpTest(t, 100)
	useTopupRewardSettingsForTest(t, 0, map[int]float64{}, map[int]float64{100: 0.1})
	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			dsn := os.Getenv(map[string]string{"mysql": "TEST_MYSQL_DSN", "postgres": "TEST_POSTGRES_DSN"}[dialect])
			if dialect == "sqlite" {
				dsn = "local"
				previousPath := common.SQLitePath
				common.SQLitePath = filepath.Join(t.TempDir(), "exclusive.db")
				t.Cleanup(func() { common.SQLitePath = previousPath })
			}
			if dsn == "" {
				t.Skip("test database DSN is not configured")
			}
			t.Setenv("EXCLUSIVE_BONUS_TEST_DSN", dsn)
			db, databaseType, err := chooseDB("EXCLUSIVE_BONUS_TEST_DSN", false)
			require.NoError(t, err)
			versionQuery := "SELECT VERSION()"
			if dialect == "sqlite" {
				versionQuery = "SELECT sqlite_version()"
			}
			var version string
			require.NoError(t, db.Raw(versionQuery).Scan(&version).Error)
			t.Logf("%s version: %s", dialect, version)
			// Isolate the matrix from any application's existing tables.
			db.Config.NamingStrategy = schema.NamingStrategy{TablePrefix: "exclusive_bonus_test_"}
			sqlDB, err := db.DB()
			require.NoError(t, err)
			previousDB, previousLogDB := DB, LOG_DB
			previousMainType, previousLogType := common.MainDatabaseType(), common.LogDatabaseType()
			DB, LOG_DB = db, db
			common.SetDatabaseTypes(databaseType, databaseType)
			initCol()
			t.Cleanup(func() {
				DB, LOG_DB = previousDB, previousLogDB
				common.SetDatabaseTypes(previousMainType, previousLogType)
				initCol()
				require.NoError(t, sqlDB.Close())
			})
			for _, upgrade := range []bool{false, true} {
				t.Run(fmt.Sprintf("upgrade_%t", upgrade), func(t *testing.T) {
					require.NoError(t, db.Migrator().DropTable(&TopUp{}, &User{}, &Log{}))
					t.Cleanup(func() { _ = db.Migrator().DropTable(&TopUp{}, &User{}, &Log{}) })
					require.NoError(t, db.AutoMigrate(&User{}, &TopUp{}, &Log{}))
					if upgrade {
						for _, column := range []string{"PublicBonusRate", "ExclusiveBonusRatio", "ExclusiveBonusQuota"} {
							require.NoError(t, db.Migrator().DropColumn(&TopUp{}, column))
						}
						require.NoError(t, db.Migrator().DropColumn(&User{}, "ExclusiveBonusRatio"))
						require.NoError(t, db.Table("exclusive_bonus_test_users").Create(map[string]interface{}{"id": 1, "username": "legacy", "password": "legacy-password-hash", "aff_code": "legacy", "quota": 25}).Error)
						require.NoError(t, db.Table("exclusive_bonus_test_top_ups").Create(map[string]interface{}{"user_id": 1, "trade_no": "legacy", "amount": 100, "promised_quota": 11000, "status": common.TopUpStatusPending}).Error)
					}
					require.NoError(t, db.AutoMigrate(&User{}, &TopUp{}, &Log{}))
					require.NoError(t, db.AutoMigrate(&User{}, &TopUp{}, &Log{}))
					if upgrade {
						legacy, err := GetUserById(1, false)
						require.NoError(t, err)
						assert.Equal(t, 25, legacy.Quota)
						assert.Zero(t, legacy.ExclusiveBonusRatio)
						assert.Equal(t, 11000, GetTopUpByTradeNo("legacy").PromisedQuota)
						assert.Error(t, db.Create(&TopUp{TradeNo: "legacy"}).Error)
						assert.Error(t, db.Create(&User{Username: "duplicate", AffCode: "legacy"}).Error)
					}
					insertInviterPairForTopupTest(t, 20, 21, 1000)
					require.NoError(t, db.Model(&User{}).Where("id = ?", 21).Update("exclusive_bonus_ratio", 2000).Error)
					order := &TopUp{UserId: 21, Amount: 100, Money: 100, TradeNo: "matrix", PaymentMethod: PaymentMethodWaffo, PaymentProvider: PaymentProviderWaffo, Status: common.TopUpStatusPending}
					require.NoError(t, order.Insert())
					require.NoError(t, RechargeWaffo(order.TradeNo, "127.0.0.1"))
					require.NoError(t, RechargeWaffo(order.TradeNo, "127.0.0.1"))
					assert.Equal(t, 13005, getUserQuotaForPaymentGuardTest(t, 21))
					assert.Equal(t, 1100, GetTopUpByTradeNo(order.TradeNo).InviteRebateQuota)
				})
			}
		})
	}
}

func TestInviteRebateAppliesToEveryPaidTopup(t *testing.T) {
	truncateTables(t)
	useQuotaPerUnitForTopUpTest(t, 100)
	useTopupRewardSettingsForTest(t, 0, map[int]float64{}, map[int]float64{})
	insertInviterPairForTopupTest(t, 760, 761, 100)

	for _, tradeNo := range []string{"rebate-every-topup-1", "rebate-every-topup-2"} {
		insertTopUpForSettlementTest(t, tradeNo, 761, 1, 1, PaymentProviderWaffo)
		require.NoError(t, RechargeWaffo(tradeNo, "203.0.113.7"))
	}

	var inviter User
	require.NoError(t, DB.Select("aff_quota", "aff_history").First(&inviter, 760).Error)
	assert.Equal(t, 12, inviter.AffQuota)
	assert.Equal(t, 22, inviter.AffHistoryQuota)
}

func TestInvalidInviterNeverBlocksPaidTopup(t *testing.T) {
	useQuotaPerUnitForTopUpTest(t, 100)
	useTopupRewardSettingsForTest(t, 40, map[int]float64{}, map[int]float64{})

	testCases := []struct {
		name      string
		inviterID int
		prepare   func(t *testing.T, inviterID int)
	}{
		{name: "missing", inviterID: 900, prepare: func(t *testing.T, inviterID int) {}},
		{name: "overflow", inviterID: 901, prepare: func(t *testing.T, inviterID int) {
			require.NoError(t, DB.Create(&User{
				Id:                inviterID,
				Username:          "overflow_inviter",
				AffCode:           "overflow-inviter-aff",
				Status:            common.UserStatusEnabled,
				AffQuota:          common.MaxWalletQuota - 10,
				AffHistoryQuota:   20,
				InviteRebateRatio: MaxInviteRebateRatio,
			}).Error)
		}},
	}

	for index, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			truncateTables(t)
			testCase.prepare(t, testCase.inviterID)
			inviteeID := 910 + index
			require.NoError(t, DB.Create(&User{
				Id:        inviteeID,
				Username:  fmt.Sprintf("invalid_invitee_%d", index),
				AffCode:   fmt.Sprintf("invalid-invitee-aff-%d", index),
				Status:    common.UserStatusEnabled,
				Quota:     5,
				InviterId: testCase.inviterID,
			}).Error)
			tradeNo := fmt.Sprintf("invalid-inviter-%d", index)
			insertTopUpForSettlementTest(t, tradeNo, inviteeID, 1, 1, PaymentProviderWaffo)

			require.NoError(t, RechargeWaffo(tradeNo, "203.0.113.2"))
			assert.Equal(t, 105, getUserQuotaForPaymentGuardTest(t, inviteeID))

			topUp := GetTopUpByTradeNo(tradeNo)
			require.NotNil(t, topUp)
			assert.Equal(t, testCase.inviterID, topUp.InviteRebateInviterId)
			assert.Zero(t, topUp.InviteRebateQuota)
			if testCase.name == "overflow" {
				var inviter User
				require.NoError(t, DB.Select("aff_quota", "aff_history").First(&inviter, testCase.inviterID).Error)
				assert.Equal(t, common.MaxWalletQuota-10, inviter.AffQuota)
				assert.Equal(t, 20, inviter.AffHistoryQuota)
			}
		})
	}
}

func TestInviterCacheIsInvalidatedAfterReward(t *testing.T) {
	truncateTables(t)
	server := useUserCacheMiniRedis(t)
	useQuotaPerUnitForTopUpTest(t, 100)
	useTopupRewardSettingsForTest(t, 40, map[int]float64{}, map[int]float64{})
	insertInviterPairForTopupTest(t, 940, 941, MaxInviteRebateRatio)

	var inviter User
	require.NoError(t, DB.First(&inviter, 940).Error)
	require.NoError(t, populateUserCache(inviter))
	assert.True(t, server.Exists(getUserCacheKey(inviter.Id)))

	insertTopUpForSettlementTest(t, "reward-cache-invalidation", 941, 1, 1, PaymentProviderWaffo)
	require.NoError(t, RechargeWaffo("reward-cache-invalidation", "203.0.113.6"))
	assert.False(t, server.Exists(getUserCacheKey(inviter.Id)))
}

func TestTopupBonusIsStoredAsActualCreditedQuota(t *testing.T) {
	useQuotaPerUnitForTopUpTest(t, 100)

	testCases := []struct {
		name     string
		discount map[int]float64
		bonus    map[int]float64
		expected int
	}{
		{name: "explicit bonus", discount: map[int]float64{}, bonus: map[int]float64{10: 0.2}, expected: 1200},
		{name: "legacy bonus", discount: map[int]float64{10: 0.1}, bonus: map[int]float64{}, expected: 1100},
	}

	for index, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			truncateTables(t)
			useTopupRewardSettingsForTest(t, 0, testCase.discount, testCase.bonus)
			userID := 950 + index
			insertUserForPaymentGuardTest(t, userID, 5)
			tradeNo := fmt.Sprintf("bonus-%d", index)
			insertTopUpForSettlementTest(t, tradeNo, userID, 10, 10, PaymentProviderWaffo)

			require.NoError(t, RechargeWaffo(tradeNo, "203.0.113.3"))
			topUp := GetTopUpByTradeNo(tradeNo)
			require.NotNil(t, topUp)
			assert.Equal(t, testCase.expected, topUp.CreditedQuota)
			assert.Equal(t, testCase.expected+5, getUserQuotaForPaymentGuardTest(t, userID))
		})
	}
}

func TestPendingTopupKeepsOrderTimePromisedQuota(t *testing.T) {
	truncateTables(t)
	useQuotaPerUnitForTopUpTest(t, 100)
	useTopupRewardSettingsForTest(t, 0, map[int]float64{}, map[int]float64{10: 0.2})
	insertUserForPaymentGuardTest(t, 970, 5)
	topUp := &TopUp{
		UserId:          970,
		Amount:          10,
		Money:           10,
		TradeNo:         "promised-quota-snapshot",
		PaymentMethod:   PaymentMethodWaffo,
		PaymentProvider: PaymentProviderWaffo,
		Status:          common.TopUpStatusPending,
		CreateTime:      time.Now().Unix(),
	}
	require.NoError(t, topUp.Insert())
	assert.Equal(t, 1200, topUp.PromisedQuota)

	operation_setting.GetPaymentSetting().AmountBonus = map[int]float64{10: 0.5}
	require.NoError(t, RechargeWaffo(topUp.TradeNo, "203.0.113.4"))

	stored := GetTopUpByTradeNo(topUp.TradeNo)
	require.NotNil(t, stored)
	assert.Equal(t, 1200, stored.PromisedQuota)
	assert.Equal(t, 1200, stored.CreditedQuota)
	assert.Equal(t, 1205, getUserQuotaForPaymentGuardTest(t, 970))
}

func TestPendingTopupUsesTokenDisplayAmountForBonusTier(t *testing.T) {
	truncateTables(t)
	insertUserForPaymentGuardTest(t, 970, 5)
	useQuotaPerUnitForTopUpTest(t, 100)
	useTopupRewardSettingsForTest(t, 0, map[int]float64{}, map[int]float64{1000: 0.2})
	originalDisplayType := operation_setting.GetGeneralSetting().QuotaDisplayType
	operation_setting.GetGeneralSetting().QuotaDisplayType = operation_setting.QuotaDisplayTypeTokens
	t.Cleanup(func() {
		operation_setting.GetGeneralSetting().QuotaDisplayType = originalDisplayType
	})

	topUp := &TopUp{
		UserId:          970,
		Amount:          10,
		Money:           10,
		TradeNo:         "token-display-bonus-tier",
		PaymentMethod:   PaymentMethodWaffo,
		PaymentProvider: PaymentProviderWaffo,
		Status:          common.TopUpStatusPending,
	}
	require.NoError(t, topUp.Insert())
	assert.Equal(t, 1200, topUp.PromisedQuota)
}

func TestCreemBonusTierUsesStandardRechargeAmount(t *testing.T) {
	truncateTables(t)
	useQuotaPerUnitForTopUpTest(t, 100)
	useTopupRewardSettingsForTest(t, 0, map[int]float64{}, map[int]float64{1: 0.1, 100: 0.5})
	originalDisplayType := operation_setting.GetGeneralSetting().QuotaDisplayType
	operation_setting.GetGeneralSetting().QuotaDisplayType = operation_setting.QuotaDisplayTypeUSD
	t.Cleanup(func() {
		operation_setting.GetGeneralSetting().QuotaDisplayType = originalDisplayType
	})
	insertUserForPaymentGuardTest(t, 971, 5)
	topUp := &TopUp{
		UserId:          971,
		Amount:          100,
		Money:           1,
		TradeNo:         "creem-standard-bonus-basis",
		PaymentMethod:   PaymentMethodCreem,
		PaymentProvider: PaymentProviderCreem,
		Status:          common.TopUpStatusPending,
		CreateTime:      time.Now().Unix(),
	}
	require.NoError(t, topUp.Insert())
	assert.Equal(t, 110, topUp.PromisedQuota)

	require.NoError(t, RechargeCreem(topUp.TradeNo, "", "", "203.0.113.5"))
	stored := GetTopUpByTradeNo(topUp.TradeNo)
	require.NotNil(t, stored)
	assert.Equal(t, 110, stored.CreditedQuota)
}

func TestCreemBonusTierUsesTokenAmountInTokenDisplayMode(t *testing.T) {
	truncateTables(t)
	insertUserForPaymentGuardTest(t, 972, 5)
	useQuotaPerUnitForTopUpTest(t, 100)
	useTopupRewardSettingsForTest(t, 0, map[int]float64{}, map[int]float64{100: 0.2})
	originalDisplayType := operation_setting.GetGeneralSetting().QuotaDisplayType
	operation_setting.GetGeneralSetting().QuotaDisplayType = operation_setting.QuotaDisplayTypeTokens
	t.Cleanup(func() {
		operation_setting.GetGeneralSetting().QuotaDisplayType = originalDisplayType
	})

	topUp := &TopUp{
		UserId:          972,
		Amount:          100,
		Money:           1,
		TradeNo:         "creem-token-display-bonus-basis",
		PaymentMethod:   PaymentMethodCreem,
		PaymentProvider: PaymentProviderCreem,
		Status:          common.TopUpStatusPending,
	}
	require.NoError(t, topUp.Insert())
	assert.Equal(t, 120, topUp.PromisedQuota)
}

func TestPendingTopupRejectsUnsafeBonusBeforePayment(t *testing.T) {
	truncateTables(t)
	useQuotaPerUnitForTopUpTest(t, 1)
	useTopupRewardSettingsForTest(t, 0, map[int]float64{}, map[int]float64{1: 1})

	overflow := &TopUp{
		UserId:          1,
		Amount:          common.MaxWalletQuota,
		Money:           1,
		TradeNo:         "unsafe-bonus-overflow",
		PaymentMethod:   PaymentMethodWaffo,
		PaymentProvider: PaymentProviderWaffo,
		Status:          common.TopUpStatusPending,
	}
	require.ErrorIs(t, overflow.Insert(), ErrTopUpQuotaInvalid)

	operation_setting.GetPaymentSetting().AmountBonus = map[int]float64{1: 1.01}
	invalid := &TopUp{
		UserId:          1,
		Amount:          1,
		Money:           1,
		TradeNo:         "unsafe-bonus-rate",
		PaymentMethod:   PaymentMethodWaffo,
		PaymentProvider: PaymentProviderWaffo,
		Status:          common.TopUpStatusPending,
	}
	require.ErrorIs(t, invalid.Insert(), ErrTopUpQuotaInvalid)
}

func TestBalanceSubscriptionDoesNotTriggerTopupReward(t *testing.T) {
	truncateTables(t)
	useQuotaPerUnitForTopUpTest(t, 100)
	useTopupRewardSettingsForTest(t, 40, map[int]float64{}, map[int]float64{})
	insertInviterPairForTopupTest(t, 980, 981, MaxInviteRebateRatio)
	require.NoError(t, DB.Model(&User{}).Where("id = ?", 981).Update("quota", 1000).Error)
	plan := insertSubscriptionPlanForPaymentGuardTest(t, 990)
	plan.PriceAmount = 1
	plan.Enabled = true
	plan.CreatedAt = time.Now().Unix()
	require.NoError(t, DB.Save(plan).Error)

	require.NoError(t, PurchaseSubscriptionWithBalance(981, plan.Id))

	var inviter User
	require.NoError(t, DB.Select("aff_quota", "aff_history").First(&inviter, 980).Error)
	assert.Equal(t, 10, inviter.AffQuota)
	assert.Equal(t, 20, inviter.AffHistoryQuota)
}
