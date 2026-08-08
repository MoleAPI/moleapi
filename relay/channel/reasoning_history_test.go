package channel_test

import (
	"encoding/json"
	"testing"

	"github.com/QuantumNous/new-api/relay/channel/ali"
	"github.com/QuantumNous/new-api/relay/channel/minimax"
	"github.com/QuantumNous/new-api/relay/channel/moonshot"
	"github.com/QuantumNous/new-api/relay/channel/zhipu_4v"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDomesticChatAdaptorsPreserveReasoningHistory(t *testing.T) {
	for _, tt := range []struct {
		name    string
		model   string
		convert func(*dto.GeneralOpenAIRequest) (any, error)
	}{
		{name: "GLM", model: "glm-5", convert: func(request *dto.GeneralOpenAIRequest) (any, error) {
			return (&zhipu_4v.Adaptor{}).ConvertOpenAIRequest(nil, &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "glm-5"}}, request)
		}},
		{name: "MiniMax", model: "MiniMax-M2.7", convert: func(request *dto.GeneralOpenAIRequest) (any, error) {
			return (&minimax.Adaptor{}).ConvertOpenAIRequest(nil, &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "MiniMax-M2.7"}}, request)
		}},
		{name: "Kimi", model: "kimi-k2.6", convert: func(request *dto.GeneralOpenAIRequest) (any, error) {
			return (&moonshot.Adaptor{}).ConvertOpenAIRequest(nil, &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "kimi-k2.6"}}, request)
		}},
		{name: "Qwen", model: "qwen3.7-plus", convert: func(request *dto.GeneralOpenAIRequest) (any, error) {
			return (&ali.Adaptor{}).ConvertOpenAIRequest(nil, &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "qwen3.7-plus"}}, request)
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			reasoningText := "think"
			clearThinking, preserveThinking, toolStream := false, true, true
			converted, err := tt.convert(&dto.GeneralOpenAIRequest{
				Model:           tt.model,
				ReasoningEffort: "high",
				Messages: []dto.Message{{
					Role:             "assistant",
					Content:          "",
					ReasoningText:    &reasoningText,
					ReasoningDetails: json.RawMessage(`[{"type":"reasoning.text","text":"think"}]`),
				}},
				ClearThinking:    &clearThinking,
				PreserveThinking: &preserveThinking,
				ToolStream:       &toolStream,
			})
			require.NoError(t, err)
			request, ok := converted.(*dto.GeneralOpenAIRequest)
			require.True(t, ok)
			require.Len(t, request.Messages, 1)
			require.NotNil(t, request.Messages[0].ReasoningText)
			assert.Equal(t, reasoningText, *request.Messages[0].ReasoningText)
			assert.JSONEq(t, `[{"type":"reasoning.text","text":"think"}]`, string(request.Messages[0].ReasoningDetails))
			require.NotNil(t, request.ClearThinking)
			assert.False(t, *request.ClearThinking)
			require.NotNil(t, request.PreserveThinking)
			assert.True(t, *request.PreserveThinking)
			assert.Equal(t, "high", request.ReasoningEffort)
			require.NotNil(t, request.ToolStream)
			assert.True(t, *request.ToolStream)
		})
	}
}
