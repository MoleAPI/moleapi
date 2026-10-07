package dto

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCodingPlanPresetBuildsGLMFallbackResponsesRoute(t *testing.T) {
	settings := &ChannelOtherSettings{}
	require.NoError(t, settings.ApplyCodingPlanPreset(CodingPlanProviderGLMChina))
	require.Equal(t, CodingPlanProviderGLMChina, settings.CodingPlanProvider)
	require.NotNil(t, settings.AdvancedCustom)

	route, ok := settings.AdvancedCustom.MatchPath(advancedCustomEndpointPathOpenAIResponses)
	require.True(t, ok)
	assert.Equal(t, advancedCustomConverterOpenAIResponsesToOpenAIChat, route.Converter)
	assert.Equal(t, "https://open.bigmodel.cn/api/coding/paas/v4/chat/completions", route.UpstreamPath)
	route, ok = settings.AdvancedCustom.MatchPath(advancedCustomEndpointPathOpenAICompletions)
	require.True(t, ok)
	assert.Equal(t, advancedCustomConverterOpenAICompletionsToChat, route.Converter)
	assert.Equal(t, "https://open.bigmodel.cn/api/coding/paas/v4/chat/completions", route.UpstreamPath)
	route, ok = settings.AdvancedCustom.MatchPath(advancedCustomEndpointPathImageGeneration)
	require.True(t, ok)
	assert.Equal(t, advancedCustomConverterNone, route.Converter)
	assert.Equal(t, "https://open.bigmodel.cn/api/coding/paas/v4/images/generations", route.UpstreamPath)
	route, ok = settings.AdvancedCustom.MatchPath(advancedCustomEndpointPathClaudeMessages)
	require.True(t, ok)
	assert.Equal(t, advancedCustomConverterNone, route.Converter)
	assert.Equal(t, "https://open.bigmodel.cn/api/anthropic/v1/messages", route.UpstreamPath)
}

func TestCodingPlanPresetBuildsNativeResponsesRoute(t *testing.T) {
	settings := &ChannelOtherSettings{}
	require.NoError(t, settings.ApplyCodingPlanPreset(CodingPlanProviderKimi))
	route, ok := settings.AdvancedCustom.MatchPath(advancedCustomEndpointPathOpenAIResponses)
	require.True(t, ok)
	assert.Equal(t, advancedCustomConverterNone, route.Converter)
	assert.Equal(t, "https://api.kimi.com/coding/v1/responses", route.UpstreamPath)
	modelListRoute, ok := settings.AdvancedCustom.ModelListRoute()
	require.True(t, ok)
	assert.Equal(t, "https://api.kimi.com/coding/v1/models", modelListRoute.UpstreamPath)
}

func TestOpenCodeGoPresetUsesNativeResponses(t *testing.T) {
	settings := &ChannelOtherSettings{}
	require.NoError(t, settings.ApplyCodingPlanPreset(CodingPlanProviderOpenCodeGo))
	route, ok := settings.AdvancedCustom.MatchPathForModel(advancedCustomEndpointPathOpenAIResponses, "deepseek-v4-flash")
	require.True(t, ok)
	assert.Equal(t, advancedCustomConverterNone, route.Converter)
	assert.Equal(t, "https://opencode.ai/zen/go/v1/responses", route.UpstreamPath)

	fallback, ok := settings.AdvancedCustom.MatchPathForModel(advancedCustomEndpointPathOpenAIResponses, "glm-5.2")
	require.True(t, ok)
	assert.Equal(t, advancedCustomConverterOpenAIResponsesToOpenAIChat, fallback.Converter)
	assert.Equal(t, "https://opencode.ai/zen/go/v1/chat/completions", fallback.UpstreamPath)

	messages, ok := settings.AdvancedCustom.MatchPath(advancedCustomEndpointPathClaudeMessages)
	require.True(t, ok)
	assert.Equal(t, "https://opencode.ai/zen/go/v1/messages", messages.UpstreamPath)
}

func TestCodingPlanPresetsIncludeVolcengineAgentAndCommandCode(t *testing.T) {
	for _, test := range []struct {
		provider string
		openAI   string
	}{
		{CodingPlanProviderVolcengineAgent, "https://ark.cn-beijing.volces.com/api/plan"},
		{CodingPlanProviderCommandCode, "https://api.commandcode.ai/provider/v1"},
	} {
		settings := &ChannelOtherSettings{}
		require.NoError(t, settings.ApplyCodingPlanPreset(test.provider))
		route, ok := settings.AdvancedCustom.MatchPath(advancedCustomEndpointPathOpenAICompletions)
		require.True(t, ok)
		assert.Equal(t, test.openAI+"/chat/completions", route.UpstreamPath)
	}
}

func TestCodingPlanPresetBuildsCustomPlaceholderRoutes(t *testing.T) {
	settings := &ChannelOtherSettings{}
	require.NoError(t, settings.ApplyCodingPlanPreset(CodingPlanProviderCustom))
	for _, expected := range []struct{ path, url string }{
		{advancedCustomEndpointPathOpenAIChat, "https://your-openai-compatible-base-url.example/v1/chat/completions"},
		{advancedCustomEndpointPathOpenAIResponses, "https://your-openai-compatible-base-url.example/v1/responses"},
		{advancedCustomEndpointPathImageGeneration, "https://your-openai-compatible-base-url.example/v1/images/generations"},
		{advancedCustomEndpointPathClaudeMessages, "https://your-anthropic-compatible-base-url.example/v1/messages"},
	} {
		route, ok := settings.AdvancedCustom.MatchPath(expected.path)
		require.True(t, ok)
		assert.Equal(t, expected.url, route.UpstreamPath)
	}
}

func TestCodingPlanPresetRejectsUnknownProvider(t *testing.T) {
	err := (&ChannelOtherSettings{}).ApplyCodingPlanPreset("missing-provider")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown coding plan provider")
}

func TestCodingPlanQuotaConfigValidatesWindows(t *testing.T) {
	config := &CodingPlanQuotaConfig{
		Unit: CodingPlanQuotaUnitTokens,
		Windows: []CodingPlanQuotaWindow{
			{DurationSeconds: 5 * 60 * 60, Limit: 1000},
			{DurationSeconds: 7 * 24 * 60 * 60, Limit: 5000},
		},
	}
	require.NoError(t, config.Validate())

	config.Windows[1].DurationSeconds = config.Windows[0].DurationSeconds
	assert.Error(t, config.Validate())
}

func TestCodingPlanQuotaConfigRejectsInvalidValues(t *testing.T) {
	for name, config := range map[string]*CodingPlanQuotaConfig{
		"unknown unit":  {Unit: "credits", Windows: []CodingPlanQuotaWindow{{DurationSeconds: 60, Limit: 1}}},
		"empty windows": {Unit: CodingPlanQuotaUnitRequests},
		"zero limit":    {Unit: CodingPlanQuotaUnitRequests, Windows: []CodingPlanQuotaWindow{{DurationSeconds: 60, Limit: 0}}},
		"short window":  {Unit: CodingPlanQuotaUnitRequests, Windows: []CodingPlanQuotaWindow{{DurationSeconds: 1, Limit: 1}}},
	} {
		t.Run(name, func(t *testing.T) {
			assert.Error(t, config.Validate())
		})
	}
}
