package relay

import (
	"context"
	"encoding/json"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	openaichannel "github.com/QuantumNous/new-api/relay/channel/openai"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/relayconvert"
	relaytypes "github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/model_setting"
	hosttypes "github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestChatCompatibilityRetryPreservesOutputAndUsage(t *testing.T) {
	service.InitHttpClient()
	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = oldTimeout })
	for _, tc := range []struct {
		name                        string
		off, stream, upstreamStream bool
	}{
		{name: "off JSON", off: true},
		{name: "off SSE", off: true, stream: true, upstreamStream: true},
		{name: "tools JSON"},
		{name: "tools SSE", stream: true, upstreamStream: true},
		{name: "tools buffered SSE", upstreamStream: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			message := `Function tools with reasoning_effort are not supported for gpt-5.6-sol in /v1/chat/completions. To use function tools, use /v1/responses or set reasoning_effort to 'none'.`
			effort, model := "medium", "gpt-5.6-sol"
			if tc.off {
				effort, model = "none", "deepseek-v4.1-flash"
				message = `Invalid option: expected one of "off"|"low"|"medium"|"high"|"xhigh"|"max"`
			}
			response := `{"id":"resp_1","object":"response","model":"gpt-5.6-sol","status":"completed","output":[{"type":"function_call","id":"fc_1","call_id":"call_1","name":"lookup","arguments":"{\"id\":9007199254740993}"}],"usage":{"input_tokens":2,"output_tokens":3,"total_tokens":5}}`
			received := make(chan []byte, 1)
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				data, err := io.ReadAll(r.Body)
				if err != nil {
					http.Error(w, "read failed", 500)
					return
				}
				received <- data
				if tc.upstreamStream {
					w.Header().Set("Content-Type", "text/event-stream")
					if tc.off {
						_, _ = io.WriteString(w, "data: {\"id\":\"chat_1\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"OK\"},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":2,\"completion_tokens\":3,\"total_tokens\":5}}\n\ndata: [DONE]\n\n")
					} else {
						_, _ = io.WriteString(w, "data: {\"type\":\"response.created\",\"response\":{\"id\":\"resp_1\",\"model\":\"gpt-5.6-sol\"}}\n\ndata: {\"type\":\"response.output_item.added\",\"output_index\":0,\"item\":{\"type\":\"function_call\",\"id\":\"fc_1\",\"call_id\":\"call_1\",\"name\":\"lookup\"}}\n\ndata: {\"type\":\"response.function_call_arguments.delta\",\"output_index\":0,\"delta\":\"{\\\"id\\\":9007199254740993}\"}\n\ndata: {\"type\":\"response.completed\",\"response\":"+response+"}\n\ndata: [DONE]\n\n")
					}
					return
				}
				w.Header().Set("Content-Type", "application/json")
				if tc.off {
					_, _ = io.WriteString(w, `{"id":"chat_1","object":"chat.completion","choices":[{"index":0,"message":{"role":"assistant","content":"OK"},"finish_reason":"stop"}],"usage":{"prompt_tokens":2,"completion_tokens":3,"total_tokens":5}}`)
				} else {
					_, _ = io.WriteString(w, response)
				}
			}))
			t.Cleanup(upstream.Close)
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
			c.Request.Header.Set("Content-Type", "application/json")
			info := &relaycommon.RelayInfo{RelayMode: relayconstant.RelayModeChatCompletions, RelayFormat: relaytypes.RelayFormatOpenAI, OriginModelName: model, RequestURLPath: c.Request.URL.Path, IsStream: tc.stream, ShouldIncludeUsage: true, DisablePing: true,
				ChannelMeta: &relaycommon.ChannelMeta{ChannelType: constant.ChannelTypeOpenAI, ChannelBaseUrl: upstream.URL, UpstreamModelName: model}}
			info.SetReasoningEffort(effort)
			// This override was already applied to the two-message replay.
			info.ParamOverride = map[string]any{"operations": []any{map[string]any{"mode": "append", "path": "messages", "value": map[string]any{"role": "user", "content": "override-once"}}}}
			wire, err := common.Marshal(map[string]any{"model": model, "reasoning_effort": effort, "stream": tc.stream, "max_completion_tokens": 0, "provider_option": common.RawMessage(`9007199254740993`), "messages": []any{map[string]any{"role": "user", "content": "hello"}, map[string]any{"role": "user", "content": "override-once"}}, "tools": []any{map[string]any{"type": "function", "function": map[string]any{"name": "lookup", "parameters": map[string]any{"type": "object", "properties": map[string]any{"id": map[string]any{"type": "integer"}}}}}}})
			require.NoError(t, err)
			body, closer, err := relaycommon.NewOutboundJSONBody(wire)
			require.NoError(t, err)
			t.Cleanup(func() { _ = closer.Close() })
			adaptor := &openaichannel.Adaptor{}
			adaptor.Init(info)
			value, apiErr, handled := retryChatCompatibility(c, info, adaptor, body, relaytypes.WithOpenAIError(relaytypes.OpenAIError{Message: message, Type: "invalid_request_error"}, 400))
			require.True(t, handled)
			require.Nil(t, apiErr)
			require.IsType(t, &dto.Usage{}, value)
			usage := value.(*dto.Usage)
			assert.Equal(t, 2, usage.PromptTokens)
			assert.Equal(t, 3, usage.CompletionTokens)
			assert.Equal(t, 5, usage.TotalTokens)
			assert.Equal(t, effort, info.ReasoningEffort, "wire spelling does not change the client's semantic effort")
			require.Len(t, received, 1)
			sent := <-received
			if tc.off {
				assert.Equal(t, "off", gjson.GetBytes(sent, "reasoning_effort").String())
				assert.Equal(t, "9007199254740993", gjson.GetBytes(sent, "provider_option").Raw)
				assert.Equal(t, 2, len(gjson.GetBytes(sent, "messages").Array()))
				assert.Equal(t, "0", gjson.GetBytes(sent, "max_completion_tokens").Raw)
				assert.Contains(t, recorder.Body.String(), `"content":"OK"`)
			} else {
				assert.Equal(t, "medium", gjson.GetBytes(sent, "reasoning.effort").String())
				assert.Equal(t, "lookup", gjson.GetBytes(sent, "tools.0.name").String())
				assert.Equal(t, 2, len(gjson.GetBytes(sent, "input").Array()))
				assert.Equal(t, "0", gjson.GetBytes(sent, "max_output_tokens").Raw)
				assert.Contains(t, recorder.Body.String(), `"name":"lookup"`)
				assert.Contains(t, recorder.Body.String(), "9007199254740993")
			}
			if tc.stream {
				assert.Contains(t, recorder.Body.String(), "data: [DONE]")
			}
			assert.Equal(t, relayconstant.RelayModeChatCompletions, info.RelayMode)
			assert.Equal(t, "/v1/chat/completions", info.RequestURLPath)
		})
	}
}

func TestChatCompatibilityRetryDoesNotGuessOrReplayDeliveredRequests(t *testing.T) {
	message := `Invalid option: expected one of "off"|"low"|"medium"|"high"|"xhigh"|"max"`
	toolMessage := `Function tools with reasoning_effort are not supported for gpt-5.6-sol in /v1/chat/completions. To use function tools, use /v1/responses or set reasoning_effort to 'none'.`
	for _, tc := range []struct {
		name, effort, message                   string
		status, channelType                     int
		passthrough, global, written, cancelled bool
	}{
		{name: "unrelated 400", effort: "none", message: "Invalid option: bad temperature", status: 400},
		{name: "minimal is not silently increased", effort: "minimal", message: message, status: 400},
		{name: "valid high is preserved", effort: "high", message: message, status: 400},
		{name: "absent effort is preserved", message: message, status: 400},
		{name: "authentication failure", effort: "none", message: message, status: 401},
		{name: "rate limit", effort: "none", message: message, status: 429},
		{name: "server failure", effort: "none", message: message, status: 500},
		{name: "channel passthrough", effort: "none", message: message, status: 400, passthrough: true},
		{name: "global passthrough", effort: "none", message: message, status: 400, global: true},
		{name: "already sent output", effort: "none", message: message, status: 400, written: true},
		{name: "cancelled client", effort: "none", message: message, status: 400, cancelled: true},
		{name: "custom routing is not guessed", effort: "none", message: message, status: 400, channelType: constant.ChannelTypeAdvancedCustom},
		{name: "no tools to convert", effort: "medium", message: toolMessage, status: 400},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
			if tc.written {
				_, err := c.Writer.Write([]byte("already sent"))
				require.NoError(t, err)
			}
			if tc.cancelled {
				ctx, cancel := context.WithCancel(c.Request.Context())
				cancel()
				c.Request = c.Request.WithContext(ctx)
			}
			oldGlobal := model_setting.GetGlobalSettings().PassThroughRequestEnabled
			model_setting.GetGlobalSettings().PassThroughRequestEnabled = tc.global
			t.Cleanup(func() { model_setting.GetGlobalSettings().PassThroughRequestEnabled = oldGlobal })
			kind := tc.channelType
			if kind == 0 {
				kind = constant.ChannelTypeOpenAI
			}
			info := &relaycommon.RelayInfo{RelayMode: relayconstant.RelayModeChatCompletions, RelayFormat: relaytypes.RelayFormatOpenAI, ChannelMeta: &relaycommon.ChannelMeta{ChannelType: kind}}
			info.ChannelSetting.PassThroughBodyEnabled = tc.passthrough
			wire, err := common.Marshal(map[string]any{"model": "gpt-5.6-sol", "reasoning_effort": tc.effort, "functions": nil})
			require.NoError(t, err)
			body, closer, err := relaycommon.NewOutboundJSONBody(wire)
			require.NoError(t, err)
			t.Cleanup(func() { _ = closer.Close() })
			value, apiErr, handled := retryChatCompatibility(c, info, &openaichannel.Adaptor{}, body, relaytypes.WithOpenAIError(relaytypes.OpenAIError{Message: tc.message}, tc.status))
			assert.False(t, handled)
			assert.Nil(t, value)
			assert.Nil(t, apiErr)
		})
	}
}

func TestTextHelperRetriesExplicitChatCompatibilityRejection(t *testing.T) {
	service.InitHttpClient()
	for _, tc := range []struct {
		name, body, message, retryPath string
		channelType                    int
	}{
		{
			name:      "disabled reasoning uses provider off spelling",
			body:      `{"model":"deepseek-v4.1-flash","messages":[{"role":"user","content":"hello"}],"reasoning_effort":"none","max_tokens":64}`,
			message:   `Invalid option: expected one of "off"|"low"|"medium"|"high"|"xhigh"|"max"`,
			retryPath: "/v1/chat/completions", channelType: constant.ChannelTypeSub2API,
		},
		{
			name:      "reasoning tools switch to Responses",
			body:      `{"model":"gpt-5.6-sol","messages":[{"role":"user","content":"hello"}],"reasoning_effort":"medium","max_completion_tokens":64,"tools":[{"type":"function","function":{"name":"lookup","parameters":{"type":"object","properties":{"id":{"type":"integer"}}}}}],"tool_choice":"required"}`,
			message:   `Function tools with reasoning_effort are not supported for gpt-5.6-sol in /v1/chat/completions. To use function tools, use /v1/responses or set reasoning_effort to 'none'.`,
			retryPath: "/v1/responses", channelType: constant.ChannelTypeOpenAI,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			type captured struct {
				path string
				body []byte
			}
			requests := make(chan captured, 3)
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, err := io.ReadAll(r.Body)
				if err != nil {
					http.Error(w, "read failed", 500)
					return
				}
				requests <- captured{r.URL.Path, body}
				w.Header().Set("Content-Type", "application/json")
				if len(requests) == 1 {
					w.WriteHeader(http.StatusBadRequest)
					data, _ := common.Marshal(map[string]any{"error": map[string]any{"message": tc.message, "type": "invalid_request_error"}})
					_, _ = w.Write(data)
					return
				}
				w.WriteHeader(http.StatusBadGateway)
				_, _ = io.WriteString(w, `{"error":{"message":"fixture retry failure","type":"upstream_error"}}`)
			}))
			t.Cleanup(upstream.Close)
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(tc.body))
			c.Request.Header.Set("Content-Type", "application/json")
			common.SetContextKey(c, constant.ContextKeyChannelType, tc.channelType)
			common.SetContextKey(c, constant.ContextKeyChannelBaseUrl, upstream.URL)
			var request dto.GeneralOpenAIRequest
			require.NoError(t, common.Unmarshal([]byte(tc.body), &request))
			info := &relaycommon.RelayInfo{Request: &request, OriginModelName: request.Model,
				RelayMode: relayconstant.RelayModeChatCompletions, RelayFormat: relaytypes.RelayFormatOpenAI, RequestURLPath: c.Request.URL.Path}
			apiErr := TextHelper(c, info)
			require.NotNil(t, apiErr)
			assert.Equal(t, http.StatusBadGateway, apiErr.StatusCode)
			require.Len(t, requests, 2, "exactly one compatibility retry")
			first, second := <-requests, <-requests
			assert.Equal(t, "/v1/chat/completions", first.path)
			assert.Equal(t, tc.retryPath, second.path)
			if tc.channelType == constant.ChannelTypeSub2API {
				assert.Equal(t, "off", gjson.GetBytes(second.body, "reasoning_effort").String())
				assert.Equal(t, int64(64), gjson.GetBytes(second.body, "max_tokens").Int())
			} else {
				assert.Equal(t, "medium", gjson.GetBytes(second.body, "reasoning.effort").String())
				assert.Equal(t, "lookup", gjson.GetBytes(second.body, "tools.0.name").String())
				assert.Equal(t, int64(64), gjson.GetBytes(second.body, "max_output_tokens").Int())
			}
			assert.Equal(t, gjson.Get(tc.body, "reasoning_effort").String(), request.ReasoningEffort, "later channels retain the client's original request")
		})
	}
}

func TestPrepareResponsesRequestRetainsConvertedAdaptor(t *testing.T) {
	type capturedRequest struct {
		path string
		body []byte
	}
	captured := make(chan capturedRequest, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		captured <- capturedRequest{path: r.URL.Path, body: body}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"chatcmpl_1","object":"chat.completion","model":"gpt-4o","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":3,"completion_tokens":2,"total_tokens":5}}`))
	}))
	defer server.Close()

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Request.Header.Set("Content-Type", "application/json")
	common.SetContextKey(c, constant.ContextKeyOriginalModel, "gpt-4o")
	common.SetContextKey(c, constant.ContextKeyChannelType, constant.ChannelTypeAdvancedCustom)
	common.SetContextKey(c, constant.ContextKeyChannelBaseUrl, server.URL)
	common.SetContextKey(c, constant.ContextKeyChannelKey, "test-key")
	common.SetContextKey(c, constant.ContextKeyChannelOtherSetting, dto.ChannelOtherSettings{
		AdvancedCustom: &dto.AdvancedCustomConfig{Routes: []dto.AdvancedCustomRoute{{
			IncomingPath: "/v1/responses", UpstreamPath: "/v1/chat/completions", Converter: relayconvert.ConverterOpenAIResponsesToOpenAIChat,
		}}},
	})
	request := &dto.OpenAIResponsesRequest{Model: "gpt-4o", Input: []byte(`"hello"`)}
	info := relaycommon.GenRelayInfoResponses(c, request)
	adaptor, body, closer, apiErr := PrepareResponsesRequest(c, info, request)
	require.Nil(t, apiErr)
	defer closer.Close()
	response, err := adaptor.DoRequest(c, info, body)
	require.NoError(t, err)
	httpResponse, ok := response.(*http.Response)
	require.True(t, ok)
	usage, apiErr := adaptor.DoResponse(c, httpResponse, info)
	require.Nil(t, apiErr)
	require.IsType(t, &dto.Usage{}, usage)
	assert.Equal(t, 5, usage.(*dto.Usage).TotalTokens)

	upstream := <-captured
	assert.Equal(t, "/v1/chat/completions", upstream.path)
	var upstreamRequest dto.GeneralOpenAIRequest
	require.NoError(t, common.Unmarshal(upstream.body, &upstreamRequest))
	require.Len(t, upstreamRequest.Messages, 1)
	assert.Equal(t, "hello", upstreamRequest.Messages[0].StringContent())
	assert.Equal(t, []relaytypes.RelayFormat{relaytypes.RelayFormatOpenAIResponses, relaytypes.RelayFormatOpenAI}, info.RequestConversionChain)
}

func TestIsResponsesEventStreamContentType(t *testing.T) {
	tests := []struct {
		name        string
		contentType string
		want        bool
	}{
		{name: "plain", contentType: "text/event-stream", want: true},
		{name: "mixed case with charset", contentType: "Text/Event-Stream; charset=utf-8", want: true},
		{name: "json", contentType: "application/json", want: false},
		{name: "empty", contentType: "", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, isResponsesEventStreamContentType(tt.contentType))
		})
	}
}

func TestRecalcQuotaFromRatiosIgnoresInvalidMultipliers(t *testing.T) {
	info := &relaycommon.RelayInfo{
		PriceData: hosttypes.PriceData{
			Quota: 100,
		},
	}
	info.PriceData.AddOtherRatio("duration", 2)

	quota, ok := recalcQuotaFromRatios(info, map[string]float64{
		"duration": 3,
		"zero":     0,
		"negative": -1,
		"nan":      math.NaN(),
		"inf":      math.Inf(1),
	})

	require.True(t, ok)
	assert.Equal(t, 150, quota)
	assert.True(t, info.PriceData.HasOtherRatio("duration"))
}

func TestRecalcQuotaFromRatiosRejectsAllInvalidAdjustedRatios(t *testing.T) {
	info := &relaycommon.RelayInfo{
		PriceData: hosttypes.PriceData{
			Quota: 100,
		},
	}
	info.PriceData.AddOtherRatio("duration", 2)

	quota, ok := recalcQuotaFromRatios(info, map[string]float64{
		"zero":     0,
		"negative": -1,
		"nan":      math.NaN(),
		"inf":      math.Inf(1),
	})

	require.False(t, ok)
	assert.Equal(t, 0, quota)
	assert.True(t, info.PriceData.HasOtherRatio("duration"))
}

func TestTextRequestViaResponsesConvertsClaudeDirectly(t *testing.T) {
	type capturedRequest struct {
		path string
		body []byte
	}
	captured := make(chan capturedRequest, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		captured <- capturedRequest{path: r.URL.Path, body: body}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"resp_1",
			"object":"response",
			"status":"completed",
			"model":"gpt-5.6-sol",
			"output":[{"type":"message","id":"msg_1","role":"assistant","content":[{"type":"output_text","text":"ok"}]}],
			"usage":{"input_tokens":3,"output_tokens":2,"total_tokens":5}
		}`))
	}))
	defer server.Close()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	c.Request.Header.Set("Content-Type", "application/json")

	info := &relaycommon.RelayInfo{
		RelayMode:              relayconstant.RelayModeChatCompletions,
		RelayFormat:            relaytypes.RelayFormatClaude,
		OriginModelName:        "gpt-5.6-sol",
		RequestConversionChain: []relaytypes.RelayFormat{relaytypes.RelayFormatClaude},
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType:       constant.ChannelTypeOpenAI,
			ChannelBaseUrl:    server.URL,
			ApiKey:            "test-key",
			UpstreamModelName: "gpt-5.6-sol",
		},
	}
	adaptor := &openaichannel.Adaptor{}
	adaptor.Init(info)
	request := &dto.ClaudeRequest{
		Model:    "gpt-5.6-sol",
		Thinking: &dto.Thinking{Type: "adaptive", Display: "summarized"},
		Messages: []dto.ClaudeMessage{{Role: "user", Content: "hello"}},
	}

	usage, apiErr := textRequestViaResponses(c, info, adaptor, request)

	require.Nil(t, apiErr)
	require.NotNil(t, usage)
	assert.Equal(t, 5, usage.TotalTokens)
	assert.Equal(t, []relaytypes.RelayFormat{relaytypes.RelayFormatClaude, relaytypes.RelayFormatOpenAIResponses}, info.RequestConversionChain)

	upstream := <-captured
	assert.Equal(t, "/v1/responses", upstream.path)
	var upstreamBody map[string]any
	require.NoError(t, common.Unmarshal(upstream.body, &upstreamBody))
	assert.NotContains(t, upstreamBody, "messages")
	reasoning, ok := upstreamBody["reasoning"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "high", reasoning["effort"])
	assert.Equal(t, "detailed", reasoning["summary"])

	var response dto.ClaudeResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	require.Len(t, response.Content, 1)
	assert.Equal(t, "ok", response.Content[0].GetText())
}

func TestApplySystemPromptIfNeededSkipsToolLoadingMessages(t *testing.T) {
	tools := json.RawMessage(`[{"type":"function","function":{"name":"get_current_time","parameters":{"type":"object","properties":{"city":{"type":"string"}}}}}]`)
	toolLoading := dto.Message{Role: "system", Tools: tools}
	user := dto.Message{Role: "user", Content: "What time is it in Beijing?"}

	tests := []struct {
		name         string
		messages     []dto.Message
		wantMessages []dto.Message
		wantOverride bool
	}{
		{
			name:     "tool loading message alone is not a system prompt",
			messages: []dto.Message{toolLoading, user},
			wantMessages: []dto.Message{
				{Role: "system", Content: "Answer in English."},
				toolLoading,
				user,
			},
		},
		{
			name:     "override targets the real system prompt only",
			messages: []dto.Message{toolLoading, {Role: "system", Content: "You are Kimi."}, user},
			wantMessages: []dto.Message{
				toolLoading,
				{Role: "system", Content: "Answer in English.\nYou are Kimi."},
				user,
			},
			wantOverride: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
			info := &relaycommon.RelayInfo{
				ChannelMeta: &relaycommon.ChannelMeta{
					ChannelSetting: dto.ChannelSettings{
						SystemPrompt:         "Answer in English.",
						SystemPromptOverride: true,
					},
				},
			}
			request := &dto.GeneralOpenAIRequest{
				Model:    "kimi-k3",
				Messages: append([]dto.Message(nil), tt.messages...),
			}

			applySystemPromptIfNeeded(c, info, request)

			require.Len(t, request.Messages, len(tt.wantMessages))
			for i, want := range tt.wantMessages {
				got := request.Messages[i]
				assert.Equal(t, want.Role, got.Role, "message %d role", i)
				assert.Equal(t, want.Content, got.Content, "message %d content", i)
				if len(want.Tools) > 0 {
					assert.JSONEq(t, string(want.Tools), string(got.Tools), "message %d tools", i)
				} else {
					assert.Empty(t, got.Tools, "message %d tools", i)
				}
			}
			_, overrideSet := common.GetContextKey(c, constant.ContextKeySystemPromptOverride)
			assert.Equal(t, tt.wantOverride, overrideSet)
		})
	}
}
