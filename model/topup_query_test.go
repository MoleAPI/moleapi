package model

import (
	"net/url"
	"strconv"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestSearchAllTopUpsFiltersAdminBillingHistory(t *testing.T) {
	originalDB := DB
	originalDatabaseType := common.MainDatabaseType()
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.LogDatabaseType())
	db, err := gorm.Open(sqlite.Open("file:"+url.QueryEscape(t.Name())+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	DB = db
	require.NoError(t, db.AutoMigrate(&User{}, &TopUp{}))
	t.Cleanup(func() {
		DB = originalDB
		common.SetDatabaseTypes(originalDatabaseType, common.LogDatabaseType())
		sqlDB, dbErr := db.DB()
		if dbErr == nil {
			_ = sqlDB.Close()
		}
	})

	alice := &User{Username: "alice", Password: "password", DisplayName: "Alice Chen", Email: "alice@example.com", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Group: "default", AffCode: "topup_query_alice"}
	bob := &User{Username: "bob", Password: "password", DisplayName: "Bob Li", Email: "bob@example.com", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Group: "default", AffCode: "topup_query_bob"}
	require.NoError(t, db.Create(alice).Error)
	require.NoError(t, db.Create(bob).Error)

	orders := []*TopUp{
		{UserId: alice.Id, TradeNo: "MO1SST-ALICE", GatewayTradeNo: "GW-ALICE", CreateTime: 100, Status: common.TopUpStatusSuccess},
		{UserId: bob.Id, TradeNo: "MO1TWP-BOB", GatewayTradeNo: "GW-BOB", CreateTime: 200, Status: common.TopUpStatusPending},
	}
	for _, order := range orders {
		require.NoError(t, db.Create(order).Error)
	}

	pageInfo := &common.PageInfo{Page: 1, PageSize: 20}
	tests := []struct {
		name      string
		params    TopUpSearchParams
		wantID    int
		wantTotal int64
	}{
		{name: "merchant order", params: TopUpSearchParams{Keyword: "MO1SST-ALICE"}, wantID: orders[0].Id, wantTotal: 1},
		{name: "gateway order", params: TopUpSearchParams{Keyword: "GW-BOB"}, wantID: orders[1].Id, wantTotal: 1},
		{name: "numeric user", params: TopUpSearchParams{UserKeyword: strconv.Itoa(bob.Id)}, wantID: orders[1].Id, wantTotal: 1},
		{name: "username", params: TopUpSearchParams{UserKeyword: "alice"}, wantID: orders[0].Id, wantTotal: 1},
		{name: "user text", params: TopUpSearchParams{UserKeyword: "Chen"}, wantID: orders[0].Id, wantTotal: 1},
		{name: "missing user", params: TopUpSearchParams{UserKeyword: "missing"}, wantTotal: 0},
		{name: "date range", params: TopUpSearchParams{StartTimestamp: 150, EndTimestamp: 250}, wantID: orders[1].Id, wantTotal: 1},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, total, searchErr := SearchAllTopUpsWithParams(test.params, pageInfo)
			require.NoError(t, searchErr)
			assert.Equal(t, test.wantTotal, total)
			if test.wantTotal == 0 {
				assert.Empty(t, got)
				return
			}
			if assert.Len(t, got, int(test.wantTotal)) {
				assert.Equal(t, test.wantID, got[0].Id)
			}
		})
	}
}

func TestGetInviteRebateTopUpsReturnsOnlyGrantedRewards(t *testing.T) {
	originalDB := DB
	originalLogDB := LOG_DB
	originalDatabaseType := common.MainDatabaseType()
	originalLogDatabaseType := common.LogDatabaseType()
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	db, err := gorm.Open(sqlite.Open("file:"+url.QueryEscape(t.Name())+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	DB = db
	LOG_DB = db
	require.NoError(t, db.AutoMigrate(&Log{}))
	t.Cleanup(func() {
		DB = originalDB
		LOG_DB = originalLogDB
		common.SetDatabaseTypes(originalDatabaseType, originalLogDatabaseType)
		sqlDB, dbErr := db.DB()
		if dbErr == nil {
			_ = sqlDB.Close()
		}
	})

	other := common.MapToJsonStr(map[string]interface{}{
		"op": map[string]interface{}{
			"action": "user.quota_subtract",
			"params": map[string]interface{}{
				"target_user_id": 70,
				"quota_raw":      10,
			},
		},
	})
	logs := []*Log{
		{UserId: 7, Type: LogTypeSystem, Content: "邀请好友充值返利 ＄0.000030 额度，受邀用户 ta*****57", Quota: 30, Other: common.MapToJsonStr(map[string]interface{}{"related_user": "ta*****57"}), CreatedAt: 300},
		{UserId: 7, Type: LogTypeSystem, Content: "邀请好友充值返利 ＄0.000020 额度，受邀用户 al***ce", Quota: 20, Other: common.MapToJsonStr(map[string]interface{}{"related_user": "al***ce"}), CreatedAt: 200},
		{UserId: 7, Type: LogTypeSystem, Content: "邀请用户赠送 ＄0.000060 额度", Quota: 60, CreatedAt: 350},
		{UserId: 7, Type: LogTypeSystem, Content: "转移邀请奖励 ＄0.000010 额度 到余额", Quota: -10, CreatedAt: 250},
		{UserId: 7, Type: LogTypeSystem, Content: "管理员调整额度 -＄0.000010 额度", Quota: -10, CreatedAt: 450},
		{UserId: 1, Type: LogTypeManage, Content: "Decreased user 70 quota", Other: other, CreatedAt: 550},
		{UserId: 99, Type: LogTypeSystem, Content: "邀请用户赠送 ＄0.000040 额度", Quota: 40, CreatedAt: 500},
	}
	for _, log := range logs {
		require.NoError(t, db.Create(log).Error)
	}

	got, total, searchErr := GetInviteRebateTopUps(7, &common.PageInfo{Page: 1, PageSize: 10}, InviteRewardHistoryParams{})
	require.NoError(t, searchErr)
	assert.Equal(t, int64(5), total)
	if assert.Len(t, got, 5) {
		assert.Equal(t, "admin_adjustment", got[0].Source)
		assert.Equal(t, -10, got[0].Quota)
		assert.Equal(t, "invite_register", got[1].Source)
		assert.Equal(t, 60, got[1].Quota)
		assert.Equal(t, "topup_rebate", got[2].Source)
		assert.Equal(t, 30, got[2].Quota)
		assert.Equal(t, "ta*****57", got[2].RelatedUser)
		assert.Equal(t, int64(300), got[2].CompleteTime)
		assert.Equal(t, "reward_transfer", got[3].Source)
		assert.Equal(t, -10, got[3].Quota)
		assert.Equal(t, "topup_rebate", got[4].Source)
		assert.Equal(t, "al***ce", got[4].RelatedUser)

		payload, marshalErr := common.Marshal(got[2])
		require.NoError(t, marshalErr)
		assert.JSONEq(t, `{"id":3,"source":"topup_rebate","quota":30,"related_user":"ta*****57","complete_time":300}`, string(payload))
		assert.NotContains(t, string(payload), "reward-first")
	}

	got, total, searchErr = GetInviteRebateTopUps(7, &common.PageInfo{Page: 1, PageSize: 2}, InviteRewardHistoryParams{
		StartTimestamp: 300,
		EndTimestamp:   450,
	})
	require.NoError(t, searchErr)
	assert.Equal(t, int64(3), total)
	if assert.Len(t, got, 2) {
		assert.Equal(t, "admin_adjustment", got[0].Source)
		assert.Equal(t, "invite_register", got[1].Source)
	}

	got, total, searchErr = GetInviteRebateTopUps(7, &common.PageInfo{Page: inviteRewardHistoryHardLimit + 1, PageSize: 1}, InviteRewardHistoryParams{})
	require.NoError(t, searchErr)
	assert.Equal(t, int64(5), total)
	assert.Empty(t, got)
}

func TestGetAdminBusinessMetricsKeepsIntentAndPaidOrdersDistinct(t *testing.T) {
	originalDB := DB
	originalDatabaseType := common.MainDatabaseType()
	originalQuotaPerUnit := common.QuotaPerUnit
	originalUSDExchangeRate := operation_setting.USDExchangeRate
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.LogDatabaseType())
	common.QuotaPerUnit = 100
	operation_setting.USDExchangeRate = 10
	db, err := gorm.Open(sqlite.Open("file:"+url.QueryEscape(t.Name())+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	DB = db
	require.NoError(t, db.AutoMigrate(&User{}, &TopUp{}, &SubscriptionPlan{}, &SubscriptionOrder{}))
	t.Cleanup(func() {
		DB = originalDB
		common.QuotaPerUnit = originalQuotaPerUnit
		operation_setting.USDExchangeRate = originalUSDExchangeRate
		common.SetDatabaseTypes(originalDatabaseType, common.LogDatabaseType())
		sqlDB, dbErr := db.DB()
		if dbErr == nil {
			_ = sqlDB.Close()
		}
	})

	users := []*User{
		{Username: "metrics-alice", Password: "password", AffCode: "metrics_alice", CreatedAt: 50},
		{Username: "metrics-bob", Password: "password", AffCode: "metrics_bob", CreatedAt: 180},
		{Username: "metrics-new-no-purchase", Password: "password", AffCode: "metrics_new_no_purchase", CreatedAt: 120},
	}
	for _, user := range users {
		require.NoError(t, db.Create(user).Error)
	}

	topUps := []*TopUp{
		{UserId: users[0].Id, Amount: 12, Money: 12, PromisedQuota: 1200, TradeNo: "intent-pending", PaymentCurrency: "cny", CreateTime: 150, Status: common.TopUpStatusPending},
		{UserId: users[0].Id, Amount: 20, Money: 20, PromisedQuota: 2000, CreditedQuota: 1000, TradeNo: "wallet-paid", PaymentCurrency: "CNY", CreateTime: 160, CompleteTime: 180, Status: common.TopUpStatusSuccess},
		{UserId: users[1].Id, Amount: 0, Money: 30, TradeNo: "subscription-paid", PaymentCurrency: "USD", CreateTime: 170, CompleteTime: 190, Status: common.TopUpStatusSuccess},
		{UserId: users[1].Id, Amount: 15, Money: 15, PromisedQuota: 1500, CreditedQuota: 1500, TradeNo: "wallet-paid-bob", PaymentCurrency: "cny", CreateTime: 175, CompleteTime: 195, Status: common.TopUpStatusSuccess},
		{UserId: users[0].Id, Amount: 10, Money: 10, TradeNo: "outside", CreateTime: 50, CompleteTime: 60, Status: common.TopUpStatusSuccess},
	}
	for _, topUp := range topUps {
		require.NoError(t, db.Create(topUp).Error)
	}

	orders := []*SubscriptionOrder{
		{UserId: users[1].Id, PlanId: 1, Money: 40, TradeNo: "subscription-pending", PaymentCurrency: "USD", CreateTime: 155, Status: common.TopUpStatusPending, PlanSnapshot: "{}"},
		{UserId: users[1].Id, PlanId: 1, Money: 30, TradeNo: "subscription-paid", PaymentCurrency: "USD", CreateTime: 170, CompleteTime: 190, Status: common.TopUpStatusSuccess, PlanSnapshot: "{}"},
	}
	for _, order := range orders {
		require.NoError(t, db.Create(order).Error)
	}

	metrics, err := GetAdminBusinessMetrics(100, 200)
	require.NoError(t, err)
	assert.Equal(t, int64(2), metrics.NewUsers)
	assert.Equal(t, int64(1), metrics.NewPurchasingUsers)
	assert.Equal(t, int64(1), metrics.NewUserPurchasingUsers)
	assert.Equal(t, int64(5), metrics.IntentOrders)
	assert.Equal(t, []AdminBusinessOrderAmount{
		{Currency: "CNY", Orders: 3, Amount: 47, AverageAmount: float64(47) / 3},
		{Currency: "USD", Orders: 2, Amount: 70, AverageAmount: 35},
	}, metrics.IntentAmounts)
	assert.Equal(t, int64(3), metrics.PaidOrders)
	assert.Equal(t, []AdminBusinessOrderAmount{
		{Currency: "CNY", Orders: 2, Amount: 35, AverageAmount: 17.5},
		{Currency: "USD", Orders: 1, Amount: 30, AverageAmount: 30},
	}, metrics.PaidAmounts)
	assert.Equal(t, int64(2), metrics.PayingUsers)
	assert.InDelta(t, 0.6, metrics.PaymentSuccessRate, 0.001)
	assert.Equal(t, int64(3), metrics.TopUpIntentOrders)
	assert.InDelta(t, 47, metrics.TopUpIntentAmountUSD, 0.001)
	assert.Equal(t, int64(2), metrics.TopUpPaidOrders)
	assert.InDelta(t, 25, metrics.TopUpPaidAmountUSD, 0.001)
	assert.Equal(t, []AdminBusinessTopUpUser{
		{Rank: 1, UserId: users[0].Id, Username: "metrics-alice", Currency: "USD", Orders: 1, Amount: 2},
		{Rank: 2, UserId: users[1].Id, Username: "metrics-bob", Currency: "USD", Orders: 1, Amount: 1.5},
	}, metrics.TopUpRanking)
}
