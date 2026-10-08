package model

import (
	"os"
	"strings"
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

type topUpWithoutInvoiceURL struct {
	Id                    int
	UserId                int `gorm:"index"`
	Amount                int64
	Money                 float64
	TradeNo               string `gorm:"unique;type:varchar(255);index"`
	GatewayTradeNo        string `gorm:"type:varchar(255);index;default:''"`
	PaymentProductId      string `gorm:"type:varchar(255);default:''"`
	PaymentMode           string `gorm:"type:varchar(16);default:''"`
	PromisedQuota         int    `gorm:"type:bigint;default:0"`
	CreditedQuota         int    `gorm:"type:bigint;default:0"`
	InviteRebateInviterId int    `gorm:"type:int;default:0;column:invite_rebate_inviter_id;index"`
	InviteRebateRatio     int    `gorm:"type:int;default:0;column:invite_rebate_ratio"`
	InviteRebateQuota     int    `gorm:"type:bigint;default:0;column:invite_rebate_quota"`
	PaymentCurrency       string `gorm:"type:varchar(8);default:''"`
	PaymentMethod         string `gorm:"type:varchar(50)"`
	PaymentProvider       string `gorm:"type:varchar(50);default:''"`
	CreateTime            int64
	CompleteTime          int64
	Status                string
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

	for _, field := range []string{"GatewayTradeNo", "PaymentProductId", "PaymentMode", "PromisedQuota", "CreditedQuota", "InviteRebateInviterId", "InviteRebateRatio", "InviteRebateQuota", "PaymentCurrency", "InvoiceURL"} {
		assert.False(t, db.Migrator().HasColumn(&TopUp{}, field))
	}

	require.NoError(t, db.AutoMigrate(&TopUp{}))
	require.NoError(t, db.AutoMigrate(&TopUp{}))

	for _, field := range []string{"GatewayTradeNo", "PaymentProductId", "PaymentMode", "PromisedQuota", "CreditedQuota", "InviteRebateInviterId", "InviteRebateRatio", "InviteRebateQuota", "PaymentCurrency", "InvoiceURL"} {
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
	assert.Empty(t, legacy.InvoiceURL)

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
		InvoiceURL:            "https://pancake.waffo.ai/invoice/PAY_test?token=test-token",
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
	assert.Equal(t, newRecord.InvoiceURL, stored.InvoiceURL)
}

func TestTopUpInvoiceURLDatabaseMatrix(t *testing.T) {
	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			var db *gorm.DB
			if dialect == "sqlite" {
				var err error
				db, err = gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
				require.NoError(t, err)
			} else {
				dsn := strings.TrimSpace(os.Getenv(map[string]string{
					"mysql": "TEST_MYSQL_DSN", "postgres": "TEST_POSTGRES_DSN",
				}[dialect]))
				if dsn == "" {
					t.Skip("test database DSN is not configured")
				}
				t.Setenv("TOPUP_MIGRATION_TEST_DSN", dsn)
				var err error
				db, _, err = chooseDB("TOPUP_MIGRATION_TEST_DSN", false)
				require.NoError(t, err)
			}
			sqlDB, err := db.DB()
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })

			const freshTable = "top_up_invoice_url_fresh_test"
			const upgradeTable = "top_up_invoice_url_upgrade_test"
			require.NoError(t, db.Migrator().DropTable(freshTable, upgradeTable))
			t.Cleanup(func() { _ = db.Migrator().DropTable(freshTable, upgradeTable) })

			invoiceURL := "https://pancake.waffo.ai/invoice/PAY_test?token=test-token"
			require.NoError(t, db.Table(freshTable).AutoMigrate(&TopUp{}))
			require.NoError(t, db.Table(freshTable).AutoMigrate(&TopUp{}))
			assert.True(t, db.Table(freshTable).Migrator().HasColumn(&TopUp{}, "InvoiceURL"))
			require.NoError(t, db.Table(freshTable).Create(&TopUp{TradeNo: "fresh-order", InvoiceURL: invoiceURL}).Error)
			var fresh TopUp
			require.NoError(t, db.Table(freshTable).Where("trade_no = ?", "fresh-order").First(&fresh).Error)
			assert.Equal(t, invoiceURL, fresh.InvoiceURL)

			require.NoError(t, db.Table(upgradeTable).AutoMigrate(&topUpWithoutInvoiceURL{}))
			legacy := &topUpWithoutInvoiceURL{UserId: 7, TradeNo: "upgrade-order", GatewayTradeNo: "gateway-order", PaymentProvider: PaymentProviderWaffoPancake, Status: common.TopUpStatusSuccess}
			require.NoError(t, db.Table(upgradeTable).Create(legacy).Error)
			assert.False(t, db.Table(upgradeTable).Migrator().HasColumn(&TopUp{}, "InvoiceURL"))
			require.NoError(t, db.Table(upgradeTable).AutoMigrate(&TopUp{}))
			require.NoError(t, db.Table(upgradeTable).AutoMigrate(&TopUp{}))
			assert.True(t, db.Table(upgradeTable).Migrator().HasColumn(&TopUp{}, "InvoiceURL"))
			assert.True(t, db.Table(upgradeTable).Migrator().HasIndex(&TopUp{}, "GatewayTradeNo"))
			var upgraded TopUp
			require.NoError(t, db.Table(upgradeTable).Where("trade_no = ?", legacy.TradeNo).First(&upgraded).Error)
			assert.Equal(t, legacy.GatewayTradeNo, upgraded.GatewayTradeNo)
			assert.Equal(t, legacy.PaymentProvider, upgraded.PaymentProvider)
			assert.Empty(t, upgraded.InvoiceURL)
			assert.Error(t, db.Table(upgradeTable).Create(&topUpWithoutInvoiceURL{TradeNo: legacy.TradeNo}).Error)
		})
	}
}
