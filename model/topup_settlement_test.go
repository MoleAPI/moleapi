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
	require.NoError(t, record.Insert())
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

func TestWaffoSettlementsPersistQuotaAndAuditOnce(t *testing.T) {
	useQuotaPerUnitForTopUpTest(t, 100)

	testCases := []struct {
		name     string
		userID   int
		tradeNo  string
		provider string
		callerIP string
	}{
		{
			name:     "waffo",
			userID:   502,
			tradeNo:  "waffo-settlement",
			provider: PaymentProviderWaffo,
			callerIP: "203.0.113.20",
		},
		{
			name:     "waffo pancake",
			userID:   503,
			tradeNo:  "waffo-pancake-settlement",
			provider: PaymentProviderWaffoPancake,
			callerIP: "",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			truncateTables(t)
			insertUserForPaymentGuardTest(t, testCase.userID, 7)
			insertTopUpForSettlementTest(t, testCase.tradeNo, testCase.userID, 2, 9.99, testCase.provider)

			var settle func() error
			if testCase.provider == PaymentProviderWaffo {
				settle = func() error { return RechargeWaffo(testCase.tradeNo, testCase.callerIP) }
			} else {
				settle = func() error { return RechargeWaffoPancake(testCase.tradeNo) }
			}

			require.NoError(t, settle())
			require.NoError(t, settle())

			topUp := GetTopUpByTradeNo(testCase.tradeNo)
			require.NotNil(t, topUp)
			assert.Equal(t, common.TopUpStatusSuccess, topUp.Status)
			assert.Equal(t, 200, topUp.CreditedQuota)
			assert.Positive(t, topUp.CompleteTime)
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
