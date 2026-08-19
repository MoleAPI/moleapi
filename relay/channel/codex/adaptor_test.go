package codex

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCodexConvertsChatToResponsesRequest(t *testing.T) {
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelSetting: dto.ChannelSettings{SystemPrompt: "system rules"},
		},
		RequestConversionChain: []types.RelayFormat{types.RelayFormatOpenAI},
	}

	converted, err := (&Adaptor{}).ConvertOpenAIRequest(nil, info, &dto.GeneralOpenAIRequest{
		Model:    "gpt-5-codex",
		Messages: []dto.Message{{Role: "user", Content: "hello"}},
	})

	require.NoError(t, err)
	req, ok := converted.(dto.OpenAIResponsesRequest)
	require.True(t, ok)
	assert.Equal(t, "gpt-5-codex", req.Model)
	assert.JSONEq(t, `"system rules"`, string(req.Instructions))
	assert.JSONEq(t, `false`, string(req.Store))
}

func TestCodexUsesResponsesURLAfterRequestConversion(t *testing.T) {
	info := &relaycommon.RelayInfo{
		RelayMode:              relayconstant.RelayModeChatCompletions,
		RequestConversionChain: []types.RelayFormat{types.RelayFormatOpenAI, types.RelayFormatOpenAIResponses},
		ChannelMeta:            &relaycommon.ChannelMeta{ChannelBaseUrl: "https://chatgpt.com"},
	}

	got, err := (&Adaptor{}).GetRequestURL(info)

	require.NoError(t, err)
	assert.Equal(t, "https://chatgpt.com/backend-api/codex/responses", got)
}

func TestCodexConvertsResponsesBackToClaude(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body: io.NopCloser(strings.NewReader(`{
			"id":"resp_1",
			"object":"response",
			"created_at":123,
			"status":"completed",
			"model":"gpt-5-codex",
			"output":[{"type":"message","id":"msg_1","status":"completed","role":"assistant","content":[{"type":"output_text","text":"OK","annotations":[]}]}],
			"usage":{"input_tokens":2,"output_tokens":1,"total_tokens":3}
		}`)),
	}
	info := &relaycommon.RelayInfo{
		RelayFormat:            types.RelayFormatClaude,
		RelayMode:              relayconstant.RelayModeChatCompletions,
		RequestConversionChain: []types.RelayFormat{types.RelayFormatClaude, types.RelayFormatOpenAIResponses},
		ChannelMeta:            &relaycommon.ChannelMeta{UpstreamModelName: "gpt-5-codex"},
	}

	usage, apiErr := (&Adaptor{}).DoResponse(c, resp, info)

	require.Nil(t, apiErr)
	gotUsage, ok := usage.(*dto.Usage)
	require.True(t, ok)
	assert.Equal(t, 3, gotUsage.TotalTokens)

	var claudeResp dto.ClaudeResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &claudeResp))
	require.Len(t, claudeResp.Content, 1)
	assert.Equal(t, "OK", claudeResp.Content[0].GetText())
}

func TestGetRequestURLAlphaSearch(t *testing.T) {
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType:    constant.ChannelTypeCodex,
			ChannelBaseUrl: "https://chatgpt.com",
		},
		RelayMode: relayconstant.RelayModeAlphaSearch,
	}

	url, err := (&Adaptor{}).GetRequestURL(info)
	require.NoError(t, err)
	assert.Equal(t, "https://chatgpt.com/backend-api/codex/alpha/search", url)
}

// The Codex backend rejects these fields, so the adaptor clears them rather
// than forwarding what the client sent.
func TestConvertOpenAIResponsesRequestDropsPenalties(t *testing.T) {
	adaptor := &Adaptor{}
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{ChannelType: constant.ChannelTypeCodex},
		RelayMode:   relayconstant.RelayModeResponses,
	}

	converted, err := adaptor.ConvertOpenAIResponsesRequest(nil, info, dto.OpenAIResponsesRequest{
		Model:            "gpt-5-codex",
		Input:            json.RawMessage(`"hello"`),
		MaxOutputTokens:  lo.ToPtr(uint(128)),
		Temperature:      lo.ToPtr(1.0),
		FrequencyPenalty: json.RawMessage(`1.5`),
		PresencePenalty:  json.RawMessage(`1.5`),
	})
	require.NoError(t, err)

	request, ok := converted.(dto.OpenAIResponsesRequest)
	require.True(t, ok)
	assert.Nil(t, request.MaxOutputTokens)
	assert.Nil(t, request.Temperature)
	assert.Nil(t, request.FrequencyPenalty)
	assert.Nil(t, request.PresencePenalty)
}
