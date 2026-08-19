package oaichat

import (
	"context"
	"testing"

	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/relayconvert/convmeta"
	kitutil "github.com/QuantumNous/new-api/relaykit/relayconvert/kitutil"
	"github.com/stretchr/testify/assert"
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

func TestOpenAIChatRequestToClaudeMessagesRestoresSignedThinkingBlocks(t *testing.T) {
	maxTokens := uint(1)
	thinking := "private thought"
	details, err := kitutil.Marshal([]dto.ClaudeMediaMessage{
		{Type: "thinking", Thinking: &thinking, Signature: "sig_thinking"},
		{Type: "redacted_thinking", Data: "opaque_data"},
	})
	require.NoError(t, err)
	assistant := dto.Message{Role: "assistant", ReasoningDetails: details}
	assistant.SetToolCalls([]dto.ToolCallRequest{{
		ID:       "call_read",
		Type:     "function",
		Function: dto.FunctionRequest{Name: "read_file", Arguments: `{}`},
	}})

	converted, err := OpenAIChatRequestToClaudeMessages(context.Background(), &convmeta.Values{}, dto.GeneralOpenAIRequest{
		Model: "claude-sonnet-4-6", MaxTokens: &maxTokens, Messages: []dto.Message{assistant},
	})
	require.NoError(t, err)
	require.Len(t, converted.Messages, 2)
	content, err := converted.Messages[1].ParseContent()
	require.NoError(t, err)
	require.Len(t, content, 3)
	assert.Equal(t, "thinking", content[0].Type)
	assert.Equal(t, "sig_thinking", content[0].Signature)
	assert.Equal(t, "redacted_thinking", content[1].Type)
	assert.Equal(t, "opaque_data", content[1].Data)
	assert.Equal(t, "tool_use", content[2].Type)
}

func TestOpenAIChatRequestToClaudeMessagesNormalizesToolInputSchema(t *testing.T) {
	tests := []struct {
		name       string
		parameters any
		wantSchema map[string]any
	}{
		{
			name:       "omitted parameters",
			parameters: nil,
			wantSchema: map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
		},
		{
			name: "missing type and properties",
			parameters: map[string]any{
				"additionalProperties": false,
			},
			wantSchema: map[string]any{
				"type":                 "object",
				"properties":           map[string]any{},
				"additionalProperties": false,
			},
		},
		{
			name: "non-string type",
			parameters: map[string]any{
				"type":       123,
				"properties": map[string]any{},
			},
			wantSchema: map[string]any{
				"type":       123,
				"properties": map[string]any{},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			maxTokens := uint(1024)
			got, err := OpenAIChatRequestToClaudeMessages(context.Background(), nil, dto.GeneralOpenAIRequest{
				Model:     "claude-test",
				MaxTokens: &maxTokens,
				Messages:  []dto.Message{{Role: "user", Content: "Call the tool."}},
				Tools: []dto.ToolCallRequest{{
					Type: "function",
					Function: dto.FunctionRequest{
						Name:        "get_current_time",
						Description: "Get the current time",
						Parameters:  tt.parameters,
					},
				}},
			})

			require.NoError(t, err)
			tools, ok := got.Tools.([]any)
			require.True(t, ok)
			require.Len(t, tools, 1)
			tool, ok := tools[0].(*dto.Tool)
			require.True(t, ok)
			assert.Equal(t, "get_current_time", tool.Name)
			assert.Equal(t, tt.wantSchema, tool.InputSchema)
		})
	}
}
