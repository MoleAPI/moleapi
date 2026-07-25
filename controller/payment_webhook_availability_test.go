package controller

import (
	"testing"

	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/stretchr/testify/require"
)

func confirmPaymentComplianceForTest(t *testing.T) {
	t.Helper()
	paymentSetting := operation_setting.GetPaymentSetting()
	originalConfirmed := paymentSetting.ComplianceConfirmed
	originalTermsVersion := paymentSetting.ComplianceTermsVersion
	t.Cleanup(func() {
		paymentSetting.ComplianceConfirmed = originalConfirmed
		paymentSetting.ComplianceTermsVersion = originalTermsVersion
	})
	paymentSetting.ComplianceConfirmed = true
	paymentSetting.ComplianceTermsVersion = operation_setting.CurrentComplianceTermsVersion
}

func TestStripeWebhookRemainsEnabledForPendingOrders(t *testing.T) {
	confirmPaymentComplianceForTest(t)
	originalAPISecret := setting.StripeApiSecret
	originalWebhookSecret := setting.StripeWebhookSecret
	originalPriceID := setting.StripePriceId
	t.Cleanup(func() {
		setting.StripeApiSecret = originalAPISecret
		setting.StripeWebhookSecret = originalWebhookSecret
		setting.StripePriceId = originalPriceID
	})

	setting.StripeWebhookSecret = ""
	setting.StripeApiSecret = "sk_test_123"
	setting.StripePriceId = "price_123"
	require.False(t, isStripeWebhookEnabled())

	setting.StripeWebhookSecret = "whsec_test"
	require.True(t, isStripeWebhookEnabled())

	setting.StripePriceId = ""
	require.True(t, isStripeWebhookEnabled())
	setting.StripeApiSecret = ""
	require.True(t, isStripeWebhookEnabled())
}

func TestCreemWebhookRemainsEnabledForPendingOrders(t *testing.T) {
	confirmPaymentComplianceForTest(t)
	originalAPIKey := setting.CreemApiKey
	originalEnabled := setting.CreemEnabled
	originalProducts := setting.CreemProducts
	originalWebhookSecret := setting.CreemWebhookSecret
	t.Cleanup(func() {
		setting.CreemApiKey = originalAPIKey
		setting.CreemEnabled = originalEnabled
		setting.CreemProducts = originalProducts
		setting.CreemWebhookSecret = originalWebhookSecret
	})

	setting.CreemEnabled = true
	setting.CreemWebhookSecret = ""
	setting.CreemApiKey = "creem_api_key"
	setting.CreemProducts = `[{"productId":"prod_123"}]`
	require.False(t, isCreemWebhookEnabled())

	setting.CreemWebhookSecret = "creem_secret"
	require.True(t, isCreemWebhookEnabled())

	setting.CreemProducts = "[]"
	require.True(t, isCreemWebhookEnabled())
	setting.CreemApiKey = ""
	require.True(t, isCreemWebhookEnabled())
}

func TestCreemTopUpCanBeDisabledWithoutDisablingWebhook(t *testing.T) {
	confirmPaymentComplianceForTest(t)
	originalAPIKey := setting.CreemApiKey
	originalEnabled := setting.CreemEnabled
	originalProducts := setting.CreemProducts
	originalWebhookSecret := setting.CreemWebhookSecret
	t.Cleanup(func() {
		setting.CreemApiKey = originalAPIKey
		setting.CreemEnabled = originalEnabled
		setting.CreemProducts = originalProducts
		setting.CreemWebhookSecret = originalWebhookSecret
	})

	setting.CreemEnabled = true
	setting.CreemWebhookSecret = "creem_secret"
	setting.CreemApiKey = "creem_api_key"
	setting.CreemProducts = `[{"productId":"prod_123"}]`
	require.True(t, isCreemTopUpEnabled())
	require.True(t, isCreemWebhookEnabled())

	setting.CreemEnabled = false
	require.False(t, isCreemTopUpEnabled())
	require.True(t, isCreemWebhookEnabled())
}

func TestLanTuWebhookRemainsEnabledWhenNewSalesAreDisabled(t *testing.T) {
	confirmPaymentComplianceForTest(t)
	originalEnabled := setting.LantuEnabled
	originalMchID := setting.LantuMchID
	originalSecretKey := setting.LantuSecretKey
	t.Cleanup(func() {
		setting.LantuEnabled = originalEnabled
		setting.LantuMchID = originalMchID
		setting.LantuSecretKey = originalSecretKey
	})

	setting.LantuEnabled = true
	setting.LantuMchID = "merchant"
	setting.LantuSecretKey = "secret"
	require.True(t, isLanTuTopUpEnabled())
	require.True(t, isLanTuWebhookEnabled())

	setting.LantuEnabled = false
	require.False(t, isLanTuTopUpEnabled())
	require.True(t, isLanTuWebhookEnabled())
}

func TestWaffoWebhookRemainsEnabledWhenNewSalesAreDisabled(t *testing.T) {
	confirmPaymentComplianceForTest(t)
	originalEnabled := setting.WaffoEnabled
	originalMerchantID := setting.WaffoMerchantId
	originalSandbox := setting.WaffoSandbox
	originalAPIKey := setting.WaffoApiKey
	originalPrivateKey := setting.WaffoPrivateKey
	originalPublicCert := setting.WaffoPublicCert
	originalSandboxAPIKey := setting.WaffoSandboxApiKey
	originalSandboxPrivateKey := setting.WaffoSandboxPrivateKey
	originalSandboxPublicCert := setting.WaffoSandboxPublicCert
	t.Cleanup(func() {
		setting.WaffoEnabled = originalEnabled
		setting.WaffoMerchantId = originalMerchantID
		setting.WaffoSandbox = originalSandbox
		setting.WaffoApiKey = originalAPIKey
		setting.WaffoPrivateKey = originalPrivateKey
		setting.WaffoPublicCert = originalPublicCert
		setting.WaffoSandboxApiKey = originalSandboxAPIKey
		setting.WaffoSandboxPrivateKey = originalSandboxPrivateKey
		setting.WaffoSandboxPublicCert = originalSandboxPublicCert
	})

	setting.WaffoEnabled = true
	setting.WaffoMerchantId = "merchant"
	setting.WaffoSandbox = false
	setting.WaffoApiKey = ""
	setting.WaffoPrivateKey = "private"
	setting.WaffoPublicCert = "public"
	require.False(t, isWaffoWebhookEnabled())

	setting.WaffoApiKey = "api"
	require.True(t, isWaffoWebhookEnabled())

	setting.WaffoEnabled = false
	require.True(t, isWaffoWebhookEnabled())

	setting.WaffoEnabled = true
	setting.WaffoSandbox = true
	setting.WaffoSandboxApiKey = ""
	setting.WaffoSandboxPrivateKey = "sandbox_private"
	setting.WaffoSandboxPublicCert = "sandbox_public"
	require.False(t, isWaffoWebhookEnabled())

	setting.WaffoSandboxApiKey = "sandbox_api"
	require.True(t, isWaffoWebhookEnabled())
}

func TestWaffoPancakeWebhookRequiresConfiguredStoreOnly(t *testing.T) {
	confirmPaymentComplianceForTest(t)
	originalMerchantID := setting.WaffoPancakeMerchantID
	originalPrivateKey := setting.WaffoPancakePrivateKey
	originalProductID := setting.WaffoPancakeProductID
	originalStoreID := setting.WaffoPancakeStoreID
	t.Cleanup(func() {
		setting.WaffoPancakeMerchantID = originalMerchantID
		setting.WaffoPancakePrivateKey = originalPrivateKey
		setting.WaffoPancakeProductID = originalProductID
		setting.WaffoPancakeStoreID = originalStoreID
	})

	setting.WaffoPancakeStoreID = ""
	setting.WaffoPancakeMerchantID = "merchant"
	setting.WaffoPancakePrivateKey = "private"
	setting.WaffoPancakeProductID = "product"
	require.False(t, isWaffoPancakeWebhookEnabled())
	require.False(t, isWaffoPancakeTopUpEnabled())

	setting.WaffoPancakeStoreID = "store"
	require.True(t, isWaffoPancakeWebhookEnabled())
	require.True(t, isWaffoPancakeTopUpEnabled())

	setting.WaffoPancakeProductID = ""
	require.True(t, isWaffoPancakeWebhookEnabled())
	require.False(t, isWaffoPancakeTopUpEnabled())

	setting.WaffoPancakePrivateKey = ""
	require.True(t, isWaffoPancakeWebhookEnabled())
}

func TestEpayWebhookRemainsEnabledWhenNewSalesAreDisabled(t *testing.T) {
	confirmPaymentComplianceForTest(t)
	originalPayAddress := operation_setting.PayAddress
	originalEpayID := operation_setting.EpayId
	originalEpayKey := operation_setting.EpayKey
	originalPayMethods := operation_setting.PayMethods
	t.Cleanup(func() {
		operation_setting.PayAddress = originalPayAddress
		operation_setting.EpayId = originalEpayID
		operation_setting.EpayKey = originalEpayKey
		operation_setting.PayMethods = originalPayMethods
	})

	operation_setting.PayAddress = "https://pay.example.com"
	operation_setting.EpayId = "epay_id"
	operation_setting.EpayKey = ""
	operation_setting.PayMethods = []map[string]string{{"type": "alipay"}}
	require.False(t, isEpayWebhookEnabled())

	operation_setting.EpayKey = "epay_key"
	require.True(t, isEpayWebhookEnabled())
	require.True(t, isEpayTopUpEnabled())

	operation_setting.PayAddress = ""
	require.True(t, isEpayWebhookEnabled())
	require.False(t, isEpayTopUpEnabled())

	operation_setting.PayMethods = nil
	require.True(t, isEpayWebhookEnabled())
}
