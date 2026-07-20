package controller

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
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

type lanTuRoundTripper func(*http.Request) (*http.Response, error)

func (roundTripper lanTuRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	return roundTripper(request)
}

func setupLanTuControllerTest(t *testing.T) *gorm.DB {
	t.Helper()
	db := setupTopUpWebhookSettlementTest(t)
	originalEnabled := setting.LantuEnabled
	originalMchID := setting.LantuMchID
	originalSecretKey := setting.LantuSecretKey
	originalMinTopUp := setting.LantuMinTopUp
	originalHTTPClient := lanTuHTTPClient
	originalServerAddress := system_setting.ServerAddress
	originalCallbackAddress := operation_setting.CustomCallbackAddress
	originalPrice := operation_setting.Price
	originalDisplayType := operation_setting.GetGeneralSetting().QuotaDisplayType
	originalTopUpRatio := common.TopupGroupRatio2JSONString()

	setting.LantuEnabled = true
	setting.LantuMchID = "merchant-test"
	setting.LantuSecretKey = "secret-test"
	setting.LantuMinTopUp = 1
	system_setting.ServerAddress = "https://app.example.com"
	operation_setting.CustomCallbackAddress = ""
	operation_setting.Price = 1
	operation_setting.GetGeneralSetting().QuotaDisplayType = operation_setting.QuotaDisplayTypeUSD
	require.NoError(t, common.UpdateTopupGroupRatioByJSONString(`{"default":1}`))
	lanTuQueryMu.Lock()
	lanTuLastQuery = map[string]time.Time{}
	lanTuQueryMu.Unlock()

	t.Cleanup(func() {
		setting.LantuEnabled = originalEnabled
		setting.LantuMchID = originalMchID
		setting.LantuSecretKey = originalSecretKey
		setting.LantuMinTopUp = originalMinTopUp
		lanTuHTTPClient = originalHTTPClient
		system_setting.ServerAddress = originalServerAddress
		operation_setting.CustomCallbackAddress = originalCallbackAddress
		operation_setting.Price = originalPrice
		operation_setting.GetGeneralSetting().QuotaDisplayType = originalDisplayType
		require.NoError(t, common.UpdateTopupGroupRatioByJSONString(originalTopUpRatio))
		lanTuQueryMu.Lock()
		lanTuLastQuery = map[string]time.Time{}
		lanTuQueryMu.Unlock()
	})
	return db
}

func installLanTuTransport(t *testing.T, roundTripper lanTuRoundTripper) {
	t.Helper()
	lanTuHTTPClient = &http.Client{Timeout: time.Second, Transport: roundTripper}
}

func lanTuHTTPResponse(t *testing.T, value any) *http.Response {
	t.Helper()
	payload, err := common.Marshal(value)
	require.NoError(t, err)
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(string(payload))),
	}
}

func runLanTuNotify(t *testing.T, tradeNo string, overrides map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	params := map[string]string{
		"code":         "0",
		"timestamp":    "1700000000",
		"mch_id":       setting.LantuMchID,
		"order_no":     "WX-SYSTEM-1",
		"out_trade_no": tradeNo,
		"pay_no":       "WX-CALLBACK-1",
		"total_fee":    "1.25",
	}
	payChannel := "wxpay"
	for key, value := range overrides {
		if key == "pay_channel" {
			payChannel = value
		} else {
			params[key] = value
		}
	}
	form := url.Values{}
	for key, value := range params {
		form.Set(key, value)
	}
	form.Set("pay_channel", payChannel)
	form.Set("sign", common.GenerateLanTuSignature(params, setting.LantuSecretKey))

	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodPost, "/api/user/lantu/notify", strings.NewReader(form.Encode()))
	context.Request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	context.Request.RemoteAddr = "203.0.113.73:1234"
	LanTuPayNotify(context)
	return recorder
}

func createLanTuTestUserAndTopUp(t *testing.T, db *gorm.DB, userID int, tradeNo string, provider string) {
	t.Helper()
	require.NoError(t, db.Create(&model.User{
		Id:       userID,
		Username: "lantu-user-" + tradeNo,
		Password: "password123",
		Group:    "default",
		Status:   common.UserStatusEnabled,
		Quota:    10,
	}).Error)
	require.NoError(t, db.Create(&model.TopUp{
		UserId:          userID,
		Amount:          1,
		Money:           1.25,
		TradeNo:         tradeNo,
		PaymentMethod:   model.PaymentMethodLanTu,
		PaymentProvider: provider,
		CreateTime:      time.Now().Unix(),
		Status:          common.TopUpStatusPending,
	}).Error)
}

func paidLanTuQueryResponse(tradeNo string, overrides map[string]any) map[string]any {
	data := map[string]any{
		"mch_id":       setting.LantuMchID,
		"out_trade_no": tradeNo,
		"pay_no":       "WX-QUERY-1",
		"total_fee":    "1.25",
		"pay_status":   1,
	}
	for key, value := range overrides {
		data[key] = value
	}
	return map[string]any{"code": 0, "data": data, "msg": "ok", "request_id": "request-1"}
}

func TestLanTuNotifySettlesLegacyPendingOrderOnce(t *testing.T) {
	db := setupLanTuControllerTest(t)
	createLanTuTestUserAndTopUp(t, db, 701, "legacy-lantu-callback", "")
	queryCount := 0
	installLanTuTransport(t, func(request *http.Request) (*http.Response, error) {
		queryCount++
		require.Equal(t, lanTuAPIBase+lanTuQueryPath, request.URL.String())
		require.NoError(t, request.ParseForm())
		signed := map[string]string{
			"mch_id":       request.Form.Get("mch_id"),
			"out_trade_no": request.Form.Get("out_trade_no"),
			"timestamp":    request.Form.Get("timestamp"),
		}
		assert.True(t, common.VerifyLanTuSignature(signed, request.Form.Get("sign"), setting.LantuSecretKey))
		return lanTuHTTPResponse(t, paidLanTuQueryResponse("legacy-lantu-callback", nil)), nil
	})

	assert.Equal(t, "SUCCESS", runLanTuNotify(t, "legacy-lantu-callback", nil).Body.String())
	assert.Equal(t, "SUCCESS", runLanTuNotify(t, "legacy-lantu-callback", nil).Body.String())
	assert.Equal(t, 1, queryCount)

	topUp := model.GetTopUpByTradeNo("legacy-lantu-callback")
	require.NotNil(t, topUp)
	assert.Equal(t, common.TopUpStatusSuccess, topUp.Status)
	assert.Equal(t, model.PaymentProviderLanTu, topUp.PaymentProvider)
	assert.Equal(t, "WX-QUERY-1", topUp.GatewayTradeNo)
	assert.Equal(t, "CNY", topUp.PaymentCurrency)
	assert.Equal(t, 100, topUp.CreditedQuota)
	var user model.User
	require.NoError(t, db.Select("quota").First(&user, 701).Error)
	assert.Equal(t, 110, user.Quota)
	var logCount int64
	require.NoError(t, db.Model(&model.Log{}).Where("user_id = ? AND type = ?", 701, model.LogTypeTopup).Count(&logCount).Error)
	assert.EqualValues(t, 1, logCount)
}

func TestLanTuNotifyRejectsMismatchedPaymentFacts(t *testing.T) {
	tests := []struct {
		name          string
		provider      string
		callback      map[string]string
		queryOverride map[string]any
		expectedQuery int
	}{
		{name: "callback code", provider: model.PaymentProviderLanTu, callback: map[string]string{"code": "1"}},
		{name: "merchant", provider: model.PaymentProviderLanTu, callback: map[string]string{"mch_id": "other-merchant"}},
		{name: "amount", provider: model.PaymentProviderLanTu, callback: map[string]string{"total_fee": "0.01"}},
		{name: "channel", provider: model.PaymentProviderLanTu, callback: map[string]string{"pay_channel": "alipay"}},
		{name: "local provider", provider: model.PaymentProviderStripe},
		{name: "query not paid", provider: model.PaymentProviderLanTu, queryOverride: map[string]any{"pay_status": 0}, expectedQuery: 1},
		{name: "query amount", provider: model.PaymentProviderLanTu, queryOverride: map[string]any{"total_fee": "0.01"}, expectedQuery: 1},
	}

	for index, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			db := setupLanTuControllerTest(t)
			tradeNo := "lantu-mismatch-" + test.name
			createLanTuTestUserAndTopUp(t, db, 720+index, tradeNo, test.provider)
			queryCount := 0
			installLanTuTransport(t, func(*http.Request) (*http.Response, error) {
				queryCount++
				return lanTuHTTPResponse(t, paidLanTuQueryResponse(tradeNo, test.queryOverride)), nil
			})

			assert.Equal(t, "FAIL", runLanTuNotify(t, tradeNo, test.callback).Body.String())
			assert.Equal(t, test.expectedQuery, queryCount)
			topUp := model.GetTopUpByTradeNo(tradeNo)
			require.NotNil(t, topUp)
			assert.Equal(t, common.TopUpStatusPending, topUp.Status)
			assert.Zero(t, topUp.CreditedQuota)
			var user model.User
			require.NoError(t, db.Select("quota").First(&user, 720+index).Error)
			assert.Equal(t, 10, user.Quota)
		})
	}
}

func TestLanTuNotifyAcknowledgesTerminalOrdersWithoutQuerying(t *testing.T) {
	tests := []struct {
		name   string
		status string
	}{
		{name: "failed", status: common.TopUpStatusFailed},
		{name: "expired", status: common.TopUpStatusExpired},
	}

	for index, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			db := setupLanTuControllerTest(t)
			tradeNo := "lantu-terminal-" + test.name
			userID := 735 + index
			createLanTuTestUserAndTopUp(t, db, userID, tradeNo, model.PaymentProviderLanTu)
			require.NoError(t, db.Model(&model.TopUp{}).Where("trade_no = ?", tradeNo).Update("status", test.status).Error)
			queryCount := 0
			installLanTuTransport(t, func(*http.Request) (*http.Response, error) {
				queryCount++
				return lanTuHTTPResponse(t, paidLanTuQueryResponse(tradeNo, nil)), nil
			})

			assert.Equal(t, "SUCCESS", runLanTuNotify(t, tradeNo, nil).Body.String())
			assert.Equal(t, "SUCCESS", runLanTuNotify(t, tradeNo, nil).Body.String())
			assert.Zero(t, queryCount)
			topUp := model.GetTopUpByTradeNo(tradeNo)
			require.NotNil(t, topUp)
			assert.Equal(t, test.status, topUp.Status)
			assert.Zero(t, topUp.CreditedQuota)
			var user model.User
			require.NoError(t, db.Select("quota").First(&user, userID).Error)
			assert.Equal(t, 10, user.Quota)
		})
	}
}

func TestLanTuQueryReservationIsScopedPerOrder(t *testing.T) {
	lanTuQueryMu.Lock()
	originalQueries := lanTuLastQuery
	lanTuLastQuery = map[string]time.Time{}
	lanTuQueryMu.Unlock()
	t.Cleanup(func() {
		lanTuQueryMu.Lock()
		lanTuLastQuery = originalQueries
		lanTuQueryMu.Unlock()
	})

	now := time.Now()
	start := make(chan struct{})
	results := make(chan bool, 2)
	for _, tradeNo := range []string{"lantu-order-a", "lantu-order-b"} {
		go func(tradeNo string) {
			<-start
			results <- reserveLanTuQuery(tradeNo, now)
		}(tradeNo)
	}
	close(start)
	assert.True(t, <-results)
	assert.True(t, <-results)
	assert.False(t, reserveLanTuQuery("lantu-order-a", now.Add(time.Second)))
	assert.True(t, reserveLanTuQuery("lantu-order-a", now.Add(lanTuQueryInterval)))
}

func TestRequestLanTuPayCreatesPendingOrderBeforeUpstreamAndMarksFailure(t *testing.T) {
	db := setupLanTuControllerTest(t)
	require.NoError(t, db.Create(&model.User{
		Id:       741,
		Username: "lantu-request-user",
		Password: "password123",
		Group:    "default",
		Status:   common.UserStatusEnabled,
	}).Error)
	sawPending := false
	installLanTuTransport(t, func(request *http.Request) (*http.Response, error) {
		require.Equal(t, lanTuAPIBase+lanTuJumpH5Path, request.URL.String())
		require.NoError(t, request.ParseForm())
		assert.Equal(t, "5m", request.Form.Get("time_expire"))
		assert.Equal(t, "https://app.example.com/wallet", request.Form.Get("return_url"))
		assert.Equal(t, "https://app.example.com/api/user/lantu/notify", request.Form.Get("notify_url"))
		tradeNo := request.Form.Get("out_trade_no")
		assert.Len(t, tradeNo, 32)
		assert.True(t, strings.HasPrefix(tradeNo, "MO1TLT"))
		pending := model.GetTopUpByTradeNo(tradeNo)
		sawPending = pending != nil && pending.Status == common.TopUpStatusPending
		signed := map[string]string{
			"mch_id":       request.Form.Get("mch_id"),
			"out_trade_no": tradeNo,
			"total_fee":    request.Form.Get("total_fee"),
			"body":         request.Form.Get("body"),
			"timestamp":    request.Form.Get("timestamp"),
			"notify_url":   request.Form.Get("notify_url"),
		}
		assert.True(t, common.VerifyLanTuSignature(signed, request.Form.Get("sign"), setting.LantuSecretKey))
		return lanTuHTTPResponse(t, map[string]any{"code": 1, "msg": "rejected", "request_id": "request-failed"}), nil
	})

	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Set("id", 741)
	context.Set("group", "default")
	context.Request = httptest.NewRequest(http.MethodPost, "/api/user/lantu/pay", strings.NewReader(`{"amount":2,"client":"h5"}`))
	context.Request.Header.Set("Content-Type", "application/json")
	RequestLanTuPay(context)

	assert.True(t, sawPending)
	assert.Contains(t, recorder.Body.String(), `"message":"error"`)
	var topUps []model.TopUp
	require.NoError(t, db.Where("user_id = ?", 741).Find(&topUps).Error)
	require.Len(t, topUps, 1)
	assert.Equal(t, common.TopUpStatusFailed, topUps[0].Status)
	assert.Equal(t, model.PaymentProviderLanTu, topUps[0].PaymentProvider)
}

func TestRequestLanTuPayRejectsPartialTokenUnitsBeforeCreatingOrder(t *testing.T) {
	db := setupLanTuControllerTest(t)
	operation_setting.GetGeneralSetting().QuotaDisplayType = operation_setting.QuotaDisplayTypeTokens
	queryCount := 0
	installLanTuTransport(t, func(*http.Request) (*http.Response, error) {
		queryCount++
		return lanTuHTTPResponse(t, map[string]any{"code": 0}), nil
	})

	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Set("id", 751)
	context.Request = httptest.NewRequest(http.MethodPost, "/api/user/lantu/pay", strings.NewReader(`{"amount":101,"client":"native"}`))
	context.Request.Header.Set("Content-Type", "application/json")
	RequestLanTuPay(context)

	assert.Contains(t, recorder.Body.String(), `"message":"error"`)
	assert.Zero(t, queryCount)
	var topUpCount int64
	require.NoError(t, db.Model(&model.TopUp{}).Count(&topUpCount).Error)
	assert.Zero(t, topUpCount)
}

func TestGetLanTuOrderStatusEnforcesOwnershipAndQueryBoundaries(t *testing.T) {
	db := setupLanTuControllerTest(t)
	createLanTuTestUserAndTopUp(t, db, 761, "lantu-status-pending", model.PaymentProviderLanTu)
	queryCount := 0
	installLanTuTransport(t, func(*http.Request) (*http.Response, error) {
		queryCount++
		return lanTuHTTPResponse(t, paidLanTuQueryResponse("lantu-status-pending", nil)), nil
	})

	runStatus := func(userID int, role int) *httptest.ResponseRecorder {
		recorder := httptest.NewRecorder()
		context, _ := gin.CreateTestContext(recorder)
		context.Set("id", userID)
		context.Set("role", role)
		context.Request = httptest.NewRequest(http.MethodGet, "/api/user/lantu/status?trade_no=lantu-status-pending", nil)
		GetLanTuOrderStatus(context)
		return recorder
	}

	assert.Contains(t, runStatus(762, common.RoleCommonUser).Body.String(), `"message":"error"`)
	assert.Equal(t, 0, queryCount)
	lanTuQueryMu.Lock()
	lanTuLastQuery["lantu-status-pending"] = time.Now()
	lanTuQueryMu.Unlock()
	assert.Contains(t, runStatus(761, common.RoleCommonUser).Body.String(), `"status":"pending"`)
	assert.Equal(t, 0, queryCount)
	assert.Contains(t, runStatus(999, common.RoleAdminUser).Body.String(), `"message":"success"`)
	assert.Equal(t, 0, queryCount)
}

func TestGetLanTuOrderStatusKeepsUnconfirmedOldOrderPending(t *testing.T) {
	db := setupLanTuControllerTest(t)
	tradeNo := "lantu-old-unconfirmed"
	createLanTuTestUserAndTopUp(t, db, 763, tradeNo, model.PaymentProviderLanTu)
	require.NoError(t, db.Model(&model.TopUp{}).Where("trade_no = ?", tradeNo).Update("create_time", time.Now().Add(-10*time.Minute).Unix()).Error)
	queryCount := 0
	installLanTuTransport(t, func(*http.Request) (*http.Response, error) {
		queryCount++
		return lanTuHTTPResponse(t, paidLanTuQueryResponse(tradeNo, map[string]any{"pay_status": 0})), nil
	})

	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Set("id", 763)
	context.Set("role", common.RoleCommonUser)
	context.Request = httptest.NewRequest(http.MethodGet, "/api/user/lantu/status?trade_no="+tradeNo, nil)
	GetLanTuOrderStatus(context)

	assert.Equal(t, 1, queryCount)
	assert.Contains(t, recorder.Body.String(), `"status":"pending"`)
	topUp := model.GetTopUpByTradeNo(tradeNo)
	require.NotNil(t, topUp)
	assert.Equal(t, common.TopUpStatusPending, topUp.Status)
}
