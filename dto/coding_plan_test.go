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

func TestCodingPlanPresetRejectsUnknownProvider(t *testing.T) {
	settings := &ChannelOtherSettings{}

	err := settings.ApplyCodingPlanPreset("missing-provider")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown coding plan provider")
}
