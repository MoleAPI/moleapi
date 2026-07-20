package model

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func insertUserForPaymentGuardTest(t *testing.T, id int, quota int) {
	t.Helper()
	user := &User{
		Id:       id,
		Username: "payment_guard_user",
		Status:   common.UserStatusEnabled,
		Quota:    quota,
	}
	require.NoError(t, DB.Create(user).Error)
}

func insertSubscriptionPlanForPaymentGuardTest(t *testing.T, id int) *SubscriptionPlan {
	t.Helper()
	plan := &SubscriptionPlan{
		Id:            id,
		Title:         "Guard Plan",
		PriceAmount:   9.99,
		Currency:      "USD",
		DurationUnit:  SubscriptionDurationMonth,
		DurationValue: 1,
		Enabled:       true,
		TotalAmount:   1000,
	}
	require.NoError(t, DB.Create(plan).Error)
	return plan
}

func insertSubscriptionOrderForPaymentGuardTest(t *testing.T, tradeNo string, userID int, planID int, paymentProvider string) {
	t.Helper()
	order := &SubscriptionOrder{
		UserId:          userID,
		PlanId:          planID,
		Money:           9.99,
		TradeNo:         tradeNo,
		PaymentMethod:   paymentProvider,
		PaymentProvider: paymentProvider,
		Status:          common.TopUpStatusPending,
		CreateTime:      time.Now().Unix(),
	}
	require.NoError(t, order.Insert())
}

func insertTopUpForPaymentGuardTest(t *testing.T, tradeNo string, userID int, paymentProvider string) {
	t.Helper()
	topUp := &TopUp{
		UserId:          userID,
		Amount:          2,
		Money:           9.99,
		TradeNo:         tradeNo,
		PaymentMethod:   paymentProvider,
		PaymentProvider: paymentProvider,
		Status:          common.TopUpStatusPending,
		CreateTime:      time.Now().Unix(),
	}
	require.NoError(t, topUp.Insert())
}

func getTopUpStatusForPaymentGuardTest(t *testing.T, tradeNo string) string {
	t.Helper()
	topUp := GetTopUpByTradeNo(tradeNo)
	require.NotNil(t, topUp)
	return topUp.Status
}

func countUserSubscriptionsForPaymentGuardTest(t *testing.T, userID int) int64 {
	t.Helper()
	var count int64
	require.NoError(t, DB.Model(&UserSubscription{}).Where("user_id = ?", userID).Count(&count).Error)
	return count
}

func getUserQuotaForPaymentGuardTest(t *testing.T, userID int) int {
	t.Helper()
	var user User
	require.NoError(t, DB.Select("quota").Where("id = ?", userID).First(&user).Error)
	return user.Quota
}

func TestRechargeWaffoPancake_RejectsMismatchedPaymentMethod(t *testing.T) {
	truncateTables(t)

	insertUserForPaymentGuardTest(t, 101, 0)
	insertTopUpForPaymentGuardTest(t, "waffo-pancake-guard", 101, PaymentProviderStripe)

	err := RechargeWaffoPancake("waffo-pancake-guard")
	require.Error(t, err)

	topUp := GetTopUpByTradeNo("waffo-pancake-guard")
	require.NotNil(t, topUp)
	assert.Equal(t, common.TopUpStatusPending, topUp.Status)
	assert.Equal(t, 0, getUserQuotaForPaymentGuardTest(t, 101))
}

func TestUpdatePendingTopUpStatus_RejectsMismatchedPaymentProvider(t *testing.T) {
	testCases := []struct {
		name                    string
		tradeNo                 string
		storedPaymentProvider   string
		expectedPaymentProvider string
		targetStatus            string
	}{
		{
			name:                    "stripe expire",
			tradeNo:                 "stripe-expire-guard",
			storedPaymentProvider:   PaymentProviderCreem,
			expectedPaymentProvider: PaymentProviderStripe,
			targetStatus:            common.TopUpStatusExpired,
		},
		{
			name:                    "waffo failed",
			tradeNo:                 "waffo-failed-guard",
			storedPaymentProvider:   PaymentProviderStripe,
			expectedPaymentProvider: PaymentProviderWaffo,
			targetStatus:            common.TopUpStatusFailed,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			truncateTables(t)
			insertUserForPaymentGuardTest(t, 150, 0)
			insertTopUpForPaymentGuardTest(t, tc.tradeNo, 150, tc.storedPaymentProvider)

			err := UpdatePendingTopUpStatus(tc.tradeNo, tc.expectedPaymentProvider, tc.targetStatus)
			require.ErrorIs(t, err, ErrPaymentMethodMismatch)
			assert.Equal(t, common.TopUpStatusPending, getTopUpStatusForPaymentGuardTest(t, tc.tradeNo))
		})
	}
}

func TestBindPendingTopUpPaymentFactsRejectsFactReplacement(t *testing.T) {
	truncateTables(t)
	insertUserForPaymentGuardTest(t, 151, 0)
	insertTopUpForPaymentGuardTest(t, "stripe-fact-binding", 151, PaymentProviderStripe)

	require.NoError(t, BindPendingTopUpPaymentFacts("stripe-fact-binding", PaymentProviderStripe, "cs_123", "USD", "", "payment"))
	err := BindPendingTopUpPaymentFacts("stripe-fact-binding", PaymentProviderStripe, "cs_other", "EUR", "", "payment+promo")
	require.ErrorIs(t, err, ErrPaymentMethodMismatch)

	topUp := GetTopUpByTradeNo("stripe-fact-binding")
	require.NotNil(t, topUp)
	assert.Equal(t, "cs_123", topUp.GatewayTradeNo)
	assert.Equal(t, "USD", topUp.PaymentCurrency)
	assert.Equal(t, "payment", topUp.PaymentMode)
}

func TestBindPendingSubscriptionOrderPaymentFactsRejectsFactReplacement(t *testing.T) {
	truncateTables(t)
	insertUserForPaymentGuardTest(t, 152, 0)
	plan := insertSubscriptionPlanForPaymentGuardTest(t, 402)
	insertSubscriptionOrderForPaymentGuardTest(t, "stripe-sub-fact-binding", 152, plan.Id, PaymentProviderStripe)

	require.NoError(t, BindPendingSubscriptionOrderPaymentFacts("stripe-sub-fact-binding", PaymentProviderStripe, "cs_123", "USD", "price_123", "payment", 9.99))
	err := BindPendingSubscriptionOrderPaymentFacts("stripe-sub-fact-binding", PaymentProviderStripe, "cs_other", "EUR", "price_other", "payment+promo", 1)
	require.ErrorIs(t, err, ErrPaymentMethodMismatch)

	order := GetSubscriptionOrderByTradeNo("stripe-sub-fact-binding")
	require.NotNil(t, order)
	assert.Equal(t, "cs_123", order.GatewayTradeNo)
	assert.Equal(t, "USD", order.PaymentCurrency)
	assert.Equal(t, "price_123", order.PaymentProductId)
	assert.Equal(t, "payment", order.PaymentMode)
	assert.InDelta(t, 9.99, order.Money, 0.000001)
}

func TestCompleteSubscriptionOrder_RejectsMismatchedPaymentProvider(t *testing.T) {
	truncateTables(t)

	insertUserForPaymentGuardTest(t, 202, 0)
	plan := insertSubscriptionPlanForPaymentGuardTest(t, 301)
	insertSubscriptionOrderForPaymentGuardTest(t, "sub-guard-order", 202, plan.Id, PaymentProviderStripe)

	err := CompleteSubscriptionOrder("sub-guard-order", `{"provider":"epay"}`, PaymentProviderEpay, "alipay")
	require.ErrorIs(t, err, ErrPaymentMethodMismatch)

	order := GetSubscriptionOrderByTradeNo("sub-guard-order")
	require.NotNil(t, order)
	assert.Equal(t, common.TopUpStatusPending, order.Status)
	assert.Zero(t, countUserSubscriptionsForPaymentGuardTest(t, 202))

	topUp := GetTopUpByTradeNo("sub-guard-order")
	assert.Nil(t, topUp)
}

func TestCompleteSubscriptionOrderPreservesPaymentAndPlanSnapshot(t *testing.T) {
	truncateTables(t)

	insertUserForPaymentGuardTest(t, 203, 0)
	plan := insertSubscriptionPlanForPaymentGuardTest(t, 302)
	plan.Title = "Original Plan"
	plan.TotalAmount = 1234
	plan.DurationUnit = SubscriptionDurationDay
	plan.DurationValue = 7
	require.NoError(t, DB.Save(plan).Error)
	insertSubscriptionOrderForPaymentGuardTest(t, "sub-payment-facts", 203, plan.Id, PaymentProviderStripe)

	require.NoError(t, DB.Model(&SubscriptionPlan{}).Where("id = ?", plan.Id).Updates(map[string]interface{}{
		"title":          "Changed Plan",
		"total_amount":   9999,
		"duration_value": 30,
	}).Error)
	require.NoError(t, CompleteSubscriptionOrderWithPaymentDetails(
		"sub-payment-facts",
		`{"event":"checkout.completed"}`,
		PaymentProviderStripe,
		PaymentMethodStripe,
		"cs_subscription_123",
		"USD",
	))
	require.NoError(t, CompleteSubscriptionOrderWithPaymentDetails(
		"sub-payment-facts",
		`{"event":"duplicate"}`,
		PaymentProviderStripe,
		PaymentMethodStripe,
		"cs_different",
		"EUR",
	))

	order := GetSubscriptionOrderByTradeNo("sub-payment-facts")
	require.NotNil(t, order)
	assert.Equal(t, common.TopUpStatusSuccess, order.Status)
	assert.Equal(t, "cs_subscription_123", order.GatewayTradeNo)
	assert.Equal(t, "USD", order.PaymentCurrency)
	assert.Contains(t, order.ProviderPayload, "checkout.completed")

	var subscription UserSubscription
	require.NoError(t, DB.Where("user_id = ? AND plan_id = ?", 203, plan.Id).First(&subscription).Error)
	assert.Equal(t, int64(1234), subscription.AmountTotal)
	assert.Equal(t, int64(7*24*60*60), subscription.EndTime-subscription.StartTime)

	topUp := GetTopUpByTradeNo("sub-payment-facts")
	require.NotNil(t, topUp)
	assert.Equal(t, PaymentProviderStripe, topUp.PaymentProvider)
	assert.Equal(t, PaymentMethodStripe, topUp.PaymentMethod)
	assert.Equal(t, "cs_subscription_123", topUp.GatewayTradeNo)
	assert.Equal(t, "USD", topUp.PaymentCurrency)
}

func TestExpireSubscriptionOrder_RejectsMismatchedPaymentProvider(t *testing.T) {
	truncateTables(t)

	insertUserForPaymentGuardTest(t, 303, 0)
	plan := insertSubscriptionPlanForPaymentGuardTest(t, 401)
	insertSubscriptionOrderForPaymentGuardTest(t, "sub-expire-guard", 303, plan.Id, PaymentProviderStripe)

	err := ExpireSubscriptionOrder("sub-expire-guard", PaymentProviderCreem)
	require.ErrorIs(t, err, ErrPaymentMethodMismatch)

	order := GetSubscriptionOrderByTradeNo("sub-expire-guard")
	require.NotNil(t, order)
	assert.Equal(t, common.TopUpStatusPending, order.Status)
}
