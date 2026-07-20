package controller

import (
	"errors"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/waffo-com/waffo-go/core"
	"github.com/waffo-com/waffo-go/types/order"
	"gorm.io/gorm"
)

func TestClassifyWaffoPaymentStatus(t *testing.T) {
	tests := map[string]string{
		core.OrderStatusPaySuccess:            common.TopUpStatusSuccess,
		core.OrderStatusOrderClose:            common.TopUpStatusFailed,
		core.OrderStatusPayInProgress:         common.TopUpStatusPending,
		core.OrderStatusAuthorizationRequired: common.TopUpStatusPending,
		core.OrderStatusAuthedWaitingCapture:  common.TopUpStatusPending,
	}

	for status, expected := range tests {
		t.Run(status, func(t *testing.T) {
			assert.Equal(t, expected, classifyWaffoPaymentStatus(status))
		})
	}
}

func TestValidateWaffoPaymentCallbackBindsEveryPaymentFact(t *testing.T) {
	originalMerchantID := setting.WaffoMerchantId
	originalSandbox := setting.WaffoSandbox
	t.Cleanup(func() {
		setting.WaffoMerchantId = originalMerchantID
		setting.WaffoSandbox = originalSandbox
	})
	setting.WaffoMerchantId = "merchant-123"
	setting.WaffoSandbox = false

	topUp := &model.TopUp{
		TradeNo:          "MO1TWF00000000000000000000000000",
		GatewayTradeNo:   "waffo-order-123",
		PaymentProductId: setting.WaffoMerchantId,
		PaymentMode:      waffoExpectedMode(),
		Money:            12.34,
		PaymentCurrency:  "USD",
		PaymentProvider:  model.PaymentProviderWaffo,
	}
	result := &core.PaymentNotificationResult{
		MerchantOrderID:  topUp.TradeNo,
		PaymentRequestID: topUp.TradeNo,
		AcquiringOrderID: "waffo-order-123",
		OrderStatus:      core.OrderStatusPaySuccess,
		OrderCurrency:    "USD",
		OrderAmount:      "12.34",
		MerchantInfo:     map[string]interface{}{"merchantId": setting.WaffoMerchantId},
	}

	currency, err := validateWaffoPaymentCallback(topUp, result)
	require.NoError(t, err)
	assert.Equal(t, "USD", currency)

	tests := []struct {
		name   string
		mutate func(*core.PaymentNotificationResult)
	}{
		{name: "merchant", mutate: func(result *core.PaymentNotificationResult) { result.MerchantInfo["merchantId"] = "other" }},
		{name: "merchant order", mutate: func(result *core.PaymentNotificationResult) { result.MerchantOrderID = "other" }},
		{name: "payment request", mutate: func(result *core.PaymentNotificationResult) { result.PaymentRequestID = "other" }},
		{name: "gateway order missing", mutate: func(result *core.PaymentNotificationResult) { result.AcquiringOrderID = "" }},
		{name: "gateway order mismatch", mutate: func(result *core.PaymentNotificationResult) { result.AcquiringOrderID = "other" }},
		{name: "currency", mutate: func(result *core.PaymentNotificationResult) { result.OrderCurrency = "EUR" }},
		{name: "amount", mutate: func(result *core.PaymentNotificationResult) { result.OrderAmount = "12.33" }},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			copyResult := *result
			copyResult.MerchantInfo = map[string]interface{}{"merchantId": setting.WaffoMerchantId}
			testCase.mutate(&copyResult)
			_, err := validateWaffoPaymentCallback(topUp, &copyResult)
			require.Error(t, err)
		})
	}

	topUp.PaymentMode = "sandbox"
	_, err = validateWaffoPaymentCallback(topUp, result)
	require.ErrorContains(t, err, "mode")
}

func TestValidateWaffoCreateOrderDataBindsOrderAndURL(t *testing.T) {
	tradeNo := "MO1TWF00000000000000000000000000"
	action, err := common.Marshal(order.OrderAction{ActionType: "WEB", WebURL: "https://checkout.waffo.com/pay/123"})
	require.NoError(t, err)
	data := &order.CreateOrderData{
		PaymentRequestID: tradeNo,
		MerchantOrderID:  tradeNo,
		AcquiringOrderID: "waffo-order-123",
		OrderStatus:      core.OrderStatusAuthorizationRequired,
		OrderAction:      string(action),
	}

	paymentURL, err := validateWaffoCreateOrderData(data, tradeNo)
	require.NoError(t, err)
	assert.Equal(t, "https://checkout.waffo.com/pay/123", paymentURL)

	deeplinkAction, err := common.Marshal(order.OrderAction{ActionType: "DEEPLINK", DeeplinkURL: "weixin://wap/pay?token=123"})
	require.NoError(t, err)
	data.OrderAction = string(deeplinkAction)
	data.OrderStatus = core.OrderStatusPayInProgress
	paymentURL, err = validateWaffoCreateOrderData(data, tradeNo)
	require.NoError(t, err)
	assert.Equal(t, "weixin://wap/pay?token=123", paymentURL)
}

func TestValidateWaffoCreateOrderDataRejectsMismatches(t *testing.T) {
	tradeNo := "MO1TWF00000000000000000000000000"
	validAction, err := common.Marshal(order.OrderAction{ActionType: "WEB", WebURL: "https://checkout.waffo.com/pay/123"})
	require.NoError(t, err)
	base := order.CreateOrderData{
		PaymentRequestID: tradeNo,
		MerchantOrderID:  tradeNo,
		AcquiringOrderID: "waffo-order-123",
		OrderStatus:      core.OrderStatusAuthorizationRequired,
		OrderAction:      string(validAction),
	}
	tests := []struct {
		name   string
		mutate func(*order.CreateOrderData)
	}{
		{name: "payment request", mutate: func(data *order.CreateOrderData) { data.PaymentRequestID = "other" }},
		{name: "merchant order", mutate: func(data *order.CreateOrderData) { data.MerchantOrderID = "other" }},
		{name: "gateway order", mutate: func(data *order.CreateOrderData) { data.AcquiringOrderID = "" }},
		{name: "status", mutate: func(data *order.CreateOrderData) { data.OrderStatus = core.OrderStatusPaySuccess }},
		{name: "empty action", mutate: func(data *order.CreateOrderData) { data.OrderAction = "" }},
		{name: "invalid action", mutate: func(data *order.CreateOrderData) { data.OrderAction = "{" }},
		{name: "insecure web URL", mutate: func(data *order.CreateOrderData) {
			action, marshalErr := common.Marshal(order.OrderAction{ActionType: "WEB", WebURL: "http://checkout.waffo.com/pay/123"})
			require.NoError(t, marshalErr)
			data.OrderAction = string(action)
		}},
		{name: "unsafe deeplink", mutate: func(data *order.CreateOrderData) {
			action, marshalErr := common.Marshal(order.OrderAction{ActionType: "DEEPLINK", DeeplinkURL: "javascript:alert(1)"})
			require.NoError(t, marshalErr)
			data.OrderAction = string(action)
		}},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			data := base
			testCase.mutate(&data)
			_, err := validateWaffoCreateOrderData(&data, tradeNo)
			require.Error(t, err)
		})
	}

	_, err = validateWaffoCreateOrderData(nil, tradeNo)
	require.Error(t, err)
}

func TestMarkWaffoOrderClosedReturnsDatabaseFailures(t *testing.T) {
	db := setupTopUpWebhookSettlementTest(t)
	require.NoError(t, db.Callback().Query().Before("gorm:query").Register("waffo_close_lookup_failure", func(tx *gorm.DB) {
		if _, ok := tx.Statement.Model.(*model.TopUp); ok {
			tx.AddError(errors.New("database unavailable"))
		}
	}))

	err := markWaffoOrderClosed("MO1TWF00000000000000000000000000")
	require.ErrorContains(t, err, "database unavailable")
}
