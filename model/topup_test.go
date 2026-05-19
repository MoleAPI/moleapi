package model

import (
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
)

func insertTopUpTestUser(t *testing.T, username string, quota int, email string) *User {
	t.Helper()

	user := &User{
		Username:    username,
		Password:    "password123",
		DisplayName: username,
		Role:        common.RoleCommonUser,
		Status:      common.UserStatusEnabled,
		Email:       email,
		Group:       "default",
		AffCode:     common.GetRandomString(8),
		Quota:       quota,
		Setting:     "{}",
	}
	require.NoError(t, DB.Create(user).Error)
	return user
}

func insertTopUpRecord(t *testing.T, userID int, tradeNo string, paymentMethod string) *TopUp {
	t.Helper()

	paymentProvider := PaymentProviderEpay
	switch paymentMethod {
	case PaymentMethodStripe:
		paymentProvider = PaymentProviderStripe
	case PaymentMethodCreem:
		paymentProvider = PaymentProviderCreem
	case PaymentMethodWaffo:
		paymentProvider = PaymentProviderWaffo
	case PaymentMethodWaffoPancake:
		paymentProvider = PaymentProviderWaffoPancake
	}

	topUp := &TopUp{
		UserId:          userID,
		Amount:          10,
		Money:           12.34,
		TradeNo:         tradeNo,
		PaymentMethod:   paymentMethod,
		PaymentProvider: paymentProvider,
		CreateTime:      1,
		Status:          common.TopUpStatusPending,
	}
	require.NoError(t, DB.Create(topUp).Error)
	return topUp
}

func reloadUser(t *testing.T, userID int) *User {
	t.Helper()

	var user User
	require.NoError(t, DB.First(&user, userID).Error)
	return &user
}

func reloadTopUp(t *testing.T, topUpID int) *TopUp {
	t.Helper()

	var topUp TopUp
	require.NoError(t, DB.First(&topUp, topUpID).Error)
	return &topUp
}

func TestRechargeRejectsMismatchedPaymentMethod(t *testing.T) {
	truncateTables(t)

	user := insertTopUpTestUser(t, strings.ReplaceAll(t.Name(), "/", "_"), 321, "")
	topUp := insertTopUpRecord(t, user.Id, "trade_recharge_mismatch", "creem")

	err := Recharge(topUp.TradeNo, "cus_test")
	require.Error(t, err)

	reloadedUser := reloadUser(t, user.Id)
	require.Equal(t, 321, reloadedUser.Quota)
	require.Empty(t, reloadedUser.StripeCustomer)

	reloadedTopUp := reloadTopUp(t, topUp.Id)
	require.Equal(t, common.TopUpStatusPending, reloadedTopUp.Status)
	require.EqualValues(t, 0, reloadedTopUp.CompleteTime)
}

func TestRechargeStoresGatewayTradeNoAndAuditLog(t *testing.T) {
	truncateTables(t)

	user := insertTopUpTestUser(t, strings.ReplaceAll(t.Name(), "/", "_"), 100, "")
	topUp := insertTopUpRecord(t, user.Id, "trade_gateway_audit", PaymentMethodStripe)

	err := RechargeStripeWithGatewayTradeNo(topUp.TradeNo, "cus_gateway_audit", "cs_gateway_123", "203.0.113.10")
	require.NoError(t, err)

	reloadedTopUp := reloadTopUp(t, topUp.Id)
	require.Equal(t, common.TopUpStatusSuccess, reloadedTopUp.Status)
	require.Equal(t, "cs_gateway_123", reloadedTopUp.GatewayTradeNo)
	require.NotZero(t, reloadedTopUp.CompleteTime)

	var log Log
	require.NoError(t, LOG_DB.Where("user_id = ? AND type = ?", user.Id, LogTypeTopup).First(&log).Error)
	require.Equal(t, "203.0.113.10", log.Ip)

	other, err := common.StrToMap(log.Other)
	require.NoError(t, err)
	adminInfo, ok := other["admin_info"].(map[string]interface{})
	require.True(t, ok)
	require.Equal(t, "203.0.113.10", adminInfo["caller_ip"])
	require.Equal(t, PaymentMethodStripe, adminInfo["payment_method"])
	require.Equal(t, PaymentProviderStripe, adminInfo["callback_payment_method"])
	require.Equal(t, common.Version, adminInfo["version"])
}

func TestRechargeCreemRejectsMismatchedPaymentMethod(t *testing.T) {
	truncateTables(t)

	user := insertTopUpTestUser(t, strings.ReplaceAll(t.Name(), "/", "_"), 654, "original@example.com")
	topUp := insertTopUpRecord(t, user.Id, "trade_creem_mismatch", "stripe")

	err := RechargeCreem(topUp.TradeNo, "new@example.com", "New Name")
	require.Error(t, err)

	reloadedUser := reloadUser(t, user.Id)
	require.Equal(t, 654, reloadedUser.Quota)
	require.Equal(t, "original@example.com", reloadedUser.Email)

	reloadedTopUp := reloadTopUp(t, topUp.Id)
	require.Equal(t, common.TopUpStatusPending, reloadedTopUp.Status)
	require.EqualValues(t, 0, reloadedTopUp.CompleteTime)
}

func TestRechargeWaffoRejectsMismatchedPaymentMethod(t *testing.T) {
	truncateTables(t)

	user := insertTopUpTestUser(t, strings.ReplaceAll(t.Name(), "/", "_"), 987, "")
	topUp := insertTopUpRecord(t, user.Id, "trade_waffo_mismatch", "stripe")

	err := RechargeWaffo(topUp.TradeNo)
	require.Error(t, err)

	reloadedUser := reloadUser(t, user.Id)
	require.Equal(t, 987, reloadedUser.Quota)

	reloadedTopUp := reloadTopUp(t, topUp.Id)
	require.Equal(t, common.TopUpStatusPending, reloadedTopUp.Status)
	require.EqualValues(t, 0, reloadedTopUp.CompleteTime)
}

func TestRechargeAwardsInviterFirstTopupRewardOnlyOnce(t *testing.T) {
	truncateTables(t)

	originalReward := common.QuotaForInviterOnFirstTopup
	common.QuotaForInviterOnFirstTopup = 2000
	t.Cleanup(func() {
		common.QuotaForInviterOnFirstTopup = originalReward
	})

	inviter := insertTopUpTestUser(t, "inviter_"+strings.ReplaceAll(t.Name(), "/", "_"), 0, "")
	invitee := insertTopUpTestUser(t, "invitee_"+strings.ReplaceAll(t.Name(), "/", "_"), 0, "")
	require.NoError(t, DB.Model(&User{}).Where("id = ?", invitee.Id).Update("inviter_id", inviter.Id).Error)

	firstTopUp := insertTopUpRecord(t, invitee.Id, "trade_first_topup_reward", PaymentMethodStripe)
	require.NoError(t, Recharge(firstTopUp.TradeNo, "cus_first"))

	reloadedInviter := reloadUser(t, inviter.Id)
	reloadedInvitee := reloadUser(t, invitee.Id)
	require.Equal(t, 2000, reloadedInviter.AffQuota)
	require.Equal(t, 2000, reloadedInviter.AffHistoryQuota)
	require.True(t, reloadedInvitee.InviterTopupRewarded)

	secondTopUp := insertTopUpRecord(t, invitee.Id, "trade_second_topup_reward", PaymentMethodStripe)
	require.NoError(t, Recharge(secondTopUp.TradeNo, "cus_second"))

	reloadedInviter = reloadUser(t, inviter.Id)
	reloadedInvitee = reloadUser(t, invitee.Id)
	require.Equal(t, 2000, reloadedInviter.AffQuota)
	require.Equal(t, 2000, reloadedInviter.AffHistoryQuota)
	require.True(t, reloadedInvitee.InviterTopupRewarded)
}
