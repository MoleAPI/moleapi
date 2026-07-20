/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
package controller

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stripe/stripe-go/v81"
	"github.com/stripe/stripe-go/v81/webhook"
)

func configureStripeWebhookForSettlementTest(t *testing.T) {
	t.Helper()
	originalAPISecret := setting.StripeApiSecret
	originalWebhookSecret := setting.StripeWebhookSecret
	originalPriceID := setting.StripePriceId
	t.Cleanup(func() {
		setting.StripeApiSecret = originalAPISecret
		setting.StripeWebhookSecret = originalWebhookSecret
		setting.StripePriceId = originalPriceID
	})
	setting.StripeApiSecret = "sk_test_settlement"
	setting.StripeWebhookSecret = "whsec_settlement"
	setting.StripePriceId = "price_settlement"
}

func runSignedStripeWebhook(t *testing.T, eventType stripe.EventType, object map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	payload, err := common.Marshal(map[string]any{
		"id":       "evt_settlement",
		"object":   "event",
		"type":     eventType,
		"livemode": false,
		"data": map[string]any{
			"object": object,
		},
	})
	require.NoError(t, err)
	signedPayload := webhook.GenerateTestSignedPayload(&webhook.UnsignedPayload{
		Payload: payload,
		Secret:  setting.StripeWebhookSecret,
	})

	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodPost, "/api/user/stripe/webhook", bytes.NewReader(payload))
	context.Request.Header.Set("Stripe-Signature", signedPayload.Header)
	context.Request.RemoteAddr = "203.0.113.80:1234"
	StripeWebhook(context)
	return recorder
}

func TestStripeWebhookRetriesUntilAtomicSettlementSucceeds(t *testing.T) {
	db := setupTopUpWebhookSettlementTest(t)
	configureStripeWebhookForSettlementTest(t)
	user := &model.User{
		Id:       611,
		Username: "stripe_retry_user",
		Password: "password123",
		Status:   common.UserStatusEnabled,
		Quota:    common.MaxQuota - 50,
	}
	require.NoError(t, db.Create(user).Error)
	topUp := &model.TopUp{
		UserId:          user.Id,
		Amount:          1,
		Money:           1,
		TradeNo:         "stripe-webhook-retry",
		PaymentMethod:   model.PaymentMethodStripe,
		PaymentProvider: model.PaymentProviderStripe,
		CreateTime:      1,
		Status:          common.TopUpStatusPending,
	}
	require.NoError(t, db.Create(topUp).Error)
	eventObject := map[string]any{
		"id":                  "cs_retry",
		"client_reference_id": topUp.TradeNo,
		"customer":            "cus_retry",
		"status":              "complete",
		"payment_status":      "paid",
		"amount_total":        100,
		"currency":            "usd",
	}

	failed := runSignedStripeWebhook(t, stripe.EventTypeCheckoutSessionCompleted, eventObject)
	assert.Equal(t, http.StatusServiceUnavailable, failed.Code)
	require.NoError(t, db.First(topUp, topUp.Id).Error)
	assert.Equal(t, common.TopUpStatusPending, topUp.Status)
	assert.Zero(t, topUp.CreditedQuota)

	require.NoError(t, db.Model(&model.User{}).Where("id = ?", user.Id).Update("quota", 10).Error)
	settled := runSignedStripeWebhook(t, stripe.EventTypeCheckoutSessionCompleted, eventObject)
	assert.Equal(t, http.StatusOK, settled.Code)
	repeated := runSignedStripeWebhook(t, stripe.EventTypeCheckoutSessionCompleted, eventObject)
	assert.Equal(t, http.StatusOK, repeated.Code)

	require.NoError(t, db.First(topUp, topUp.Id).Error)
	assert.Equal(t, common.TopUpStatusSuccess, topUp.Status)
	assert.Equal(t, 100, topUp.CreditedQuota)
	assert.Equal(t, "cs_retry", topUp.GatewayTradeNo)
	assert.Equal(t, "USD", topUp.PaymentCurrency)
	var storedUser model.User
	require.NoError(t, db.Select("id", "quota", "stripe_customer").First(&storedUser, user.Id).Error)
	assert.Equal(t, 110, storedUser.Quota)
	assert.Equal(t, "cus_retry", storedUser.StripeCustomer)
}

func TestStripeAsyncFailureCannotOverwriteSettledOrder(t *testing.T) {
	db := setupTopUpWebhookSettlementTest(t)
	configureStripeWebhookForSettlementTest(t)
	user := &model.User{
		Id:       612,
		Username: "stripe_final_state_user",
		Password: "password123",
		Status:   common.UserStatusEnabled,
		Quota:    10,
	}
	require.NoError(t, db.Create(user).Error)
	topUp := &model.TopUp{
		UserId:          user.Id,
		Amount:          1,
		Money:           1,
		TradeNo:         "stripe-final-state",
		PaymentMethod:   model.PaymentMethodStripe,
		PaymentProvider: model.PaymentProviderStripe,
		CreateTime:      1,
		Status:          common.TopUpStatusPending,
	}
	require.NoError(t, db.Create(topUp).Error)
	require.NoError(t, model.RechargeStripeWithPaymentDetails(topUp.TradeNo, "cus_final", "cs_final", "USD", "203.0.113.81"))

	response := runSignedStripeWebhook(t, stripe.EventTypeCheckoutSessionAsyncPaymentFailed, map[string]any{
		"client_reference_id": topUp.TradeNo,
	})
	assert.Equal(t, http.StatusOK, response.Code)

	require.NoError(t, db.First(topUp, topUp.Id).Error)
	assert.Equal(t, common.TopUpStatusSuccess, topUp.Status)
	assert.Equal(t, 100, topUp.CreditedQuota)
	assert.Equal(t, "cs_final", topUp.GatewayTradeNo)
	assert.Equal(t, "USD", topUp.PaymentCurrency)
}

func TestStripeAsyncFailureMarksSubscriptionOrderFailed(t *testing.T) {
	db := setupTopUpWebhookSettlementTest(t)
	configureStripeWebhookForSettlementTest(t)
	order := &model.SubscriptionOrder{
		UserId:          613,
		PlanId:          1,
		Money:           9.99,
		TradeNo:         "sub_ref_stripe-async-failed",
		PaymentMethod:   model.PaymentMethodStripe,
		PaymentProvider: model.PaymentProviderStripe,
		CreateTime:      1,
		Status:          common.TopUpStatusPending,
	}
	require.NoError(t, db.Create(order).Error)

	response := runSignedStripeWebhook(t, stripe.EventTypeCheckoutSessionAsyncPaymentFailed, map[string]any{
		"client_reference_id": order.TradeNo,
	})
	assert.Equal(t, http.StatusOK, response.Code)

	require.NoError(t, db.First(order, order.Id).Error)
	assert.Equal(t, common.TopUpStatusFailed, order.Status)
	assert.Positive(t, order.CompleteTime)
	var topUpCount int64
	require.NoError(t, db.Model(&model.TopUp{}).Where("trade_no = ?", order.TradeNo).Count(&topUpCount).Error)
	assert.Zero(t, topUpCount)
}
