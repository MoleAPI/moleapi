package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type legacyTopUpWithoutPaymentSnapshot struct {
	Id              int
	UserId          int
	Amount          int64
	Money           float64
	TradeNo         string `gorm:"unique;type:varchar(255);index"`
	PaymentMethod   string `gorm:"type:varchar(50)"`
	PaymentProvider string `gorm:"type:varchar(50);default:''"`
	CreateTime      int64
	CompleteTime    int64
	Status          string
}

func TestTopUpAutoMigrationExpandsExistingTableIdempotently(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.Table("top_ups").AutoMigrate(&legacyTopUpWithoutPaymentSnapshot{}))
	require.NoError(t, db.Table("top_ups").Create(&legacyTopUpWithoutPaymentSnapshot{
		UserId:          7,
		Amount:          12,
		Money:           6.9,
		TradeNo:         "legacy-topup",
		PaymentMethod:   "alipay",
		PaymentProvider: PaymentProviderEpay,
		CreateTime:      100,
		Status:          common.TopUpStatusSuccess,
	}).Error)

	for _, field := range []string{"GatewayTradeNo", "PaymentProductId", "PaymentMode", "PromisedQuota", "CreditedQuota", "InviteRebateInviterId", "InviteRebateRatio", "InviteRebateQuota", "PaymentCurrency"} {
		assert.False(t, db.Migrator().HasColumn(&TopUp{}, field))
	}

	require.NoError(t, db.AutoMigrate(&TopUp{}))
	require.NoError(t, db.AutoMigrate(&TopUp{}))

	for _, field := range []string{"GatewayTradeNo", "PaymentProductId", "PaymentMode", "PromisedQuota", "CreditedQuota", "InviteRebateInviterId", "InviteRebateRatio", "InviteRebateQuota", "PaymentCurrency"} {
		assert.True(t, db.Migrator().HasColumn(&TopUp{}, field))
	}
	assert.True(t, db.Migrator().HasIndex(&TopUp{}, "GatewayTradeNo"))

	var legacy TopUp
	require.NoError(t, db.Where("trade_no = ?", "legacy-topup").First(&legacy).Error)
	assert.Empty(t, legacy.GatewayTradeNo)
	assert.Empty(t, legacy.PaymentProductId)
	assert.Empty(t, legacy.PaymentMode)
	assert.Zero(t, legacy.PromisedQuota)
	assert.Zero(t, legacy.CreditedQuota)
	assert.Zero(t, legacy.InviteRebateInviterId)
	assert.Zero(t, legacy.InviteRebateRatio)
	assert.Zero(t, legacy.InviteRebateQuota)
	assert.Empty(t, legacy.PaymentCurrency)

	newRecord := &TopUp{
		UserId:                8,
		Amount:                20,
		Money:                 19.5,
		TradeNo:               "snapshot-topup",
		GatewayTradeNo:        "gateway-123",
		PaymentProductId:      "price-123",
		PaymentMode:           "payment",
		PromisedQuota:         10_000_000,
		CreditedQuota:         10_000_000,
		InviteRebateInviterId: 9,
		InviteRebateRatio:     100,
		InviteRebateQuota:     100_000,
		PaymentCurrency:       "USD",
		PaymentMethod:         PaymentMethodStripe,
		PaymentProvider:       PaymentProviderStripe,
		CreateTime:            200,
		CompleteTime:          300,
		Status:                common.TopUpStatusSuccess,
	}
	require.NoError(t, db.Create(newRecord).Error)

	var stored TopUp
	require.NoError(t, db.Where("trade_no = ?", newRecord.TradeNo).First(&stored).Error)
	assert.Equal(t, newRecord.GatewayTradeNo, stored.GatewayTradeNo)
	assert.Equal(t, newRecord.PaymentProductId, stored.PaymentProductId)
	assert.Equal(t, newRecord.PaymentMode, stored.PaymentMode)
	assert.Equal(t, newRecord.PromisedQuota, stored.PromisedQuota)
	assert.Equal(t, newRecord.CreditedQuota, stored.CreditedQuota)
	assert.Equal(t, newRecord.InviteRebateInviterId, stored.InviteRebateInviterId)
	assert.Equal(t, newRecord.InviteRebateRatio, stored.InviteRebateRatio)
	assert.Equal(t, newRecord.InviteRebateQuota, stored.InviteRebateQuota)
	assert.Equal(t, newRecord.PaymentCurrency, stored.PaymentCurrency)
}
