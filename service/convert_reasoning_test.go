package service

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/stretchr/testify/require"
)

func TestClaudeToOpenAIRequestPreservesThinkingWithToolUse(t *testing.T) {
	thinking := "checked the available tool before calling it"
	req := dto.ClaudeRequest{
		Model: "claude-sonnet-4-6",
		Messages: []dto.ClaudeMessage{
			{
				Role: "assistant",
				Content: []dto.ClaudeMediaMessage{
					{Type: "thinking", Thinking: &thinking},
					{
						Type:  "tool_use",
						Id:    "toolu_123",
						Name:  "read_file",
						Input: map[string]any{"path": "README.md"},
					},
				},
			},
		},
	}
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ChannelType: constant.ChannelTypeOpenAI}}

	converted, err := ClaudeToOpenAIRequest(req, info)
	require.NoError(t, err)
	require.Len(t, converted.Messages, 1)

	msg := converted.Messages[0]
	require.Equal(t, "assistant", msg.Role)
	require.Equal(t, thinking, msg.GetReasoningContent())
	require.NotNil(t, msg.ToolCalls)
	require.Len(t, msg.ParseToolCalls(), 1)

	encoded, err := common.Marshal(msg)
	require.NoError(t, err)
	require.Contains(t, string(encoded), `"reasoning_content":"`+thinking+`"`)
}

func TestClaudeToOpenAIRequestPreservesRedactedThinkingWithToolUse(t *testing.T) {
	signature := "sig_opaque"
	req := dto.ClaudeRequest{
		Model: "claude-sonnet-4-6",
		Messages: []dto.ClaudeMessage{
			{
				Role: "assistant",
				Content: []dto.ClaudeMediaMessage{
					{Type: "redacted_thinking", Signature: signature},
					{
						Type:  "tool_use",
						Id:    "toolu_456",
						Name:  "search",
						Input: map[string]any{"query": "status"},
					},
				},
			},
		},
	}
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ChannelType: constant.ChannelTypeOpenAI}}

	converted, err := ClaudeToOpenAIRequest(req, info)
	require.NoError(t, err)
	require.Len(t, converted.Messages, 1)
	require.Equal(t, signature, converted.Messages[0].GetReasoningContent())
	require.Len(t, converted.Messages[0].ParseToolCalls(), 1)
}
