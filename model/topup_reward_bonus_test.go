package model

import (
	"fmt"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func useTopupRewardSettingsForTest(t *testing.T, reward int, discount map[int]float64, bonus map[int]float64) {
	t.Helper()
	originalReward := common.QuotaForInviterOnFirstTopup
	originalDiscount := operation_setting.GetPaymentSetting().AmountDiscount
	originalBonus := operation_setting.GetPaymentSetting().AmountBonus
	common.QuotaForInviterOnFirstTopup = reward
	operation_setting.GetPaymentSetting().AmountDiscount = discount
	operation_setting.GetPaymentSetting().AmountBonus = bonus
	t.Cleanup(func() {
		common.QuotaForInviterOnFirstTopup = originalReward
		operation_setting.GetPaymentSetting().AmountDiscount = originalDiscount
		operation_setting.GetPaymentSetting().AmountBonus = originalBonus
	})
}

func insertInviterPairForTopupTest(t *testing.T, inviterID int, inviteeID int) {
	t.Helper()
	require.NoError(t, DB.Create(&User{
		Id:              inviterID,
		Username:        fmt.Sprintf("inviter_%d", inviterID),
		AffCode:         fmt.Sprintf("inviter-aff-%d", inviterID),
		Status:          common.UserStatusEnabled,
		AffQuota:        10,
		AffHistoryQuota: 20,
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

func TestEveryRealTopupPathRewardsInviterExactlyOnce(t *testing.T) {
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
			return RechargeWaffoPancakeWithPaymentDetails(tradeNo, "gateway", "USD", "203.0.113.1")
		}},
		{name: "manual completion", provider: PaymentProviderEpay, settle: func(tradeNo string) error {
			return ManualCompleteTopUp(tradeNo, "203.0.113.1")
		}},
	}

	for index, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			truncateTables(t)
			inviterID := 700 + index*2
			inviteeID := inviterID + 1
			insertInviterPairForTopupTest(t, inviterID, inviteeID)
			tradeNo := fmt.Sprintf("reward-%d", index)
			insertTopUpForSettlementTest(t, tradeNo, inviteeID, 1, 1, testCase.provider)

			require.NoError(t, testCase.settle(tradeNo))
			require.NoError(t, testCase.settle(tradeNo))

			var inviter User
			require.NoError(t, DB.Select("aff_quota", "aff_history").First(&inviter, inviterID).Error)
			assert.Equal(t, 50, inviter.AffQuota)
			assert.Equal(t, 60, inviter.AffHistoryQuota)

			var invitee User
			require.NoError(t, DB.Select("inviter_topup_rewarded").First(&invitee, inviteeID).Error)
			assert.True(t, invitee.InviterTopupRewarded)

			var rewardLogs int64
			require.NoError(t, LOG_DB.Model(&Log{}).Where("user_id = ? AND type = ?", inviterID, LogTypeSystem).Count(&rewardLogs).Error)
			assert.EqualValues(t, 1, rewardLogs)
		})
	}
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
				Id:              inviterID,
				Username:        "overflow_inviter",
				AffCode:         "overflow-inviter-aff",
				Status:          common.UserStatusEnabled,
				AffQuota:        common.MaxQuota - 10,
				AffHistoryQuota: 20,
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

			var invitee User
			require.NoError(t, DB.Select("inviter_topup_rewarded").First(&invitee, inviteeID).Error)
			assert.True(t, invitee.InviterTopupRewarded)
			if testCase.name == "overflow" {
				var inviter User
				require.NoError(t, DB.Select("aff_quota", "aff_history").First(&inviter, testCase.inviterID).Error)
				assert.Equal(t, common.MaxQuota-10, inviter.AffQuota)
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
	insertInviterPairForTopupTest(t, 940, 941)

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
		Amount:          int64(common.MaxQuota),
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
	insertInviterPairForTopupTest(t, 980, 981)
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
	var invitee User
	require.NoError(t, DB.Select("inviter_topup_rewarded").First(&invitee, 981).Error)
	assert.False(t, invitee.InviterTopupRewarded)
}
