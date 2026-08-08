package openai

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

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
