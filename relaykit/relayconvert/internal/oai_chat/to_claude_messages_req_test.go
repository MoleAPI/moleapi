package oaichat

import (
	"context"
	"testing"

	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/relayconvert/convmeta"
	"github.com/stretchr/testify/require"
)

func TestOpenAIChatRequestToClaudeMessagesPreservesReasoningWithToolCall(t *testing.T) {
	reasoning := "I need the file contents before answering."
	maxTokens := uint(1)
	assistant := dto.Message{
		Role:             "assistant",
		ReasoningContent: &reasoning,
	}
	assistant.SetToolCalls([]dto.ToolCallRequest{
		{
			ID:   "call_read",
			Type: "function",
			Function: dto.FunctionRequest{
				Name:      "read_file",
				Arguments: `{"path":"README.md"}`,
			},
		},
	})

	converted, err := OpenAIChatRequestToClaudeMessages(context.Background(), &convmeta.Values{}, dto.GeneralOpenAIRequest{
		Model:     "claude-sonnet-4-6",
		MaxTokens: &maxTokens,
		Messages:  []dto.Message{assistant},
	})
	require.NoError(t, err)
	require.Len(t, converted.Messages, 2)
	require.Equal(t, "assistant", converted.Messages[1].Role)

	content, err := converted.Messages[1].ParseContent()
	require.NoError(t, err)
	require.Len(t, content, 2)
	require.Equal(t, "thinking", content[0].Type)
	require.NotNil(t, content[0].Thinking)
	require.Equal(t, reasoning, *content[0].Thinking)
	require.Equal(t, "tool_use", content[1].Type)
	require.Equal(t, "call_read", content[1].Id)
	require.Equal(t, "read_file", content[1].Name)
}
