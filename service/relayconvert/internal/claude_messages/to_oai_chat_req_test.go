package claudemessages

import (
	"testing"

	"github.com/QuantumNous/new-api/dto"
	"github.com/stretchr/testify/require"
)

func TestClaudeMessagesRequestToOpenAIChatPreservesThinkingWithToolCall(t *testing.T) {
	thinking := "checked the available tool before calling it"
	tests := []struct {
		name string
		part dto.ClaudeMediaMessage
		want string
	}{
		{
			name: "thinking",
			part: dto.ClaudeMediaMessage{Type: "thinking", Thinking: &thinking},
			want: thinking,
		},
		{
			name: "redacted thinking",
			part: dto.ClaudeMediaMessage{Type: "redacted_thinking", Signature: "sig_opaque"},
			want: "sig_opaque",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			converted, err := ClaudeMessagesRequestToOpenAIChat(dto.ClaudeRequest{
				Model: "claude-sonnet-4-6",
				Messages: []dto.ClaudeMessage{
					{
						Role: "assistant",
						Content: []dto.ClaudeMediaMessage{
							tt.part,
							{
								Type:  "tool_use",
								Id:    "toolu_123",
								Name:  "read_file",
								Input: map[string]any{"path": "README.md"},
							},
						},
					},
				},
			}, nil)
			require.NoError(t, err)
			require.Len(t, converted.Messages, 1)
			require.Equal(t, tt.want, converted.Messages[0].GetReasoningContent())

			toolCalls := converted.Messages[0].ParseToolCalls()
			require.Len(t, toolCalls, 1)
			require.Equal(t, "toolu_123", toolCalls[0].ID)
			require.Equal(t, "read_file", toolCalls[0].Function.Name)
		})
	}
}
