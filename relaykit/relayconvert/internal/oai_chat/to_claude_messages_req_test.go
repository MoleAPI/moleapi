package oaichat

import (
	"context"
	"testing"

	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/relayconvert/convmeta"
	relaymedia "github.com/QuantumNous/new-api/relaykit/relayconvert/internal/media"
	kitutil "github.com/QuantumNous/new-api/relaykit/relayconvert/kitutil"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestOpenAIChatRequestToClaudeMessagesPreservesCacheControl(t *testing.T) {
	relaymedia.SetMediaResolver(relaymedia.MediaResolver{
		GetBase64Data: func(context.Context, types.FileSource, ...string) (string, string, error) {
			return "aW1hZ2U=", "image/png", nil
		},
	})
	t.Cleanup(func() { relaymedia.SetMediaResolver(relaymedia.MediaResolver{}) })
	for _, tt := range []struct {
		name string
		body string
		path string
		want string
	}{
		{"automatic 5m", `"cache_control":{"type":"ephemeral"},"messages":[{"role":"user","content":"hello"}]`, "cache_control", `{"type":"ephemeral"}`},
		{"automatic 1h", `"cache_control":{"type":"ephemeral","ttl":"1h"},"messages":[{"role":"user","content":"hello"}]`, "cache_control", `{"type":"ephemeral","ttl":"1h"}`},
		{"system", `"messages":[{"role":"system","content":[{"type":"text","text":"instructions","cache_control":{"type":"ephemeral"}}]},{"role":"user","content":"hello"}]`, "system.0.cache_control", `{"type":"ephemeral"}`},
		{"user text", `"messages":[{"role":"user","content":[{"type":"text","text":"hello","cache_control":{"type":"ephemeral","ttl":"1h"}}]}]`, "messages.0.content.0.cache_control", `{"type":"ephemeral","ttl":"1h"}`},
		{"assistant text", `"messages":[{"role":"user","content":"hello"},{"role":"assistant","content":[{"type":"text","text":"answer","cache_control":{"type":"ephemeral"}}]}]`, "messages.1.content.0.cache_control", `{"type":"ephemeral"}`},
		{"image", `"messages":[{"role":"user","content":[{"type":"image_url","image_url":{"url":"data:image/png;base64,aW1hZ2U="},"cache_control":{"type":"ephemeral"}}]}]`, "messages.0.content.0.cache_control", `{"type":"ephemeral"}`},
		{"tool", `"tools":[{"type":"function","function":{"name":"read_file","parameters":{"type":"object","properties":{}}},"cache_control":{"type":"ephemeral","ttl":"1h"}}],"messages":[{"role":"user","content":"hello"}]`, "tools.0.cache_control", `{"type":"ephemeral","ttl":"1h"}`},
		{"not enabled when omitted", `"messages":[{"role":"user","content":"hello"}]`, "", ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var request dto.GeneralOpenAIRequest
			require.NoError(t, kitutil.Unmarshal([]byte(`{"model":"claude-test","max_tokens":16,`+tt.body+`}`), &request))
			converted, err := OpenAIChatRequestToClaudeMessages(context.Background(), &convmeta.Values{}, request)
			require.NoError(t, err)
			encoded, err := kitutil.Marshal(converted)
			require.NoError(t, err)
			if tt.path == "" {
				assert.NotContains(t, string(encoded), `"cache_control"`)
				return
			}
			actual := gjson.GetBytes(encoded, tt.path)
			require.True(t, actual.Exists(), "missing cache policy in %s", encoded)
			assert.JSONEq(t, tt.want, actual.Raw)
		})
	}
}

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
