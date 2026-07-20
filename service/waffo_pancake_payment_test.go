package service

import (
	"errors"
	"net/url"
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupWaffoPancakePaymentTest(t *testing.T) *gorm.DB {
	t.Helper()
	previousDB := model.DB
	previousStoreID := setting.WaffoPancakeStoreID
	previousEnvironment := setting.WaffoPancakeEnvironment
	db, err := gorm.Open(sqlite.Open("file:"+url.QueryEscape(t.Name())+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.TopUp{}, &model.SubscriptionOrder{}))
	model.DB = db
	setting.WaffoPancakeStoreID = "store_ours"
	setting.WaffoPancakeEnvironment = "prod"
	t.Cleanup(func() {
		model.DB = previousDB
		setting.WaffoPancakeStoreID = previousStoreID
		setting.WaffoPancakeEnvironment = previousEnvironment
	})
	return db
}

func TestResolveWaffoPancakeTradeNoValidatesStoreAmountAndCurrency(t *testing.T) {
	db := setupWaffoPancakePaymentTest(t)
	require.NoError(t, db.Create(&model.TopUp{
		UserId:          101,
		Money:           9.99,
		TradeNo:         "pancake-topup",
		PaymentProvider: model.PaymentProviderWaffoPancake,
		PaymentMode:     "prod",
	}).Error)
	event := &WaffoPancakeWebhookEvent{
		StoreID: "store_other",
		Mode:    "prod",
		Data: WaffoPancakeWebhookData{
			OrderMerchantExternalID:       "pancake-topup",
			MerchantProvidedBuyerIdentity: WaffoPancakeBuyerIdentityFromUserID(101),
			Amount:                        "9.99",
			Currency:                      "USD",
		},
	}

	_, err := ResolveWaffoPancakeTradeNo(event)
	assert.ErrorContains(t, err, "store mismatch")
	event.StoreID = "store_ours"
	event.Data.Amount = "0.01"
	_, err = ResolveWaffoPancakeTradeNo(event)
	assert.ErrorContains(t, err, "amount mismatch")
	event.Data.Amount = "9.99"
	event.Data.Currency = "EUR"
	_, err = ResolveWaffoPancakeTradeNo(event)
	assert.ErrorContains(t, err, "currency mismatch")
	event.Data.Currency = "usd"
	event.Mode = "test"
	_, err = ResolveWaffoPancakeTradeNo(event)
	assert.ErrorContains(t, err, "environment mismatch")
	event.Mode = "prod"
	tradeNo, err := ResolveWaffoPancakeTradeNo(event)
	require.NoError(t, err)
	assert.Equal(t, "pancake-topup", tradeNo)
}

func TestResolveWaffoPancakeSubscriptionTradeNoRejectsWrongStore(t *testing.T) {
	db := setupWaffoPancakePaymentTest(t)
	require.NoError(t, db.Create(&model.SubscriptionOrder{
		UserId:          202,
		Money:           19.9,
		TradeNo:         "pancake-subscription",
		PaymentProvider: model.PaymentProviderWaffoPancake,
		PaymentMode:     "prod",
	}).Error)
	event := &WaffoPancakeWebhookEvent{
		StoreID: "store_other",
		Mode:    "prod",
		Data: WaffoPancakeWebhookData{
			OrderMerchantExternalID:       "pancake-subscription",
			MerchantProvidedBuyerIdentity: WaffoPancakeBuyerIdentityFromUserID(202),
			Amount:                        "19.90",
			Currency:                      "USD",
		},
	}

	_, err := ResolveWaffoPancakeSubscriptionTradeNo(event)
	assert.ErrorContains(t, err, "store mismatch")
	assert.ErrorIs(t, err, ErrWaffoPancakePaymentRejected)
}

func TestResolveWaffoPancakeTradeNoPreservesDatabaseErrors(t *testing.T) {
	db := setupWaffoPancakePaymentTest(t)
	queryErr := errors.New("temporary database failure")
	require.NoError(t, db.Callback().Query().Before("gorm:query").Register("test:waffo-pancake-query-failure", func(tx *gorm.DB) {
		tx.AddError(queryErr)
	}))
	t.Cleanup(func() {
		require.NoError(t, db.Callback().Query().Remove("test:waffo-pancake-query-failure"))
	})

	_, err := ResolveWaffoPancakeTradeNo(&WaffoPancakeWebhookEvent{Data: WaffoPancakeWebhookData{
		OrderMerchantExternalID: "paid-order",
	}})
	require.ErrorIs(t, err, queryErr)
	assert.NotErrorIs(t, err, ErrWaffoPancakePaymentRejected)
}
