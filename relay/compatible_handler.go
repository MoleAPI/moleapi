package relay

import (
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/relay/channel"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/samber/lo"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"

	"github.com/gin-gonic/gin"
)

func TextHelper(c *gin.Context, info *relaycommon.RelayInfo) (newAPIError *types.NewAPIError) {
	info.InitChannelMeta(c)

	textReq, ok := info.Request.(*dto.GeneralOpenAIRequest)
	if !ok {
		return types.NewErrorWithStatusCode(fmt.Errorf("invalid request type, expected dto.GeneralOpenAIRequest, got %T", info.Request), types.ErrorCodeInvalidRequest, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
	}

	request, err := common.DeepCopy(textReq)
	if err != nil {
		return types.NewError(fmt.Errorf("failed to copy request to GeneralOpenAIRequest: %w", err), types.ErrorCodeInvalidRequest, types.ErrOptionWithSkipRetry())
	}

	if request.WebSearchOptions != nil {
		c.Set("chat_completion_web_search_context_size", request.WebSearchOptions.SearchContextSize)
	}

	err = helper.ModelMappedHelper(c, info, request)
	if err != nil {
		return types.NewError(err, types.ErrorCodeChannelModelMappedError, types.ErrOptionWithSkipRetry())
	}
	if err := helper.ApplyReasoningModelSuffix(c, info, request); err != nil {
		return newConvertRequestFailedError(c, info, err)
	}

	includeUsage := true
	// 判断用户是否需要返回使用情况
	if request.StreamOptions != nil {
		includeUsage = request.StreamOptions.IncludeUsage
	}

	// 如果不支持StreamOptions，将StreamOptions设置为nil
	if !info.SupportStreamOptions || !lo.FromPtrOr(request.Stream, false) {
		request.StreamOptions = nil
	} else {
		// 如果支持StreamOptions，且请求中没有设置StreamOptions，根据配置文件设置StreamOptions
		if constant.ForceStreamOption {
			request.StreamOptions = &dto.StreamOptions{
				IncludeUsage: true,
			}
		}
	}

	info.ShouldIncludeUsage = includeUsage

	adaptor := GetAdaptor(info.ApiType)
	if adaptor == nil {
		return types.NewError(fmt.Errorf("invalid api type: %d", info.ApiType), types.ErrorCodeInvalidApiType, types.ErrOptionWithSkipRetry())
	}
	adaptor.Init(info)

	passThroughGlobal := model_setting.GetGlobalSettings().PassThroughRequestEnabled
	if info.RelayMode == relayconstant.RelayModeChatCompletions &&
		!passThroughGlobal &&
		!info.ChannelSetting.PassThroughBodyEnabled &&
		service.ShouldChatCompletionsUseResponsesGlobal(info.ChannelId, info.ChannelType, info.OriginModelName) {
		applySystemPromptIfNeeded(c, info, request)
		usage, newApiErr := textRequestViaResponses(c, info, adaptor, request)
		if newApiErr != nil {
			return newApiErr
		}

		var containAudioTokens = usage.CompletionTokenDetails.AudioTokens > 0 || usage.PromptTokensDetails.AudioTokens > 0
		var containsAudioRatios = ratio_setting.ContainsAudioRatio(info.OriginModelName) || ratio_setting.ContainsAudioCompletionRatio(info.OriginModelName)

		if containAudioTokens && containsAudioRatios {
			service.PostAudioConsumeQuota(c, info, usage, nil)
		} else {
			service.PostTextConsumeQuota(c, info, usage, nil)
		}
		return nil
	}

	var requestBody io.Reader

	if passThroughGlobal || info.ChannelSetting.PassThroughBodyEnabled {
		storage, err := common.GetBodyStorage(c)
		if err != nil {
			return types.NewErrorWithStatusCode(err, types.ErrorCodeReadRequestBodyFailed, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
		}
		if common.DebugEnabled {
			if debugBytes, bErr := storage.Bytes(); bErr == nil {
				logger.LogDebug(c, "requestBody: %s", debugBytes)
			}
		}
		requestBody = common.NewReplayableBodyReader(storage)
	} else {
		convertedRequest, err := adaptor.ConvertOpenAIRequest(c, info, request)
		if err != nil {
			return newConvertRequestFailedError(c, info, err)
		}
		relaycommon.AppendRequestConversionFromRequest(info, convertedRequest)

		if req, ok := convertedRequest.(*dto.GeneralOpenAIRequest); ok {
			applySystemPromptIfNeeded(c, info, req)
		}

		if chatRequest, ok := convertedRequest.(*dto.GeneralOpenAIRequest); ok {
			reasoningField := strings.TrimSpace(info.ChannelSetting.ReasoningContentField)
			if reasoningField == "" {
				reasoningField = dto.ReasoningFieldContent
			}
			reasoningMessages := 0
			reasoningDetails := 0
			toolCallMessages := 0
			toolCallReasoningMessages := 0
			sourceContent, sourceReasoning, sourceText := 0, 0, 0
			for i := range chatRequest.Messages {
				message := &chatRequest.Messages[i]
				hasReasoning := message.ReasoningContent != nil || message.Reasoning != nil || message.ReasoningText != nil || len(message.ReasoningDetails) > 0
				if hasReasoning {
					reasoningMessages++
				}
				switch {
				case message.ReasoningContent != nil:
					sourceContent++
				case message.Reasoning != nil:
					sourceReasoning++
				case message.ReasoningText != nil:
					sourceText++
				}
				if len(message.ReasoningDetails) > 0 {
					reasoningDetails++
				}
				if len(message.ToolCalls) > 0 {
					toolCallMessages++
					if hasReasoning {
						toolCallReasoningMessages++
					}
				}
				message.NormalizeReasoningContentField(reasoningField)
			}
			if toolCallMessages > 0 && (info.RetryIndex > 0 || sourceReasoning > 0 || sourceText > 0 || reasoningDetails > 0 || toolCallReasoningMessages == 0) {
				logger.LogInfo(c, fmt.Sprintf("reasoning relay: channel_id=%d retry=%d target_field=%s source_reasoning_content=%d source_reasoning=%d source_reasoning_text=%d reasoning_details=%d reasoning_messages=%d tool_call_messages=%d tool_call_reasoning_messages=%d", info.ChannelId, info.RetryIndex, reasoningField, sourceContent, sourceReasoning, sourceText, reasoningDetails, reasoningMessages, toolCallMessages, toolCallReasoningMessages))
			}
		}

		jsonData, err := common.Marshal(convertedRequest)
		if err != nil {
			return types.NewError(err, types.ErrorCodeJsonMarshalFailed, types.ErrOptionWithSkipRetry())
		}

		// remove disabled fields for OpenAI API
		jsonData, err = relaycommon.RemoveDisabledFields(jsonData, info.ChannelOtherSettings, info.ChannelSetting.PassThroughBodyEnabled)
		if err != nil {
			return types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
		}

		// apply param override
		if len(info.ParamOverride) > 0 {
			jsonData, err = relaycommon.ApplyParamOverrideWithRelayInfo(jsonData, info)
			if err != nil {
				return newAPIErrorFromParamOverride(err)
			}
		}

		logger.LogDebug(c, "text request body: %s", jsonData)

		body, closer, err := relaycommon.NewOutboundJSONBody(jsonData)
		if err != nil {
			return types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
		}
		defer closer.Close()
		jsonData = nil
		requestBody = body
	}

	var usage any
	var newApiErr *types.NewAPIError
	var httpResp *http.Response
	resp, err := adaptor.DoRequest(c, info, requestBody)
	if err != nil {
		return types.NewOpenAIError(err, types.ErrorCodeDoRequestFailed, http.StatusInternalServerError)
	}

	statusCodeMappingStr := c.GetString("status_code_mapping")

	if resp != nil {
		httpResp = resp.(*http.Response)
		info.IsStream = info.IsStream || strings.HasPrefix(httpResp.Header.Get("Content-Type"), "text/event-stream")
		if httpResp.StatusCode != http.StatusOK {
			upstreamErr := service.RelayErrorHandler(c.Request.Context(), httpResp, false)
			var handled bool
			usage, newApiErr, handled = retryChatCompatibility(c, info, adaptor, requestBody, upstreamErr)
			if !handled {
				newApiErr = upstreamErr
			}
			if newApiErr != nil {
				service.ResetStatusCode(newApiErr, statusCodeMappingStr)
				return newApiErr
			}
		}
	}

	if usage == nil {
		usage, newApiErr = adaptor.DoResponse(c, httpResp, info)
	}
	if newApiErr != nil {
		// reset status code 重置状态码
		service.ResetStatusCode(newApiErr, statusCodeMappingStr)
		return newApiErr
	}

	var containAudioTokens = usage.(*dto.Usage).CompletionTokenDetails.AudioTokens > 0 || usage.(*dto.Usage).PromptTokensDetails.AudioTokens > 0
	var containsAudioRatios = ratio_setting.ContainsAudioRatio(info.OriginModelName) || ratio_setting.ContainsAudioCompletionRatio(info.OriginModelName)

	if containAudioTokens && containsAudioRatios {
		service.PostAudioConsumeQuota(c, info, usage.(*dto.Usage), nil)
	} else {
		service.PostTextConsumeQuota(c, info, usage.(*dto.Usage), nil)
	}
	return nil
}

// ponytail: retry only these explicit pre-generation rejections once; unknown
// parameter errors keep the normal channel retry policy instead of guessing.
func retryChatCompatibility(c *gin.Context, info *relaycommon.RelayInfo, adaptor channel.Adaptor, body io.Reader, upstreamErr *types.NewAPIError) (any, *types.NewAPIError, bool) {
	if upstreamErr.StatusCode != http.StatusBadRequest || c.Writer.Written() || c.Request.Context().Err() != nil ||
		info.RelayMode != relayconstant.RelayModeChatCompletions || info.RelayFormat != types.RelayFormatOpenAI ||
		info.ChannelSetting.PassThroughBodyEnabled || model_setting.GetGlobalSettings().PassThroughRequestEnabled {
		return nil, nil, false
	}
	switch info.ChannelType {
	case constant.ChannelTypeOpenAI, constant.ChannelTypeAzure, constant.ChannelTypeNewAPI, constant.ChannelTypeSub2API:
	default:
		return nil, nil, false
	}
	message := upstreamErr.Error()
	offSpelling := strings.Contains(message, `Invalid option: expected one of "off"|"low"|"medium"|"high"|"xhigh"|"max"`)
	responsesTools := strings.Contains(message, "Function tools with reasoning_effort are not supported for ") &&
		strings.Contains(message, "in /v1/chat/completions.") && strings.Contains(message, "use /v1/responses")
	replayable, ok := body.(common.ReplayableBody)
	if !ok || (!offSpelling && !responsesTools) {
		return nil, nil, false
	}
	reader, err := replayable.NewReader()
	if err != nil {
		return nil, nil, false
	}
	defer reader.Close()
	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, nil, false
	}
	if offSpelling && gjson.GetBytes(data, "reasoning_effort").String() == "none" {
		data, err = sjson.SetBytes(data, "reasoning_effort", "off")
		if err != nil {
			return nil, nil, false
		}
		retryBody, closer, err := relaycommon.NewOutboundJSONBody(data)
		if err != nil {
			return nil, types.NewError(err, types.ErrorCodeConvertRequestFailed), true
		}
		defer closer.Close()
		logger.LogInfo(c, fmt.Sprintf("chat compatibility retry: channel_id=%d reasoning_effort none -> off", info.ChannelId))
		response, err := adaptor.DoRequest(c, info, retryBody)
		if err != nil {
			return nil, types.NewOpenAIError(err, types.ErrorCodeDoRequestFailed, http.StatusBadGateway), true
		}
		resp, ok := response.(*http.Response)
		if !ok || resp == nil {
			return nil, types.NewErrorWithStatusCode(fmt.Errorf("invalid compatibility retry response"), types.ErrorCodeBadResponse, http.StatusBadGateway), true
		}
		if resp.StatusCode != http.StatusOK {
			return nil, service.RelayErrorHandler(c.Request.Context(), resp, false), true
		}
		info.IsStream = info.IsStream || isResponsesEventStreamContentType(resp.Header.Get("Content-Type"))
		usage, apiErr := adaptor.DoResponse(c, resp, info)
		return usage, apiErr, true
	}
	if responsesTools {
		var request dto.GeneralOpenAIRequest
		if err := common.Unmarshal(data, &request); err != nil || request.ReasoningEffort == "none" {
			return nil, nil, false
		}
		functionTools := len(gjson.GetBytes(data, "functions").Array()) > 0
		for _, tool := range request.Tools {
			functionTools = functionTools || tool.Type == "function"
		}
		if !functionTools {
			return nil, nil, false
		}
		result, err := service.ConvertRequest(c, info, types.RelayFormatOpenAIResponses, &request)
		if err != nil {
			return nil, nil, false
		}
		responses, ok := result.Value.(*dto.OpenAIResponsesRequest)
		if !ok {
			return nil, nil, false
		}
		logger.LogInfo(c, fmt.Sprintf("chat compatibility retry: channel_id=%d function tools via Responses", info.ChannelId))
		// The replay contains the final mapped and overridden request. Applying
		// channel overrides twice could change counts, limits, or message content.
		usage, apiErr := relayResponsesRequest(c, info, adaptor, responses, true)
		return usage, apiErr, true
	}
	return nil, nil, false
}
