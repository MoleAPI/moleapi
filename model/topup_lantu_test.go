package model

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRechargeLanTuSettlesLegacyPendingOrderOnce(t *testing.T) {
	truncateTables(t)
	useQuotaPerUnitForTopUpTest(t, 100)
	insertUserForPaymentGuardTest(t, 541, 10)
	require.NoError(t, (&TopUp{
		UserId:        541,
		Amount:        1,
		Money:         1.25,
		TradeNo:       "legacy-lantu-pending",
		PaymentMethod: PaymentMethodLanTu,
		CreateTime:    time.Now().Unix(),
		Status:        common.TopUpStatusPending,
	}).Insert())

	require.NoError(t, RechargeLanTuWithPaymentDetails("legacy-lantu-pending", "wx-pay-1", "CNY", "203.0.113.71"))
	require.NoError(t, RechargeLanTuWithPaymentDetails("legacy-lantu-pending", "wx-pay-2", "USD", "203.0.113.72"))

	topUp := GetTopUpByTradeNo("legacy-lantu-pending")
	require.NotNil(t, topUp)
	assert.Equal(t, common.TopUpStatusSuccess, topUp.Status)
	assert.Equal(t, PaymentProviderLanTu, topUp.PaymentProvider)
	assert.Equal(t, "wx-pay-1", topUp.GatewayTradeNo)
	assert.Equal(t, "CNY", topUp.PaymentCurrency)
	assert.Equal(t, 100, topUp.CreditedQuota)
	assert.Equal(t, 110, getUserQuotaForPaymentGuardTest(t, 541))

	log := getTopUpLogForSettlementTest(t, 541)
	assertTopUpAuditForSettlementTest(t, log, "203.0.113.71", PaymentMethodLanTu, PaymentMethodLanTu)
}
