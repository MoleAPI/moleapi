package controller

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/Calcium-Ion/go-epay/epay"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupSubscriptionEpayTest(t *testing.T) *gorm.DB {
	t.Helper()
	db := setupTopUpWebhookSettlementTest(t)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(2)
	originalPrice := operation_setting.Price
	operation_setting.Price = 7.3
	require.NoError(t, db.AutoMigrate(&model.SubscriptionPlan{}, &model.UserSubscription{}))
	t.Cleanup(func() {
		operation_setting.Price = originalPrice
	})
	return db
}

func insertSubscriptionEpayFixtures(t *testing.T, db *gorm.DB, planID int, userID int, price float64) {
	t.Helper()
	plan := &model.SubscriptionPlan{
		Id:            planID,
		Title:         "Epay Plan",
		PriceAmount:   price,
		Currency:      "USD",
		DurationUnit:  model.SubscriptionDurationMonth,
		DurationValue: 1,
		Enabled:       true,
		TotalAmount:   1000,
	}
	user := &model.User{
		Id:       userID,
		Username: "subscription_epay_user",
		Password: "password123",
		Status:   common.UserStatusEnabled,
	}
	require.NoError(t, db.Create(user).Error)
	require.NoError(t, db.Create(plan).Error)
	model.InvalidateSubscriptionPlanCache(planID)
	t.Cleanup(func() {
		model.InvalidateSubscriptionPlanCache(planID)
	})
}

func signedSubscriptionEpayQuery(tradeNo string, tradeStatus string, money string) string {
	return signedSubscriptionEpayQueryWithPID(tradeNo, tradeStatus, money, operation_setting.EpayId)
}

func signedSubscriptionEpayQueryWithPID(tradeNo string, tradeStatus string, money string, pid string) string {
	params := epay.GenerateParams(map[string]string{
		"pid":          pid,
		"type":         "alipay",
		"trade_no":     "epay-subscription-gateway-123",
		"out_trade_no": tradeNo,
		"name":         "SUB:Epay Plan",
		"money":        money,
		"trade_status": tradeStatus,
	}, operation_setting.EpayKey)
	query := url.Values{}
	for key, value := range params {
		query.Set(key, value)
	}
	return query.Encode()
}

func TestSubscriptionEpayNotifyRejectsSignedCallbackForAnotherMerchant(t *testing.T) {
	db := setupSubscriptionEpayTest(t)
	insertSubscriptionEpayFixtures(t, db, 9104, 9204, 1)
	order := &model.SubscriptionOrder{
		UserId:          9204,
		PlanId:          9104,
		Money:           7.3,
		TradeNo:         "subscription-epay-merchant-mismatch",
		PaymentMethod:   "alipay",
		PaymentProvider: model.PaymentProviderEpay,
		Status:          common.TopUpStatusPending,
	}
	require.NoError(t, order.Insert())

	query := signedSubscriptionEpayQueryWithPID(order.TradeNo, epay.StatusTradeSuccess, "7.30", "another-merchant")
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodGet, "/api/subscription/epay/notify?"+query, nil)
	SubscriptionEpayNotify(context)

	assert.Equal(t, "fail", recorder.Body.String())
	assert.Equal(t, common.TopUpStatusPending, model.GetSubscriptionOrderByTradeNo(order.TradeNo).Status)
}

func runSignedSubscriptionEpayCallback(t *testing.T, path string, tradeNo string, tradeStatus string, money string, handler gin.HandlerFunc) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodGet, path+"?"+signedSubscriptionEpayQuery(tradeNo, tradeStatus, money), nil)
	handler(context)
	return recorder
}

func TestSubscriptionEpayNotifyUsesLocalCurrencyOrderSnapshot(t *testing.T) {
	db := setupSubscriptionEpayTest(t)
	insertSubscriptionEpayFixtures(t, db, 9101, 9201, 1.005)

	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodPost, "/api/subscription/epay/pay", strings.NewReader(`{"plan_id":9101,"payment_method":"alipay"}`))
	context.Request.Header.Set("Content-Type", "application/json")
	context.Set("id", 9201)
	SubscriptionRequestEpay(context)

	var response struct {
		Message string            `json:"message"`
		Data    map[string]string `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	require.Equal(t, "success", response.Message)
	require.Equal(t, "7.34", response.Data["money"])
	tradeNo := response.Data["out_trade_no"]
	require.NotEmpty(t, tradeNo)

	order := model.GetSubscriptionOrderByTradeNo(tradeNo)
	require.NotNil(t, order)
	assert.Equal(t, 7.34, order.Money)
	assert.Equal(t, model.PaymentProviderEpay, order.PaymentProvider)
	assert.NotEmpty(t, order.PlanSnapshot)

	// A later price setting change must not alter the amount accepted for this order.
	operation_setting.Price = 99
	nonSuccess := runSignedSubscriptionEpayCallback(t, "/api/subscription/epay/notify", tradeNo, "WAIT_BUYER_PAY", "7.34", SubscriptionEpayNotify)
	assert.Equal(t, "success", nonSuccess.Body.String())
	assert.Equal(t, common.TopUpStatusPending, model.GetSubscriptionOrderByTradeNo(tradeNo).Status)

	mismatched := runSignedSubscriptionEpayCallback(t, "/api/subscription/epay/notify", tradeNo, epay.StatusTradeSuccess, "7.33", SubscriptionEpayNotify)
	assert.Equal(t, "fail", mismatched.Body.String())
	assert.Equal(t, common.TopUpStatusPending, model.GetSubscriptionOrderByTradeNo(tradeNo).Status)

	settled := runSignedSubscriptionEpayCallback(t, "/api/subscription/epay/notify", tradeNo, epay.StatusTradeSuccess, "7.34", SubscriptionEpayNotify)
	assert.Equal(t, "success", settled.Body.String())
	order = model.GetSubscriptionOrderByTradeNo(tradeNo)
	require.NotNil(t, order)
	assert.Equal(t, common.TopUpStatusSuccess, order.Status)
	assert.Equal(t, "epay-subscription-gateway-123", order.GatewayTradeNo)
	assert.Equal(t, "CNY", order.PaymentCurrency)

	var subscriptionCount int64
	require.NoError(t, db.Model(&model.UserSubscription{}).Where("user_id = ?", 9201).Count(&subscriptionCount).Error)
	assert.Equal(t, int64(1), subscriptionCount)
}

func TestSubscriptionEpayNotifyRejectsAnotherPaymentProvider(t *testing.T) {
	db := setupSubscriptionEpayTest(t)
	insertSubscriptionEpayFixtures(t, db, 9102, 9202, 1)
	order := &model.SubscriptionOrder{
		UserId:          9202,
		PlanId:          9102,
		Money:           7.3,
		TradeNo:         "subscription-epay-provider-mismatch",
		PaymentMethod:   model.PaymentMethodStripe,
		PaymentProvider: model.PaymentProviderStripe,
		Status:          common.TopUpStatusPending,
	}
	require.NoError(t, order.Insert())

	response := runSignedSubscriptionEpayCallback(t, "/api/subscription/epay/notify", order.TradeNo, epay.StatusTradeSuccess, "7.30", SubscriptionEpayNotify)
	assert.Equal(t, "fail", response.Body.String())
	assert.Equal(t, common.TopUpStatusPending, model.GetSubscriptionOrderByTradeNo(order.TradeNo).Status)
}

func TestSubscriptionEpayReturnChecksAmountBeforeCompleting(t *testing.T) {
	db := setupSubscriptionEpayTest(t)
	insertSubscriptionEpayFixtures(t, db, 9103, 9203, 2)
	order := &model.SubscriptionOrder{
		UserId:          9203,
		PlanId:          9103,
		Money:           14.6,
		TradeNo:         "subscription-epay-browser-return",
		PaymentMethod:   "alipay",
		PaymentProvider: model.PaymentProviderEpay,
		Status:          common.TopUpStatusPending,
	}
	require.NoError(t, order.Insert())

	mismatched := runSignedSubscriptionEpayCallback(t, "/api/subscription/epay/return", order.TradeNo, epay.StatusTradeSuccess, "14.59", SubscriptionEpayReturn)
	assert.Equal(t, http.StatusFound, mismatched.Code)
	failureLocation, err := url.Parse(mismatched.Header().Get("Location"))
	require.NoError(t, err)
	assert.Equal(t, "/wallet", failureLocation.Path)
	assert.Equal(t, "fail", failureLocation.Query().Get("pay"))
	assert.Equal(t, common.TopUpStatusPending, model.GetSubscriptionOrderByTradeNo(order.TradeNo).Status)

	settled := runSignedSubscriptionEpayCallback(t, "/api/subscription/epay/return", order.TradeNo, epay.StatusTradeSuccess, "14.60", SubscriptionEpayReturn)
	assert.Equal(t, http.StatusFound, settled.Code)
	successLocation, err := url.Parse(settled.Header().Get("Location"))
	require.NoError(t, err)
	assert.Equal(t, "/wallet", successLocation.Path)
	assert.Equal(t, "success", successLocation.Query().Get("pay"))
	storedOrder := model.GetSubscriptionOrderByTradeNo(order.TradeNo)
	require.NotNil(t, storedOrder)
	assert.Equal(t, common.TopUpStatusSuccess, storedOrder.Status)
	assert.Equal(t, "epay-subscription-gateway-123", storedOrder.GatewayTradeNo)
}
