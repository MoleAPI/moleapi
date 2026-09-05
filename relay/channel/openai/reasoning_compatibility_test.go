package openai

import (
	"testing"

	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNativeReasoningEffortPassesThroughUnchanged(t *testing.T) {
	for _, effort := range []string{"", "high", "xhigh", "max", "provider_specific"} {
		t.Run(effort, func(t *testing.T) {
			info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{
				ChannelType: constant.ChannelTypeOpenAI, UpstreamModelName: "gpt-6-astra",
			}}
			chat := &dto.GeneralOpenAIRequest{Model: "gpt-6-astra", ReasoningEffort: effort}
			converted, err := (&Adaptor{}).ConvertOpenAIRequest(nil, info, chat)
			require.NoError(t, err)
			assert.Equal(t, effort, converted.(*dto.GeneralOpenAIRequest).ReasoningEffort)
			assert.Equal(t, effort, info.ReasoningEffort)

			responses := dto.OpenAIResponsesRequest{Model: "gpt-6-astra", Reasoning: &dto.Reasoning{Effort: effort}}
			converted, err = (&Adaptor{}).ConvertOpenAIResponsesRequest(nil, info, responses)
			require.NoError(t, err)
			assert.Equal(t, effort, converted.(dto.OpenAIResponsesRequest).Reasoning.Effort)
			assert.Equal(t, effort, info.ReasoningEffort)
		})
	}
}

func TestCanonicalizeChatReasoningJSONPreservesProviderFields(t *testing.T) {
	input := `{"id":"chat_1","vendor":{"kept":true},"choices":[{"message":{"role":"assistant","reasoning_text":"think","reasoning_details":[{"type":"opaque"}],"content":"answer"}}]}`

	output := canonicalizeChatReasoningJSON(input)

	assert.JSONEq(t, `{"id":"chat_1","vendor":{"kept":true},"choices":[{"message":{"role":"assistant","reasoning_content":"think","reasoning_text":"think","reasoning_details":[{"type":"opaque"}],"content":"answer"}}]}`, output)
}

func TestCanonicalizeChatReasoningJSONKeepsExistingCanonicalValue(t *testing.T) {
	input := `{"choices":[{"delta":{"reasoning_content":"canonical","reasoning":"alias"}}]}`

	assert.Equal(t, input, canonicalizeChatReasoningJSON(input))
	assert.Equal(t, "[DONE]", canonicalizeChatReasoningJSON("[DONE]"))
}

func TestCanonicalizeChatReasoningJSONPreservesLargeNumbersAndSkipsNullAlias(t *testing.T) {
	input := `{"vendor_id":9007199254740993,"choices":[{"message":{"reasoning_content":null,"reasoning":null,"reasoning_text":"think"}}]}`

	output := canonicalizeChatReasoningJSON(input)

	assert.Contains(t, output, `"vendor_id":9007199254740993`)
	assert.JSONEq(t, `{"vendor_id":9007199254740993,"choices":[{"message":{"reasoning_content":"think","reasoning":null,"reasoning_text":"think"}}]}`, output)
}
