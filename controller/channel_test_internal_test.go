package controller

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	"github.com/QuantumNous/new-api/pkg/channelprobe"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetChannelDefaultBaseURLsUsesBuiltInDefaults(t *testing.T) {
	originalBaseURLs := constant.ChannelBaseURLs
	constant.ChannelBaseURLs = append([]string(nil), originalBaseURLs...)
	constant.ChannelBaseURLs[constant.ChannelTypeDeepSeek] = "https://deepseek.server.example"
	t.Cleanup(func() {
		constant.ChannelBaseURLs = originalBaseURLs
	})

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/channel/default_base_urls", nil)
	GetChannelDefaultBaseURLs(c)

	require.Equal(t, http.StatusOK, recorder.Code)
	var response struct {
		Success bool           `json:"success"`
		Data    map[int]string `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	require.True(t, response.Success)
	assert.Equal(t, "https://deepseek.server.example", response.Data[constant.ChannelTypeDeepSeek])
	assert.Equal(t, "https://api.openai.com", response.Data[constant.ChannelTypeOpenAI])
	assert.NotContains(t, response.Data, constant.ChannelTypeAzure)
	assert.NotContains(t, response.Data, constant.ChannelTypeNewAPI)
	assert.NotContains(t, response.Data, constant.ChannelTypeTaskPlugin)
}

func TestValidateChannelProxy(t *testing.T) {
	tests := []struct {
		name    string
		proxy   string
		wantErr bool
	}{
		{name: "empty"},
		{name: "http", proxy: "http://proxy.example:8080"},
		{name: "https", proxy: "https://proxy.example:8443"},
		{name: "socks5", proxy: "socks5://proxy.example"},
		{name: "socks5h", proxy: "socks5h://proxy.example:1080/"},
		{name: "unsupported", proxy: "ftp://proxy.example", wantErr: true},
		{name: "path", proxy: "socks5://proxy.example:1080/path", wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			setting, err := common.Marshal(dto.ChannelSettings{Proxy: test.proxy})
			require.NoError(t, err)
			channel := &model.Channel{
				Type:    constant.ChannelTypeOpenAI,
				Setting: common.GetPointer(string(setting)),
			}

			err = validateChannel(channel, false)

			if test.wantErr {
				require.ErrorContains(t, err, "invalid channel proxy")
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestValidateChannelRequiresNewAPIBaseURL(t *testing.T) {
	tests := []struct {
		name    string
		baseURL *string
		wantErr bool
	}{
		{name: "missing", wantErr: true},
		{name: "blank", baseURL: common.GetPointer("  "), wantErr: true},
		{name: "configured", baseURL: common.GetPointer("https://new-api.example")},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			channel := &model.Channel{
				Type:    constant.ChannelTypeNewAPI,
				BaseURL: test.baseURL,
			}

			err := validateChannel(channel, false)

			if test.wantErr {
				require.ErrorContains(t, err, "New API channel base URL cannot be empty")
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestNewAPIChannelRegistration(t *testing.T) {
	apiType, ok := common.ChannelType2APIType(constant.ChannelTypeNewAPI)

	require.True(t, ok)
	assert.Equal(t, constant.APITypeNewAPI, apiType)
	assert.Equal(t, "New API", constant.GetChannelTypeName(constant.ChannelTypeNewAPI))
	require.Greater(t, len(constant.ChannelBaseURLs), constant.ChannelTypeNewAPI)
	assert.Empty(t, constant.ChannelBaseURLs[constant.ChannelTypeNewAPI])
}

func TestResponsesCompactChannelSupport(t *testing.T) {
	tests := []struct {
		name        string
		channelType int
		apiType     int
		want        bool
	}{
		{name: "OpenAI", channelType: constant.ChannelTypeOpenAI, apiType: constant.APITypeOpenAI, want: true},
		{name: "Azure", channelType: constant.ChannelTypeAzure, apiType: constant.APITypeOpenAI, want: true},
		{name: "Codex", channelType: constant.ChannelTypeCodex, apiType: constant.APITypeCodex, want: true},
		{name: "Advanced Custom", channelType: constant.ChannelTypeAdvancedCustom, apiType: constant.APITypeAdvancedCustom, want: true},
		{name: "Sub2API", channelType: constant.ChannelTypeSub2API, apiType: constant.APITypeSub2API, want: true},
		{name: "New API", channelType: constant.ChannelTypeNewAPI, apiType: constant.APITypeNewAPI, want: true},
		{name: "Anthropic", channelType: constant.ChannelTypeAnthropic, apiType: constant.APITypeAnthropic, want: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.want, common.SupportsResponsesCompact(test.channelType, test.apiType))
		})
	}
}

func TestMultiprotocolGatewayEndpointTypes(t *testing.T) {
	want := []constant.EndpointType{
		constant.EndpointTypeOpenAI,
		constant.EndpointTypeOpenAIResponse,
		constant.EndpointTypeOpenAIResponseCompact,
		constant.EndpointTypeAnthropic,
		constant.EndpointTypeGemini,
		constant.EndpointTypeOpenAIAlphaSearch,
	}

	assert.Equal(t, want, common.GetEndpointTypesByChannelType(constant.ChannelTypeNewAPI, "gpt-5"))
	assert.Equal(t, want, common.GetEndpointTypesByChannelType(constant.ChannelTypeSub2API, "gpt-5"))
}

func TestCopyChannelRejectsInvalidLegacyProxySettings(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	settingBytes, err := common.Marshal(dto.ChannelSettings{
		Proxy: "socks5://proxy.example/legacy-path",
	})
	require.NoError(t, err)
	setting := string(settingBytes)
	origin := &model.Channel{
		Type:    constant.ChannelTypeOpenAI,
		Name:    "legacy proxy channel",
		Key:     "test-key",
		Models:  "gpt-test",
		Group:   "default",
		Setting: &setting,
	}
	require.NoError(t, db.Create(origin).Error)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", origin.Id)}}
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/channel/copy", nil)

	CopyChannel(ctx)

	assert.Contains(t, recorder.Body.String(), "invalid channel settings")
	var channelCount int64
	require.NoError(t, db.Model(&model.Channel{}).Count(&channelCount).Error)
	assert.Equal(t, int64(1), channelCount)
}

func TestDeleteChannelResetsProxyCacheWhenPreReadFails(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Log{}, &model.AuditLog{}))
	service.ResetProxyClientCache()
	t.Cleanup(service.ResetProxyClientCache)

	proxyURL := "http://proxy.example:8080"
	beforeDelete, err := service.GetHttpClientWithProxy(proxyURL)
	require.NoError(t, err)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Params = gin.Params{{Key: "id", Value: "999999"}}
	ctx.Request = httptest.NewRequest(http.MethodDelete, "/api/channel/999999", nil)

	DeleteChannel(ctx)

	assert.Contains(t, recorder.Body.String(), `"success":true`)
	afterDelete, err := service.GetHttpClientWithProxy(proxyURL)
	require.NoError(t, err)
	assert.NotSame(t, beforeDelete, afterDelete)
}

func TestDeleteChannelBatchReportsAndAuditsActualDeletedCount(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Log{}, &model.AuditLog{}))
	channel := &model.Channel{Name: "existing", Key: "test-key"}
	require.NoError(t, db.Create(channel).Error)

	requestBody, err := common.Marshal(ChannelBatch{Ids: []int{channel.Id, 999999}})
	require.NoError(t, err)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodDelete, "/api/channel/batch", bytes.NewReader(requestBody))
	ctx.Request.Header.Set("Content-Type", "application/json")

	DeleteChannelBatch(ctx)

	var response struct {
		Success bool  `json:"success"`
		Data    int64 `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.True(t, response.Success)
	assert.Equal(t, int64(1), response.Data)

	var auditLog model.AuditLog
	require.NoError(t, db.Order("id desc").First(&auditLog).Error)
	var auditData struct {
		Operation struct {
			Params map[string]any `json:"params"`
		} `json:"op"`
	}
	encodedAudit, err := common.Marshal(auditLog.Other)
	require.NoError(t, err)
	require.NoError(t, common.Unmarshal(encodedAudit, &auditData))
	assert.Equal(t, float64(1), auditData.Operation.Params["count"])
}

func TestSettleTestQuotaUsesTieredBilling(t *testing.T) {
	info := &relaycommon.RelayInfo{
		TieredBillingSnapshot: &billingexpr.BillingSnapshot{
			BillingMode:   "tiered_expr",
			ExprString:    `param("stream") == true ? tier("stream", p * 3) : tier("base", p * 2)`,
			ExprHash:      billingexpr.ExprHashString(`param("stream") == true ? tier("stream", p * 3) : tier("base", p * 2)`),
			GroupRatio:    1,
			EstimatedTier: "stream",
			QuotaPerUnit:  common.QuotaPerUnit,
			ExprVersion:   1,
		},
		BillingRequestInput: &billingexpr.RequestInput{
			Body: []byte(`{"stream":true}`),
		},
	}

	quota, result := settleTestQuota(info, types.PriceData{
		ModelRatio:      1,
		CompletionRatio: 2,
	}, &dto.Usage{
		PromptTokens: 1000,
	})

	require.Equal(t, 1500, quota)
	require.NotNil(t, result)
	require.Equal(t, "stream", result.MatchedTier)
}

func TestBuildTestLogOtherInjectsTieredInfo(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())

	info := &relaycommon.RelayInfo{
		TieredBillingSnapshot: &billingexpr.BillingSnapshot{
			BillingMode: "tiered_expr",
			ExprString:  `tier("base", p * 2)`,
		},
		ChannelMeta: &relaycommon.ChannelMeta{},
	}
	priceData := types.PriceData{
		GroupRatioInfo: types.GroupRatioInfo{GroupRatio: 1},
	}
	usage := &dto.Usage{
		PromptTokensDetails: dto.InputTokenDetails{
			CachedTokens: 12,
		},
	}

	requestRules := []billingexpr.RequestRuleTrace{{
		Cond:       `param("service_tier") == "fast"`,
		Multiplier: 2,
		Matched:    true,
	}}
	other := buildTestLogOther(ctx, info, priceData, usage, &billingexpr.TieredResult{
		MatchedTier:  "base",
		RequestRules: requestRules,
	}, newChannelProbeSpec("manual"), channelprobe.Evaluation{Mode: channelprobe.ModeHi, Outcome: channelprobe.OutcomePass})

	fields := other.Snapshot()
	require.Equal(t, "tiered_expr", fields["billing_mode"])
	require.Equal(t, "base", fields["matched_tier"])
	require.Equal(t, requestRules, fields["request_rules"])
	require.NotEmpty(t, fields["expr_b64"])
}

func TestResolveChannelTestUserIDUsesRequestUser(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Set("id", 2)

	userID, err := resolveChannelTestUserID(ctx)

	require.NoError(t, err)
	require.Equal(t, 2, userID)
}

func TestChannelTestModelsUsesFirstAvailableModel(t *testing.T) {
	channel := &model.Channel{Models: " a ,b,a"}
	assert.Equal(t, []string{"a"}, channelTestModels(channel))
}

func TestChannelTestModelsSkipsAutoPausedModels(t *testing.T) {
	state := channelprobe.State{}
	state.Pause("paused", "confirmed", 10, 100, channelprobe.Failure{OccurredAt: 10, Requests: 3})
	otherInfo, err := channelprobe.StateIntoOtherInfo("", state)
	require.NoError(t, err)
	channel := &model.Channel{Models: "paused,healthy", OtherInfo: otherInfo}

	assert.Equal(t, []string{"healthy"}, channelTestModels(channel))
}

func TestStructuredModelFailureUsesOnlyModelScopedProviderErrors(t *testing.T) {
	other := func(status int, code string) string {
		encoded, err := common.Marshal(map[string]any{"status_code": status, "error_code": code, "error_type": "upstream_error"})
		require.NoError(t, err)
		return string(encoded)
	}
	tests := []struct {
		name   string
		status int
		code   string
		want   bool
	}{
		{name: "missing model", status: 404, code: "model_not_found", want: true},
		{name: "wrong path is not a model failure", status: 404, code: "resource_not_found"},
		{name: "provider response", status: 503, code: "bad_response_status_code", want: true},
		{name: "network is channel scoped", status: 500, code: "do_request_failed"},
		{name: "invalid user parameters", status: 400, code: "invalid_request"},
		{name: "insufficient balance", status: 402, code: "insufficient_user_quota"},
		{name: "content policy", status: 422, code: "content_policy_violation"},
		{name: "cancelled", status: 499, code: "request_cancelled"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, ok := structuredModelFailure(&model.Log{ChannelId: 1, ModelName: "model-a", RequestId: "request", CreatedAt: 10, Other: other(tt.status, tt.code)})
			assert.Equal(t, tt.want, ok)
		})
	}
}

func TestCollectModelFailureCandidatesDeduplicatesRequests(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Log{}))
	previous := common.AutomaticDisableChannelEnabled
	common.AutomaticDisableChannelEnabled = true
	t.Cleanup(func() { common.AutomaticDisableChannelEnabled = previous })
	autoBan := 1
	channel := &model.Channel{Name: "candidate", Status: common.ChannelStatusEnabled, Models: "model-a,model-b", Group: "default", AutoBan: &autoBan}
	require.NoError(t, db.Create(channel).Error)
	now := int64(10_000)
	other, err := common.Marshal(map[string]any{"status_code": 404, "error_code": "model_not_found", "error_type": "upstream_error"})
	require.NoError(t, err)
	for _, requestID := range []string{"request-1", "request-1", "request-2", "request-3"} {
		require.NoError(t, db.Create(&model.Log{
			CreatedAt: now - 1, Type: model.LogTypeError, ChannelId: channel.Id,
			ModelName: "model-a", RequestId: requestID, Other: string(other),
		}).Error)
	}
	require.NoError(t, db.Create(&model.Log{
		CreatedAt: now - 1, Type: model.LogTypeError, ChannelId: channel.Id,
		ModelName: "model-b", TokenName: "模型测试", RequestId: "probe", Other: string(other),
	}).Error)

	candidates := collectModelFailureCandidates([]*model.Channel{channel}, now)

	require.Len(t, candidates[channel.Id], 1)
	assert.Equal(t, "model-a", candidates[channel.Id][0].Model)
	assert.Equal(t, 3, candidates[channel.Id][0].Failure.Requests)
}

func TestCollectModelFailureCandidatesRespectsProbeCooldown(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Log{}))
	previous := common.AutomaticDisableChannelEnabled
	common.AutomaticDisableChannelEnabled = true
	t.Cleanup(func() { common.AutomaticDisableChannelEnabled = previous })
	autoBan := 1
	now := int64(10_000)
	state := channelprobe.State{}
	state.RecordAutoProbe("model-a", true, "candidate_pass", now-60, 0, nil)
	otherInfo, err := channelprobe.StateIntoOtherInfo("", state)
	require.NoError(t, err)
	channel := &model.Channel{Name: "cooldown", Status: common.ChannelStatusEnabled, Models: "model-a", Group: "default", AutoBan: &autoBan, OtherInfo: otherInfo}
	require.NoError(t, db.Create(channel).Error)
	other, err := common.Marshal(map[string]any{"status_code": 404, "error_code": "model_not_found", "error_type": "upstream_error"})
	require.NoError(t, err)
	for _, requestID := range []string{"request-1", "request-2", "request-3"} {
		require.NoError(t, db.Create(&model.Log{
			CreatedAt: now - 1, Type: model.LogTypeError, ChannelId: channel.Id,
			ModelName: "model-a", RequestId: requestID, Other: string(other),
		}).Error)
	}

	assert.Empty(t, collectModelFailureCandidates([]*model.Channel{channel}, now)[channel.Id])
	assert.Len(t, collectModelFailureCandidates([]*model.Channel{channel}, now+int64(modelProbeCooldown/time.Second))[channel.Id], 1)
}

func TestGetChannelModelHealthReturnsCurrentStatusAndDailyStats(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Log{}))
	state := channelprobe.State{}
	state.Pause("model-a", "confirmed", 10, 300, channelprobe.Failure{OccurredAt: 10, Requests: 3})
	otherInfo, err := channelprobe.StateIntoOtherInfo("", state)
	require.NoError(t, err)
	channel := &model.Channel{Name: "health", Status: common.ChannelStatusEnabled, Models: "model-a,model-b", Group: "default", OtherInfo: otherInfo}
	require.NoError(t, db.Create(channel).Error)
	now := time.Now().Unix()
	require.NoError(t, db.Create(&[]model.Log{
		{CreatedAt: now, Type: model.LogTypeConsume, ChannelId: channel.Id, ModelName: "model-a", TokenName: "user"},
		{CreatedAt: now, Type: model.LogTypeError, ChannelId: channel.Id, ModelName: "model-a", TokenName: "user"},
		{CreatedAt: now, Type: model.LogTypeError, ChannelId: channel.Id, ModelName: "model-a", TokenName: "模型测试"},
	}).Error)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/channel/%d/model-health", channel.Id), nil)
	c.Params = gin.Params{{Key: "id", Value: strconv.Itoa(channel.Id)}}

	GetChannelModelHealth(c)

	require.Equal(t, http.StatusOK, w.Code)
	var response struct {
		Success bool `json:"success"`
		Data    struct {
			Models []channelModelHealth `json:"models"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(w.Body.Bytes(), &response))
	require.True(t, response.Success)
	require.Len(t, response.Data.Models, 2)
	assert.Equal(t, "disabled", response.Data.Models[0].Status)
	assert.Equal(t, int64(2), response.Data.Models[0].Today.Requests)
	assert.Equal(t, int64(1), response.Data.Models[0].Today.Failures)
	assert.Equal(t, 50.0, response.Data.Models[0].Today.FailureRate)
}

func TestModelHealthProbeMatchesEndpointFamily(t *testing.T) {
	channel := &model.Channel{Models: "text-embed-v4,bge-m3,gpt-4o,chat-backup"}
	assert.Equal(t, string(constant.EndpointTypeEmbeddings), modelProbeEndpoint(channel, "text-embed-v4"))
	assert.Equal(t, string(constant.EndpointTypeOpenAI), modelProbeEndpoint(channel, "gpt-4o"))
}

func TestCollectModelFailureCandidatesRequiresConsecutiveFailures(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Log{}))
	previous := common.AutomaticDisableChannelEnabled
	common.AutomaticDisableChannelEnabled = true
	t.Cleanup(func() { common.AutomaticDisableChannelEnabled = previous })
	autoBan := 1
	channel := &model.Channel{Name: "candidate", Status: common.ChannelStatusEnabled, Models: "model-a", Group: "default", AutoBan: &autoBan}
	require.NoError(t, db.Create(channel).Error)
	now := int64(10_000)
	other, err := common.Marshal(map[string]any{"status_code": 503, "error_code": "bad_response_status_code", "error_type": "upstream_error"})
	require.NoError(t, err)
	for index, logType := range []int{model.LogTypeError, model.LogTypeError, model.LogTypeError, model.LogTypeConsume, model.LogTypeError, model.LogTypeError} {
		require.NoError(t, db.Create(&model.Log{
			CreatedAt: now - int64(index+1), Type: logType, ChannelId: channel.Id,
			ModelName: "model-a", RequestId: fmt.Sprintf("request-%d", index), Other: string(other),
		}).Error)
	}

	assert.Len(t, collectModelFailureCandidates([]*model.Channel{channel}, now)[channel.Id], 1)

	require.NoError(t, db.Create(&model.Log{
		CreatedAt: now, Type: model.LogTypeConsume, ChannelId: channel.Id,
		ModelName: "model-a", RequestId: "latest-success",
	}).Error)
	assert.Empty(t, collectModelFailureCandidates([]*model.Channel{channel}, now)[channel.Id])
}

func TestSelectChannelsForAutomaticTestAutoDisableUsesEligibleChannels(t *testing.T) {
	autoBanEnabled := 1
	autoBanDisabled := 0
	channels := []*model.Channel{
		{Id: 1, Status: common.ChannelStatusEnabled, AutoBan: &autoBanEnabled},
		{Id: 2, Status: common.ChannelStatusAutoDisabled, AutoBan: &autoBanEnabled},
		{Id: 3, Status: common.ChannelStatusEnabled, AutoBan: &autoBanDisabled},
		{Id: 4, Status: common.ChannelStatusManuallyDisabled, AutoBan: &autoBanEnabled},
	}

	selected := selectChannelsForAutomaticTest(channels, operation_setting.ChannelTestModeAutoDisable)

	require.Len(t, selected, 2)
	require.Equal(t, 1, selected[0].Id)
	require.Equal(t, 2, selected[1].Id)
}

func TestSelectChannelsForAutomaticTestPassiveRecoveryUsesAutoDisabledChannels(t *testing.T) {
	channels := []*model.Channel{
		{Id: 1, Status: common.ChannelStatusEnabled},
		{Id: 2, Status: common.ChannelStatusAutoDisabled},
		{Id: 3, Status: common.ChannelStatusAutoDisabled, OtherSettings: `{"channel_probe_enabled":false}`},
		{Id: 4, Status: common.ChannelStatusManuallyDisabled},
	}

	selected := selectChannelsForAutomaticTest(channels, operation_setting.ChannelTestModePassiveRecovery)

	require.Len(t, selected, 1)
	require.Equal(t, 2, selected[0].Id)
}

func TestSelectChannelsForAutomaticTestScheduledSkipsManualDisabled(t *testing.T) {
	channels := []*model.Channel{
		{Id: 1, Status: common.ChannelStatusEnabled},
		{Id: 2, Status: common.ChannelStatusAutoDisabled},
		{Id: 3, Status: common.ChannelStatusManuallyDisabled},
	}

	selected := selectChannelsForAutomaticTest(channels, operation_setting.ChannelTestModeScheduledAll)

	require.Len(t, selected, 2)
	require.Equal(t, 1, selected[0].Id)
	require.Equal(t, 2, selected[1].Id)
}

func TestSelectChannelsForAutomaticTestAutoDetectUsesChannelToggle(t *testing.T) {
	channels := []*model.Channel{
		{Id: 1, Status: common.ChannelStatusEnabled, OtherSettings: `{"channel_probe_enabled":true}`},
		{Id: 2, Status: common.ChannelStatusEnabled, OtherSettings: `{"channel_probe_enabled":false}`},
		{Id: 3, Status: common.ChannelStatusAutoDisabled, OtherSettings: `{"channel_probe_enabled":true}`},
		{Id: 4, Status: common.ChannelStatusManuallyDisabled, OtherSettings: `{"channel_probe_enabled":true}`},
	}

	selected := selectChannelsForAutomaticTest(channels, operation_setting.ChannelTestModeAutoDetect)

	require.Len(t, selected, 2)
	require.Equal(t, 1, selected[0].Id)
	require.Equal(t, 3, selected[1].Id)
}

func TestSelectChannelsForAutomaticTestAllIgnoresChannelDetectionToggle(t *testing.T) {
	channels := []*model.Channel{
		{Id: 1, Status: common.ChannelStatusEnabled, OtherSettings: `{"channel_probe_enabled":false}`},
		{Id: 2, Status: common.ChannelStatusAutoDisabled},
		{Id: 3, Status: common.ChannelStatusManuallyDisabled},
	}

	selected := selectChannelsForAutomaticTest(channels, operation_setting.ChannelTestModeScheduledAll)

	require.Len(t, selected, 2)
	require.Equal(t, 1, selected[0].Id)
	require.Equal(t, 2, selected[1].Id)
}

func TestChannelStatusFilterAutoMatchesEitherAutomaticSetting(t *testing.T) {
	autoBan := 1
	autoBanOff := 0
	tests := []struct {
		name    string
		channel *model.Channel
		want    bool
	}{
		{name: "automatic detection defaults on", channel: &model.Channel{}, want: true},
		{name: "automatic detection off", channel: &model.Channel{OtherSettings: `{"channel_probe_enabled":false}`}, want: false},
		{name: "automatic disable on", channel: &model.Channel{AutoBan: &autoBan}, want: true},
		{name: "neither setting", channel: &model.Channel{AutoBan: &autoBanOff, OtherSettings: `{"channel_probe_enabled":false}`}, want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.want, channelHasAutomaticSettings(test.channel))
		})
	}
	assert.Equal(t, channelStatusFilterAuto, parseStatusFilter("auto"))
}

func TestRunChannelTestWorkersHonorsConfiguredConcurrency(t *testing.T) {
	originalInterval := common.RequestInterval
	common.RequestInterval = 0
	t.Cleanup(func() { common.RequestInterval = originalInterval })

	channels := []*model.Channel{
		{Id: 1, Status: common.ChannelStatusEnabled},
		{Id: 2, Status: common.ChannelStatusEnabled},
		{Id: 3, Status: common.ChannelStatusEnabled},
		{Id: 4, Status: common.ChannelStatusEnabled},
	}
	started := make(chan struct{}, len(channels))
	release := make(chan struct{})
	var active atomic.Int32
	var maxActive atomic.Int32
	progress := make([]int, 0, len(channels)+1)
	summaryResult := make(chan channelTestSummary, 1)

	go func() {
		summaryResult <- runChannelTestWorkers(
			context.Background(),
			channels,
			2,
			func(_ context.Context, _ *model.Channel) channelTestSummary {
				current := active.Add(1)
				defer active.Add(-1)
				for {
					observed := maxActive.Load()
					if current <= observed || maxActive.CompareAndSwap(observed, current) {
						break
					}
				}
				started <- struct{}{}
				<-release
				return channelTestSummary{Tested: 1, Succeeded: 1}
			},
			func(processed, _ int) {
				progress = append(progress, processed)
			},
		)
	}()

	<-started
	<-started
	select {
	case <-started:
		t.Fatal("started more channel tests than the configured concurrency")
	default:
	}
	close(release)

	summary := <-summaryResult

	assert.Equal(t, int32(2), maxActive.Load())
	assert.Equal(t, channelTestSummary{Tested: 4, Succeeded: 4}, summary)
	assert.Equal(t, []int{0, 1, 2, 3, 4}, progress)
}

func TestRunChannelTestWorkersStopsAfterCancellation(t *testing.T) {
	originalInterval := common.RequestInterval
	common.RequestInterval = 0
	t.Cleanup(func() { common.RequestInterval = originalInterval })

	ctx, cancel := context.WithCancel(context.Background())
	channels := []*model.Channel{
		{Id: 1, Status: common.ChannelStatusEnabled},
		{Id: 2, Status: common.ChannelStatusEnabled},
		{Id: 3, Status: common.ChannelStatusEnabled},
		{Id: 4, Status: common.ChannelStatusEnabled},
	}
	started := make(chan struct{}, len(channels))
	progress := make([]int, 0, 1)
	summaryResult := make(chan channelTestSummary, 1)

	go func() {
		summaryResult <- runChannelTestWorkers(
			ctx,
			channels,
			2,
			func(ctx context.Context, _ *model.Channel) channelTestSummary {
				started <- struct{}{}
				<-ctx.Done()
				return channelTestSummary{Tested: 1, Succeeded: 1}
			},
			func(processed, _ int) {
				progress = append(progress, processed)
			},
		)
	}()

	<-started
	<-started
	cancel()

	summary := <-summaryResult

	select {
	case <-started:
		t.Fatal("started another channel test after cancellation")
	default:
	}
	assert.Equal(t, channelTestSummary{Tested: 2, Succeeded: 2}, summary)
	assert.Equal(t, []int{0}, progress)
}

func TestTestAllChannelsRejectsExistingActiveTask(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.SystemTask{}, &model.SystemTaskLock{}))

	existing, err := model.CreateSystemTask(model.SystemTaskTypeChannelTest, nil, nil)
	require.NoError(t, err)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/channel/test", nil)

	TestAllChannels(ctx)

	require.Equal(t, http.StatusConflict, recorder.Code)
	require.Contains(t, recorder.Body.String(), existing.TaskID)
	require.Contains(t, recorder.Body.String(), "已有通道测试任务正在运行或等待中")
}

func TestChannelHealthCheckCountsLocalErrorsAsFailed(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Log{}))
	channel := &model.Channel{
		Type:   constant.ChannelTypeMidjourney,
		Key:    "test-key",
		Status: common.ChannelStatusEnabled,
		Models: "model-a",
	}
	require.NoError(t, db.Create(channel).Error)

	summary := testChannelForHealthCheck(context.Background(), channel, 0, true, 1)

	assert.Equal(t, channelTestSummary{Tested: 1, Failed: 1}, summary)
}
