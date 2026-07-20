package controller

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/Calcium-Ion/go-epay/epay"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupTopUpWebhookSettlementTest(t *testing.T) *gorm.DB {
	t.Helper()
	confirmPaymentComplianceForTest(t)

	originalDB := model.DB
	originalLogDB := model.LOG_DB
	originalMainDatabaseType := common.MainDatabaseType()
	originalLogDatabaseType := common.LogDatabaseType()
	originalRedisEnabled := common.RedisEnabled
	originalQuotaPerUnit := common.QuotaPerUnit
	originalPayAddress := operation_setting.PayAddress
	originalEpayID := operation_setting.EpayId
	originalEpayKey := operation_setting.EpayKey
	originalPayMethods := operation_setting.PayMethods

	db, err := gorm.Open(sqlite.Open("file:"+url.QueryEscape(t.Name())+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)

	model.DB = db
	model.LOG_DB = db
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	common.RedisEnabled = false
	common.QuotaPerUnit = 100
	operation_setting.PayAddress = "https://payment.example.com"
	operation_setting.EpayId = "partner-test"
	operation_setting.EpayKey = "secret-test"
	operation_setting.PayMethods = []map[string]string{{"type": "alipay"}}
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.TopUp{}, &model.Log{}, &model.SubscriptionOrder{}))

	t.Cleanup(func() {
		model.DB = originalDB
		model.LOG_DB = originalLogDB
		common.SetDatabaseTypes(originalMainDatabaseType, originalLogDatabaseType)
		common.RedisEnabled = originalRedisEnabled
		common.QuotaPerUnit = originalQuotaPerUnit
		operation_setting.PayAddress = originalPayAddress
		operation_setting.EpayId = originalEpayID
		operation_setting.EpayKey = originalEpayKey
		operation_setting.PayMethods = originalPayMethods
		_ = sqlDB.Close()
	})
	return db
}

func runSignedEpayNotify(t *testing.T, tradeNo string, tradeStatus string, money string) *httptest.ResponseRecorder {
	return runSignedEpayNotifyWithPID(t, tradeNo, tradeStatus, money, operation_setting.EpayId)
}

func runSignedEpayNotifyWithPID(t *testing.T, tradeNo string, tradeStatus string, money string, pid string) *httptest.ResponseRecorder {
	t.Helper()
	params := epay.GenerateParams(map[string]string{
		"pid":          pid,
		"type":         "alipay",
		"trade_no":     "epay-gateway-123",
		"out_trade_no": tradeNo,
		"name":         "TUC1",
		"money":        money,
		"trade_status": tradeStatus,
	}, operation_setting.EpayKey)
	query := url.Values{}
	for key, value := range params {
		query.Set(key, value)
	}

	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodGet, "/api/user/epay/notify?"+query.Encode(), nil)
	context.Request.RemoteAddr = "203.0.113.70:1234"
	EpayNotify(context)
	return recorder
}

func TestEpayNotifyRejectsSignedCallbackForAnotherMerchant(t *testing.T) {
	db := setupTopUpWebhookSettlementTest(t)
	user := &model.User{Id: 604, Username: "epay_merchant_user", Password: "password123", Status: common.UserStatusEnabled}
	require.NoError(t, db.Create(user).Error)
	topUp := &model.TopUp{
		UserId:          user.Id,
		Amount:          1,
		Money:           1,
		TradeNo:         "epay-merchant-mismatch",
		PaymentMethod:   "alipay",
		PaymentProvider: model.PaymentProviderEpay,
		Status:          common.TopUpStatusPending,
	}
	require.NoError(t, db.Create(topUp).Error)

	response := runSignedEpayNotifyWithPID(t, topUp.TradeNo, epay.StatusTradeSuccess, "1.00", "another-merchant")
	assert.Equal(t, "fail", response.Body.String())
	require.NoError(t, db.First(topUp, topUp.Id).Error)
	assert.Equal(t, common.TopUpStatusPending, topUp.Status)
}

func TestEpayNotifySettlesPendingOrderAfterCheckoutAddressIsRemoved(t *testing.T) {
	db := setupTopUpWebhookSettlementTest(t)
	user := &model.User{Id: 605, Username: "epay_delayed_user", Password: "password123", Status: common.UserStatusEnabled}
	require.NoError(t, db.Create(user).Error)
	topUp := &model.TopUp{
		UserId:          user.Id,
		Amount:          1,
		Money:           1,
		TradeNo:         "epay-delayed-callback",
		PaymentMethod:   "alipay",
		PaymentProvider: model.PaymentProviderEpay,
		Status:          common.TopUpStatusPending,
	}
	require.NoError(t, db.Create(topUp).Error)
	operation_setting.PayAddress = ""

	response := runSignedEpayNotify(t, topUp.TradeNo, epay.StatusTradeSuccess, "1.00")
	assert.Equal(t, "success", response.Body.String())
	require.NoError(t, db.First(topUp, topUp.Id).Error)
	assert.Equal(t, common.TopUpStatusSuccess, topUp.Status)
}

func TestEpayNotifyAcknowledgesSuccessfulTradeOnlyAfterAtomicSettlement(t *testing.T) {
	db := setupTopUpWebhookSettlementTest(t)
	user := &model.User{
		Id:       601,
		Username: "epay_settlement_user",
		Password: "password123",
		Status:   common.UserStatusEnabled,
		Quota:    common.MaxQuota - 50,
	}
	require.NoError(t, db.Create(user).Error)
	topUp := &model.TopUp{
		UserId:          user.Id,
		Amount:          1,
		Money:           1,
		TradeNo:         "epay-atomic-settlement",
		PaymentMethod:   "wechat",
		PaymentProvider: model.PaymentProviderEpay,
		CreateTime:      1,
		Status:          common.TopUpStatusPending,
	}
	require.NoError(t, db.Create(topUp).Error)

	nonSuccess := runSignedEpayNotify(t, topUp.TradeNo, "WAIT_BUYER_PAY", "1.00")
	assert.Equal(t, "success", nonSuccess.Body.String())

	overflow := runSignedEpayNotify(t, topUp.TradeNo, epay.StatusTradeSuccess, "1.00")
	assert.Equal(t, "fail", overflow.Body.String())

	var storedTopUp model.TopUp
	require.NoError(t, db.Where("trade_no = ?", topUp.TradeNo).First(&storedTopUp).Error)
	assert.Equal(t, common.TopUpStatusPending, storedTopUp.Status)
	assert.Zero(t, storedTopUp.CreditedQuota)
	assert.Zero(t, storedTopUp.CompleteTime)
	assert.Empty(t, storedTopUp.GatewayTradeNo)
	assert.Equal(t, "wechat", storedTopUp.PaymentMethod)

	var storedUser model.User
	require.NoError(t, db.Select("id", "quota").First(&storedUser, user.Id).Error)
	assert.Equal(t, common.MaxQuota-50, storedUser.Quota)

	require.NoError(t, db.Model(&model.User{}).Where("id = ?", user.Id).Update("quota", 10).Error)
	settled := runSignedEpayNotify(t, topUp.TradeNo, epay.StatusTradeSuccess, "1.00")
	assert.Equal(t, "success", settled.Body.String())
	repeated := runSignedEpayNotify(t, topUp.TradeNo, epay.StatusTradeSuccess, "1.00")
	assert.Equal(t, "success", repeated.Body.String())

	require.NoError(t, db.Where("trade_no = ?", topUp.TradeNo).First(&storedTopUp).Error)
	assert.Equal(t, common.TopUpStatusSuccess, storedTopUp.Status)
	assert.Equal(t, 100, storedTopUp.CreditedQuota)
	assert.Positive(t, storedTopUp.CompleteTime)
	assert.Equal(t, "epay-gateway-123", storedTopUp.GatewayTradeNo)
	assert.Equal(t, "alipay", storedTopUp.PaymentMethod)

	require.NoError(t, db.Select("id", "quota").First(&storedUser, user.Id).Error)
	assert.Equal(t, 110, storedUser.Quota)

	var logs []model.Log
	require.NoError(t, db.Where("user_id = ? AND type = ?", user.Id, model.LogTypeTopup).Find(&logs).Error)
	require.Len(t, logs, 1)
	assert.Equal(t, "203.0.113.70", logs[0].Ip)
	var other map[string]interface{}
	require.NoError(t, common.UnmarshalJsonStr(logs[0].Other, &other))
	adminInfo, ok := other["admin_info"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "alipay", adminInfo["payment_method"])
	assert.Equal(t, model.PaymentProviderEpay, adminInfo["callback_payment_method"])

	wrongProvider := &model.TopUp{
		UserId:          user.Id,
		Amount:          1,
		Money:           1,
		TradeNo:         "epay-provider-mismatch",
		PaymentMethod:   model.PaymentMethodStripe,
		PaymentProvider: model.PaymentProviderStripe,
		CreateTime:      1,
		Status:          common.TopUpStatusPending,
	}
	require.NoError(t, db.Create(wrongProvider).Error)
	mismatched := runSignedEpayNotify(t, wrongProvider.TradeNo, epay.StatusTradeSuccess, "1.00")
	assert.Equal(t, "fail", mismatched.Body.String())
	require.NoError(t, db.First(wrongProvider, wrongProvider.Id).Error)
	assert.Equal(t, common.TopUpStatusPending, wrongProvider.Status)
}

func TestEpayNotifyRejectsMismatchedPaymentAmount(t *testing.T) {
	db := setupTopUpWebhookSettlementTest(t)
	user := &model.User{
		Id:       602,
		Username: "epay_amount_user",
		Password: "password123",
		Status:   common.UserStatusEnabled,
		Quota:    10,
	}
	require.NoError(t, db.Create(user).Error)
	topUp := &model.TopUp{
		UserId:          user.Id,
		Amount:          1,
		Money:           1.25,
		TradeNo:         "epay-amount-mismatch",
		PaymentMethod:   "alipay",
		PaymentProvider: model.PaymentProviderEpay,
		CreateTime:      1,
		Status:          common.TopUpStatusPending,
	}
	require.NoError(t, db.Create(topUp).Error)

	response := runSignedEpayNotify(t, topUp.TradeNo, epay.StatusTradeSuccess, "1.00")
	assert.Equal(t, "fail", response.Body.String())

	require.NoError(t, db.First(topUp, topUp.Id).Error)
	assert.Equal(t, common.TopUpStatusPending, topUp.Status)
	assert.Zero(t, topUp.CreditedQuota)
	var storedUser model.User
	require.NoError(t, db.Select("id", "quota").First(&storedUser, user.Id).Error)
	assert.Equal(t, 10, storedUser.Quota)
}

func TestEpayNotifyUsesTheSameHalfCentFormattingAsPurchase(t *testing.T) {
	db := setupTopUpWebhookSettlementTest(t)
	user := &model.User{
		Id:       603,
		Username: "epay_rounding_user",
		Password: "password123",
		Status:   common.UserStatusEnabled,
		Quota:    10,
	}
	require.NoError(t, db.Create(user).Error)
	topUp := &model.TopUp{
		UserId:          user.Id,
		Amount:          1,
		Money:           1.005,
		TradeNo:         "epay-half-cent-rounding",
		PaymentMethod:   "alipay",
		PaymentProvider: model.PaymentProviderEpay,
		CreateTime:      1,
		Status:          common.TopUpStatusPending,
	}
	require.NoError(t, db.Create(topUp).Error)

	response := runSignedEpayNotify(t, topUp.TradeNo, epay.StatusTradeSuccess, "1.00")
	assert.Equal(t, "success", response.Body.String())

	require.NoError(t, db.First(topUp, topUp.Id).Error)
	assert.Equal(t, common.TopUpStatusSuccess, topUp.Status)
	assert.Equal(t, 100, topUp.CreditedQuota)
	var storedUser model.User
	require.NoError(t, db.Select("id", "quota").First(&storedUser, user.Id).Error)
	assert.Equal(t, 110, storedUser.Quota)
}
