package openai

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/relaykit/dto"
	kitreasoning "github.com/QuantumNous/new-api/relaykit/relayconvert/reasoning"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChatReasoningOnOffConflictCompatibility(t *testing.T) {
	for _, tc := range []struct {
		name, effort, nested, wantEffort, wantError string
		conflict                                    bool
	}{
		{name: "flat off beats nested on", effort: "none", nested: `{"enabled":true,"exclude":true,"vendor_id":9007199254740993}`, wantEffort: "none", conflict: true},
		{name: "flat on beats nested off", effort: "high", nested: `{"enabled":false}`, wantEffort: "high", conflict: true},
		{name: "flat off beats nested effort", effort: "none", nested: `{"effort":"high"}`, wantEffort: "none", conflict: true},
		{name: "flat on beats zero budget", effort: "low", nested: `{"max_tokens":0}`, wantEffort: "low", conflict: true},
		{name: "flat off beats positive budget", effort: "none", nested: `{"max_tokens":4096}`, wantEffort: "none", conflict: true},
		{name: "consistent controls", effort: "high", nested: `{"enabled":true,"effort":"high"}`, wantEffort: "high"},
		{name: "nested only", nested: `{"enabled":false}`, wantEffort: "none"},
		{name: "invalid budget stays invalid", effort: "none", nested: `{"max_tokens":-2}`, wantError: "budget must be"},
		{name: "invalid nested effort stays invalid", effort: "none", nested: `{"effort":"invalid"}`, wantError: "unsupported reasoning effort"},
		{name: "invalid flat effort stays invalid", effort: "invalid", nested: `{"enabled":false}`, wantError: "unsupported reasoning effort"},
		{name: "malformed controls stay invalid", effort: "none", nested: `{"enabled":"yes"}`, wantError: "invalid reasoning config"},
		{name: "contradictory budget stays invalid", effort: "high", nested: `{"enabled":false,"max_tokens":4096}`, wantError: "non-zero budget enables thinking"},
		{name: "different positive efforts stay invalid", effort: "low", nested: `{"effort":"high"}`, wantError: "explicit efforts"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, channelType := range []int{constant.ChannelTypeOpenAI, constant.ChannelTypeCustom, constant.ChannelTypeOpenRouter} {
				original := &dto.GeneralOpenAIRequest{
					Model: "deepseek-v4.1-flash", ReasoningEffort: tc.effort, Reasoning: []byte(tc.nested),
					Messages: []dto.Message{{Role: "user", Content: "Hello"}},
				}
				if tc.conflict {
					_, err := kitreasoning.FromOpenAIChat(original)
					require.ErrorContains(t, err, "explicit fields disagree about whether thinking is enabled")
				}
				outbound, err := common.DeepCopy(original)
				require.NoError(t, err)
				info := &relaycommon.RelayInfo{
					OriginModelName: original.Model, Request: original,
					ChannelMeta: &relaycommon.ChannelMeta{ChannelType: channelType, UpstreamModelName: "deepseek-v4-1-flash-260910"},
				}
				err = helper.ApplyReasoningModelSuffix(nil, info, outbound)
				if err == nil {
					_, err = (&Adaptor{}).ConvertOpenAIRequest(nil, info, outbound)
				}
				if tc.wantError != "" {
					require.ErrorContains(t, err, tc.wantError)
					assert.True(t, kitreasoning.IsClientError(err))
					continue
				}
				require.NoError(t, err)
				assert.Equal(t, tc.wantEffort, info.ReasoningEffort)
				assert.Equal(t, "deepseek-v4-1-flash-260910", outbound.Model)
				assert.Equal(t, original.Messages, outbound.Messages)
				if !tc.conflict {
					assert.Equal(t, tc.nested, string(original.Reasoning))
				}
				// Check the serialized request too: accepting the request locally
				// must not send the same contradictory controls to the provider.
				wire, err := common.Marshal(outbound)
				require.NoError(t, err)
				var sent dto.GeneralOpenAIRequest
				require.NoError(t, common.Unmarshal(wire, &sent))
				intent, err := kitreasoning.FromOpenAIChat(&sent)
				require.NoError(t, err)
				assert.Equal(t, tc.wantEffort, string(kitreasoning.EffectiveEffort(intent)))
				if tc.name == "flat off beats nested on" {
					assert.JSONEq(t, `{"exclude":true,"vendor_id":9007199254740993}`, string(original.Reasoning))
					if channelType != constant.ChannelTypeOpenAI {
						assert.Contains(t, string(sent.Reasoning), `"exclude":true`)
					}
				}
			}
		})
	}
}

func TestChatReasoningCompatibilityPreservesPassThrough(t *testing.T) {
	request := &dto.GeneralOpenAIRequest{Model: "deepseek-v4.1-flash", ReasoningEffort: "none", Reasoning: []byte(`{"enabled":true}`)}
	info := &relaycommon.RelayInfo{Request: request, ChannelMeta: &relaycommon.ChannelMeta{}}
	info.ChannelSetting.PassThroughBodyEnabled = true
	require.NoError(t, helper.ApplyReasoningModelSuffix(nil, info, request))
	assert.Equal(t, "none", request.ReasoningEffort)
	assert.Equal(t, `{"enabled":true}`, string(request.Reasoning))
}

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
