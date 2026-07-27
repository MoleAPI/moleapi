package volcengine

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVolcengineGeminiModeUsesChatCompletionsURL(t *testing.T) {
	got, err := (&Adaptor{}).GetRequestURL(&relaycommon.RelayInfo{
		RelayMode: relayconstant.RelayModeGemini,
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelBaseUrl: "https://ark.cn-beijing.volces.com",
		},
	})
	require.NoError(t, err)
	assert.Equal(t, "https://ark.cn-beijing.volces.com/api/v3/chat/completions", got)
}

func TestVolcengineConvertsGeminiRequestToChat(t *testing.T) {
	maxOutputTokens := uint(16)
	converted, err := (&Adaptor{}).ConvertGeminiRequest(nil, &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "doubao-seed-1-6"},
	}, &dto.GeminiChatRequest{
		Contents: []dto.GeminiChatContent{{
			Role:  "user",
			Parts: []dto.GeminiPart{{Text: "hello"}},
		}},
		GenerationConfig: dto.GeminiChatGenerationConfig{
			MaxOutputTokens: &maxOutputTokens,
		},
	})
	require.NoError(t, err)

	chatReq, ok := converted.(*dto.GeneralOpenAIRequest)
	require.True(t, ok)
	require.Len(t, chatReq.Messages, 1)
	assert.Equal(t, "user", chatReq.Messages[0].Role)
	assert.Equal(t, "hello", chatReq.Messages[0].StringContent())
}

func TestVolcengineConvertsChatResponseBackToResponses(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body: io.NopCloser(strings.NewReader(`{
			"id":"chatcmpl_test",
			"object":"chat.completion",
			"created":123,
			"model":"doubao-seed-1-6",
			"choices":[{"index":0,"message":{"role":"assistant","content":"OK"},"finish_reason":"stop"}],
			"usage":{"prompt_tokens":2,"completion_tokens":1,"total_tokens":3}
		}`)),
	}
	info := &relaycommon.RelayInfo{
		RelayFormat:            types.RelayFormatOpenAIResponses,
		RelayMode:              relayconstant.RelayModeResponses,
		RequestConversionChain: []types.RelayFormat{types.RelayFormatOpenAIResponses, types.RelayFormatOpenAI},
		ChannelMeta:            &relaycommon.ChannelMeta{UpstreamModelName: "doubao-seed-1-6"},
	}

	usage, apiErr := (&Adaptor{}).DoResponse(c, resp, info)
	require.Nil(t, apiErr)

	gotUsage, ok := usage.(*dto.Usage)
	require.True(t, ok)
	assert.Equal(t, 3, gotUsage.TotalTokens)

	var responsesResp dto.OpenAIResponsesResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &responsesResp))
	assert.NotEmpty(t, responsesResp.ID)
	assert.JSONEq(t, `"completed"`, string(responsesResp.Status))
	require.Len(t, responsesResp.Output, 1)
	require.Len(t, responsesResp.Output[0].Content, 1)
	assert.Equal(t, "OK", responsesResp.Output[0].Content[0].Text)
}
