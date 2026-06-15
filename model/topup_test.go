package model

import (
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
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

func TestEnsureTopUpAuditColumnsAddsGatewayTradeNoToExistingTable(t *testing.T) {
	originalDB := DB
	originalUsingSQLite := common.UsingSQLite
	t.Cleanup(func() {
		DB = originalDB
		common.UsingSQLite = originalUsingSQLite
	})

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	DB = db
	common.UsingSQLite = true

	require.NoError(t, DB.Exec(`CREATE TABLE top_ups (
		id integer PRIMARY KEY AUTOINCREMENT,
		user_id integer,
		amount integer,
		money real,
		trade_no varchar(255),
		payment_method varchar(50),
		payment_provider varchar(50) DEFAULT '',
		create_time integer,
		complete_time integer,
		status varchar(50)
	)`).Error)

	require.False(t, DB.Migrator().HasColumn(&TopUp{}, "gateway_trade_no"))
	require.NoError(t, ensureTopUpAuditColumns())
	require.True(t, DB.Migrator().HasColumn(&TopUp{}, "gateway_trade_no"))

	topUp := &TopUp{
		UserId:          1,
		Amount:          1,
		Money:           6.9,
		TradeNo:         "trade_schema_upgrade",
		PaymentMethod:   "alipay",
		PaymentProvider: PaymentProviderEpay,
		CreateTime:      1,
		Status:          common.TopUpStatusPending,
	}
	require.NoError(t, topUp.Insert())
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

func TestGetAllTopUpsFiltersByUserKeywordAndTime(t *testing.T) {
	truncateTables(t)

	now := time.Now().Unix()
	target := insertTopUpTestUser(t, "target_"+strings.ReplaceAll(t.Name(), "/", "_"), 0, "target@example.com")
	target.DisplayName = "Target Display"
	require.NoError(t, DB.Save(target).Error)
	other := insertTopUpTestUser(t, "other_"+strings.ReplaceAll(t.Name(), "/", "_"), 0, "other@example.com")

	insertTopUpRecord(t, target.Id, "trade_target_old", "alipay")
	require.NoError(t, DB.Model(&TopUp{}).Where("trade_no = ?", "trade_target_old").Update("create_time", now-40*24*60*60).Error)
	insertTopUpRecord(t, target.Id, "trade_target_recent", "alipay")
	require.NoError(t, DB.Model(&TopUp{}).Where("trade_no = ?", "trade_target_recent").Update("create_time", now-2*24*60*60).Error)
	insertTopUpRecord(t, other.Id, "trade_other_recent", "alipay")
	require.NoError(t, DB.Model(&TopUp{}).Where("trade_no = ?", "trade_other_recent").Update("create_time", now-2*24*60*60).Error)

	pageInfo := &common.PageInfo{Page: 1, PageSize: 10}
	topups, total, err := GetAllTopUps(pageInfo, TopUpQueryParams{
		UserKeyword:    "target@example.com",
		StartTimestamp: now - 30*24*60*60,
	})
	require.NoError(t, err)
	require.EqualValues(t, 1, total)
	require.Len(t, topups, 1)
	require.Equal(t, "trade_target_recent", topups[0].TradeNo)

	topups, total, err = GetAllTopUps(pageInfo, TopUpQueryParams{
		UserKeyword: "999999999999999999999999999999999999",
	})
	require.NoError(t, err)
	require.EqualValues(t, 0, total)
	require.Empty(t, topups)
}

func TestGetUserTopUpsAppliesDateAndOrderFiltersOnlyForUser(t *testing.T) {
	truncateTables(t)

	now := time.Now().Unix()
	target := insertTopUpTestUser(t, "self_"+strings.ReplaceAll(t.Name(), "/", "_"), 0, "self@example.com")
	other := insertTopUpTestUser(t, "self_other_"+strings.ReplaceAll(t.Name(), "/", "_"), 0, "self_other@example.com")

	insertTopUpRecord(t, target.Id, "self_recent_match", "alipay")
	require.NoError(t, DB.Model(&TopUp{}).Where("trade_no = ?", "self_recent_match").Update("create_time", now-2*24*60*60).Error)
	insertTopUpRecord(t, target.Id, "self_old_match", "alipay")
	require.NoError(t, DB.Model(&TopUp{}).Where("trade_no = ?", "self_old_match").Update("create_time", now-40*24*60*60).Error)
	insertTopUpRecord(t, other.Id, "self_recent_match_other", "alipay")
	require.NoError(t, DB.Model(&TopUp{}).Where("trade_no = ?", "self_recent_match_other").Update("create_time", now-2*24*60*60).Error)

	pageInfo := &common.PageInfo{Page: 1, PageSize: 10}
	topups, total, err := GetUserTopUps(target.Id, pageInfo, TopUpQueryParams{
		Keyword:        "self_recent_match",
		StartTimestamp: now - 30*24*60*60,
	})
	require.NoError(t, err)
	require.EqualValues(t, 1, total)
	require.Len(t, topups, 1)
	require.Equal(t, "self_recent_match", topups[0].TradeNo)
}

func TestGetUserQuotaChangeLogsExcludesConsumeAndNonQuotaSystem(t *testing.T) {
	truncateTables(t)

	user := insertTopUpTestUser(t, "quota_logs_"+strings.ReplaceAll(t.Name(), "/", "_"), 0, "quota_logs@example.com")
	now := time.Now().Unix()
	records := []*Log{
		{UserId: user.Id, Username: user.Username, CreatedAt: now - 4, Type: LogTypeManage, Content: "管理员增加用户额度 10"},
		{UserId: user.Id, Username: user.Username, CreatedAt: now - 3, Type: LogTypeSystem, Content: "成功启用两步验证"},
		{UserId: user.Id, Username: user.Username, CreatedAt: now - 2, Type: LogTypeConsume, Content: "消费额度", Quota: 10},
		{UserId: user.Id, Username: user.Username, CreatedAt: now - 1, Type: LogTypeSystem, Content: "用户签到，获得额度 10"},
	}
	for _, record := range records {
		require.NoError(t, LOG_DB.Create(record).Error)
	}

	pageInfo := &common.PageInfo{Page: 1, PageSize: 10}
	logs, total, err := GetUserQuotaChangeLogs(user.Id, 0, 0, pageInfo)
	require.NoError(t, err)
	require.EqualValues(t, 2, total)
	require.Len(t, logs, 2)
	require.Equal(t, "用户签到，获得额度 10", logs[0].Content)
	require.Equal(t, "管理员增加用户额度 10", logs[1].Content)
}
