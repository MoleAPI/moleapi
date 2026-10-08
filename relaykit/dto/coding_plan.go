package dto

import (
	"fmt"
	"strings"
	"time"
)

const (
	CodingPlanProviderGLMChina        = "glm-coding-plan"
	CodingPlanProviderGLMGlobal       = "glm-coding-plan-international"
	CodingPlanProviderKimi            = "kimi-coding-plan"
	CodingPlanProviderDoubao          = "doubao-coding-plan"
	CodingPlanProviderVolcengineAgent = "volcengine-agent-plan"
	CodingPlanProviderQwenCoding      = "qwen-coding-plan"
	CodingPlanProviderQwenTokenPlan   = "qwen-token-plan"
	CodingPlanProviderMiniMax         = "minimax-token-plan"
	CodingPlanProviderOpenCodeGo      = "opencode-go"
	CodingPlanProviderCommandCode     = "command-code"
	CodingPlanProviderCustom          = "custom-coding-plan"
)

const (
	CodingPlanQuotaUnitRequests = "requests"
	CodingPlanQuotaUnitTokens   = "tokens"
	CodingPlanQuotaMaxWindows   = 8
	CodingPlanQuotaMinWindow    = int64(time.Minute / time.Second)
	CodingPlanQuotaMaxWindow    = int64((365 * 24 * time.Hour) / time.Second)
)

// CodingPlanQuotaWindow is a rolling local quota window. It is deliberately
// independent from a provider's subscription API: it protects a channel even
// when the upstream does not expose a usable quota endpoint.
type CodingPlanQuotaWindow struct {
	DurationSeconds int64 `json:"duration_seconds"`
	Limit           int64 `json:"limit"`
}

type CodingPlanQuotaConfig struct {
	Unit    string                  `json:"unit"`
	Windows []CodingPlanQuotaWindow `json:"windows"`
}

func (c *CodingPlanQuotaConfig) Validate() error {
	if c == nil {
		return nil
	}
	switch strings.TrimSpace(c.Unit) {
	case CodingPlanQuotaUnitRequests, CodingPlanQuotaUnitTokens:
	default:
		return fmt.Errorf("invalid coding plan quota unit: %s", c.Unit)
	}
	if len(c.Windows) == 0 || len(c.Windows) > CodingPlanQuotaMaxWindows {
		return fmt.Errorf("coding plan quota windows must contain 1-%d items", CodingPlanQuotaMaxWindows)
	}
	seen := make(map[int64]struct{}, len(c.Windows))
	for _, window := range c.Windows {
		if window.DurationSeconds < CodingPlanQuotaMinWindow || window.DurationSeconds > CodingPlanQuotaMaxWindow {
			return fmt.Errorf("invalid coding plan quota window duration: %d", window.DurationSeconds)
		}
		if window.Limit <= 0 {
			return fmt.Errorf("coding plan quota window limit must be positive")
		}
		if _, ok := seen[window.DurationSeconds]; ok {
			return fmt.Errorf("duplicate coding plan quota window duration: %d", window.DurationSeconds)
		}
		seen[window.DurationSeconds] = struct{}{}
	}
	return nil
}

type CodingPlanPreset struct {
	ID               string
	OpenAIBaseURL    string
	AnthropicBaseURL string
	ResponsesBaseURL string
	ModelListBaseURL string
}

var codingPlanPresets = []CodingPlanPreset{
	{ID: CodingPlanProviderGLMChina, OpenAIBaseURL: "https://open.bigmodel.cn/api/coding/paas/v4", AnthropicBaseURL: "https://open.bigmodel.cn/api/anthropic", ModelListBaseURL: "https://open.bigmodel.cn/api/coding/paas/v4"},
	{ID: CodingPlanProviderGLMGlobal, OpenAIBaseURL: "https://api.z.ai/api/coding/paas/v4", AnthropicBaseURL: "https://api.z.ai/api/anthropic", ModelListBaseURL: "https://api.z.ai/api/coding/paas/v4"},
	{ID: CodingPlanProviderKimi, OpenAIBaseURL: "https://api.kimi.com/coding/v1", AnthropicBaseURL: "https://api.kimi.com/coding", ResponsesBaseURL: "https://api.kimi.com/coding/v1", ModelListBaseURL: "https://api.kimi.com/coding/v1"},
	{ID: CodingPlanProviderDoubao, OpenAIBaseURL: "https://ark.cn-beijing.volces.com/api/coding/v3", AnthropicBaseURL: "https://ark.cn-beijing.volces.com/api/coding", ResponsesBaseURL: "https://ark.cn-beijing.volces.com/api/coding/v3", ModelListBaseURL: "https://ark.cn-beijing.volces.com/api/coding/v3"},
	{ID: CodingPlanProviderVolcengineAgent, OpenAIBaseURL: "https://ark.cn-beijing.volces.com/api/plan", AnthropicBaseURL: "https://ark.cn-beijing.volces.com/api/plan", ResponsesBaseURL: "https://ark.cn-beijing.volces.com/api/plan", ModelListBaseURL: "https://ark.cn-beijing.volces.com/api/plan"},
	{ID: CodingPlanProviderQwenCoding, OpenAIBaseURL: "https://coding-intl.dashscope.aliyuncs.com/v1", AnthropicBaseURL: "https://coding-intl.dashscope.aliyuncs.com/apps/anthropic", ModelListBaseURL: "https://coding-intl.dashscope.aliyuncs.com/v1"},
	{ID: CodingPlanProviderQwenTokenPlan, OpenAIBaseURL: "https://token-plan.ap-southeast-1.maas.aliyuncs.com/compatible-mode/v1", AnthropicBaseURL: "https://token-plan.ap-southeast-1.maas.aliyuncs.com/apps/anthropic", ResponsesBaseURL: "https://token-plan.ap-southeast-1.maas.aliyuncs.com/compatible-mode/v1", ModelListBaseURL: "https://token-plan.ap-southeast-1.maas.aliyuncs.com/compatible-mode/v1"},
	{ID: CodingPlanProviderMiniMax, OpenAIBaseURL: "https://api.minimax.io/v1", AnthropicBaseURL: "https://api.minimax.io/anthropic", ResponsesBaseURL: "https://api.minimax.io/v1", ModelListBaseURL: "https://api.minimax.io/v1"},
	{ID: CodingPlanProviderOpenCodeGo, OpenAIBaseURL: "https://opencode.ai/zen/go/v1", AnthropicBaseURL: "https://opencode.ai/zen/go", ModelListBaseURL: "https://opencode.ai/zen/go/v1"},
	{ID: CodingPlanProviderCommandCode, OpenAIBaseURL: "https://api.commandcode.ai/provider/v1", AnthropicBaseURL: "https://api.commandcode.ai/provider", ResponsesBaseURL: "https://api.commandcode.ai/provider/v1", ModelListBaseURL: "https://api.commandcode.ai/provider/v1"},
	{ID: CodingPlanProviderCustom, OpenAIBaseURL: "https://your-openai-compatible-base-url.example/v1", AnthropicBaseURL: "https://your-anthropic-compatible-base-url.example", ResponsesBaseURL: "https://your-openai-compatible-base-url.example/v1", ModelListBaseURL: "https://your-openai-compatible-base-url.example/v1"},
}

func ResolveCodingPlanPreset(value string) (CodingPlanPreset, bool) {
	value = normalizeCodingPlanPresetValue(value)
	if value == "" {
		return CodingPlanPreset{}, false
	}
	for _, preset := range codingPlanPresets {
		if strings.EqualFold(value, preset.ID) || strings.EqualFold(value, normalizeCodingPlanPresetValue(preset.OpenAIBaseURL)) || strings.EqualFold(value, normalizeCodingPlanPresetValue(preset.AnthropicBaseURL)) || strings.EqualFold(value, normalizeCodingPlanPresetValue(preset.ResponsesBaseURL)) {
			return preset, true
		}
	}
	return CodingPlanPreset{}, false
}

func (s *ChannelOtherSettings) ApplyCodingPlanPreset(provider string) error {
	if s == nil {
		return fmt.Errorf("coding plan settings are required")
	}
	preset, ok := ResolveCodingPlanPreset(provider)
	if !ok {
		return fmt.Errorf("unknown coding plan provider: %s", strings.TrimSpace(provider))
	}
	config := preset.AdvancedCustomConfig()
	if err := config.Validate(); err != nil {
		return err
	}
	s.CodingPlanProvider = preset.ID
	s.AdvancedCustom = config
	return nil
}

func (p CodingPlanPreset) AdvancedCustomConfig() *AdvancedCustomConfig {
	routes := make([]AdvancedCustomRoute, 0, 7)
	if p.OpenAIBaseURL != "" {
		chatURL := codingPlanJoinPath(p.OpenAIBaseURL, "chat/completions")
		imageURL := codingPlanJoinPath(p.OpenAIBaseURL, "images/generations")
		routes = append(routes, codingPlanRoute(advancedCustomEndpointPathOpenAIChat, chatURL, advancedCustomConverterNone), codingPlanRoute(advancedCustomEndpointPathOpenAICompletions, chatURL, advancedCustomConverterOpenAICompletionsToChat), codingPlanRoute("/v1beta/models/{model}:generateContent", chatURL, advancedCustomConverterGeminiContentToOpenAIChat), codingPlanRoute(advancedCustomEndpointPathImageGeneration, imageURL, advancedCustomConverterNone))
		if p.ID == CodingPlanProviderOpenCodeGo {
			responsesRoute := codingPlanRoute(advancedCustomEndpointPathOpenAIResponses, codingPlanJoinPath(p.OpenAIBaseURL, "responses"), advancedCustomConverterNone)
			responsesRoute.Models = []string{"deepseek-v4-flash"}
			routes = append(routes, responsesRoute)
		}
		if p.ResponsesBaseURL == "" {
			routes = append(routes, codingPlanRoute(advancedCustomEndpointPathOpenAIResponses, chatURL, advancedCustomConverterOpenAIResponsesToOpenAIChat))
		}
	}
	if p.ResponsesBaseURL != "" {
		routes = append(routes, codingPlanRoute(advancedCustomEndpointPathOpenAIResponses, codingPlanJoinPath(p.ResponsesBaseURL, "responses"), advancedCustomConverterNone))
	}
	if p.AnthropicBaseURL != "" {
		routes = append(routes, codingPlanRoute(advancedCustomEndpointPathClaudeMessages, codingPlanJoinPath(p.AnthropicBaseURL, "v1/messages"), advancedCustomConverterNone))
	}
	if p.ModelListBaseURL != "" {
		routes = append(routes, codingPlanRoute(AdvancedCustomModelListPath, codingPlanJoinPath(p.ModelListBaseURL, "models"), advancedCustomConverterNone))
	}
	return &AdvancedCustomConfig{Routes: routes}
}

func codingPlanRoute(incomingPath, upstreamPath, converter string) AdvancedCustomRoute {
	return AdvancedCustomRoute{IncomingPath: incomingPath, UpstreamPath: upstreamPath, Converter: converter, Auth: &AdvancedCustomRouteAuth{Type: AdvancedCustomAuthTypeHeader, Name: "Authorization", Value: "Bearer {api_key}"}}
}

func codingPlanJoinPath(baseURL, path string) string {
	return strings.TrimRight(strings.TrimSpace(baseURL), "/") + "/" + strings.TrimLeft(strings.TrimSpace(path), "/")
}

func normalizeCodingPlanPresetValue(value string) string {
	return strings.TrimRight(strings.TrimSpace(value), "/")
}
