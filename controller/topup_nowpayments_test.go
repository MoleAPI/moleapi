package controller

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type nowPaymentsRoundTripper func(*http.Request) (*http.Response, error)

func (roundTripper nowPaymentsRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	return roundTripper(request)
}

func setupNowPaymentsControllerTest(t *testing.T) *gorm.DB {
	t.Helper()
	db := setupTopUpWebhookSettlementTest(t)
	originalEnabled := setting.NowPaymentsEnabled
	originalAPIKey := setting.NowPaymentsApiKey
	originalIPNSecret := setting.NowPaymentsIPNSecret
	originalSandbox := setting.NowPaymentsSandbox
	originalCurrency := setting.NowPaymentsCurrency
	originalUnitPrice := setting.NowPaymentsUnitPrice
	originalMinTopUp := setting.NowPaymentsMinTopUp
	originalHTTPClient := nowPaymentsHTTPClient
	originalServerAddress := system_setting.ServerAddress
	originalCallbackAddress := operation_setting.CustomCallbackAddress
	originalDisplayType := operation_setting.GetGeneralSetting().QuotaDisplayType
	originalTopUpRatio := common.TopupGroupRatio2JSONString()

	setting.NowPaymentsEnabled = true
	setting.NowPaymentsApiKey = "api-test"
	setting.NowPaymentsIPNSecret = "ipn-test"
	setting.NowPaymentsSandbox = false
	setting.NowPaymentsCurrency = "USD"
	setting.NowPaymentsUnitPrice = 1
	setting.NowPaymentsMinTopUp = 1
	system_setting.ServerAddress = "https://app.example.com"
	operation_setting.CustomCallbackAddress = ""
	operation_setting.GetGeneralSetting().QuotaDisplayType = operation_setting.QuotaDisplayTypeUSD
	require.NoError(t, common.UpdateTopupGroupRatioByJSONString(`{"default":1}`))

	t.Cleanup(func() {
		setting.NowPaymentsEnabled = originalEnabled
		setting.NowPaymentsApiKey = originalAPIKey
		setting.NowPaymentsIPNSecret = originalIPNSecret
		setting.NowPaymentsSandbox = originalSandbox
		setting.NowPaymentsCurrency = originalCurrency
		setting.NowPaymentsUnitPrice = originalUnitPrice
		setting.NowPaymentsMinTopUp = originalMinTopUp
		nowPaymentsHTTPClient = originalHTTPClient
		system_setting.ServerAddress = originalServerAddress
		operation_setting.CustomCallbackAddress = originalCallbackAddress
		operation_setting.GetGeneralSetting().QuotaDisplayType = originalDisplayType
		require.NoError(t, common.UpdateTopupGroupRatioByJSONString(originalTopUpRatio))
	})
	return db
}

func installNowPaymentsTransport(t *testing.T, roundTripper nowPaymentsRoundTripper) {
	t.Helper()
	nowPaymentsHTTPClient = &http.Client{Timeout: time.Second, Transport: roundTripper}
}

func nowPaymentsHTTPResponse(t *testing.T, status int, value any) *http.Response {
	t.Helper()
	payload, err := common.Marshal(value)
	require.NoError(t, err)
	return &http.Response{
		StatusCode: status,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(string(payload))),
	}
}

func createNowPaymentsTestUserAndTopUp(t *testing.T, db *gorm.DB, userID int, tradeNo string, money float64) {
	t.Helper()
	require.NoError(t, db.Create(&model.User{
		Id:       userID,
		Username: "nowpayments-user-" + tradeNo,
		Password: "password123",
		Group:    "default",
		Status:   common.UserStatusEnabled,
		Quota:    10,
	}).Error)
	require.NoError(t, db.Create(&model.TopUp{
		UserId:          userID,
		Amount:          2,
		Money:           money,
		TradeNo:         tradeNo,
		PaymentMethod:   model.PaymentMethodNowPayments,
		PaymentProvider: model.PaymentProviderNowPayments,
		PaymentCurrency: "USD",
		CreateTime:      time.Now().Unix(),
		Status:          common.TopUpStatusPending,
	}).Error)
}

func getUserQuotaForNowPaymentsTest(t *testing.T, db *gorm.DB, userID int) int {
	t.Helper()
	var user model.User
	require.NoError(t, db.Select("quota").First(&user, userID).Error)
	return user.Quota
}

func runNowPaymentsWebhook(t *testing.T, payload map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	body, err := common.Marshal(payload)
	require.NoError(t, err)
	signature, err := signNowPaymentsPayload(body, setting.NowPaymentsIPNSecret)
	require.NoError(t, err)

	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodPost, "/api/nowpayments/webhook", strings.NewReader(string(body)))
	context.Request.Header.Set("x-nowpayments-sig", signature)
	context.Request.RemoteAddr = "203.0.113.74:1234"
	NowPaymentsWebhook(context)
	return recorder
}

func TestNowPaymentsSignaturePayloadKeepsCanonicalNumbers(t *testing.T) {
	payload, err := nowPaymentsSignaturePayload([]byte(`{"url":"https://pay.example/a?x=<tag>","id":12345678901234567890,"nested":{"b":2,"a":1}}`))

	require.NoError(t, err)
	assert.Equal(t, `{"id":12345678901234567890,"nested":{"a":1,"b":2},"url":"https://pay.example/a?x=<tag>"}`, string(payload))
}

func TestNowPaymentsWebhookSettlesFinishedOrderOnce(t *testing.T) {
	db := setupNowPaymentsControllerTest(t)
	tradeNo := "nowpayments-finished"
	createNowPaymentsTestUserAndTopUp(t, db, 801, tradeNo, 2.50)
	payload := map[string]any{
		"payment_id":       "np-payment-123",
		"payment_status":   "finished",
		"price_amount":     2.5,
		"price_currency":   "usd",
		"order_id":         tradeNo,
		"pay_currency":     "btc",
		"actually_paid":    0.0001,
		"outcome_amount":   2.5,
		"outcome_currency": "usdttrc20",
	}

	assert.Equal(t, "OK", runNowPaymentsWebhook(t, payload).Body.String())
	assert.Equal(t, "OK", runNowPaymentsWebhook(t, payload).Body.String())

	topUp := model.GetTopUpByTradeNo(tradeNo)
	require.NotNil(t, topUp)
	assert.Equal(t, common.TopUpStatusSuccess, topUp.Status)
	assert.Equal(t, 200, topUp.CreditedQuota)
	assert.Equal(t, "np-payment-123", topUp.GatewayTradeNo)
	assert.Equal(t, "USD", topUp.PaymentCurrency)

	var user model.User
	require.NoError(t, db.Select("quota").First(&user, 801).Error)
	assert.Equal(t, 210, user.Quota)

	var logs []model.Log
	require.NoError(t, db.Where("user_id = ? AND type = ?", 801, model.LogTypeTopup).Find(&logs).Error)
	require.Len(t, logs, 1)
	assert.Equal(t, "203.0.113.74", logs[0].Ip)
}

func TestNowPaymentsWebhookLeavesPartialPaymentPending(t *testing.T) {
	db := setupNowPaymentsControllerTest(t)
	tradeNo := "nowpayments-partial"
	createNowPaymentsTestUserAndTopUp(t, db, 802, tradeNo, 2.50)

	assert.Equal(t, "OK", runNowPaymentsWebhook(t, map[string]any{
		"payment_status": "partially_paid",
		"order_id":       tradeNo,
	}).Body.String())

	topUp := model.GetTopUpByTradeNo(tradeNo)
	require.NotNil(t, topUp)
	assert.Equal(t, common.TopUpStatusPending, topUp.Status)
	assert.Zero(t, topUp.CreditedQuota)
	assert.Equal(t, 10, getUserQuotaForNowPaymentsTest(t, db, 802))
}

func TestNowPaymentsWebhookRejectsMismatchedAmount(t *testing.T) {
	db := setupNowPaymentsControllerTest(t)
	tradeNo := "nowpayments-amount-mismatch"
	createNowPaymentsTestUserAndTopUp(t, db, 803, tradeNo, 2.50)

	response := runNowPaymentsWebhook(t, map[string]any{
		"payment_id":     "np-payment-456",
		"payment_status": "finished",
		"price_amount":   2.49,
		"price_currency": "USD",
		"order_id":       tradeNo,
	})

	assert.Equal(t, http.StatusBadRequest, response.Code)
	topUp := model.GetTopUpByTradeNo(tradeNo)
	require.NotNil(t, topUp)
	assert.Equal(t, common.TopUpStatusPending, topUp.Status)
	assert.Zero(t, topUp.CreditedQuota)
	assert.Equal(t, 10, getUserQuotaForNowPaymentsTest(t, db, 803))
}

func TestRequestNowPaymentsPayCreatesPendingInvoice(t *testing.T) {
	db := setupNowPaymentsControllerTest(t)
	require.NoError(t, db.Create(&model.User{
		Id:       804,
		Username: "nowpayments-request-user",
		Password: "password123",
		Group:    "default",
		Status:   common.UserStatusEnabled,
	}).Error)
	sawPending := false
	installNowPaymentsTransport(t, func(request *http.Request) (*http.Response, error) {
		require.Equal(t, nowPaymentsAPIBase+nowPaymentsInvoicePath, request.URL.String())
		assert.Equal(t, "api-test", request.Header.Get("x-api-key"))
		var invoice nowPaymentsInvoiceRequest
		require.NoError(t, common.DecodeJson(request.Body, &invoice))
		assert.Equal(t, 2.0, invoice.PriceAmount)
		assert.Equal(t, "usd", invoice.PriceCurrency)
		assert.Equal(t, "https://app.example.com/api/nowpayments/webhook", invoice.IPNCallbackURL)
		assert.Equal(t, "https://app.example.com/wallet?show_history=true", invoice.SuccessURL)
		assert.Equal(t, "https://app.example.com/wallet", invoice.CancelURL)
		assert.True(t, strings.HasPrefix(invoice.OrderID, "USRTNP00000804"))
		pending := model.GetTopUpByTradeNo(invoice.OrderID)
		sawPending = pending != nil && pending.Status == common.TopUpStatusPending
		return nowPaymentsHTTPResponse(t, http.StatusOK, map[string]any{
			"id":             "invoice-123",
			"invoice_url":    "https://nowpayments.io/payment/?iid=invoice-123",
			"order_id":       invoice.OrderID,
			"price_amount":   2,
			"price_currency": "USD",
		}), nil
	})

	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Set("id", 804)
	context.Set("group", "default")
	context.Request = httptest.NewRequest(http.MethodPost, "/api/user/nowpayments/pay", strings.NewReader(`{"amount":2}`))
	context.Request.Header.Set("Content-Type", "application/json")
	RequestNowPaymentsPay(context)

	assert.True(t, sawPending)
	assert.Contains(t, recorder.Body.String(), `"message":"success"`)
	assert.Contains(t, recorder.Body.String(), `"checkout_url":"https://nowpayments.io/payment/?iid=invoice-123"`)

	var topUps []model.TopUp
	require.NoError(t, db.Where("user_id = ?", 804).Find(&topUps).Error)
	require.Len(t, topUps, 1)
	assert.Equal(t, common.TopUpStatusPending, topUps[0].Status)
	assert.Equal(t, "invoice-123", topUps[0].PaymentProductId)
	assert.Equal(t, model.PaymentProviderNowPayments, topUps[0].PaymentProvider)
}
