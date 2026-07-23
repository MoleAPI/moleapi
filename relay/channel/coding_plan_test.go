package channel

import (
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCodingPlanRequestURLUsesFinalRequestFormat(t *testing.T) {
	info := &relaycommon.RelayInfo{
		RelayMode:              relayconstant.RelayModeChatCompletions,
		RelayFormat:            types.RelayFormatClaude,
		RequestConversionChain: []types.RelayFormat{types.RelayFormatClaude, types.RelayFormatOpenAIResponses},
		ChannelMeta:            &relaycommon.ChannelMeta{ChannelBaseUrl: "kimi-coding-plan"},
	}

	got, ok := CodingPlanRequestURL(info)

	require.True(t, ok)
	assert.Equal(t, "https://api.kimi.com/coding/v1/responses", got)
}

func TestCodingPlanRequestURLNativeRoutes(t *testing.T) {
	tests := []struct {
		name string
		info *relaycommon.RelayInfo
		want string
	}{
		{
			name: "claude messages",
			info: &relaycommon.RelayInfo{
				RelayFormat:            types.RelayFormatClaude,
				RequestConversionChain: []types.RelayFormat{types.RelayFormatClaude},
				ChannelMeta:            &relaycommon.ChannelMeta{ChannelBaseUrl: "doubao-coding-plan"},
			},
			want: "https://ark.cn-beijing.volces.com/api/coding/v1/messages",
		},
		{
			name: "chat completions",
			info: &relaycommon.RelayInfo{
				RelayMode:              relayconstant.RelayModeChatCompletions,
				RelayFormat:            types.RelayFormatOpenAI,
				RequestConversionChain: []types.RelayFormat{types.RelayFormatOpenAI},
				ChannelMeta:            &relaycommon.ChannelMeta{ChannelBaseUrl: "doubao-coding-plan"},
			},
			want: "https://ark.cn-beijing.volces.com/api/coding/v3/chat/completions",
		},
		{
			name: "image generations",
			info: &relaycommon.RelayInfo{
				RelayMode:              relayconstant.RelayModeImagesGenerations,
				RelayFormat:            types.RelayFormatOpenAI,
				RequestConversionChain: []types.RelayFormat{types.RelayFormatOpenAI},
				ChannelMeta:            &relaycommon.ChannelMeta{ChannelBaseUrl: "doubao-coding-plan"},
			},
			want: "https://ark.cn-beijing.volces.com/api/coding/v3/images/generations",
		},
		{
			name: "responses image bridge",
			info: &relaycommon.RelayInfo{
				RelayMode:              relayconstant.RelayModeImagesGenerations,
				RelayFormat:            types.RelayFormatOpenAIResponses,
				RequestConversionChain: []types.RelayFormat{types.RelayFormatOpenAIResponses, types.RelayFormatOpenAIImage},
				ChannelMeta:            &relaycommon.ChannelMeta{ChannelBaseUrl: "kimi-coding-plan"},
			},
			want: "https://api.kimi.com/coding/v1/images/generations",
		},
		{
			name: "responses",
			info: &relaycommon.RelayInfo{
				RelayMode:              relayconstant.RelayModeResponses,
				RelayFormat:            types.RelayFormatOpenAIResponses,
				RequestConversionChain: []types.RelayFormat{types.RelayFormatOpenAIResponses},
				ChannelMeta:            &relaycommon.ChannelMeta{ChannelBaseUrl: "kimi-coding-plan"},
			},
			want: "https://api.kimi.com/coding/v1/responses",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := CodingPlanRequestURL(tt.info)
			require.True(t, ok)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestCodingPlanRequestURLFallsBackWhenResponsesPathIsUnknown(t *testing.T) {
	info := &relaycommon.RelayInfo{
		RelayMode:              relayconstant.RelayModeResponses,
		RelayFormat:            types.RelayFormatOpenAIResponses,
		RequestConversionChain: []types.RelayFormat{types.RelayFormatOpenAIResponses},
		ChannelMeta:            &relaycommon.ChannelMeta{ChannelBaseUrl: "glm-coding-plan"},
	}

	got, ok := CodingPlanRequestURL(info)

	require.False(t, ok)
	assert.Empty(t, got)
	assert.False(t, CodingPlanSupportsResponses("glm-coding-plan"))
}
