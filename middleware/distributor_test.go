package middleware

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
	kitdto "github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestGetModelRequestNormalizesClientModelAliasesFromPath(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name     string
		path     string
		expected string
	}{
		{
			name:     "mole gpt alias",
			path:     "/v1beta/models/mole-gpt5.6-sol:generateContent",
			expected: "gpt-5.6-sol",
		},
		{
			name:     "bare gpt alias",
			path:     "/v1beta/models/gpt5.6-sol:generateContent",
			expected: "gpt-5.6-sol",
		},
		{
			name:     "mole non gpt alias",
			path:     "/v1beta/models/mole-claude-opus-4-6:generateContent",
			expected: "claude-opus-4-6",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
			ctx.Request = httptest.NewRequest(http.MethodPost, tt.path, nil)

			request, shouldSelectChannel, err := getModelRequest(ctx)

			require.NoError(t, err)
			require.NotNil(t, request)
			assert.True(t, shouldSelectChannel)
			assert.Equal(t, tt.expected, request.Model)
		})
	}
}

func TestRequestPathForChannelSelectionNormalizesPlaygroundChat(t *testing.T) {
	assert.Equal(t, "/v1/chat/completions", requestPathForChannelSelection("/pg/chat/completions", "gpt-4o"))
	assert.Equal(t, "/v1/images/generations", requestPathForChannelSelection("/pg/chat/completions", "gpt-image-2"))
	assert.Equal(t, "/v1/images/generations", requestPathForChannelSelection("/v1/chat/completions", "gpt-image-2"))
	assert.Equal(t, "/v1/images/generations", requestPathForChannelSelection("/v1/responses", "gpt-image-2"))
	assert.Equal(t, "/v1/responses", requestPathForChannelSelection("/v1/responses", "gpt-4o"))
}

func TestDistributeUsesNormalizedPlaygroundPathForChannelSelection(t *testing.T) {
	originalDB := model.DB
	originalMemoryCacheEnabled := common.MemoryCacheEnabled
	db, err := gorm.Open(sqlite.Open("file:distributor_playground_path?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.Ability{}))
	model.DB = db
	common.MemoryCacheEnabled = true
	t.Cleanup(func() {
		model.DB = originalDB
		common.MemoryCacheEnabled = originalMemoryCacheEnabled
		if originalMemoryCacheEnabled && originalDB != nil &&
			originalDB.Migrator().HasTable(&model.Channel{}) && originalDB.Migrator().HasTable(&model.Ability{}) {
			model.InitChannelCache()
		}
		sqlDB, closeErr := db.DB()
		if closeErr == nil {
			require.NoError(t, sqlDB.Close())
		}
	})

	channel := &model.Channel{
		Id:     341,
		Type:   constant.ChannelTypeAdvancedCustom,
		Key:    "test-key",
		Status: common.ChannelStatusEnabled,
		Name:   "playground-route",
		Models: "gpt-5.6-luna",
		Group:  "relay",
	}
	channel.SetOtherSettings(kitdto.ChannelOtherSettings{AdvancedCustom: &kitdto.AdvancedCustomConfig{
		Routes: []kitdto.AdvancedCustomRoute{{IncomingPath: "/v1/chat/completions"}},
	}})
	require.NoError(t, db.Create(channel).Error)
	priority := int64(0)
	require.NoError(t, db.Create(&model.Ability{
		Group:     "relay",
		Model:     "gpt-5.6-luna",
		ChannelId: 341,
		Enabled:   true,
		Priority:  &priority,
		Weight:    100,
	}).Error)
	model.InitChannelCache()

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/pg/chat/completions", strings.NewReader(`{"model":"gpt-5.6-luna"}`))
	ctx.Request.Header.Set("Content-Type", "application/json")
	common.SetContextKey(ctx, constant.ContextKeyUsingGroup, "relay")

	Distribute()(ctx)

	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Equal(t, 341, common.GetContextKeyInt(ctx, constant.ContextKeyChannelId))
}

func distributorTaskPluginSource(key string, channelType int) string {
	return fmt.Sprintf(`
export const meta = {
  apiVersion: 1,
  key: %q,
  name: %q,
  version: "1.0.0",
  author: {name: "Test"},
  channelTypes: [%d],
  models: ["task-model"],
  fetchMode: "per_task",
};
export function buildSubmitRequest() { return {}; }
export function parseSubmitResponse() { return {taskId: "task"}; }
export function buildQueryRequest() { return {}; }
export function parseTaskResult() { return {status: "SUCCESS"}; }
`, key, key, channelType)
}

func TestTokenModelLimitAllowsLegacyAliasAndModifierVariant(t *testing.T) {
	aliasOnly := map[string]bool{"claude-3-7-sonnet-thinking": true}
	assert.True(t, tokenModelLimitAllows(aliasOnly, "claude-3-7-sonnet-thinking"))
	assert.False(t, tokenModelLimitAllows(aliasOnly, "claude-3-7-sonnet"))

	baseOnly := map[string]bool{"claude-3-7-sonnet": true}
	assert.True(t, tokenModelLimitAllows(baseOnly, "claude-3-7-sonnet@thinking:on"))
	assert.True(t, tokenModelLimitAllows(baseOnly, "claude-3-7-sonnet-thinking"))

	wildcard := map[string]bool{"gemini-2.5-flash-thinking-*": true}
	assert.True(t, tokenModelLimitAllows(wildcard, "gemini-2.5-flash-thinking-8192"))
}

func TestTokenModelLimitAllowsExemptAtNameByFullName(t *testing.T) {
	settings := model_setting.GetGlobalSettings()
	original := append([]string(nil), settings.ThinkingModelBlacklist...)
	t.Cleanup(func() { settings.ThinkingModelBlacklist = original })
	settings.ThinkingModelBlacklist = append(original, "re:.*@sha256:.*")

	fullOnly := map[string]bool{"opaque@sha256:deadbeef": true}
	assert.True(t, tokenModelLimitAllows(fullOnly, "opaque@sha256:deadbeef"))

	baseOnly := map[string]bool{"opaque": true}
	assert.False(t, tokenModelLimitAllows(baseOnly, "opaque@sha256:deadbeef"))
}

func TestNoAvailableChannelMessageNamesClaimingTaskPlugin(t *testing.T) {
	require.NoError(t, i18n.Init())
	registry := jsplugin.NewRegistry()
	plugin, err := registry.Register(distributorTaskPluginSource("claimer", constant.ChannelTypeKling), jsplugin.Options{})
	require.NoError(t, err)

	pinned, _ := gin.CreateTestContext(nil)
	pinned.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", nil)
	pinned.Request.Header.Set("Accept-Language", "en")
	pinned.Set(jsplugin.ContextKeyPinnedPlugin, jsplugin.PinnedPlugin{Generation: registry.Generation(), Plugin: plugin})
	message := noAvailableChannelMessage(pinned, "default", "kling-v1")
	assert.Contains(t, message, `"claimer"`)
	assert.Contains(t, message, "disable or override")
	assert.Contains(t, message, "kling-v1")

	plain, _ := gin.CreateTestContext(nil)
	plain.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	plain.Request.Header.Set("Accept-Language", "en")
	generic := noAvailableChannelMessage(plain, "default", "gpt-4o")
	assert.NotContains(t, generic, "task plugin")
	assert.Contains(t, generic, "gpt-4o")
}
