package dto

import (
	"encoding/json"
	"testing"

	kitutil "github.com/QuantumNous/new-api/relaykit/relayconvert/kitutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMessageReasoningAliasesNormalizeForUpstream(t *testing.T) {
	for _, tt := range []struct {
		name   string
		input  string
		field  string
		output string
	}{
		{name: "reasoning content", input: `{"reasoning_content":"think"}`, field: ReasoningFieldContent, output: `{"reasoning_content":"think"}`},
		{name: "vllm reasoning", input: `{"reasoning":"think"}`, field: ReasoningFieldReasoning, output: `{"reasoning":"think"}`},
		{name: "opencode reasoning text", input: `{"reasoning_text":"think"}`, field: ReasoningFieldContent, output: `{"reasoning_content":"think"}`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var input map[string]json.RawMessage
			require.NoError(t, kitutil.Unmarshal([]byte(tt.input), &input))
			message := Message{Content: ""}
			if value := input[ReasoningFieldContent]; value != nil {
				require.NoError(t, kitutil.Unmarshal(value, &message.ReasoningContent))
			}
			if value := input[ReasoningFieldReasoning]; value != nil {
				require.NoError(t, kitutil.Unmarshal(value, &message.Reasoning))
			}
			if value := input[ReasoningFieldText]; value != nil {
				require.NoError(t, kitutil.Unmarshal(value, &message.ReasoningText))
			}

			message.NormalizeReasoningContentField(tt.field)
			actual := map[string]any{}
			if message.ReasoningContent != nil {
				actual[ReasoningFieldContent] = *message.ReasoningContent
			}
			if message.Reasoning != nil {
				actual[ReasoningFieldReasoning] = *message.Reasoning
			}
			if message.ReasoningText != nil {
				actual[ReasoningFieldText] = *message.ReasoningText
			}
			expected := map[string]any{}
			require.NoError(t, kitutil.Unmarshal([]byte(tt.output), &expected))
			assert.Equal(t, expected, actual)
		})
	}
}

func TestMessageReasoningNormalizationPreservesPriorityAndDetails(t *testing.T) {
	content, reasoning, reasoningText := "canonical", "alias", "text alias"
	details := json.RawMessage(`[{"type":"reasoning.text","text":"opaque"}]`)
	message := Message{
		ReasoningContent: &content,
		Reasoning:        &reasoning,
		ReasoningText:    &reasoningText,
		ReasoningDetails: details,
	}

	message.NormalizeReasoningContentField(ReasoningFieldReasoning)

	require.NotNil(t, message.Reasoning)
	assert.Equal(t, content, *message.Reasoning)
	assert.Nil(t, message.ReasoningContent)
	assert.Nil(t, message.ReasoningText)
	assert.JSONEq(t, string(details), string(message.ReasoningDetails))
}

func TestMessageReasoningNormalizationUsesNonEmptyAlias(t *testing.T) {
	empty, reasoning := "", "think"
	message := Message{ReasoningContent: &empty, Reasoning: &reasoning}

	message.NormalizeReasoningContentField(ReasoningFieldContent)

	require.NotNil(t, message.ReasoningContent)
	assert.Equal(t, reasoning, *message.ReasoningContent)
	assert.Nil(t, message.Reasoning)
}

func TestGeneralOpenAIRequestPreservesThinkingHistoryFlags(t *testing.T) {
	clearThinking, preserveThinking, toolStream := false, true, true
	encoded, err := kitutil.Marshal(GeneralOpenAIRequest{
		Model:            "qwen3.7-plus",
		ClearThinking:    &clearThinking,
		PreserveThinking: &preserveThinking,
		ToolStream:       &toolStream,
	})
	require.NoError(t, err)
	assert.JSONEq(t, `{"model":"qwen3.7-plus","clear_thinking":false,"preserve_thinking":true,"tool_stream":true}`, string(encoded))
}
