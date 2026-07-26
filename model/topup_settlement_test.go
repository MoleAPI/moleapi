package model

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func useQuotaPerUnitForTopUpTest(t *testing.T, quotaPerUnit float64) {
	t.Helper()
	original := common.QuotaPerUnit
	common.QuotaPerUnit = quotaPerUnit
	t.Cleanup(func() {
		common.QuotaPerUnit = original
	})
}

func insertTopUpForSettlementTest(t *testing.T, tradeNo string, userID int, amount int64, money float64, paymentProvider string) {
	t.Helper()
	record := &TopUp{
		UserId:          userID,
		Amount:          amount,
		Money:           money,
		TradeNo:         tradeNo,
		PaymentMethod:   paymentProvider,
		PaymentProvider: paymentProvider,
		Status:          common.TopUpStatusPending,
		CreateTime:      time.Now().Unix(),
	}
	// Insert directly to exercise legacy pending orders without a promised quota snapshot.
	require.NoError(t, DB.Create(record).Error)
}

func getTopUpLogForSettlementTest(t *testing.T, userID int) Log {
	t.Helper()
	var logs []Log
	require.NoError(t, LOG_DB.Where("user_id = ? AND type = ?", userID, LogTypeTopup).Find(&logs).Error)
	require.Len(t, logs, 1)
	return logs[0]
}

func assertTopUpAuditForSettlementTest(t *testing.T, log Log, callerIP string, paymentMethod string, callbackPaymentMethod string) {
	t.Helper()
	var other map[string]interface{}
	require.NoError(t, common.UnmarshalJsonStr(log.Other, &other))
	adminInfo, ok := other["admin_info"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, callerIP, log.Ip)
	assert.Equal(t, callerIP, adminInfo["caller_ip"])
	assert.Equal(t, paymentMethod, adminInfo["payment_method"])
	assert.Equal(t, callbackPaymentMethod, adminInfo["callback_payment_method"])
}

func TestManualCompleteTopUpSettlesStripeOnce(t *testing.T) {
	truncateTables(t)
	useQuotaPerUnitForTopUpTest(t, 100)

	insertUserForPaymentGuardTest(t, 501, 10)
	insertTopUpForSettlementTest(t, "manual-stripe", 501, 99, 1.25, PaymentProviderStripe)

	require.NoError(t, ManualCompleteTopUp("manual-stripe", "203.0.113.10"))

	topUp := GetTopUpByTradeNo("manual-stripe")
	require.NotNil(t, topUp)
	assert.Equal(t, common.TopUpStatusSuccess, topUp.Status)
	assert.Equal(t, 125, topUp.CreditedQuota)
	assert.Positive(t, topUp.CompleteTime)
	assert.Equal(t, 135, getUserQuotaForPaymentGuardTest(t, 501))

	log := getTopUpLogForSettlementTest(t, 501)
	assertTopUpAuditForSettlementTest(t, log, "203.0.113.10", PaymentMethodStripe, "admin")

	require.NoError(t, ManualCompleteTopUp("manual-stripe", "203.0.113.11"))
	assert.Equal(t, 135, getUserQuotaForPaymentGuardTest(t, 501))
	getTopUpLogForSettlementTest(t, 501)
}

func TestManualCompleteTopUpUsesCreemQuotaSnapshot(t *testing.T) {
	truncateTables(t)
	useQuotaPerUnitForTopUpTest(t, 100)

	insertUserForPaymentGuardTest(t, 526, 10)
	insertTopUpForSettlementTest(t, "manual-creem", 526, 120, 9.99, PaymentProviderCreem)

	require.NoError(t, ManualCompleteTopUp("manual-creem", "203.0.113.12"))

	topUp := GetTopUpByTradeNo("manual-creem")
	require.NotNil(t, topUp)
	assert.Equal(t, 120, topUp.CreditedQuota)
	assert.Equal(t, 130, getUserQuotaForPaymentGuardTest(t, 526))
}

func TestManualCompleteTopUpPreservesLegacyLargeBalance(t *testing.T) {
	truncateTables(t)
	useQuotaPerUnitForTopUpTest(t, 100)

	legacyBalance := common.MaxQuota + 1_000
	insertUserForPaymentGuardTest(t, 527, legacyBalance)
	insertTopUpForSettlementTest(t, "manual-legacy-balance", 527, 1, 7.3, PaymentProviderEpay)

	require.NoError(t, ManualCompleteTopUp("manual-legacy-balance", "203.0.113.13"))
	assert.Equal(t, legacyBalance+100, getUserQuotaForPaymentGuardTest(t, 527))

	topUp := GetTopUpByTradeNo("manual-legacy-balance")
	require.NotNil(t, topUp)
	assert.Equal(t, common.TopUpStatusSuccess, topUp.Status)
	assert.Equal(t, 100, topUp.CreditedQuota)
}

func TestManualCompleteTopUpSupportsProvidersWithLegacyLargeBalance(t *testing.T) {
	useQuotaPerUnitForTopUpTest(t, 100)

	for index, provider := range []string{
		PaymentProviderEpay,
		PaymentProviderLanTu,
		PaymentProviderNowPayments,
		PaymentProviderWaffo,
		PaymentProviderWaffoPancake,
		PaymentProviderCreem,
		PaymentProviderStripe,
	} {
		t.Run(provider, func(t *testing.T) {
			truncateTables(t)
			userID := 540 + index
			legacyBalance := common.MaxQuota + 1_000 + index
			insertUserForPaymentGuardTest(t, userID, legacyBalance)
			insertTopUpForSettlementTest(t, "manual-"+provider, userID, 1, 1, provider)

			require.NoError(t, ManualCompleteTopUp("manual-"+provider, "203.0.113.14"))

			topUp := GetTopUpByTradeNo("manual-" + provider)
			require.NotNil(t, topUp)
			assert.Equal(t, common.TopUpStatusSuccess, topUp.Status)
			assert.Positive(t, topUp.CreditedQuota)
			assert.Equal(t, legacyBalance+topUp.CreditedQuota, getUserQuotaForPaymentGuardTest(t, userID))
		})
	}
}

func TestRechargeStripeSettlesPaymentDetailsAndCustomerOnce(t *testing.T) {
	truncateTables(t)
	useQuotaPerUnitForTopUpTest(t, 100)

	insertUserForPaymentGuardTest(t, 521, 10)
	insertTopUpForSettlementTest(t, "stripe-payment-details", 521, 9, 1.25, PaymentProviderStripe)

	require.NoError(t, RechargeStripeWithPaymentDetails("stripe-payment-details", "cus_123", "cs_123", "USD", "203.0.113.60"))
	require.NoError(t, RechargeStripeWithPaymentDetails("stripe-payment-details", "cus_other", "cs_other", "EUR", "203.0.113.61"))

	topUp := GetTopUpByTradeNo("stripe-payment-details")
	require.NotNil(t, topUp)
	assert.Equal(t, common.TopUpStatusSuccess, topUp.Status)
	assert.Equal(t, 125, topUp.CreditedQuota)
	assert.Equal(t, "cs_123", topUp.GatewayTradeNo)
	assert.Equal(t, "USD", topUp.PaymentCurrency)
	assert.Positive(t, topUp.CompleteTime)

	var user User
	require.NoError(t, DB.Select("quota", "stripe_customer").Where("id = ?", 521).First(&user).Error)
	assert.Equal(t, 135, user.Quota)
	assert.Equal(t, "cus_123", user.StripeCustomer)

	log := getTopUpLogForSettlementTest(t, 521)
	assertTopUpAuditForSettlementTest(t, log, "203.0.113.60", PaymentMethodStripe, PaymentProviderStripe)
}

func TestRechargeStripeUsesStoredCreditAmountForNewOrders(t *testing.T) {
	truncateTables(t)
	useQuotaPerUnitForTopUpTest(t, 100)

	insertUserForPaymentGuardTest(t, 525, 10)
	insertTopUpForSettlementTest(t, "stripe-new-order", 525, 9, 1.25, PaymentProviderStripe)
	require.NoError(t, DB.Model(&TopUp{}).Where("trade_no = ?", "stripe-new-order").Update("payment_currency", "USD").Error)

	require.NoError(t, RechargeStripeWithPaymentDetails("stripe-new-order", "", "cs_new", "USD", "203.0.113.66"))

	topUp := GetTopUpByTradeNo("stripe-new-order")
	require.NotNil(t, topUp)
	assert.Equal(t, 900, topUp.CreditedQuota)
	assert.Equal(t, 910, getUserQuotaForPaymentGuardTest(t, 525))
}

func TestRechargeStripePreservesCustomerWhenCallbackOmitsIt(t *testing.T) {
	truncateTables(t)
	useQuotaPerUnitForTopUpTest(t, 100)

	insertUserForPaymentGuardTest(t, 524, 10)
	require.NoError(t, DB.Model(&User{}).Where("id = ?", 524).Update("stripe_customer", "cus_existing").Error)
	insertTopUpForSettlementTest(t, "stripe-empty-customer", 524, 1, 1, PaymentProviderStripe)

	require.NoError(t, RechargeStripeWithPaymentDetails("stripe-empty-customer", "", "cs_124", "USD", "203.0.113.65"))

	var user User
	require.NoError(t, DB.Select("quota", "stripe_customer").Where("id = ?", 524).First(&user).Error)
	assert.Equal(t, 110, user.Quota)
	assert.Equal(t, "cus_existing", user.StripeCustomer)
}

func TestRechargeStripeRollsBackCustomerOnQuotaOverflow(t *testing.T) {
	truncateTables(t)
	useQuotaPerUnitForTopUpTest(t, 100)

	insertUserForPaymentGuardTest(t, 522, common.MaxQuota-50)
	require.NoError(t, DB.Model(&User{}).Where("id = ?", 522).Update("stripe_customer", "cus_existing").Error)
	insertTopUpForSettlementTest(t, "stripe-overflow", 522, 1, 1, PaymentProviderStripe)

	err := RechargeStripeWithPaymentDetails("stripe-overflow", "cus_new", "cs_overflow", "USD", "203.0.113.62")
	require.EqualError(t, err, "充值失败，请稍后重试")

	topUp := GetTopUpByTradeNo("stripe-overflow")
	require.NotNil(t, topUp)
	assert.Equal(t, common.TopUpStatusPending, topUp.Status)
	assert.Zero(t, topUp.CreditedQuota)
	assert.Empty(t, topUp.GatewayTradeNo)
	assert.Empty(t, topUp.PaymentCurrency)
	assert.Zero(t, topUp.CompleteTime)

	var user User
	require.NoError(t, DB.Select("quota", "stripe_customer").Where("id = ?", 522).First(&user).Error)
	assert.Equal(t, common.MaxQuota-50, user.Quota)
	assert.Equal(t, "cus_existing", user.StripeCustomer)
}

func TestRechargeCreemSettlesPaymentDetailsAndEmailOnce(t *testing.T) {
	truncateTables(t)

	insertUserForPaymentGuardTest(t, 523, 10)
	insertTopUpForSettlementTest(t, "creem-payment-details", 523, 120, 9.99, PaymentProviderCreem)

	require.NoError(t, RechargeCreemWithPaymentDetails("creem-payment-details", "buyer@example.com", "Buyer", "ord_123", "USD", "203.0.113.63"))
	require.NoError(t, RechargeCreemWithPaymentDetails("creem-payment-details", "other@example.com", "Other", "ord_other", "EUR", "203.0.113.64"))

	topUp := GetTopUpByTradeNo("creem-payment-details")
	require.NotNil(t, topUp)
	assert.Equal(t, common.TopUpStatusSuccess, topUp.Status)
	assert.Equal(t, 120, topUp.CreditedQuota)
	assert.Equal(t, "ord_123", topUp.GatewayTradeNo)
	assert.Equal(t, "USD", topUp.PaymentCurrency)
	assert.Positive(t, topUp.CompleteTime)

	var user User
	require.NoError(t, DB.Select("quota", "email").Where("id = ?", 523).First(&user).Error)
	assert.Equal(t, 130, user.Quota)
	assert.Equal(t, "buyer@example.com", user.Email)

	log := getTopUpLogForSettlementTest(t, 523)
	assertTopUpAuditForSettlementTest(t, log, "203.0.113.63", PaymentMethodCreem, PaymentProviderCreem)
}

func TestWaffoSettlementsPersistQuotaAndAuditOnce(t *testing.T) {
	useQuotaPerUnitForTopUpTest(t, 100)

	testCases := []struct {
		name     string
		userID   int
		tradeNo  string
		provider string
		callerIP string
		gateway  string
		currency string
	}{
		{
			name:     "waffo",
			userID:   502,
			tradeNo:  "waffo-settlement",
			provider: PaymentProviderWaffo,
			callerIP: "203.0.113.20",
			gateway:  "waffo-gateway-1",
			currency: "USD",
		},
		{
			name:     "waffo pancake",
			userID:   503,
			tradeNo:  "waffo-pancake-settlement",
			provider: PaymentProviderWaffoPancake,
			callerIP: "203.0.113.21",
			gateway:  "pancake-gateway-1",
			currency: "USD",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			truncateTables(t)
			insertUserForPaymentGuardTest(t, testCase.userID, 7)
			insertTopUpForSettlementTest(t, testCase.tradeNo, testCase.userID, 2, 9.99, testCase.provider)

			var settle func() error
			if testCase.provider == PaymentProviderWaffo {
				settle = func() error {
					return RechargeWaffoWithPaymentDetails(testCase.tradeNo, testCase.gateway, testCase.currency, testCase.callerIP)
				}
			} else {
				settle = func() error {
					return RechargeWaffoPancakeWithPaymentDetails(testCase.tradeNo, testCase.gateway, testCase.currency, testCase.callerIP)
				}
			}

			require.NoError(t, settle())
			require.NoError(t, settle())

			topUp := GetTopUpByTradeNo(testCase.tradeNo)
			require.NotNil(t, topUp)
			assert.Equal(t, common.TopUpStatusSuccess, topUp.Status)
			assert.Equal(t, 200, topUp.CreditedQuota)
			assert.Positive(t, topUp.CompleteTime)
			assert.Equal(t, testCase.gateway, topUp.GatewayTradeNo)
			assert.Equal(t, testCase.currency, topUp.PaymentCurrency)
			assert.Equal(t, 207, getUserQuotaForPaymentGuardTest(t, testCase.userID))

			log := getTopUpLogForSettlementTest(t, testCase.userID)
			assertTopUpAuditForSettlementTest(t, log, testCase.callerIP, testCase.provider, testCase.provider)
		})
	}
}

func TestRechargeWaffoRejectsInvalidQuotaWithoutPartialSettlement(t *testing.T) {
	testCases := []struct {
		name         string
		amount       int64
		quotaPerUnit float64
	}{
		{name: "zero", amount: 0, quotaPerUnit: 100},
		{name: "overflow", amount: int64(common.MaxQuota), quotaPerUnit: 2},
	}

	for index, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			truncateTables(t)
			useQuotaPerUnitForTopUpTest(t, testCase.quotaPerUnit)
			userID := 510 + index
			tradeNo := "invalid-quota-" + testCase.name
			insertUserForPaymentGuardTest(t, userID, 50)
			insertTopUpForSettlementTest(t, tradeNo, userID, testCase.amount, 1, PaymentProviderWaffo)

			err := RechargeWaffo(tradeNo, "203.0.113.30")
			require.EqualError(t, err, "充值失败，请稍后重试")

			topUp := GetTopUpByTradeNo(tradeNo)
			require.NotNil(t, topUp)
			assert.Equal(t, common.TopUpStatusPending, topUp.Status)
			assert.Zero(t, topUp.CreditedQuota)
			assert.Zero(t, topUp.CompleteTime)
			assert.Equal(t, 50, getUserQuotaForPaymentGuardTest(t, userID))

			var logCount int64
			require.NoError(t, LOG_DB.Model(&Log{}).Where("user_id = ? AND type = ?", userID, LogTypeTopup).Count(&logCount).Error)
			assert.Zero(t, logCount)
		})
	}
}

func TestRechargeWaffoRollsBackWhenUserIsMissing(t *testing.T) {
	truncateTables(t)
	useQuotaPerUnitForTopUpTest(t, 100)
	insertTopUpForSettlementTest(t, "missing-user", 9999, 2, 1, PaymentProviderWaffo)

	err := RechargeWaffo("missing-user", "203.0.113.40")
	require.EqualError(t, err, "充值失败，请稍后重试")

	topUp := GetTopUpByTradeNo("missing-user")
	require.NotNil(t, topUp)
	assert.Equal(t, common.TopUpStatusPending, topUp.Status)
	assert.Zero(t, topUp.CreditedQuota)
	assert.Zero(t, topUp.CompleteTime)
}

func TestRechargeWaffoRejectsUserQuotaOverflow(t *testing.T) {
	truncateTables(t)
	useQuotaPerUnitForTopUpTest(t, 100)
	insertUserForPaymentGuardTest(t, 520, common.MaxQuota-50)
	insertTopUpForSettlementTest(t, "user-quota-overflow", 520, 1, 1, PaymentProviderWaffo)

	err := RechargeWaffo("user-quota-overflow", "203.0.113.50")
	require.EqualError(t, err, "充值失败，请稍后重试")

	topUp := GetTopUpByTradeNo("user-quota-overflow")
	require.NotNil(t, topUp)
	assert.Equal(t, common.TopUpStatusPending, topUp.Status)
	assert.Zero(t, topUp.CreditedQuota)
	assert.Zero(t, topUp.CompleteTime)
	assert.Equal(t, common.MaxQuota-50, getUserQuotaForPaymentGuardTest(t, 520))
}
