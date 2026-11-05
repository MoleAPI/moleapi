package xai

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStreamUsageRetainsCacheAndReasoningForSettlement(t *testing.T) {
	previousTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 300
	t.Cleanup(func() { constant.StreamingTimeout = previousTimeout })
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "grok-4"}}
	body := `{"id":"chat-test","object":"chat.completion.chunk","choices":[],"usage":{"prompt_tokens":100,"completion_tokens":20,"total_tokens":120,"prompt_tokens_details":{"cached_tokens":60},"completion_tokens_details":{"reasoning_tokens":12}}}`
	resp := &http.Response{Body: io.NopCloser(strings.NewReader("data: " + body + "\n\ndata: [DONE]\n\n"))}
	usage, apiErr := xAIStreamHandler(c, info, resp)
	require.Nil(t, apiErr)
	require.NotNil(t, usage)
	assert.Equal(t, 100, usage.PromptTokens)
	assert.Equal(t, 20, usage.CompletionTokens)
	assert.Equal(t, 60, usage.PromptTokensDetails.CachedTokens)
	assert.Equal(t, 12, usage.CompletionTokenDetails.ReasoningTokens)
	for line := range strings.SplitSeq(recorder.Body.String(), "\n") {
		payload, ok := strings.CutPrefix(line, "data: ")
		if !ok || payload == "[DONE]" {
			continue
		}
		var chunk dto.ChatCompletionsStreamResponse
		require.NoError(t, common.UnmarshalJsonStr(payload, &chunk))
		require.NotNil(t, chunk.Usage)
		assert.Equal(t, usage.PromptTokensDetails, chunk.Usage.PromptTokensDetails)
		assert.Equal(t, usage.CompletionTokenDetails, chunk.Usage.CompletionTokenDetails)
	}
	assert.Contains(t, recorder.Body.String(), "[DONE]")
}
