package controller

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFormatWaffoPancakeAmount_UsesDisplayPriceString(t *testing.T) {
	testCases := []struct {
		name     string
		amount   float64
		expected string
	}{
		{name: "whole amount", amount: 29, expected: "29.00"},
		{name: "decimal amount", amount: 29.9, expected: "29.90"},
		{name: "round half up to cents", amount: 29.999, expected: "30.00"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.expected, formatWaffoPancakeAmount(tc.amount))
		})
	}
}

func TestRespondWaffoPancakeResolutionErrorRetriesOnlyInternalFailures(t *testing.T) {
	gin.SetMode(gin.TestMode)
	event := &service.WaffoPancakeWebhookEvent{ID: "evt_1", Data: service.WaffoPancakeWebhookData{OrderID: "ord_1"}}

	tests := []struct {
		name       string
		err        error
		statusCode int
		body       string
	}{
		{name: "permanent rejection", err: fmt.Errorf("%w: mismatch", service.ErrWaffoPancakePaymentRejected), statusCode: http.StatusOK, body: "OK"},
		{name: "temporary database failure", err: errors.New("database unavailable"), statusCode: http.StatusInternalServerError, body: "retry"},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			context, _ := gin.CreateTestContext(recorder)
			context.Request = httptest.NewRequest(http.MethodPost, "/api/waffo-pancake/webhook/prod", nil)
			respondWaffoPancakeResolutionError(context, "充值订单", event, testCase.err)

			assert.Equal(t, testCase.statusCode, recorder.Code)
			assert.Equal(t, testCase.body, recorder.Body.String())
		})
	}
}

func TestGetWaffoPancakePayMoney(t *testing.T) {
	originalUnitPrice := setting.WaffoPancakeUnitPrice
	originalQuotaDisplayType := operation_setting.GetGeneralSetting().QuotaDisplayType
	originalDiscounts := make(map[int]float64, len(operation_setting.GetPaymentSetting().AmountDiscount))
	for k, v := range operation_setting.GetPaymentSetting().AmountDiscount {
		originalDiscounts[k] = v
	}
	originalTopupGroupRatio := common.TopupGroupRatio2JSONString()

	t.Cleanup(func() {
		setting.WaffoPancakeUnitPrice = originalUnitPrice
		operation_setting.GetGeneralSetting().QuotaDisplayType = originalQuotaDisplayType
		operation_setting.GetPaymentSetting().AmountDiscount = originalDiscounts
		require.NoError(t, common.UpdateTopupGroupRatioByJSONString(originalTopupGroupRatio))
	})

	setting.WaffoPancakeUnitPrice = 2.5
	operation_setting.GetPaymentSetting().AmountDiscount = map[int]float64{
		10:                           0.8,
		int(common.QuotaPerUnit * 2): 0.8,
		20:                           0,
	}
	require.NoError(t, common.UpdateTopupGroupRatioByJSONString(`{"default":1,"vip":1.2}`))

	testCases := []struct {
		name             string
		amount           int64
		group            string
		quotaDisplayType string
		expected         float64
	}{
		{
			name:             "currency display applies unit price group ratio and discount",
			amount:           10,
			group:            "vip",
			quotaDisplayType: operation_setting.QuotaDisplayTypeUSD,
			expected:         24,
		},
		{
			name:             "tokens display converts quota to display units before pricing",
			amount:           int64(common.QuotaPerUnit * 3),
			group:            "vip",
			quotaDisplayType: operation_setting.QuotaDisplayTypeTokens,
			expected:         7.2,
		},
		{
			name:             "non-positive discount falls back to no discount",
			amount:           20,
			group:            "default",
			quotaDisplayType: operation_setting.QuotaDisplayTypeUSD,
			expected:         50,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			operation_setting.GetGeneralSetting().QuotaDisplayType = tc.quotaDisplayType
			actual := getWaffoPancakePayMoney(tc.amount, tc.group)
			require.InDelta(t, tc.expected, actual, 0.000001)
		})
	}
}

func TestListWaffoPancakeCatalogNeverReadsCredentialsFromURL(t *testing.T) {
	gin.SetMode(gin.TestMode)
	originalMerchantID := setting.WaffoPancakeMerchantID
	originalPrivateKey := setting.WaffoPancakePrivateKey
	t.Cleanup(func() {
		setting.WaffoPancakeMerchantID = originalMerchantID
		setting.WaffoPancakePrivateKey = originalPrivateKey
	})
	setting.WaffoPancakeMerchantID = ""
	setting.WaffoPancakePrivateKey = ""

	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodPost, "/api/option/waffo-pancake/catalog?merchant_id=merchant&private_key=secret", nil)
	ListWaffoPancakeCatalog(context)

	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), "凭证未配置")
}

func TestListWaffoPancakeCatalogLimitsCredentialBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodPost, "/api/option/waffo-pancake/catalog", strings.NewReader(strings.Repeat("x", (16<<10)+1)))

	ListWaffoPancakeCatalog(context)

	assert.Equal(t, http.StatusRequestEntityTooLarge, recorder.Code)
}
