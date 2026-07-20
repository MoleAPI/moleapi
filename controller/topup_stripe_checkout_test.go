package controller

import (
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stripe/stripe-go/v81"
)

func TestStripeTopUpHandlersRejectDisabledSales(t *testing.T) {
	paymentSetting := operation_setting.GetPaymentSetting()
	originalConfirmed := paymentSetting.ComplianceConfirmed
	originalTermsVersion := paymentSetting.ComplianceTermsVersion
	t.Cleanup(func() {
		paymentSetting.ComplianceConfirmed = originalConfirmed
		paymentSetting.ComplianceTermsVersion = originalTermsVersion
	})
	paymentSetting.ComplianceConfirmed = false
	paymentSetting.ComplianceTermsVersion = ""

	for _, testCase := range []struct {
		name string
		call func(*gin.Context)
	}{
		{
			name: "quote",
			call: func(c *gin.Context) {
				stripeAdaptor.RequestAmount(c, &StripePayRequest{Amount: 10})
			},
		},
		{
			name: "checkout",
			call: func(c *gin.Context) {
				stripeAdaptor.RequestPay(c, &StripePayRequest{Amount: 10, PaymentMethod: "stripe"})
			},
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			context, _ := gin.CreateTestContext(recorder)
			testCase.call(context)

			assert.Equal(t, 200, recorder.Code)
			assert.Contains(t, recorder.Body.String(), "Stripe 支付未启用")
		})
	}
}

func TestCalculateStripeTopUpQuoteMatchesDisplayedAndCheckoutAmount(t *testing.T) {
	originalUnitPrice := setting.StripeUnitPrice
	originalMinTopUp := setting.StripeMinTopUp
	originalDisplayType := operation_setting.GetGeneralSetting().QuotaDisplayType
	originalDiscounts := operation_setting.GetPaymentSetting().AmountDiscount
	originalGroupRatios := common.TopupGroupRatio2JSONString()
	t.Cleanup(func() {
		setting.StripeUnitPrice = originalUnitPrice
		setting.StripeMinTopUp = originalMinTopUp
		operation_setting.GetGeneralSetting().QuotaDisplayType = originalDisplayType
		operation_setting.GetPaymentSetting().AmountDiscount = originalDiscounts
		require.NoError(t, common.UpdateTopupGroupRatioByJSONString(originalGroupRatios))
	})

	setting.StripeMinTopUp = 1
	require.NoError(t, common.UpdateTopupGroupRatioByJSONString(`{"default":1,"vip":1.2}`))

	testCases := []struct {
		name            string
		displayType     string
		amount          int64
		group           string
		unitPrice       float64
		discount        float64
		currency        stripe.Currency
		wantCredit      int64
		wantMoney       float64
		wantUnitAmount  int64
		wantDisplayText string
	}{
		{
			name:            "USD display applies group ratio and preset discount",
			displayType:     operation_setting.QuotaDisplayTypeUSD,
			amount:          10,
			group:           "vip",
			unitPrice:       2.5,
			discount:        0.8,
			currency:        stripe.CurrencyUSD,
			wantCredit:      10,
			wantMoney:       24,
			wantUnitAmount:  2400,
			wantDisplayText: "24.00",
		},
		{
			name:            "CNY display uses the configured gateway unit price",
			displayType:     operation_setting.QuotaDisplayTypeCNY,
			amount:          10,
			group:           "vip",
			unitPrice:       7.3,
			discount:        0.8,
			currency:        "cny",
			wantCredit:      10,
			wantMoney:       70.08,
			wantUnitAmount:  7008,
			wantDisplayText: "70.08",
		},
		{
			name:            "token display converts tokens to the same base credit amount",
			displayType:     operation_setting.QuotaDisplayTypeTokens,
			amount:          int64(common.QuotaPerUnit * 3),
			group:           "vip",
			unitPrice:       2.5,
			discount:        0.8,
			currency:        stripe.CurrencyUSD,
			wantCredit:      3,
			wantMoney:       7.2,
			wantUnitAmount:  720,
			wantDisplayText: "7.20",
		},
		{
			name:            "minor units round once before display and checkout",
			displayType:     operation_setting.QuotaDisplayTypeUSD,
			amount:          1,
			group:           "default",
			unitPrice:       1.005,
			currency:        stripe.CurrencyUSD,
			wantCredit:      1,
			wantMoney:       1.01,
			wantUnitAmount:  101,
			wantDisplayText: "1.01",
		},
		{
			name:            "zero decimal currency has no hidden fractional amount",
			displayType:     operation_setting.QuotaDisplayTypeUSD,
			amount:          1,
			group:           "default",
			unitPrice:       123.6,
			currency:        stripe.CurrencyJPY,
			wantCredit:      1,
			wantMoney:       124,
			wantUnitAmount:  124,
			wantDisplayText: "124",
		},
		{
			name:            "special whole amount currency keeps Stripe two decimal encoding",
			displayType:     operation_setting.QuotaDisplayTypeUSD,
			amount:          1,
			group:           "default",
			unitPrice:       123.6,
			currency:        "isk",
			wantCredit:      1,
			wantMoney:       124,
			wantUnitAmount:  12400,
			wantDisplayText: "124",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			setting.StripeUnitPrice = tc.unitPrice
			operation_setting.GetGeneralSetting().QuotaDisplayType = tc.displayType
			operation_setting.GetPaymentSetting().AmountDiscount = map[int]float64{}
			if tc.discount > 0 {
				operation_setting.GetPaymentSetting().AmountDiscount[int(tc.amount)] = tc.discount
			}

			quote, err := calculateStripeTopUpQuote(tc.amount, tc.group, tc.currency)
			require.NoError(t, err)
			assert.Equal(t, tc.wantCredit, quote.creditAmount)
			assert.Equal(t, tc.wantUnitAmount, quote.unitAmount)
			assert.InDelta(t, tc.wantMoney, quote.money, 0.000001)
			assert.Equal(t, tc.wantDisplayText, formatStripeMoney(quote))
		})
	}
}

func TestCalculateStripeTopUpQuoteRejectsUnsafeAmounts(t *testing.T) {
	originalUnitPrice := setting.StripeUnitPrice
	originalMinTopUp := setting.StripeMinTopUp
	originalDisplayType := operation_setting.GetGeneralSetting().QuotaDisplayType
	t.Cleanup(func() {
		setting.StripeUnitPrice = originalUnitPrice
		setting.StripeMinTopUp = originalMinTopUp
		operation_setting.GetGeneralSetting().QuotaDisplayType = originalDisplayType
	})

	setting.StripeMinTopUp = 1
	setting.StripeUnitPrice = 1
	operation_setting.GetGeneralSetting().QuotaDisplayType = operation_setting.QuotaDisplayTypeTokens
	_, err := calculateStripeTopUpQuote(int64(common.QuotaPerUnit)+1, "default", stripe.CurrencyUSD)
	require.ErrorContains(t, err, "完整")

	operation_setting.GetGeneralSetting().QuotaDisplayType = operation_setting.QuotaDisplayTypeUSD
	setting.StripeUnitPrice = 100_000
	_, err = calculateStripeTopUpQuote(maxStripeTopUpUSD, "default", stripe.CurrencyUSD)
	require.ErrorContains(t, err, "过高")
}

func TestStripeTopUpCheckoutUsesExactInlinePrice(t *testing.T) {
	originalPromotionCodes := setting.StripePromotionCodesEnabled
	t.Cleanup(func() { setting.StripePromotionCodesEnabled = originalPromotionCodes })
	setting.StripePromotionCodesEnabled = true

	params := newStripeTopUpCheckoutParams(
		"MO1TST00000000000000000000000000",
		"cus_123",
		"ignored@example.com",
		stripeTopUpPriceConfig{productID: "prod_topup", currency: stripe.CurrencyUSD},
		2400,
		"https://example.com/success",
		"https://example.com/cancel",
	)

	require.NotNil(t, params.Mode)
	assert.Equal(t, string(stripe.CheckoutSessionModePayment), *params.Mode)
	require.Len(t, params.LineItems, 1)
	lineItem := params.LineItems[0]
	assert.Nil(t, lineItem.Price)
	require.NotNil(t, lineItem.PriceData)
	assert.Equal(t, "usd", *lineItem.PriceData.Currency)
	assert.Equal(t, "prod_topup", *lineItem.PriceData.Product)
	assert.EqualValues(t, 2400, *lineItem.PriceData.UnitAmount)
	assert.EqualValues(t, 1, *lineItem.Quantity)
	require.NotNil(t, params.AllowPromotionCodes)
	assert.True(t, *params.AllowPromotionCodes)
	assert.Equal(t, "cus_123", *params.Customer)
	assert.Nil(t, params.CustomerEmail)
}

func TestStripeSubscriptionCheckoutIsOneTimePayment(t *testing.T) {
	params := newStripeSubscriptionCheckoutParams(
		"MO1SST00000000000000000000000000",
		"",
		"buyer@example.com",
		"price_one_time",
	)

	require.NotNil(t, params.Mode)
	assert.Equal(t, string(stripe.CheckoutSessionModePayment), *params.Mode)
	require.Len(t, params.LineItems, 1)
	assert.Equal(t, "price_one_time", *params.LineItems[0].Price)
	assert.EqualValues(t, 1, *params.LineItems[0].Quantity)
	assert.Nil(t, params.LineItems[0].PriceData)
	require.NotNil(t, params.CustomerCreation)
	assert.Equal(t, string(stripe.CheckoutSessionCustomerCreationAlways), *params.CustomerCreation)
	assert.Equal(t, "buyer@example.com", *params.CustomerEmail)
}

func TestValidateStripePaymentFactsBindsSessionSubtotalAndDiscountedTotal(t *testing.T) {
	tradeNo := "MO1TST00000000000000000000000000"
	event := stripe.Event{Data: &stripe.EventData{Object: map[string]interface{}{
		"id":                  "cs_123",
		"client_reference_id": tradeNo,
		"status":              "complete",
		"payment_status":      "paid",
		"mode":                "payment",
		"currency":            "usd",
		"amount_subtotal":     int64(2400),
		"amount_total":        int64(2000),
	}}}

	currency, err := validateStripePaymentFacts(tradeNo, "cs_123", "USD", stripePaymentPromoMode, 24, event)
	require.NoError(t, err)
	assert.Equal(t, "USD", currency)

	_, err = validateStripePaymentFacts(tradeNo, "cs_123", "USD", stripePaymentMode, 24, event)
	require.ErrorContains(t, err, "实付")

	tests := []struct {
		name   string
		mutate func(map[string]interface{})
	}{
		{name: "session", mutate: func(object map[string]interface{}) { object["id"] = "cs_other" }},
		{name: "order", mutate: func(object map[string]interface{}) { object["client_reference_id"] = "other" }},
		{name: "currency", mutate: func(object map[string]interface{}) { object["currency"] = "eur" }},
		{name: "subtotal", mutate: func(object map[string]interface{}) { object["amount_subtotal"] = int64(2399) }},
		{name: "zero total", mutate: func(object map[string]interface{}) { object["amount_total"] = int64(0) }},
		{name: "over total", mutate: func(object map[string]interface{}) { object["amount_total"] = int64(2401) }},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			object := map[string]interface{}{}
			for key, value := range event.Data.Object {
				object[key] = value
			}
			testCase.mutate(object)
			copyEvent := stripe.Event{Data: &stripe.EventData{Object: object}}
			_, err := validateStripePaymentFacts(tradeNo, "cs_123", "USD", stripePaymentPromoMode, 24, copyEvent)
			require.Error(t, err)
		})
	}
}

func TestValidateStripePaymentFactsKeepsLegacyPendingOrdersCompatible(t *testing.T) {
	event := stripe.Event{Data: &stripe.EventData{Object: map[string]interface{}{
		"id":                  "cs_legacy",
		"client_reference_id": "legacy-order",
		"status":              "complete",
		"payment_status":      "paid",
		"mode":                "payment",
		"currency":            "usd",
		"amount_subtotal":     int64(100),
		"amount_total":        int64(100),
	}}}
	currency, err := validateStripePaymentFacts("legacy-order", "", "", "", 1, event)
	require.NoError(t, err)
	assert.Equal(t, "USD", currency)

	event.Data.Object["amount_subtotal"] = int64(99)
	event.Data.Object["amount_total"] = int64(99)
	_, err = validateStripePaymentFacts("legacy-order", "", "", "", 1, event)
	require.ErrorContains(t, err, "小计")
}
