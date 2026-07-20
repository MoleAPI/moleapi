package model

import (
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type legacySubscriptionOrder struct {
	Id              int    `gorm:"primaryKey"`
	TradeNo         string `gorm:"unique;type:varchar(255);index"`
	PaymentProvider string `gorm:"type:varchar(50);default:''"`
}

func (legacySubscriptionOrder) TableName() string {
	return "subscription_orders"
}

func TestSubscriptionOrderAutoMigrationPreservesLegacyOrders(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&legacySubscriptionOrder{}))
	require.NoError(t, db.Create(&legacySubscriptionOrder{
		TradeNo:         "legacy-subscription-order",
		PaymentProvider: PaymentProviderStripe,
	}).Error)

	require.NoError(t, db.AutoMigrate(&SubscriptionOrder{}))
	require.NoError(t, db.AutoMigrate(&SubscriptionOrder{}))
	for _, field := range []string{"GatewayTradeNo", "PaymentProductId", "PaymentMode", "PaymentCurrency", "PlanSnapshot"} {
		assert.True(t, db.Migrator().HasColumn(&SubscriptionOrder{}, field))
	}
	assert.True(t, db.Migrator().HasIndex(&SubscriptionOrder{}, "GatewayTradeNo"))

	var stored SubscriptionOrder
	require.NoError(t, db.Where("trade_no = ?", "legacy-subscription-order").First(&stored).Error)
	assert.Empty(t, stored.GatewayTradeNo)
	assert.Empty(t, stored.PaymentProductId)
	assert.Empty(t, stored.PaymentMode)
	assert.Empty(t, stored.PaymentCurrency)
	assert.Empty(t, stored.PlanSnapshot)
}
