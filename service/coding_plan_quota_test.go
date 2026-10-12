package service

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/clickhouse"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestEvaluateCodingPlanQuotaUsesRollingConsumeLogs(t *testing.T) {
	dialects := []struct {
		name string
		env  string
		open func(string) gorm.Dialector
	}{
		{name: "sqlite", open: func(string) gorm.Dialector { return sqlite.Open(":memory:") }},
		{name: "mysql", env: "TEST_MYSQL_DSN", open: mysql.Open},
		{name: "postgres", env: "TEST_POSTGRES_DSN", open: func(dsn string) gorm.Dialector {
			return postgres.New(postgres.Config{DSN: dsn, PreferSimpleProtocol: true})
		}},
		{name: "clickhouse", env: "TEST_CLICKHOUSE_DSN", open: clickhouse.Open},
	}
	for _, dialect := range dialects {
		t.Run(dialect.name, func(t *testing.T) {
			dsn := os.Getenv(dialect.env)
			if dialect.env != "" && dsn == "" {
				t.Skip(dialect.env + " is not configured")
			}
			db, err := gorm.Open(dialect.open(dsn), &gorm.Config{})
			require.NoError(t, err)
			require.NoError(t, db.AutoMigrate(&model.Log{}))
			t.Cleanup(func() {
				sqlDB, sqlErr := db.DB()
				if sqlErr == nil {
					require.NoError(t, sqlDB.Close())
				}
			})
			testEvaluateCodingPlanQuotaUsesRollingConsumeLogs(t, db)
		})
	}
}

func testEvaluateCodingPlanQuotaUsesRollingConsumeLogs(t *testing.T, db *gorm.DB) {
	previousLogDB := model.LOG_DB
	model.LOG_DB = db
	t.Cleanup(func() { model.LOG_DB = previousLogDB })

	settings, err := common.Marshal(dto.ChannelOtherSettings{
		CodingPlanQuota: &dto.CodingPlanQuotaConfig{
			Unit: dto.CodingPlanQuotaUnitRequests,
			Windows: []dto.CodingPlanQuotaWindow{{
				DurationSeconds: 3600,
				Limit:           2,
			}},
		},
	})
	require.NoError(t, err)
	now := time.Unix(1_700_000_000, 0)
	require.NoError(t, db.Create(&model.Log{
		ChannelId:    7,
		CreatedAt:    now.Add(-10 * time.Minute).Unix(),
		Type:         model.LogTypeConsume,
		PromptTokens: 10,
	}).Error)
	require.NoError(t, db.Create(&model.Log{
		ChannelId: 7,
		CreatedAt: now.Add(-5 * time.Minute).Unix(),
		Type:      model.LogTypeConsume,
	}).Error)
	require.NoError(t, db.Create(&model.Log{
		ChannelId: 7,
		CreatedAt: now.Add(-5 * time.Minute).Unix(),
		Type:      model.LogTypeError,
	}).Error)

	status, err := EvaluateCodingPlanQuota(&model.Channel{
		Id:            7,
		Type:          constant.ChannelTypeAdvancedCustom,
		OtherSettings: string(settings),
	}, now)
	require.NoError(t, err)
	assert.True(t, status.Blocked)
	assert.Contains(t, status.Reason, CodingPlanQuotaReasonPrefix)
	require.NotNil(t, status.ResetAt)
	assert.Equal(t, now.Add(50*time.Minute), *status.ResetAt)
	assert.Contains(t, status.Reason, "reset_at=")
}

func TestCheckCodingPlanQuotaAfterConsumeDisablesAtLimit(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.Ability{}, &model.Log{}, &model.User{}))

	previousDB, previousLogDB := model.DB, model.LOG_DB
	previousDatabaseType := common.MainDatabaseType()
	previousMemoryCache := common.MemoryCacheEnabled
	previousAutoDisable := common.AutomaticDisableChannelEnabled
	model.DB, model.LOG_DB = db, db
	common.SetMainDatabaseType(common.DatabaseTypeSQLite)
	common.MemoryCacheEnabled = false
	common.AutomaticDisableChannelEnabled = true
	t.Cleanup(func() {
		model.DB, model.LOG_DB = previousDB, previousLogDB
		common.SetMainDatabaseType(previousDatabaseType)
		common.MemoryCacheEnabled = previousMemoryCache
		common.AutomaticDisableChannelEnabled = previousAutoDisable
	})

	config := &dto.CodingPlanQuotaConfig{Unit: dto.CodingPlanQuotaUnitRequests, Windows: []dto.CodingPlanQuotaWindow{{DurationSeconds: 3600, Limit: 1}}}
	settings, err := common.Marshal(dto.ChannelOtherSettings{CodingPlanQuota: config})
	require.NoError(t, err)
	autoBan := 1
	channel := &model.Channel{Name: "plan", Key: "key", Type: constant.ChannelTypeAdvancedCustom, Status: common.ChannelStatusEnabled, AutoBan: &autoBan, OtherSettings: string(settings)}
	require.NoError(t, db.Create(channel).Error)
	require.NoError(t, db.Create(&model.Log{ChannelId: channel.Id, CreatedAt: time.Now().Unix(), Type: model.LogTypeConsume}).Error)

	CheckCodingPlanQuotaAfterConsume(channel.Id, channel.Type, config)
	assert.Eventually(t, func() bool {
		loaded, loadErr := model.GetChannelById(channel.Id, true)
		return loadErr == nil && loaded.Status == common.ChannelStatusAutoDisabled
	}, 3*time.Second, 20*time.Millisecond)
}

func TestParseOfficialCodingPlanQuotaResponses(t *testing.T) {
	t.Run("glm", func(t *testing.T) {
		plan, windows, err := parseGLMCodingPlanQuota([]byte(`{"success":true,"data":{"level":"pro","limits":[{"type":"TOKENS_LIMIT","unit":3,"number":5,"percentage":12,"nextResetTime":1790000000000},{"type":"TIME_LIMIT","unit":5,"percentage":3}]}}`))
		require.NoError(t, err)
		assert.Equal(t, "pro", plan)
		require.Len(t, windows, 2)
		assert.Equal(t, "5 hours", windows[0].Name)
		assert.Equal(t, float64(12), windows[0].Used)
		assert.True(t, windows[1].Aside)
	})

	t.Run("kimi", func(t *testing.T) {
		_, windows, err := parseKimiCodingPlanQuota([]byte(`{"usage":{"limit":"100","used":"12","resetTime":"2026-09-30T05:24:18.44Z"},"limits":[{"window":{"duration":5,"timeUnit":"TIME_UNIT_HOUR"},"detail":{"limit":"100","remaining":"88","resetTime":"2026-09-25T20:00:00Z"}}]}`))
		require.NoError(t, err)
		require.Len(t, windows, 2)
		assert.Equal(t, float64(12), windows[0].Used)
		assert.Equal(t, "7 days", windows[1].Name)
	})

	t.Run("minimax", func(t *testing.T) {
		_, windows, err := parseMiniMaxCodingPlanQuota([]byte(`{"model_remains":[{"model_name":"general","start_time":1790000000,"end_time":1790018000,"current_interval_remaining_percent":0,"current_interval_status":1,"weekly_start_time":1790000000,"weekly_end_time":1790604800,"current_weekly_remaining_percent":25,"current_weekly_status":1}],"base_resp":{"status_code":0,"status_msg":"success"}}`))
		require.NoError(t, err)
		require.Len(t, windows, 2)
		assert.Equal(t, float64(100), windows[0].Used)
		assert.Equal(t, float64(75), windows[1].Used)
	})

	t.Run("opencode", func(t *testing.T) {
		_, windows, err := parseOpenCodeCodingPlanQuota([]byte(`{"usage":{"rolling":{"percent":37,"resetsAt":"2026-08-26T14:12:03Z"},"weekly":{"percent":100,"resetsAt":"2026-08-30T14:12:03Z"}}}`))
		require.NoError(t, err)
		require.Len(t, windows, 2)
		assert.Equal(t, float64(100), windows[1].Used)
	})

	t.Run("codex", func(t *testing.T) {
		plan, windows, err := parseCodexQuota([]byte(`{"plan_type":"pro","rate_limit":{"primary_window":{"used_percent":100,"reset_at":1790000000},"secondary_window":{"used_percent":40,"reset_at":1790604800}}}`))
		require.NoError(t, err)
		assert.Equal(t, "pro", plan)
		require.Len(t, windows, 2)
		assert.Equal(t, float64(100), windows[0].Used)
	})

	t.Run("volcengine coding plan", func(t *testing.T) {
		plan, windows, err := parseVolcengineCodingQuota([]byte(`{"Result":{"QuotaUsage":[{"Level":"session","Percent":72,"ResetTimestamp":1790000000},{"Level":"weekly","Percent":31}]}}`))
		require.NoError(t, err)
		assert.Equal(t, "Coding Plan", plan)
		require.Len(t, windows, 2)
		assert.Equal(t, "5 hours", windows[0].Name)
		assert.Equal(t, float64(72), windows[0].Used)
	})

	t.Run("volcengine agent plan", func(t *testing.T) {
		plan, windows, err := parseVolcengineAgentQuota([]byte(`{"Result":{"PlanType":"Large","AFPFiveHour":{"Quota":"100.0","Used":"25.0","ResetTime":1790000000000},"AFPDaily":{"Quota":"150.0","Used":"30.0"},"AFPWeekly":{"Quota":"200.0","Used":"40.0"}}}`))
		require.NoError(t, err)
		assert.Equal(t, "Large", plan)
		require.Len(t, windows, 3)
		assert.Equal(t, "5 hours", windows[0].Name)
		assert.Equal(t, float64(25), windows[0].Used)
		assert.Equal(t, "1 day", windows[1].Name)
		assert.Equal(t, float64(20), windows[1].Used)
	})

	t.Run("command code", func(t *testing.T) {
		plan, windows, err := parseCommandCodeQuota(
			map[string]any{"credits": map[string]any{"planId": "pro", "monthlyCredits": 70, "purchasedCredits": 10}, "windowLimits": map[string]any{"limited": true, "fiveHour": map[string]any{"used": 3, "cap": 10}, "weekly": map[string]any{"used": 12, "cap": 40}}},
			map[string]any{"data": map[string]any{"planId": "pro", "currentPeriodEnd": "2026-12-31T00:00:00Z"}},
			map[string]any{"totalCost": 20},
		)
		require.NoError(t, err)
		assert.Equal(t, "pro", plan)
		require.Len(t, windows, 3)
		assert.Equal(t, float64(30), windows[0].Used)
		assert.Equal(t, float64(30), windows[1].Used)
		assert.Equal(t, float64(20), windows[2].Used)
	})
}

func TestOfficialCodingPlanQuotaProviderRecognition(t *testing.T) {
	for _, provider := range []string{dto.CodingPlanProviderDoubao, dto.CodingPlanProviderVolcengineAgent, dto.CodingPlanProviderCommandCode} {
		settings, err := common.Marshal(dto.ChannelOtherSettings{CodingPlanProvider: provider})
		require.NoError(t, err)
		channel := &model.Channel{Type: constant.ChannelTypeAdvancedCustom, OtherSettings: string(settings)}
		assert.True(t, hasOfficialCodingPlanQuota(channel), provider)
	}

	for _, name := range []string{"VOLC_ACCESS_KEY_ID", "VOLC_ACCESSKEY", "VOLC_ACCESS_KEY", "VOLC_SECRET_ACCESS_KEY", "VOLC_SECRETKEY", "VOLC_SECRET_KEY"} {
		t.Setenv(name, "")
	}
	settings, err := common.Marshal(dto.ChannelOtherSettings{CodingPlanProvider: dto.CodingPlanProviderDoubao})
	require.NoError(t, err)
	status, err := EvaluateOfficialCodingPlanQuota(&model.Channel{Type: constant.ChannelTypeAdvancedCustom, OtherSettings: string(settings)}, time.Now())
	require.NoError(t, err)
	assert.False(t, status.Available)
}

func TestVolcengineSignUsesStableRequestContract(t *testing.T) {
	now := time.Date(2024, time.June, 21, 0, 0, 0, 0, time.UTC)
	query := volcengineCanonicalQuery(volcengineAgentPlanAction, volcengineDefaultRegion)
	authorization, xDate, bodyHash := volcengineSign("AKLTtest", "secretkey", volcengineDefaultRegion, query, nil, now)
	assert.Equal(t, "20240621T000000Z", xDate)
	assert.Equal(t, "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855", bodyHash)
	assert.Contains(t, authorization, "Credential=AKLTtest/20240621/cn-beijing/ark/request")
	assert.Contains(t, authorization, "SignedHeaders=host;x-date;x-content-sha256;content-type")
	assert.Len(t, authorization[strings.LastIndex(authorization, "Signature=")+len("Signature="):], 64)
}

func TestOfficialCodingPlanQuotaStatusIgnoresExpiredWindows(t *testing.T) {
	reset := time.Unix(1_700_000_000, 0)
	status := quotaStatus(codingPlanProviderQuota{
		Provider: dto.CodingPlanProviderKimi,
		Windows:  []codingPlanProviderWindow{{Used: 100, ResetsAt: &reset}},
	}, time.Unix(1_700_000_001, 0))
	assert.True(t, status.Available)
	assert.False(t, status.Blocked)
}

func TestOfficialCodingPlanQuotaStatusUsesLatestBlockedReset(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	firstReset := now.Add(time.Hour)
	lastReset := now.Add(7 * 24 * time.Hour)
	status := quotaStatus(codingPlanProviderQuota{
		Provider: dto.CodingPlanProviderKimi,
		Windows: []codingPlanProviderWindow{
			{Name: "5 hours", Used: 100, ResetsAt: &firstReset},
			{Name: "7 days", Used: 100, ResetsAt: &lastReset},
		},
	}, now)

	assert.True(t, status.Blocked)
	require.NotNil(t, status.ResetAt)
	assert.Equal(t, lastReset, *status.ResetAt)
	assert.Contains(t, status.Reason, fmt.Sprintf("reset_at=%d", lastReset.Unix()))
}

func TestEvaluateOfficialCodexQuotaUsesWhamEndpoint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/backend-api/wham/usage", r.URL.Path)
		assert.Equal(t, "Bearer access", r.Header.Get("Authorization"))
		assert.Equal(t, "account", r.Header.Get("chatgpt-account-id"))
		_, _ = w.Write([]byte(`{"plan_type":"pro","rate_limit":{"primary_window":{"used_percent":100,"reset_at":4102444800}}}`))
	}))
	defer server.Close()

	key, err := common.Marshal(map[string]string{
		"access_token": "access",
		"account_id":   "account",
	})
	require.NoError(t, err)
	baseURL := server.URL
	status, err := EvaluateOfficialCodingPlanQuota(&model.Channel{
		Id:      1001,
		Type:    constant.ChannelTypeCodex,
		Key:     string(key),
		BaseURL: &baseURL,
	}, time.Unix(1_700_000_000, 0))
	require.NoError(t, err)
	assert.True(t, status.Available)
	assert.True(t, status.Blocked)
}

func TestOfficialQuotaAvailabilityRequiresProbeAfterResponseDisable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"plan_type":"pro","rate_limit":{"primary_window":{"used_percent":50,"reset_at":4102444800}}}`))
	}))
	defer server.Close()

	key, err := common.Marshal(map[string]string{"access_token": "access", "account_id": "account"})
	require.NoError(t, err)
	baseURL := server.URL
	channel := &model.Channel{Id: 1002, Name: "codex", Type: constant.ChannelTypeCodex, Key: string(key), BaseURL: &baseURL, Status: common.ChannelStatusAutoDisabled}
	channel.SetOtherInfo(map[string]any{"status_reason": CodingPlanResponseReasonPrefix + "quota exhausted"})

	recoverable := UpdateOfficialCodingPlanQuotaChannels([]*model.Channel{channel}, time.Unix(1_700_000_000, 0))
	require.Len(t, recoverable, 1)
	assert.Equal(t, channel.Id, recoverable[0].Id)
}
