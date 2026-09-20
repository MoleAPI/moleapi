package service

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func buildChannelAffinityTemplateContextForTest(meta channelAffinityMeta) *gin.Context {
	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	setChannelAffinityContext(ctx, meta)
	return ctx
}

func TestApplyChannelAffinityOverrideTemplate_NoTemplate(t *testing.T) {
	ctx := buildChannelAffinityTemplateContextForTest(channelAffinityMeta{
		RuleName: "rule-no-template",
	})
	base := map[string]any{
		"temperature": 0.7,
	}

	merged, applied := ApplyChannelAffinityOverrideTemplate(ctx, base)
	require.False(t, applied)
	require.Equal(t, base, merged)
}

func TestGetChannelAffinitySessionSeed(t *testing.T) {
	ctx := buildChannelAffinityTemplateContextForTest(channelAffinityMeta{CacheKey: "new-api:channel_affinity:v1:rule:model:group:key"})

	require.Equal(t, "new-api:channel_affinity:v1:rule:model:group:key|channel:42", GetChannelAffinitySessionSeed(ctx, 42))
	require.NotEqual(t, GetChannelAffinitySessionSeed(ctx, 42), GetChannelAffinitySessionSeed(ctx, 43))
	require.Empty(t, GetChannelAffinitySessionSeed(ctx, 0))
	require.Empty(t, GetChannelAffinitySessionSeed(buildChannelAffinityTemplateContextForTest(channelAffinityMeta{}), 42))
}

func TestApplyChannelAffinityOverrideTemplate_MergeTemplate(t *testing.T) {
	ctx := buildChannelAffinityTemplateContextForTest(channelAffinityMeta{
		RuleName: "rule-with-template",
		ParamTemplate: map[string]any{
			"temperature": 0.2,
			"top_p":       0.95,
		},
		UsingGroup:     "default",
		ModelName:      "gpt-4.1",
		RequestPath:    "/v1/responses",
		KeySourceType:  "gjson",
		KeySourcePath:  "prompt_cache_key",
		KeyHint:        "abcd...wxyz",
		KeyFingerprint: "abcd1234",
	})
	base := map[string]any{
		"temperature": 0.7,
		"max_tokens":  2000,
	}

	merged, applied := ApplyChannelAffinityOverrideTemplate(ctx, base)
	require.True(t, applied)
	require.Equal(t, 0.7, merged["temperature"])
	require.Equal(t, 0.95, merged["top_p"])
	require.Equal(t, 2000, merged["max_tokens"])
	require.Equal(t, 0.7, base["temperature"])

	anyInfo, ok := ctx.Get(ginKeyChannelAffinityLogInfo)
	require.True(t, ok)
	info, ok := anyInfo.(map[string]any)
	require.True(t, ok)
	overrideInfoAny, ok := info["override_template"]
	require.True(t, ok)
	overrideInfo, ok := overrideInfoAny.(map[string]any)
	require.True(t, ok)
	require.Equal(t, true, overrideInfo["applied"])
	require.Equal(t, "rule-with-template", overrideInfo["rule_name"])
	require.EqualValues(t, 2, overrideInfo["param_override_keys"])
}

func TestApplyChannelAffinityOverrideTemplate_MergeOperations(t *testing.T) {
	ctx := buildChannelAffinityTemplateContextForTest(channelAffinityMeta{
		RuleName: "rule-with-ops-template",
		ParamTemplate: map[string]any{
			"operations": []map[string]any{
				{
					"mode":  "pass_headers",
					"value": []string{"Originator"},
				},
			},
		},
	})
	base := map[string]any{
		"temperature": 0.7,
		"operations": []map[string]any{
			{
				"path":  "model",
				"mode":  "trim_prefix",
				"value": "openai/",
			},
		},
	}

	merged, applied := ApplyChannelAffinityOverrideTemplate(ctx, base)
	require.True(t, applied)
	require.Equal(t, 0.7, merged["temperature"])

	opsAny, ok := merged["operations"]
	require.True(t, ok)
	ops, ok := opsAny.([]any)
	require.True(t, ok)
	require.Len(t, ops, 2)

	firstOp, ok := ops[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "pass_headers", firstOp["mode"])

	secondOp, ok := ops[1].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "trim_prefix", secondOp["mode"])
}

func TestShouldSkipRetryAfterChannelAffinityFailure(t *testing.T) {
	tests := []struct {
		name string
		ctx  func() *gin.Context
		want bool
	}{
		{
			name: "nil context",
			ctx: func() *gin.Context {
				return nil
			},
			want: false,
		},
		{
			name: "explicit skip retry flag in context",
			ctx: func() *gin.Context {
				ctx := buildChannelAffinityTemplateContextForTest(channelAffinityMeta{
					RuleName:   "rule-explicit-flag",
					SkipRetry:  false,
					UsingGroup: "default",
					ModelName:  "gpt-5",
				})
				ctx.Set(ginKeyChannelAffinitySkipRetry, true)
				return ctx
			},
			want: true,
		},
		{
			name: "fallback to matched rule meta",
			ctx: func() *gin.Context {
				return buildChannelAffinityTemplateContextForTest(channelAffinityMeta{
					RuleName:   "rule-skip-retry",
					SkipRetry:  true,
					UsingGroup: "default",
					ModelName:  "gpt-5",
				})
			},
			want: true,
		},
		{
			name: "no flag and no skip retry meta",
			ctx: func() *gin.Context {
				return buildChannelAffinityTemplateContextForTest(channelAffinityMeta{
					RuleName:   "rule-no-skip-retry",
					SkipRetry:  false,
					UsingGroup: "default",
					ModelName:  "gpt-5",
				})
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, ShouldSkipRetryAfterChannelAffinityFailure(tt.ctx()))
		})
	}
}

func TestExtractChannelAffinityValue_RequestHeader(t *testing.T) {
	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	ctx.Request.Header.Set("X-Affinity-Key", " tenant-123 ")

	value := extractChannelAffinityValue(ctx, operation_setting.ChannelAffinityKeySource{
		Type: "request_header",
		Key:  "X-Affinity-Key",
	})

	require.Equal(t, "tenant-123", value)
}

func TestGetPreferredChannelByAffinity_RequestHeaderKeySource(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rule := operation_setting.ChannelAffinityRule{
		Name:       "header-affinity",
		ModelRegex: []string{"^gpt-.*$"},
		PathRegex:  []string{"/v1/responses"},
		KeySources: []operation_setting.ChannelAffinityKeySource{
			{Type: "request_header", Key: "X-Affinity-Key"},
		},
		IncludeRuleName:  true,
		IncludeModelName: true,
	}

	affinityValue := fmt.Sprintf("header-hit-%d", time.Now().UnixNano())
	cacheKeySuffix := buildChannelAffinityCacheKeySuffix(rule, "gpt-5", "default", affinityValue)

	cache := getChannelAffinityCache()
	require.NoError(t, cache.SetWithTTL(cacheKeySuffix, 9528, time.Minute))
	t.Cleanup(func() {
		_, _ = cache.DeleteMany([]string{cacheKeySuffix})
	})

	setting := operation_setting.GetChannelAffinitySetting()
	originalRules := setting.Rules
	setting.Rules = append([]operation_setting.ChannelAffinityRule{rule}, originalRules...)
	t.Cleanup(func() {
		setting.Rules = originalRules
	})

	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	ctx.Request.Header.Set("X-Affinity-Key", affinityValue)

	channelID, found := GetPreferredChannelByAffinity(ctx, "gpt-5", "default")
	require.True(t, found)
	require.Equal(t, 9528, channelID)

	meta, ok := getChannelAffinityMeta(ctx)
	require.True(t, ok)
	require.Equal(t, "request_header", meta.KeySourceType)
	require.Equal(t, "X-Affinity-Key", meta.KeySourceKey)
	require.Equal(t, buildChannelAffinityKeyHint(affinityValue), meta.KeyHint)
}

func TestChannelAffinitySessionModes(t *testing.T) {
	setting := operation_setting.GetChannelAffinitySetting()
	previous := *setting
	t.Cleanup(func() { *setting = previous })
	for _, tc := range []struct {
		name, mode, global string
		legacy, skip, off  bool
	}{
		{"legacy strict", "", "prefer", true, true, false},
		{"legacy prefer", "", "strict", false, false, false},
		{"explicit prefer", "prefer", "strict", true, false, false},
		{"explicit strict", "strict", "prefer", false, true, false},
		{"inherit", "inherit", "strict", false, true, false},
		{"off", "off", "strict", true, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rule := operation_setting.ChannelAffinityRule{Name: "mode-test", ModelRegex: []string{".*"}, KeySources: []operation_setting.ChannelAffinityKeySource{{Type: "request_header", Key: "X-Test-Session"}}, SessionMode: tc.mode, SkipRetryOnFailure: tc.legacy, ParamOverrideTemplate: map[string]any{"test": true}}
			setting.Enabled, setting.SessionMode, setting.Rules = true, tc.global, []operation_setting.ChannelAffinityRule{rule}
			key := buildChannelAffinityCacheKeySuffix(rule, "model", "default", tc.name)
			cache := getChannelAffinityCache()
			require.NoError(t, cache.SetWithTTL(key, 12, time.Minute))
			t.Cleanup(func() { _, _ = cache.DeleteMany([]string{key}) })
			ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
			ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
			ctx.Request.Header.Set("X-Test-Session", tc.name)
			id, found := GetPreferredChannelByAffinity(ctx, "model", "default")
			assert.Equal(t, !tc.off, found)
			if !tc.off {
				assert.Equal(t, 12, id)
			}
			meta, ok := getChannelAffinityMeta(ctx)
			require.True(t, ok)
			assert.Equal(t, tc.skip, meta.SkipRetry)
			assert.Equal(t, true, meta.ParamTemplate["test"])
			if tc.off {
				RecordChannelAffinity(ctx, 99)
				cached, _, err := cache.Get(key)
				require.NoError(t, err)
				assert.Equal(t, 12, cached, "off must not overwrite existing session mappings")
			}
		})
	}
}

func TestClearCurrentChannelAffinityCache(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cacheKeySuffix := fmt.Sprintf("codex cli trace:default:clear-current-%d", time.Now().UnixNano())
	cacheKeyFull := channelAffinityCacheNamespace + ":" + cacheKeySuffix
	cache := getChannelAffinityCache()
	require.NoError(t, cache.SetWithTTL(cacheKeySuffix, 9527, time.Minute))
	t.Cleanup(func() {
		_, _ = cache.DeleteMany([]string{cacheKeySuffix})
	})

	ctx := buildChannelAffinityTemplateContextForTest(channelAffinityMeta{
		CacheKey:   cacheKeyFull,
		TTLSeconds: 60,
		RuleName:   "codex cli trace",
		SkipRetry:  true,
	})
	require.True(t, ShouldSkipRetryAfterChannelAffinityFailure(ctx))

	deleted := ClearCurrentChannelAffinityCache(ctx)
	require.True(t, deleted)
	_, found, err := cache.Get(cacheKeySuffix)
	require.NoError(t, err)
	require.False(t, found)
	require.False(t, ShouldSkipRetryAfterChannelAffinityFailure(ctx))
}

func TestChannelAffinityHitCodexTemplatePassHeadersEffective(t *testing.T) {
	gin.SetMode(gin.TestMode)

	setting := operation_setting.GetChannelAffinitySetting()
	require.NotNil(t, setting)

	var codexRule *operation_setting.ChannelAffinityRule
	for i := range setting.Rules {
		rule := &setting.Rules[i]
		if strings.EqualFold(strings.TrimSpace(rule.Name), "codex cli trace") {
			codexRule = rule
			break
		}
	}
	require.NotNil(t, codexRule)

	affinityValue := fmt.Sprintf("pc-hit-%d", time.Now().UnixNano())
	cacheKeySuffix := buildChannelAffinityCacheKeySuffix(*codexRule, "gpt-5", "default", affinityValue)

	cache := getChannelAffinityCache()
	require.NoError(t, cache.SetWithTTL(cacheKeySuffix, 9527, time.Minute))
	t.Cleanup(func() {
		_, _ = cache.DeleteMany([]string{cacheKeySuffix})
	})

	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(fmt.Sprintf(`{"prompt_cache_key":"%s"}`, affinityValue)))
	ctx.Request.Header.Set("Content-Type", "application/json")

	channelID, found := GetPreferredChannelByAffinity(ctx, "gpt-5", "default")
	require.True(t, found)
	require.Equal(t, 9527, channelID)

	baseOverride := map[string]any{
		"temperature": 0.2,
	}
	mergedOverride, applied := ApplyChannelAffinityOverrideTemplate(ctx, baseOverride)
	require.True(t, applied)
	require.Equal(t, 0.2, mergedOverride["temperature"])

	info := &relaycommon.RelayInfo{
		RequestHeaders: map[string]string{
			"Originator": "Codex CLI",
			"Session_id": "sess-123",
			"User-Agent": "codex-cli-test",
		},
		ChannelMeta: &relaycommon.ChannelMeta{
			ParamOverride: mergedOverride,
			HeadersOverride: map[string]any{
				"X-Static": "legacy-static",
			},
		},
	}

	_, err := relaycommon.ApplyParamOverrideWithRelayInfo([]byte(`{"model":"gpt-5"}`), info)
	require.NoError(t, err)
	require.True(t, info.UseRuntimeHeadersOverride)

	require.Equal(t, "legacy-static", info.RuntimeHeadersOverride["x-static"])
	require.Equal(t, "Codex CLI", info.RuntimeHeadersOverride["originator"])
	require.Equal(t, "sess-123", info.RuntimeHeadersOverride["session_id"])
	require.Equal(t, "codex-cli-test", info.RuntimeHeadersOverride["user-agent"])

	_, exists := info.RuntimeHeadersOverride["x-codex-beta-features"]
	require.False(t, exists)
	_, exists = info.RuntimeHeadersOverride["x-codex-turn-metadata"]
	require.False(t, exists)
}
