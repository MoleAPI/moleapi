package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestUpdateChannelNewAPIBalanceUsesTheSmallerRemainingQuota(t *testing.T) {
	for _, tc := range []struct {
		name    string
		usage   string
		want    float64
		wantErr string
	}{
		{name: "user quota below the key quota", usage: `{"code":true,"data":{"total_available":2000000,"unlimited_quota":false,"user_quota":{"total_available":500000}}}`, want: 0.5},
		{name: "key quota below the user quota", usage: `{"code":true,"data":{"total_available":500000,"unlimited_quota":false,"user_quota":{"total_available":2000000}}}`, want: 0.5},
		{name: "unlimited key uses the user quota", usage: `{"code":true,"data":{"total_available":0,"unlimited_quota":true,"user_quota":{"total_available":3000000}}}`, want: 3},
		{name: "key quota alone without user quota", usage: `{"code":true,"data":{"total_available":1500000,"unlimited_quota":false}}`, want: 1.5},
		{name: "unlimited key without user quota", usage: `{"code":true,"data":{"total_available":0,"unlimited_quota":true}}`, wantErr: "Allow querying user quota"},
		{name: "upstream rejects the key", usage: `{"success":false,"message":"invalid token"}`, wantErr: "invalid token"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			setupSub2APIBalanceTest(t)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/api/usage/token/":
					assert.Equal(t, "Bearer sk-upstream", r.Header.Get("Authorization"))
					_, _ = w.Write([]byte(tc.usage))
				case "/api/status":
					assert.Empty(t, r.Header.Get("Authorization"), "the key is not sent to the public status endpoint")
					_, _ = w.Write([]byte(`{"success":true,"data":{"quota_per_unit":1000000}}`))
				default:
					http.NotFound(w, r)
				}
			}))
			defer server.Close()
			baseURL := server.URL + "/"
			channel := &model.Channel{Type: constant.ChannelTypeNewAPI, BaseURL: &baseURL, Key: "sk-upstream", Name: "new-api-upstream"}
			require.NoError(t, model.DB.Create(channel).Error)

			result, err := updateChannelBalance(channel)
			if tc.wantErr != "" {
				require.ErrorContains(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			assert.InDelta(t, tc.want, result.Balance, 1e-9)
			assert.InDelta(t, tc.want, channel.Balance, 1e-9)
		})
	}
}

// setupSub2APIBalanceTest wires an in-memory SQLite DB into model.DB so
// Channel.UpdateBalance can persist, mirroring the pattern used by other
// controller tests.
func setupSub2APIBalanceTest(t *testing.T) {
	t.Helper()
	originalDB := model.DB
	database, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Discard,
	})
	require.NoError(t, err)
	sqlDB, err := database.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, database.AutoMigrate(&model.Channel{}))
	model.DB = database
	t.Cleanup(func() {
		model.DB = originalDB
	})
}

func newSub2APITestChannel(t *testing.T, serverURL string) *model.Channel {
	t.Helper()
	channel := &model.Channel{
		Type:    constant.ChannelTypeSub2API,
		BaseURL: &serverURL,
		Key:     "sk-test",
		Name:    "sub2api-test",
	}
	require.NoError(t, model.DB.Create(channel).Error)
	return channel
}

func TestUpdateChannelSub2APIBalanceRemainingPreferred(t *testing.T) {
	setupSub2APIBalanceTest(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/usage", r.URL.Path)
		assert.Equal(t, "Bearer sk-test", r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"balance": 100.0, "remaining": 986.17194888, "isValid": true, "unit": "USD"}`))
	}))
	defer server.Close()

	channel := newSub2APITestChannel(t, server.URL)
	balance, err := updateChannelSub2APIBalance(channel)
	require.NoError(t, err)
	assert.InDelta(t, 986.17194888, balance, 1e-9)
	assert.InDelta(t, 986.17194888, channel.Balance, 1e-9)
}

func TestUpdateChannelSub2APIBalanceWalletFallback(t *testing.T) {
	setupSub2APIBalanceTest(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"balance": 42.5, "isValid": true, "unit": "USD"}`))
	}))
	defer server.Close()

	channel := newSub2APITestChannel(t, server.URL)
	balance, err := updateChannelSub2APIBalance(channel)
	require.NoError(t, err)
	assert.InDelta(t, 42.5, balance, 1e-9)
}

func TestUpdateChannelSub2APIBalanceSubscriptionKeyMissingFields(t *testing.T) {
	setupSub2APIBalanceTest(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"balance": null, "remaining": null, "isValid": true, "mode": "unrestricted"}`))
	}))
	defer server.Close()

	channel := newSub2APITestChannel(t, server.URL)
	_, err := updateChannelSub2APIBalance(channel)
	require.Error(t, err)
}

func TestUpdateChannelSub2APIBalanceInvalidKey(t *testing.T) {
	setupSub2APIBalanceTest(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"isValid": false}`))
	}))
	defer server.Close()

	channel := newSub2APITestChannel(t, server.URL)
	_, err := updateChannelSub2APIBalance(channel)
	require.EqualError(t, err, "Sub2API key is invalid (isValid=false)")
}

func TestUpdateChannelSub2APIBalanceUnlimitedSubscription(t *testing.T) {
	setupSub2APIBalanceTest(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"remaining": -1, "isValid": true, "mode": "unrestricted", "subscription": {"daily_limit_usd": null, "weekly_limit_usd": null, "monthly_limit_usd": null}}`))
	}))
	defer server.Close()

	channel := newSub2APITestChannel(t, server.URL)
	require.NoError(t, model.DB.Model(channel).Update("balance", 12.5).Error)

	_, err := updateChannelSub2APIBalance(channel)
	require.EqualError(t, err, "Sub2API /v1/usage reports an unlimited subscription without a finite balance")

	stored := model.Channel{}
	require.NoError(t, model.DB.First(&stored, channel.Id).Error)
	assert.InDelta(t, 12.5, stored.Balance, 1e-9)
}

func TestUpdateChannelSub2APIBalanceNegativeWalletBalance(t *testing.T) {
	setupSub2APIBalanceTest(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"balance": -1, "remaining": -1, "isValid": true, "mode": "unrestricted", "unit": "USD"}`))
	}))
	defer server.Close()

	channel := newSub2APITestChannel(t, server.URL)
	balance, err := updateChannelSub2APIBalance(channel)
	require.NoError(t, err)
	assert.InDelta(t, -1, balance, 1e-9)
}

func TestUpdateChannelSub2APIBalanceMissingBaseURL(t *testing.T) {
	setupSub2APIBalanceTest(t)
	empty := ""
	channel := &model.Channel{BaseURL: &empty, Key: "sk-test"}
	_, err := updateChannelSub2APIBalance(channel)
	require.Error(t, err)
}

func TestUpdateStandardChannelBalanceRoutesSub2API(t *testing.T) {
	setupSub2APIBalanceTest(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"balance": 77.7, "isValid": true, "unit": "USD"}`))
	}))
	defer server.Close()

	channel := newSub2APITestChannel(t, server.URL)
	result, err := updateChannelBalance(channel)
	require.NoError(t, err)
	assert.InDelta(t, 77.7, result.Balance, 1e-9)
}
