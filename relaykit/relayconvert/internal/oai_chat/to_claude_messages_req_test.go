package oaichat

import (
	"context"
	"encoding/json"
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
		Messages:  []dto.Message{assistant, {Role: "assistant", Content: "Checking the file."}},
	})
	require.NoError(t, err)
	require.Len(t, converted.Messages, 3)
	require.Equal(t, "assistant", converted.Messages[1].Role)
	assert.Equal(t, "Checking the file.", converted.Messages[2].Content)

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
		parameters json.RawMessage
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
			name:       "missing type and properties",
			parameters: json.RawMessage(`{"additionalProperties":false}`),
			wantSchema: map[string]any{
				"type":                 "object",
				"properties":           map[string]any{},
				"additionalProperties": false,
			},
		},
		{
			name:       "non-string type",
			parameters: json.RawMessage(`{"type":123,"properties":{}}`),
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
			wantSchema, err := kitutil.Marshal(tt.wantSchema)
			require.NoError(t, err)
			assert.JSONEq(t, string(wantSchema), string(tool.InputSchema))
		})
	}
}

func TestOpenAIChatRequestToClaudeMessagesMergesAdjacentAssistantToolCalls(t *testing.T) {
	toolCall := func(id string) []dto.ToolCallRequest {
		return []dto.ToolCallRequest{{
			ID:       id,
			Type:     "function",
			Function: dto.FunctionRequest{Name: "lookup_" + id, Arguments: `{"q":"` + id + `"}`},
		}}
	}
	assistant := func(content any, toolCalls []dto.ToolCallRequest) dto.Message {
		message := dto.Message{Role: "assistant", Content: content}
		if toolCalls != nil {
			message.SetToolCalls(toolCalls)
		}
		return message
	}

	tests := []struct {
		name         string
		messages     []dto.Message
		wantTexts    []string
		wantToolUses []string
	}{
		{
			name: "both assistants have tool calls",
			messages: []dto.Message{
				{Role: "user", Content: "Look up a and b."},
				assistant("", toolCall("call_a")),
				assistant("", toolCall("call_b")),
				{Role: "tool", ToolCallId: "call_a", Content: "result a"},
				{Role: "tool", ToolCallId: "call_b", Content: "result b"},
			},
			wantToolUses: []string{"call_a", "call_b"},
		},
		{
			name: "tool calls followed by plain text",
			messages: []dto.Message{
				{Role: "user", Content: "Look up a."},
				assistant(nil, toolCall("call_a")),
				assistant("Checking a.", nil),
				{Role: "tool", ToolCallId: "call_a", Content: "result a"},
			},
			wantTexts:    []string{"Checking a."},
			wantToolUses: []string{"call_a"},
		},
		{
			name: "literal ellipsis with tool calls is preserved",
			messages: []dto.Message{
				{Role: "user", Content: "Look up a and b."},
				assistant("...", toolCall("call_a")),
				assistant("Checking b.", toolCall("call_b")),
				{Role: "tool", ToolCallId: "call_a", Content: "result a"},
				{Role: "tool", ToolCallId: "call_b", Content: "result b"},
			},
			wantTexts:    []string{"... Checking b."},
			wantToolUses: []string{"call_a", "call_b"},
		},
		{
			name: "literal ellipsis replacing a placeholder survives the next merge",
			messages: []dto.Message{
				{Role: "user", Content: "Look up a."},
				assistant(nil, toolCall("call_a")),
				assistant("...", nil),
				assistant("Checking a.", nil),
				{Role: "tool", ToolCallId: "call_a", Content: "result a"},
			},
			wantTexts:    []string{"... Checking a."},
			wantToolUses: []string{"call_a"},
		},
		{
			name: "literal ellipsis before tool calls survives repeated merges",
			messages: []dto.Message{
				{Role: "user", Content: "Look up a."},
				assistant("...", nil),
				assistant("", toolCall("call_a")),
				assistant("Checking a.", nil),
				{Role: "tool", ToolCallId: "call_a", Content: "result a"},
			},
			wantTexts:    []string{"... Checking a."},
			wantToolUses: []string{"call_a"},
		},
		{
			name: "three tool-only assistants need no placeholder",
			messages: []dto.Message{
				{Role: "user", Content: "Look up a, b and c."},
				assistant(nil, toolCall("call_a")),
				assistant("", toolCall("call_b")),
				assistant("", toolCall("call_c")),
				{Role: "tool", ToolCallId: "call_a", Content: "result a"},
				{Role: "tool", ToolCallId: "call_b", Content: "result b"},
				{Role: "tool", ToolCallId: "call_c", Content: "result c"},
			},
			wantToolUses: []string{"call_a", "call_b", "call_c"},
		},
		{
			name: "both assistants are plain text",
			messages: []dto.Message{
				{Role: "user", Content: "Hello."},
				assistant("First.", nil),
				assistant("Second.", nil),
			},
			wantTexts: []string{"First. Second."},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			maxTokens := uint(1024)
			got, err := OpenAIChatRequestToClaudeMessages(context.Background(), nil, dto.GeneralOpenAIRequest{
				Model:     "claude-test",
				MaxTokens: &maxTokens,
				Messages:  tt.messages,
			})
			require.NoError(t, err)

			var wantToolResults []dto.ClaudeMediaMessage
			for _, message := range tt.messages {
				if message.Role == "tool" {
					wantToolResults = append(wantToolResults, dto.ClaudeMediaMessage{
						Type:      "tool_result",
						ToolUseId: message.ToolCallId,
						Content:   message.Content,
					})
				}
			}
			if len(wantToolResults) > 0 {
				require.Len(t, got.Messages, 3)
				assert.Equal(t, "user", got.Messages[2].Role)
				assert.Equal(t, wantToolResults, got.Messages[2].Content)
			} else {
				require.Len(t, got.Messages, 2)
			}
			assert.Equal(t, "assistant", got.Messages[1].Role)

			var texts, toolUses []string
			switch content := got.Messages[1].Content.(type) {
			case string:
				texts = append(texts, content)
			case []dto.ClaudeMediaMessage:
				for _, block := range content {
					switch block.Type {
					case "text":
						texts = append(texts, block.GetText())
					case "tool_use":
						toolUses = append(toolUses, block.Id)
						assert.Equal(t, "lookup_"+block.Id, block.Name)
						assert.Equal(t, map[string]any{"q": block.Id}, block.Input)
					}
				}
			default:
				require.Failf(t, "unexpected assistant content type", "%T", content)
			}
			assert.Equal(t, tt.wantTexts, texts)
			assert.Equal(t, tt.wantToolUses, toolUses)
		})
	}
}

func TestOpenAIChatRequestToClaudeMessagesStopSequences(t *testing.T) {
	tests := []struct {
		name string
		stop any
		want []string
	}{
		{name: "string", stop: "END", want: []string{"END"}},
		{name: "empty string", stop: "", want: []string{""}},
		{name: "string slice", stop: []string{"END", ""}, want: []string{"END", ""}},
		{name: "mixed slice", stop: []any{"END", 1, nil, "", "STOP"}, want: []string{"END", "", "STOP"}},
		{name: "non-string slice", stop: []any{1, nil}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			maxTokens := uint(16)
			got, err := OpenAIChatRequestToClaudeMessages(context.Background(), nil, dto.GeneralOpenAIRequest{
				Model:     "claude-test",
				MaxTokens: &maxTokens,
				Messages:  []dto.Message{{Role: "user", Content: "hello"}},
				Stop:      tt.stop,
			})
			require.NoError(t, err)
			require.NotNil(t, got)
			if len(tt.want) == 0 {
				assert.Empty(t, got.StopSequences)
				return
			}
			assert.Equal(t, tt.want, got.StopSequences)
		})
	}
}
