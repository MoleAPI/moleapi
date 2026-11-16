package vertex

import (
	"bytes"
	"errors"
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
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVertexTextRequestFormats(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, model := range []string{"gemini-4-argon", "claude-sonnet-4-5@20250929", "meta/llama-4-maas"} {
		for _, format := range []types.RelayFormat{types.RelayFormatClaude, types.RelayFormatGemini, types.RelayFormatOpenAIResponses} {
			t.Run(model+"/"+string(format), func(t *testing.T) {
				c, _ := gin.CreateTestContext(httptest.NewRecorder())
				info := &relaycommon.RelayInfo{RelayFormat: format, OriginModelName: model, ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: model}}
				a := &Adaptor{}
				a.Init(info)
				var converted any
				var err error
				switch format {
				case types.RelayFormatClaude:
					var req dto.ClaudeRequest
					require.NoError(t, common.Unmarshal([]byte(`{"model":"`+model+`","max_tokens":128,"messages":[{"role":"user","content":"hello"}]}`), &req))
					converted, err = a.ConvertClaudeRequest(c, info, &req)
				case types.RelayFormatGemini:
					var req dto.GeminiChatRequest
					require.NoError(t, common.Unmarshal([]byte(`{"contents":[{"role":"user","parts":[{"text":"hello"}]}],"generationConfig":{"maxOutputTokens":128}}`), &req))
					converted, err = a.ConvertGeminiRequest(c, info, &req)
				case types.RelayFormatOpenAIResponses:
					var req dto.OpenAIResponsesRequest
					require.NoError(t, common.Unmarshal([]byte(`{"model":"`+model+`","input":"hello","max_output_tokens":128}`), &req))
					converted, err = a.ConvertOpenAIResponsesRequest(c, info, req)
				}
				require.NoError(t, err)
				data, err := common.Marshal(converted)
				require.NoError(t, err)
				assert.Contains(t, string(data), "hello")
				switch a.RequestMode {
				case RequestModeGemini:
					req, ok := converted.(*dto.GeminiChatRequest)
					require.True(t, ok)
					require.NotNil(t, req.GenerationConfig.MaxOutputTokens)
					assert.EqualValues(t, 128, *req.GenerationConfig.MaxOutputTokens)
				case RequestModeClaude:
					req, ok := converted.(*VertexAIClaudeRequest)
					require.True(t, ok)
					assert.Equal(t, anthropicVersion, req.AnthropicVersion)
					require.NotNil(t, req.MaxTokens)
					assert.EqualValues(t, 128, *req.MaxTokens)
					assert.NotContains(t, string(data), `"model"`)
				case RequestModeOpenSource:
					req, ok := converted.(*dto.GeneralOpenAIRequest)
					require.True(t, ok)
					assert.Equal(t, model, req.Model)
				}
			})
		}
	}
}

func TestVertexUnsupportedEndpoints(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "gemini-4-argon"}}
	a := &Adaptor{}
	a.Init(info)
	for _, test := range []struct {
		name    string
		convert func() error
	}{
		{"embedding", func() error { _, err := a.ConvertEmbeddingRequest(c, info, dto.EmbeddingRequest{}); return err }},
		{"audio", func() error { _, err := a.ConvertAudioRequest(c, info, dto.AudioRequest{}); return err }},
		{"rerank", func() error { _, err := a.ConvertRerankRequest(c, 0, dto.RerankRequest{}); return err }},
		{"image", func() error { _, err := a.ConvertImageRequest(c, info, dto.ImageRequest{}); return err }},
		{"native embedding", func() error {
			info.RequestURLPath = "/v1beta/models/gemini-embedding-001:embedContent"
			_, err := a.GetRequestURL(info)
			return err
		}},
		{"native batch embedding", func() error {
			info.RequestURLPath = "/v1beta/models/gemini-embedding-001:batchEmbedContents"
			_, err := a.GetRequestURL(info)
			return err
		}},
		{"Imagen Responses", func() error {
			info.UpstreamModelName = "imagen-4.0-generate-001"
			_, err := a.ConvertOpenAIResponsesRequest(c, info, dto.OpenAIResponsesRequest{})
			return err
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			var apiErr *types.NewAPIError
			require.True(t, errors.As(test.convert(), &apiErr))
			assert.Equal(t, http.StatusBadRequest, apiErr.StatusCode)
			assert.True(t, types.IsSkipRetryError(apiErr))
		})
	}
}

func TestVertexPartnerResponsesReply(t *testing.T) {
	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 300
	t.Cleanup(func() { constant.StreamingTimeout = oldTimeout })
	for _, model := range []string{"claude-sonnet-4-5@20250929", "meta/llama-4-maas"} {
		for _, stream := range []bool{false, true} {
			t.Run(model+map[bool]string{false: "/json", true: "/stream"}[stream], func(t *testing.T) {
				recorder := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(recorder)
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
				c.Set(common.RequestIdKey, "vertex-partner-test")
				info := &relaycommon.RelayInfo{IsStream: stream, DisablePing: true,
					RelayMode: relayconstant.RelayModeResponses, RelayFormat: types.RelayFormatOpenAIResponses,
					OriginModelName: model, ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: model}}
				a := &Adaptor{}
				a.Init(info)
				body := `{"id":"chatcmpl-fixture","object":"chat.completion","model":"` + model + `","choices":[{"index":0,"message":{"role":"assistant","content":"hello"},"finish_reason":"stop"}],"usage":{"prompt_tokens":2,"completion_tokens":5,"total_tokens":7}}`
				if stream {
					body = "data: " + strings.Replace(body, `"message":`, `"delta":`, 1) + "\n\ndata: [DONE]\n\n"
				}
				if a.RequestMode == RequestModeClaude {
					body = `{"id":"msg_fixture","type":"message","role":"assistant","model":"` + model + `","content":[{"type":"text","text":"hello"}],"stop_reason":"end_turn","usage":{"input_tokens":2,"output_tokens":5}}`
					if stream {
						body = "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_fixture\",\"type\":\"message\",\"role\":\"assistant\",\"content\":[],\"usage\":{\"input_tokens\":2,\"output_tokens\":0}}}\n\n" +
							"event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"text\",\"text\":\"\"}}\n\n" +
							"event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"hello\"}}\n\n" +
							"event: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":0}\n\n" +
							"event: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"},\"usage\":{\"output_tokens\":5}}\n\n" +
							"event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n"
					}
				}
				usage, apiErr := a.DoResponse(c, &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, info)
				require.Nil(t, apiErr)
				gotUsage, ok := usage.(*dto.Usage)
				require.True(t, ok)
				assert.Equal(t, 2, gotUsage.PromptTokens)
				assert.Equal(t, 5, gotUsage.CompletionTokens)
				assert.Equal(t, 7, gotUsage.TotalTokens)
				assert.Contains(t, recorder.Body.String(), "hello")
				assert.Contains(t, recorder.Body.String(), `"output_tokens":5`)
				if stream {
					assert.Contains(t, recorder.Body.String(), "event: response.completed")
				} else {
					assert.Contains(t, recorder.Body.String(), `"object":"response"`)
				}
			})
		}
	}
}

func TestVertexImagenReplyFormats(t *testing.T) {
	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 300
	t.Cleanup(func() { constant.StreamingTimeout = oldTimeout })
	for _, path := range []string{"/v1/images/generations", "/v1/chat/completions", "/v1/responses"} {
		for _, stream := range []bool{false, true} {
			t.Run(path+map[bool]string{false: "/json", true: "/stream"}[stream], func(t *testing.T) {
				recorder := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(recorder)
				c.Request = httptest.NewRequest(http.MethodPost, path, nil)
				format := types.RelayFormat(types.RelayFormatOpenAIImage)
				if path == "/v1/responses" {
					format = types.RelayFormatOpenAIResponses
					c.Set("responses_image_generation_bridge", true)
				} else if path == "/v1/chat/completions" {
					format = types.RelayFormatOpenAI
					c.Set("chat_image_completion_bridge", true)
				}
				info := &relaycommon.RelayInfo{IsStream: stream, DisablePing: true, RelayMode: relayconstant.RelayModeImagesGenerations,
					RelayFormat: format, RequestURLPath: path, OriginModelName: "imagen-4.0-generate-001", ImageRequestCount: 3,
					Request: &dto.ImageRequest{ResponseFormat: "b64_json"}, ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "imagen-4.0-generate-001"}}
				info.PriceData.UsePrice = true
				info.PriceData.AddOtherRatio("n", 3)
				body := `{"predictions":[{"bytesBase64Encoded":"iVBORw0KGgoAAAANSUhEUgAAAAIAAAACCAYAAABytg0kAAAAFUlEQVR4nGNkYGD4z8DAwMAEIkAYAA4oAQOSwUJsAAAAAElFTkSuQmCC"},{},{"raiFilteredReason":"filtered","bytesBase64Encoded":"ignored"}]}`
				a := &Adaptor{}
				a.Init(info)
				usage, apiErr := a.DoResponse(c, &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, info)
				require.Nil(t, apiErr)
				assert.Equal(t, 258, usage.(*dto.Usage).PromptTokens)
				assert.EqualValues(t, 1, info.PriceData.OtherRatios()["n"])
				got := recorder.Body.String()
				assert.Contains(t, got, "iVBORw0KGgo")
				assert.NotContains(t, got, "ignored")
				if stream {
					assert.Contains(t, recorder.Header().Get("Content-Type"), "text/event-stream")
					if path == "/v1/responses" {
						assert.Contains(t, got, "event: response.completed")
					} else if path == "/v1/chat/completions" {
						assert.Contains(t, got, `"object":"chat.completion.chunk"`)
					} else {
						assert.Contains(t, got, "image_generation.completed")
					}
				} else {
					if path == "/v1/responses" {
						assert.Contains(t, got, `"object":"response"`)
					} else if path == "/v1/chat/completions" {
						assert.Contains(t, got, `"object":"chat.completion"`)
					} else {
						assert.Contains(t, got, `"data":[`)
					}
				}
			})
		}
	}
	t.Run("no image payload", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "imagen-4.0-generate-001"}}
		a := &Adaptor{}
		a.Init(info)
		usage, apiErr := a.DoResponse(c, &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"predictions":[{},{"raiFilteredReason":"filtered","bytesBase64Encoded":"ignored"}]}`))}, info)
		require.NotNil(t, apiErr)
		assert.Nil(t, usage)
		assert.Empty(t, recorder.Body.String())
	})
}

func TestVertexGeminiResponsesRequest(t *testing.T) {
	var request dto.OpenAIResponsesRequest
	require.NoError(t, common.Unmarshal([]byte(`{
		"model":"gemini-4-argon",
		"instructions":"Follow the user's request",
		"input":"Look up the weather",
		"max_output_tokens":128,
		"tools":[{"type":"function","name":"weather","parameters":{"type":"object"}}]
	}`), &request))
	info := &relaycommon.RelayInfo{
		OriginModelName: request.Model,
		RelayFormat:     types.RelayFormatOpenAIResponses,
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType:       constant.ChannelTypeVertexAi,
			UpstreamModelName: request.Model,
			ApiVersion:        `{"default":"global"}`,
			ApiKey:            "test-key",
			ChannelOtherSettings: dto.ChannelOtherSettings{
				VertexKeyType:   dto.VertexKeyTypeAPIKey,
				VertexProjectID: "test-project",
			},
		},
	}
	adaptor := &Adaptor{}
	adaptor.Init(info)
	converted, err := adaptor.ConvertOpenAIResponsesRequest(nil, info, request)
	require.NoError(t, err)
	geminiRequest, ok := converted.(*dto.GeminiChatRequest)
	require.True(t, ok)
	require.NotNil(t, geminiRequest.SystemInstructions)
	assert.Equal(t, "Follow the user's request", geminiRequest.SystemInstructions.Parts[0].Text)
	require.Len(t, geminiRequest.Contents, 1)
	assert.Equal(t, "Look up the weather", geminiRequest.Contents[0].Parts[0].Text)
	require.NotNil(t, geminiRequest.GenerationConfig.MaxOutputTokens)
	assert.EqualValues(t, 128, *geminiRequest.GenerationConfig.MaxOutputTokens)
	assert.Contains(t, string(geminiRequest.Tools), `"name":"weather"`)
	url, err := adaptor.GetRequestURL(info)
	require.NoError(t, err)
	assert.Equal(t, "https://aiplatform.googleapis.com/v1/projects/test-project/locations/global/publishers/google/models/gemini-4-argon:generateContent?key=test-key", url)
}

func TestVertexGeminiResponsesReply(t *testing.T) {
	gin.SetMode(gin.TestMode)
	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 300
	t.Cleanup(func() { constant.StreamingTimeout = oldTimeout })

	for _, stream := range []bool{false, true} {
		t.Run(map[bool]string{false: "json", true: "stream"}[stream], func(t *testing.T) {
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
			c.Set(common.RequestIdKey, "vertex-responses-test")
			info := &relaycommon.RelayInfo{
				IsStream: stream, DisablePing: true,
				RelayMode: relayconstant.RelayModeResponses, RelayFormat: types.RelayFormatOpenAIResponses,
				RequestURLPath: "/v1/responses", OriginModelName: "gemini-4-argon",
				ChannelMeta: &relaycommon.ChannelMeta{ChannelType: constant.ChannelTypeVertexAi, UpstreamModelName: "gemini-4-argon"},
			}
			body := []byte(`{"candidates":[{"content":{"role":"model","parts":[{"text":"hello"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":2,"candidatesTokenCount":3,"thoughtsTokenCount":2,"totalTokenCount":7}}`)
			if stream {
				body = []byte("data: " + string(body) + "\n\ndata: [DONE]\n\n")
			}
			adaptor := &Adaptor{}
			adaptor.Init(info)
			usage, apiError := adaptor.DoResponse(c, &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(bytes.NewReader(body))}, info)
			require.Nil(t, apiError)
			gotUsage, ok := usage.(*dto.Usage)
			require.True(t, ok)
			assert.Equal(t, 2, gotUsage.PromptTokens)
			assert.Equal(t, 5, gotUsage.CompletionTokens)
			assert.Equal(t, 7, gotUsage.TotalTokens)
			got := recorder.Body.String()
			if stream {
				assert.Equal(t, "text/event-stream", recorder.Header().Get("Content-Type"))
				assert.Contains(t, got, "event: response.output_text.delta")
				assert.Contains(t, got, `"delta":"hello"`)
				assert.Contains(t, got, "event: response.completed")
				assert.Less(t, strings.Index(got, "event: response.created"), strings.Index(got, "event: response.completed"))
			} else {
				var response dto.OpenAIResponsesResponse
				require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
				var status string
				require.NoError(t, common.Unmarshal(response.Status, &status))
				assert.Equal(t, "completed", status)
				assert.Contains(t, got, `"text":"hello"`)
			}
			assert.Contains(t, got, `"input_tokens":2`)
			assert.Contains(t, got, `"output_tokens":5`)
		})
	}
}

func TestVertexResponsesFunctionCallIDs(t *testing.T) {
	settings := model_setting.GetGeminiSettings()
	old := settings.RemoveFunctionResponseIdEnabled
	t.Cleanup(func() { settings.RemoveFunctionResponseIdEnabled = old })
	for _, strip := range []bool{false, true} {
		settings.RemoveFunctionResponseIdEnabled = strip
		var request dto.OpenAIResponsesRequest
		require.NoError(t, common.Unmarshal([]byte(`{
			"model":"gemini-3.8-flash",
			"input":[
				{"type":"function_call","call_id":"call_weather","name":"weather","arguments":"{}"},
				{"type":"function_call_output","call_id":"call_weather","output":"sunny"}
			]
		}`), &request))
		info := &relaycommon.RelayInfo{
			OriginModelName: request.Model, RelayFormat: types.RelayFormatOpenAIResponses,
			ChannelMeta: &relaycommon.ChannelMeta{ChannelType: constant.ChannelTypeVertexAi, UpstreamModelName: request.Model},
		}
		adaptor := &Adaptor{}
		adaptor.Init(info)
		converted, err := adaptor.ConvertOpenAIResponsesRequest(nil, info, request)
		require.NoError(t, err)
		got, ok := converted.(*dto.GeminiChatRequest)
		require.True(t, ok)
		require.Len(t, got.Contents, 2)
		call := got.Contents[0].Parts[0].FunctionCall
		response := got.Contents[1].Parts[0].FunctionResponse
		require.NotNil(t, call)
		require.NotNil(t, response)
		assert.Equal(t, "weather", call.FunctionName)
		assert.Equal(t, "weather", response.Name)
		if strip {
			assert.Empty(t, call.ID)
			assert.Empty(t, response.ID)
		} else {
			assert.NotEmpty(t, call.ID)
			assert.NotEmpty(t, response.ID)
		}
	}
}
