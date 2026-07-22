package channel

import (
	"strings"

	channelconstant "github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/types"
)

func IsCodingPlanBase(baseURL string) bool {
	_, ok := channelconstant.ChannelSpecialBases[strings.TrimSpace(baseURL)]
	return ok
}

func CodingPlanSupportsResponses(baseURL string) bool {
	plan, ok := channelconstant.ChannelSpecialBases[strings.TrimSpace(baseURL)]
	return ok && strings.TrimSpace(plan.ResponsesBaseURL) != ""
}

func CodingPlanRequestURL(info *relaycommon.RelayInfo) (string, bool) {
	if info == nil || info.ChannelMeta == nil {
		return "", false
	}
	plan, ok := channelconstant.ChannelSpecialBases[strings.TrimSpace(info.ChannelBaseUrl)]
	if !ok {
		return "", false
	}

	if info.RelayMode == relayconstant.RelayModeResponsesCompact {
		return joinCodingPlanURL(plan.ResponsesBaseURL, "/responses/compact")
	}

	switch info.GetFinalRequestRelayFormat() {
	case types.RelayFormatClaude:
		return joinCodingPlanURL(plan.ClaudeBaseURL, "/v1/messages")
	case types.RelayFormatOpenAIResponses:
		return joinCodingPlanURL(plan.ResponsesBaseURL, "/responses")
	case types.RelayFormatOpenAI:
		if info.RelayMode == relayconstant.RelayModeCompletions {
			return joinCodingPlanURL(plan.OpenAIBaseURL, "/completions")
		}
		return joinCodingPlanURL(plan.OpenAIBaseURL, "/chat/completions")
	default:
		return "", false
	}
}

func joinCodingPlanURL(baseURL string, path string) (string, bool) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		return "", false
	}
	return baseURL + path, true
}
