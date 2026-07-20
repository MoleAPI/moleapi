package model

import (
	"net/url"
	"strconv"
	"testing"

	"github.com/QuantumNous/new-api/common"

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
		name   string
		params TopUpSearchParams
		wantID int
	}{
		{name: "merchant order", params: TopUpSearchParams{Keyword: "MO1SST-ALICE"}, wantID: orders[0].Id},
		{name: "gateway order", params: TopUpSearchParams{Keyword: "GW-BOB"}, wantID: orders[1].Id},
		{name: "numeric user", params: TopUpSearchParams{UserKeyword: strconv.Itoa(bob.Id)}, wantID: orders[1].Id},
		{name: "user text", params: TopUpSearchParams{UserKeyword: "Chen"}, wantID: orders[0].Id},
		{name: "date range", params: TopUpSearchParams{StartTimestamp: 150, EndTimestamp: 250}, wantID: orders[1].Id},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, total, searchErr := SearchAllTopUpsWithParams(test.params, pageInfo)
			require.NoError(t, searchErr)
			assert.Equal(t, int64(1), total)
			if assert.Len(t, got, 1) {
				assert.Equal(t, test.wantID, got[0].Id)
			}
		})
	}
}
