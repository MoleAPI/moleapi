package zhipu_4v

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

func TestZhipuV4CodingPlanRoutesUseOfficialPaths(t *testing.T) {
	adaptor := &Adaptor{}

	chatURL, err := adaptor.GetRequestURL(&relaycommon.RelayInfo{
		RelayFormat: types.RelayFormatOpenAI,
		RelayMode:   relayconstant.RelayModeChatCompletions,
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelBaseUrl:    "glm-coding-plan",
			UpstreamModelName: "glm-5-turbo",
		},
	})
	require.NoError(t, err)
	assert.Equal(t, "https://open.bigmodel.cn/api/coding/paas/v4/chat/completions", chatURL)

	claudeURL, err := adaptor.GetRequestURL(&relaycommon.RelayInfo{
		RelayFormat: types.RelayFormatClaude,
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelBaseUrl:    "glm-coding-plan",
			UpstreamModelName: "glm-5-turbo",
		},
	})
	require.NoError(t, err)
	assert.Equal(t, "https://open.bigmodel.cn/api/anthropic/v1/messages", claudeURL)

	responsesViaChatURL, err := adaptor.GetRequestURL(&relaycommon.RelayInfo{
		RelayFormat:            types.RelayFormatOpenAIResponses,
		RelayMode:              relayconstant.RelayModeResponses,
		RequestConversionChain: []types.RelayFormat{types.RelayFormatOpenAIResponses, types.RelayFormatOpenAI},
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelBaseUrl:    "glm-coding-plan",
			UpstreamModelName: "glm-5-turbo",
		},
	})
	require.NoError(t, err)
	assert.Equal(t, "https://open.bigmodel.cn/api/coding/paas/v4/chat/completions", responsesViaChatURL)
}

func TestZhipuV4ConvertsResponsesRequestToChat(t *testing.T) {
	maxOutputTokens := uint(16)
	adaptor := &Adaptor{}

	converted, err := adaptor.ConvertOpenAIResponsesRequest(nil, &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{ChannelBaseUrl: "glm-coding-plan"},
	}, dto.OpenAIResponsesRequest{
		Model:           "glm-5-turbo",
		Input:           mustZhipuV4RawMessage(t, "hello"),
		MaxOutputTokens: &maxOutputTokens,
	})
	require.NoError(t, err)

	chatReq, ok := converted.(*dto.GeneralOpenAIRequest)
	require.True(t, ok)
	assert.Equal(t, "glm-5-turbo", chatReq.Model)
	require.Len(t, chatReq.Messages, 1)
	assert.Equal(t, "user", chatReq.Messages[0].Role)
	assert.Equal(t, "hello", chatReq.Messages[0].StringContent())
	require.NotNil(t, chatReq.MaxTokens)
	assert.Equal(t, maxOutputTokens, *chatReq.MaxTokens)
}

func TestZhipuV4PassesNativeResponsesRequestThrough(t *testing.T) {
	request := dto.OpenAIResponsesRequest{Model: "glm-4.5"}

	converted, err := (&Adaptor{}).ConvertOpenAIResponsesRequest(nil, nil, request)
	require.NoError(t, err)
	assert.Equal(t, request, converted)
}

func TestZhipuV4ConvertsGeminiRequestToChat(t *testing.T) {
	maxOutputTokens := uint(16)
	adaptor := &Adaptor{}

	converted, err := adaptor.ConvertGeminiRequest(nil, &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "glm-5-turbo"},
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

func TestZhipuV4ConvertsChatResponseBackToResponses(t *testing.T) {
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
			"model":"glm-5-turbo",
			"choices":[{"index":0,"message":{"role":"assistant","content":"OK"},"finish_reason":"stop"}],
			"usage":{"prompt_tokens":2,"completion_tokens":1,"total_tokens":3}
		}`)),
	}
	info := &relaycommon.RelayInfo{
		RelayFormat:            types.RelayFormatOpenAIResponses,
		RelayMode:              relayconstant.RelayModeResponses,
		RequestConversionChain: []types.RelayFormat{types.RelayFormatOpenAIResponses, types.RelayFormatOpenAI},
		ChannelMeta:            &relaycommon.ChannelMeta{UpstreamModelName: "glm-5-turbo"},
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

func mustZhipuV4RawMessage(t *testing.T, v any) []byte {
	t.Helper()
	raw, err := common.Marshal(v)
	require.NoError(t, err)
	return raw
}
