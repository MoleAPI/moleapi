package advancedcustom

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/relay/channel"
	"github.com/QuantumNous/new-api/relay/channel/claude"
	"github.com/QuantumNous/new-api/relay/channel/gemini"
	"github.com/QuantumNous/new-api/relay/channel/openai"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/relayconvert"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/samber/lo"
)

const ChannelName = "advanced_custom"

const advancedCustomModelPlaceholder = "{model}"

type Adaptor struct {
	openaiAdaptor openai.Adaptor
	claudeAdaptor claude.Adaptor
	geminiAdaptor gemini.Adaptor

	resolved  bool
	converted bool
	route     dto.AdvancedCustomRoute
	converter string
}

func (a *Adaptor) Init(info *relaycommon.RelayInfo) {
	a.openaiAdaptor.Init(info)
	a.claudeAdaptor.Init(info)
	a.geminiAdaptor.Init(info)
}

func (a *Adaptor) ConvertOpenAIRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeneralOpenAIRequest) (any, error) {
	converter, err := a.resolveForConversion(c, info)
	if err != nil {
		return nil, err
	}
	if converter == relayconvert.ConverterNone {
		return a.convertOpenAICompatibleRequest(c, info, request)
	}

	switch converter {
	case relayconvert.ConverterOpenAIChatToClaudeMessages,
		relayconvert.ConverterOpenAIChatToOpenAIResponses,
		relayconvert.ConverterOpenAIChatToGeminiContent:
		result, err := service.ConvertRequestByID(c, info, converter, request)
		if err != nil {
			return nil, err
		}
		return result.Value, nil
	case relayconvert.ConverterOpenAICompletionsToOpenAIChat:
		return a.convertOpenAICompatibleRequest(c, info, openAICompletionsRequestToChat(request))
	default:
		return nil, fmt.Errorf("converter %q does not support OpenAI chat completions requests", converter)
	}
}

func (a *Adaptor) ConvertClaudeRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.ClaudeRequest) (any, error) {
	converter, err := a.resolveForConversion(c, info)
	if err != nil {
		return nil, err
	}

	switch converter {
	case relayconvert.ConverterNone:
		return a.claudeAdaptor.ConvertClaudeRequest(c, info, request)
	case relayconvert.ConverterClaudeMessagesToOpenAIChat:
		result, err := service.ConvertRequestByID(c, info, converter, request)
		if err != nil {
			return nil, err
		}
		chatRequest, ok := result.Value.(*dto.GeneralOpenAIRequest)
		if !ok {
			return nil, fmt.Errorf("expected OpenAI chat completions request, got %T", result.Value)
		}
		return a.convertOpenAICompatibleRequest(c, info, chatRequest)
	case relayconvert.ConverterClaudeMessagesToOpenAIResponses:
		result, err := service.ConvertRequestByID(c, info, converter, request)
		if err != nil {
			return nil, err
		}
		responsesRequest, ok := result.Value.(*dto.OpenAIResponsesRequest)
		if !ok {
			return nil, fmt.Errorf("expected OpenAI responses request, got %T", result.Value)
		}
		return responsesRequest, nil
	default:
		return nil, fmt.Errorf("converter %q does not support Anthropic Messages requests", converter)
	}
}

func (a *Adaptor) ConvertGeminiRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeminiChatRequest) (any, error) {
	converter, err := a.resolveForConversion(c, info)
	if err != nil {
		return nil, err
	}

	switch converter {
	case relayconvert.ConverterNone:
		return a.geminiAdaptor.ConvertGeminiRequest(c, info, request)
	case relayconvert.ConverterGeminiContentToClaudeMessages:
		result, err := service.ConvertRequestByID(c, info, converter, request)
		if err != nil {
			return nil, err
		}
		claudeRequest, ok := result.Value.(*dto.ClaudeRequest)
		if !ok {
			return nil, fmt.Errorf("expected Anthropic Messages request, got %T", result.Value)
		}
		return claudeRequest, nil
	case relayconvert.ConverterGeminiContentToOpenAIChat:
		result, err := service.ConvertRequestByID(c, info, converter, request)
		if err != nil {
			return nil, err
		}
		chatRequest, ok := result.Value.(*dto.GeneralOpenAIRequest)
		if !ok {
			return nil, fmt.Errorf("expected OpenAI chat completions request, got %T", result.Value)
		}
		return a.convertOpenAICompatibleRequest(c, info, chatRequest)
	default:
		return nil, fmt.Errorf("converter %q does not support Gemini generateContent requests", converter)
	}
}

func (a *Adaptor) ConvertOpenAIResponsesRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.OpenAIResponsesRequest) (any, error) {
	converter, err := a.resolveForConversion(c, info)
	if err != nil {
		return nil, err
	}
	switch converter {
	case relayconvert.ConverterNone:
		return a.convertOpenAICompatibleResponsesRequest(c, info, request)
	case relayconvert.ConverterOpenAIResponsesToClaudeMessages:
		result, err := service.ConvertRequestByID(c, info, converter, request)
		if err != nil {
			return nil, err
		}
		claudeRequest, ok := result.Value.(*dto.ClaudeRequest)
		if !ok {
			return nil, fmt.Errorf("expected Anthropic Messages request, got %T", result.Value)
		}
		return claudeRequest, nil
	case relayconvert.ConverterOpenAIResponsesToOpenAIChat:
		result, err := service.ConvertRequestByID(c, info, converter, request)
		if err != nil {
			return nil, err
		}
		chatRequest, ok := result.Value.(*dto.GeneralOpenAIRequest)
		if !ok {
			return nil, fmt.Errorf("expected OpenAI chat completions request, got %T", result.Value)
		}
		return a.convertOpenAICompatibleRequest(c, info, chatRequest)
	case relayconvert.ConverterOpenAIResponsesToGemini:
		result, err := service.ConvertRequestByID(c, info, converter, request)
		if err != nil {
			return nil, err
		}
		geminiRequest, ok := result.Value.(*dto.GeminiChatRequest)
		if !ok {
			return nil, fmt.Errorf("expected Gemini generateContent request, got %T", result.Value)
		}
		return geminiRequest, nil
	default:
		return nil, fmt.Errorf("converter %q does not support OpenAI Responses requests", converter)
	}
}

func (a *Adaptor) ConvertEmbeddingRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.EmbeddingRequest) (any, error) {
	converter, err := a.resolveForConversion(c, info)
	if err != nil {
		return nil, err
	}
	if converter != relayconvert.ConverterNone {
		return nil, fmt.Errorf("converter %q does not support embedding requests", converter)
	}
	return a.convertOpenAICompatibleEmbeddingRequest(c, info, request)
}

func (a *Adaptor) ConvertAudioRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.AudioRequest) (io.Reader, error) {
	converter, err := a.resolveForConversion(c, info)
	if err != nil {
		return nil, err
	}
	if converter != relayconvert.ConverterNone {
		return nil, fmt.Errorf("converter %q does not support audio requests", converter)
	}
	return a.convertOpenAICompatibleAudioRequest(c, info, request)
}

func (a *Adaptor) ConvertImageRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.ImageRequest) (any, error) {
	converter, err := a.resolveForConversion(c, info)
	if err != nil {
		return nil, err
	}
	if converter != relayconvert.ConverterNone {
		return nil, fmt.Errorf("converter %q does not support image requests", converter)
	}
	return a.convertOpenAICompatibleImageRequest(c, info, request)
}

func (a *Adaptor) ConvertRerankRequest(c *gin.Context, relayMode int, request dto.RerankRequest) (any, error) {
	a.converted = true
	return a.openaiAdaptor.ConvertRerankRequest(c, relayMode, request)
}

func (a *Adaptor) GetRequestURL(info *relaycommon.RelayInfo) (string, error) {
	if err := a.resolve(nil, info); err != nil {
		return "", err
	}
	return a.routeURL(info)
}

func (a *Adaptor) BuildModelListRequest(info *relaycommon.RelayInfo) (string, http.Header, error) {
	return a.buildManagementRequest(info, dto.AdvancedCustomModelListPath)
}

func (a *Adaptor) BuildBalanceRequest(info *relaycommon.RelayInfo) (string, http.Header, error) {
	return a.buildManagementRequest(info, dto.AdvancedCustomBalancePath)
}

func (a *Adaptor) buildManagementRequest(info *relaycommon.RelayInfo, managementPath string) (string, http.Header, error) {
	if info == nil {
		return "", nil, errors.New("missing relay info")
	}
	config := info.ChannelOtherSettings.AdvancedCustom
	if config == nil {
		return "", nil, errors.New("advanced_custom is required")
	}
	if err := config.Validate(); err != nil {
		return "", nil, err
	}
	var route dto.AdvancedCustomRoute
	var ok bool
	switch managementPath {
	case dto.AdvancedCustomModelListPath:
		route, ok = config.ModelListRoute()
	case dto.AdvancedCustomBalancePath:
		route, ok = config.BalanceRoute()
	default:
		return "", nil, fmt.Errorf("unsupported advanced custom management path: %s", managementPath)
	}
	if !ok {
		return "", nil, fmt.Errorf("advanced custom channel does not configure a %s route", managementPath)
	}
	converter := strings.TrimSpace(route.Converter)
	if converter == "" {
		converter = relayconvert.ConverterNone
	}
	if converter != relayconvert.ConverterNone {
		return "", nil, fmt.Errorf("converter %q does not support %s requests", converter, managementPath)
	}

	requestURL, err := buildRouteURL(route, converter, info)
	if err != nil {
		return "", nil, err
	}

	header := http.Header{}
	auth := route.Auth
	if auth == nil {
		header.Set("Authorization", "Bearer "+info.ApiKey)
		return requestURL, header, nil
	}

	switch strings.TrimSpace(auth.Type) {
	case dto.AdvancedCustomAuthTypeNone, dto.AdvancedCustomAuthTypeQuery:
	case dto.AdvancedCustomAuthTypeHeader:
		header.Set(strings.TrimSpace(auth.Name), applyAuthTemplate(auth.Value, info.ApiKey))
	default:
		return "", nil, fmt.Errorf("invalid advanced custom auth type: %s", auth.Type)
	}
	return requestURL, header, nil
}

func (a *Adaptor) SetupRequestHeader(c *gin.Context, header *http.Header, info *relaycommon.RelayInfo) error {
	if err := a.resolve(c, info); err != nil {
		return err
	}

	channel.SetupApiRequestHeader(info, c, header)
	auth := a.route.Auth
	if auth == nil {
		header.Set("Authorization", "Bearer "+info.ApiKey)
	} else {
		switch strings.TrimSpace(auth.Type) {
		case dto.AdvancedCustomAuthTypeNone:
		case dto.AdvancedCustomAuthTypeHeader:
			header.Set(strings.TrimSpace(auth.Name), applyAuthTemplate(auth.Value, info.ApiKey))
		case dto.AdvancedCustomAuthTypeQuery:
		default:
			return fmt.Errorf("invalid advanced custom auth type: %s", auth.Type)
		}
	}

	if shouldApplyClaudeHeaders(a.converter, info) {
		applyClaudeHeaders(c, header, info)
	}

	return nil
}

func (a *Adaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (any, error) {
	if err := a.resolve(c, info); err != nil {
		return nil, err
	}
	if !a.converted && a.converter != relayconvert.ConverterNone {
		return nil, errors.New("advanced custom converter routes cannot be used with pass-through request body")
	}

	if info.RelayMode == relayconstant.RelayModeAudioTranscription ||
		info.RelayMode == relayconstant.RelayModeAudioTranslation ||
		(info.RelayMode == relayconstant.RelayModeImagesEdits && !isJSONRequest(c)) {
		return channel.DoFormRequest(a, c, info, requestBody)
	}
	if info.RelayMode == relayconstant.RelayModeRealtime {
		return channel.DoWssRequest(a, c, info, requestBody)
	}
	return channel.DoApiRequest(a, c, info, requestBody)
}

func (a *Adaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (usage any, err *types.NewAPIError) {
	if err := a.resolve(c, info); err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeInvalidRequest, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
	}

	switch a.converter {
	case relayconvert.ConverterNone:
		return a.doNativeResponse(c, resp, info)
	case relayconvert.ConverterClaudeMessagesToOpenAIChat,
		relayconvert.ConverterGeminiContentToOpenAIChat:
		return a.openaiAdaptor.DoResponse(c, resp, info)
	case relayconvert.ConverterOpenAICompletionsToOpenAIChat:
		return a.doOpenAICompletionsConvertedResponse(c, resp, info)
	case relayconvert.ConverterOpenAIChatToClaudeMessages:
		return a.claudeAdaptor.DoResponse(c, resp, info)
	case relayconvert.ConverterGeminiContentToClaudeMessages,
		relayconvert.ConverterOpenAIResponsesToClaudeMessages:
		return a.doClaudeConvertedResponse(c, resp, info)
	case relayconvert.ConverterOpenAIChatToGeminiContent:
		return a.geminiAdaptor.DoResponse(c, resp, info)
	case relayconvert.ConverterOpenAIResponsesToGemini:
		return a.geminiAdaptor.DoResponse(c, resp, info)
	case relayconvert.ConverterOpenAIChatToOpenAIResponses,
		relayconvert.ConverterClaudeMessagesToOpenAIResponses:
		if info.IsStream {
			return openai.OaiResponsesToChatStreamHandler(c, info, resp)
		}
		return openai.OaiResponsesToChatHandler(c, info, resp)
	case relayconvert.ConverterOpenAIResponsesToOpenAIChat:
		if info.IsStream {
			return openai.OaiChatToResponsesStreamHandler(c, info, resp)
		}
		return openai.OaiChatToResponsesHandler(c, info, resp)
	default:
		return nil, types.NewOpenAIError(fmt.Errorf("unsupported advanced custom converter: %s", a.converter), types.ErrorCodeInvalidRequest, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
	}
}

type openAICompletionsResponse struct {
	Id      string                    `json:"id"`
	Object  string                    `json:"object"`
	Created any                       `json:"created"`
	Model   string                    `json:"model"`
	Choices []openAICompletionsChoice `json:"choices"`
	Usage   dto.Usage                 `json:"usage"`
}

type openAICompletionsChoice struct {
	Text         string `json:"text"`
	Index        int    `json:"index"`
	Logprobs     any    `json:"logprobs"`
	FinishReason any    `json:"finish_reason"`
}

type openAICompletionsStreamResponse struct {
	Id      string                    `json:"id"`
	Object  string                    `json:"object"`
	Created int64                     `json:"created"`
	Model   string                    `json:"model"`
	Choices []openAICompletionsChoice `json:"choices"`
	Usage   *dto.Usage                `json:"usage,omitempty"`
}

func (a *Adaptor) doOpenAICompletionsConvertedResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (*dto.Usage, *types.NewAPIError) {
	if info.IsStream {
		return a.doOpenAICompletionsConvertedStreamResponse(c, resp, info)
	}
	if resp == nil || resp.Body == nil {
		return nil, types.NewOpenAIError(fmt.Errorf("invalid response"), types.ErrorCodeBadResponse, http.StatusInternalServerError)
	}
	defer service.CloseResponseBodyGracefully(resp)

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeReadResponseBodyFailed, http.StatusInternalServerError)
	}

	var chatResponse dto.OpenAITextResponse
	if err := common.Unmarshal(responseBody, &chatResponse); err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}
	if oaiError := chatResponse.GetOpenAIError(); oaiError != nil && oaiError.Type != "" {
		return nil, types.WithOpenAIError(*oaiError, resp.StatusCode)
	}

	completionResponse := openAICompletionsResponse{
		Id:      chatResponse.Id,
		Object:  "text_completion",
		Created: chatResponse.Created,
		Model:   chatResponse.Model,
		Choices: make([]openAICompletionsChoice, 0, len(chatResponse.Choices)),
		Usage:   chatResponse.Usage,
	}
	completionText := strings.Builder{}
	for _, choice := range chatResponse.Choices {
		text := choice.Message.StringContent()
		completionText.WriteString(text)
		completionResponse.Choices = append(completionResponse.Choices, openAICompletionsChoice{
			Text:         text,
			Index:        choice.Index,
			Logprobs:     nil,
			FinishReason: choice.FinishReason,
		})
	}
	if completionResponse.Usage.PromptTokens == 0 {
		completionTokens := completionResponse.Usage.CompletionTokens
		if completionTokens == 0 {
			completionTokens = service.CountTextToken(completionText.String(), info.UpstreamModelName)
		}
		completionResponse.Usage = dto.Usage{
			PromptTokens:     info.GetEstimatePromptTokens(),
			CompletionTokens: completionTokens,
			TotalTokens:      info.GetEstimatePromptTokens() + completionTokens,
		}
	}

	responseBody, err = common.Marshal(completionResponse)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeJsonMarshalFailed, http.StatusInternalServerError)
	}
	service.IOCopyBytesGracefully(c, resp, responseBody)
	return &completionResponse.Usage, nil
}

func (a *Adaptor) doOpenAICompletionsConvertedStreamResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (*dto.Usage, *types.NewAPIError) {
	if resp == nil || resp.Body == nil {
		return nil, types.NewOpenAIError(fmt.Errorf("invalid response"), types.ErrorCodeBadResponse, http.StatusInternalServerError)
	}
	defer service.CloseResponseBodyGracefully(resp)

	usage := &dto.Usage{}
	containStreamUsage := false
	responseTextBuilder := strings.Builder{}
	var streamErr *types.NewAPIError

	helper.StreamScannerHandler(c, resp, info, func(data string, sr *helper.StreamResult) {
		if streamErr != nil {
			sr.Stop(streamErr)
			return
		}

		var chatChunk dto.ChatCompletionsStreamResponse
		if err := common.UnmarshalJsonStr(data, &chatChunk); err != nil {
			streamErr = types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
			sr.Stop(streamErr)
			return
		}
		if service.ValidUsage(chatChunk.Usage) {
			usage = chatChunk.Usage
			containStreamUsage = true
		}

		completionChunk := openAICompletionsStreamResponse{
			Id:      chatChunk.Id,
			Object:  "text_completion",
			Created: chatChunk.Created,
			Model:   chatChunk.Model,
			Choices: make([]openAICompletionsChoice, 0, len(chatChunk.Choices)),
			Usage:   chatChunk.Usage,
		}
		for _, choice := range chatChunk.Choices {
			text := choice.Delta.GetContentString()
			responseTextBuilder.WriteString(text)
			completionChunk.Choices = append(completionChunk.Choices, openAICompletionsChoice{
				Text:         text,
				Index:        choice.Index,
				Logprobs:     nil,
				FinishReason: choice.FinishReason,
			})
		}
		if len(completionChunk.Choices) > 0 || completionChunk.Usage != nil {
			if err := helper.ObjectData(c, completionChunk); err != nil {
				streamErr = types.NewOpenAIError(err, types.ErrorCodeBadResponse, http.StatusInternalServerError)
				sr.Stop(streamErr)
			}
		}
	})
	if streamErr != nil {
		return nil, streamErr
	}
	if !containStreamUsage {
		usage = service.ResponseText2Usage(c, responseTextBuilder.String(), info.UpstreamModelName, info.GetEstimatePromptTokens())
	}
	if info.ShouldIncludeUsage && !containStreamUsage {
		_ = helper.ObjectData(c, openAICompletionsStreamResponse{
			Id:      helper.GetResponseID(c),
			Object:  "text_completion",
			Created: common.GetTimestamp(),
			Model:   info.UpstreamModelName,
			Choices: []openAICompletionsChoice{},
			Usage:   usage,
		})
	}
	helper.Done(c)
	return usage, nil
}

func (a *Adaptor) doClaudeConvertedResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (any, *types.NewAPIError) {
	if info.IsStream {
		return a.doClaudeConvertedStreamResponse(c, resp, info)
	}
	return a.doClaudeConvertedJSONResponse(c, resp, info)
}

func (a *Adaptor) doClaudeConvertedJSONResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (*dto.Usage, *types.NewAPIError) {
	if resp == nil || resp.Body == nil {
		return nil, types.NewOpenAIError(fmt.Errorf("invalid response"), types.ErrorCodeBadResponse, http.StatusInternalServerError)
	}
	defer service.CloseResponseBodyGracefully(resp)

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeReadResponseBodyFailed, http.StatusInternalServerError)
	}
	var claudeResponse dto.ClaudeResponse
	if err := common.Unmarshal(responseBody, &claudeResponse); err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}
	if claudeErr := claudeResponse.GetClaudeError(); claudeErr != nil && claudeErr.Type != "" {
		return nil, types.WithClaudeError(*claudeErr, resp.StatusCode)
	}

	result, err := relayconvert.ConvertResponse(c, info, info.RelayFormat, &claudeResponse)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}
	responseBody, err = common.Marshal(result.Value)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeJsonMarshalFailed, http.StatusInternalServerError)
	}
	service.IOCopyBytesGracefully(c, resp, responseBody)
	return result.Usage, nil
}

func (a *Adaptor) doClaudeConvertedStreamResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (*dto.Usage, *types.NewAPIError) {
	if resp == nil || resp.Body == nil {
		return nil, types.NewOpenAIError(fmt.Errorf("invalid response"), types.ErrorCodeBadResponse, http.StatusInternalServerError)
	}
	defer service.CloseResponseBodyGracefully(resp)

	responseID := ""
	if c != nil {
		responseID = helper.GetResponseID(c)
	}
	created := common.GetTimestamp()
	claudeInfo := &relayconvert.ClaudeResponseInfo{
		ResponseId:   responseID,
		Created:      created,
		Model:        info.UpstreamModelName,
		ResponseText: strings.Builder{},
		Usage:        &dto.Usage{},
	}
	state, err := relayconvert.NewResponseStreamState(types.RelayFormatClaude, info.RelayFormat, relayconvert.ResponseStreamOptions{
		ID:           responseID,
		Model:        info.UpstreamModelName,
		Created:      created,
		IncludeUsage: info.ShouldIncludeUsage,
	})
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponse, http.StatusInternalServerError)
	}

	var streamErr *types.NewAPIError
	helper.StreamScannerHandler(c, resp, info, func(data string, sr *helper.StreamResult) {
		if streamErr != nil {
			sr.Stop(streamErr)
			return
		}
		var claudeResponse dto.ClaudeResponse
		if err := common.UnmarshalJsonStr(data, &claudeResponse); err != nil {
			sr.Error(err)
			return
		}
		if claudeErr := claudeResponse.GetClaudeError(); claudeErr != nil && claudeErr.Type != "" {
			streamErr = types.WithClaudeError(*claudeErr, resp.StatusCode)
			sr.Stop(streamErr)
			return
		}
		if claudeResponse.Type == "message_delta" {
			claudeResponse.Usage = relayconvert.BuildMessageDeltaPatchUsage(&claudeResponse, claudeInfo)
		}
		chatResponse := relayconvert.StreamResponseClaude2OpenAI(&claudeResponse)
		if !relayconvert.FormatClaudeResponseInfo(&claudeResponse, chatResponse, claudeInfo) {
			return
		}

		results, err := relayconvert.ConvertStreamResponseChunk(c, info, state, &claudeResponse)
		if err != nil {
			streamErr = types.NewOpenAIError(err, types.ErrorCodeBadResponse, http.StatusInternalServerError)
			sr.Stop(streamErr)
			return
		}
		for _, result := range results {
			if err := sendAdvancedCustomConvertedStreamResult(c, result); err != nil {
				streamErr = types.NewOpenAIError(err, types.ErrorCodeBadResponse, http.StatusInternalServerError)
				sr.Stop(streamErr)
				return
			}
		}
	})
	if streamErr != nil {
		return nil, streamErr
	}

	usage := state.Usage()
	if usage == nil || usage.TotalTokens == 0 {
		usage = service.ResponseText2Usage(c, state.UsageText(), info.UpstreamModelName, info.GetEstimatePromptTokens())
		state.SetUsage(usage)
	}
	finalResults, err := relayconvert.FinalizeStreamResponse(c, info, state)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponse, http.StatusInternalServerError)
	}
	for _, result := range finalResults {
		if err := sendAdvancedCustomConvertedStreamResult(c, result); err != nil {
			return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponse, http.StatusInternalServerError)
		}
	}
	return usage, nil
}

func sendAdvancedCustomConvertedStreamResult(c *gin.Context, result relayconvert.ResponseResult) error {
	switch value := result.Value.(type) {
	case relayconvert.ChatToResponsesStreamEvent:
		data, err := common.Marshal(value.Payload)
		if err != nil {
			return err
		}
		return helper.ResponseChunkData(c, dto.ResponsesStreamResponse{Type: value.Type}, string(data))
	case dto.GeminiChatResponse:
		return sendAdvancedCustomGeminiStreamResponse(c, &value)
	case *dto.GeminiChatResponse:
		return sendAdvancedCustomGeminiStreamResponse(c, value)
	default:
		return fmt.Errorf("unsupported converted stream response type %T", result.Value)
	}
}

func sendAdvancedCustomGeminiStreamResponse(c *gin.Context, response *dto.GeminiChatResponse) error {
	if response == nil {
		return nil
	}
	data, err := common.Marshal(response)
	if err != nil {
		return err
	}
	c.Render(-1, common.CustomEvent{Data: "data: " + string(data)})
	return helper.FlushWriter(c)
}

func (a *Adaptor) GetModelList() []string {
	models := make([]string, 0, len(openai.ModelList)+len(claude.ModelList)+len(gemini.ModelList))
	models = append(models, openai.ModelList...)
	models = append(models, claude.ModelList...)
	models = append(models, gemini.ModelList...)
	return lo.Uniq(models)
}

func (a *Adaptor) GetChannelName() string {
	return ChannelName
}

func (a *Adaptor) doNativeResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (any, *types.NewAPIError) {
	switch info.RelayFormat {
	case types.RelayFormatClaude:
		return a.claudeAdaptor.DoResponse(c, resp, info)
	case types.RelayFormatGemini:
		return a.geminiAdaptor.DoResponse(c, resp, info)
	default:
		return a.openaiAdaptor.DoResponse(c, resp, info)
	}
}

func (a *Adaptor) resolveForConversion(c *gin.Context, info *relaycommon.RelayInfo) (string, error) {
	if err := a.resolve(c, info); err != nil {
		return "", err
	}
	a.converted = true
	return a.converter, nil
}

func (a *Adaptor) resolve(c *gin.Context, info *relaycommon.RelayInfo) error {
	if a.resolved {
		return nil
	}
	if info == nil {
		return errors.New("missing relay info")
	}
	config := info.ChannelOtherSettings.AdvancedCustom
	if config == nil {
		return errors.New("advanced_custom is required")
	}
	if err := config.Validate(); err != nil {
		return err
	}

	incomingPath := incomingRequestPath(c, info)
	route, ok := config.MatchPathForModel(incomingPath, info.OriginModelName)
	if ok {
		route.Converter = strings.TrimSpace(route.Converter)
		if route.Converter == "" {
			route.Converter = relayconvert.ConverterNone
		}
		a.route = route
		a.converter = route.Converter
		a.resolved = true
		return nil
	}
	return fmt.Errorf("advanced custom channel does not support request path %s for model %s", incomingPath, info.OriginModelName)
}

func incomingRequestPath(c *gin.Context, info *relaycommon.RelayInfo) string {
	if info != nil && info.RequestURLPath != "" {
		return strings.Split(info.RequestURLPath, "?")[0]
	}
	if c != nil && c.Request != nil && c.Request.URL != nil {
		return c.Request.URL.Path
	}
	return ""
}

func (a *Adaptor) routeURL(info *relaycommon.RelayInfo) (string, error) {
	return buildRouteURL(a.route, a.converter, info)
}

func buildRouteURL(route dto.AdvancedCustomRoute, converter string, info *relaycommon.RelayInfo) (string, error) {
	parsedURL, err := resolveUpstreamTargetURL(applyUpstreamPathTemplate(strings.TrimSpace(route.UpstreamPath), info), info)
	if err != nil {
		return "", err
	}
	if shouldUseGeminiStreamURL(converter, info) {
		useGeminiStreamGenerateContentURL(parsedURL)
	}
	if info != nil && info.RelayMode == relayconstant.RelayModeRealtime {
		switch parsedURL.Scheme {
		case "https":
			parsedURL.Scheme = "wss"
		case "http":
			parsedURL.Scheme = "ws"
		}
	}
	if route.Auth != nil && strings.TrimSpace(route.Auth.Type) == dto.AdvancedCustomAuthTypeQuery {
		query := parsedURL.Query()
		query.Set(strings.TrimSpace(route.Auth.Name), applyAuthTemplate(route.Auth.Value, info.ApiKey))
		parsedURL.RawQuery = query.Encode()
	}
	return parsedURL.String(), nil
}

func resolveUpstreamTargetURL(upstreamPath string, info *relaycommon.RelayInfo) (*url.URL, error) {
	if strings.HasPrefix(upstreamPath, "/") {
		if strings.HasPrefix(upstreamPath, "//") {
			return nil, errors.New("advanced custom upstream path must be a full URL or a path starting with /")
		}
		if info == nil || strings.TrimSpace(info.ChannelBaseUrl) == "" {
			return nil, errors.New("channel base URL is required when advanced custom upstream path is relative")
		}
		return joinBaseURLAndUpstreamPath(info.ChannelBaseUrl, upstreamPath)
	}

	parsedURL, err := url.Parse(upstreamPath)
	if err != nil {
		return nil, err
	}
	if parsedURL.Scheme == "" || parsedURL.Host == "" {
		return nil, errors.New("advanced custom upstream path must be a full URL or a path starting with /")
	}
	if !strings.EqualFold(parsedURL.Scheme, "http") && !strings.EqualFold(parsedURL.Scheme, "https") {
		return nil, errors.New("advanced custom upstream path must use http or https")
	}
	return parsedURL, nil
}

func joinBaseURLAndUpstreamPath(baseURL string, upstreamPath string) (*url.URL, error) {
	parsedBaseURL, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil {
		return nil, err
	}
	if parsedBaseURL.Scheme == "" || parsedBaseURL.Host == "" {
		return nil, errors.New("channel base URL must be a full URL when advanced custom upstream path is relative")
	}
	if !strings.EqualFold(parsedBaseURL.Scheme, "http") && !strings.EqualFold(parsedBaseURL.Scheme, "https") {
		return nil, errors.New("channel base URL must use http or https when advanced custom upstream path is relative")
	}

	parsedPath, err := url.Parse(upstreamPath)
	if err != nil {
		return nil, err
	}
	parsedBaseURL.Path = strings.TrimRight(parsedBaseURL.Path, "/") + "/" + strings.TrimLeft(parsedPath.Path, "/")
	parsedBaseURL.RawPath = ""
	parsedBaseURL.RawQuery = parsedPath.RawQuery
	parsedBaseURL.Fragment = parsedPath.Fragment
	return parsedBaseURL, nil
}

func applyUpstreamPathTemplate(upstreamPath string, info *relaycommon.RelayInfo) string {
	if info == nil {
		return upstreamPath
	}
	return strings.ReplaceAll(upstreamPath, advancedCustomModelPlaceholder, info.UpstreamModelName)
}

func shouldUseGeminiStreamURL(converter string, info *relaycommon.RelayInfo) bool {
	return info != nil &&
		info.IsStream &&
		(converter == relayconvert.ConverterOpenAIChatToGeminiContent ||
			converter == relayconvert.ConverterOpenAIResponsesToGemini)
}

func useGeminiStreamGenerateContentURL(parsedURL *url.URL) {
	if strings.Contains(parsedURL.Path, ":generateContent") {
		parsedURL.Path = strings.Replace(parsedURL.Path, ":generateContent", ":streamGenerateContent", 1)
	}
	if strings.Contains(parsedURL.Path, ":streamGenerateContent") {
		query := parsedURL.Query()
		query.Set("alt", "sse")
		parsedURL.RawQuery = query.Encode()
	}
}

func shouldApplyClaudeHeaders(converter string, info *relaycommon.RelayInfo) bool {
	return converter == relayconvert.ConverterOpenAIChatToClaudeMessages ||
		converter == relayconvert.ConverterOpenAIResponsesToClaudeMessages ||
		converter == relayconvert.ConverterGeminiContentToClaudeMessages ||
		(converter == relayconvert.ConverterNone && info != nil && info.RelayFormat == types.RelayFormatClaude)
}

func applyClaudeHeaders(c *gin.Context, header *http.Header, info *relaycommon.RelayInfo) {
	anthropicVersion := ""
	if c != nil && c.Request != nil {
		anthropicVersion = c.Request.Header.Get("anthropic-version")
	}
	if anthropicVersion == "" {
		anthropicVersion = "2023-06-01"
	}
	header.Set("anthropic-version", anthropicVersion)
	if c != nil {
		claude.CommonClaudeHeadersOperation(c, header, info)
	}
}

func applyAuthTemplate(template string, apiKey string) string {
	return strings.ReplaceAll(template, "{api_key}", apiKey)
}

func isJSONRequest(c *gin.Context) bool {
	if c == nil || c.Request == nil {
		return false
	}
	return strings.Contains(strings.ToLower(c.Request.Header.Get("Content-Type")), "application/json")
}

func (a *Adaptor) convertOpenAICompatibleRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeneralOpenAIRequest) (any, error) {
	old := info.ChannelType
	info.ChannelType = constant.ChannelTypeOpenAI
	converted, err := a.openaiAdaptor.ConvertOpenAIRequest(c, info, request)
	info.ChannelType = old
	return converted, err
}

func openAICompletionsRequestToChat(request *dto.GeneralOpenAIRequest) *dto.GeneralOpenAIRequest {
	converted := *request
	converted.Prompt = nil
	converted.Messages = []dto.Message{{
		Role:    "user",
		Content: openAICompletionsPromptText(request.Prompt),
	}}
	return &converted
}

func openAICompletionsPromptText(prompt any) string {
	switch value := prompt.(type) {
	case nil:
		return ""
	case string:
		return value
	case []string:
		return strings.Join(value, "\n")
	case []any:
		parts := make([]string, 0, len(value))
		for _, item := range value {
			parts = append(parts, fmt.Sprint(item))
		}
		return strings.Join(parts, "\n")
	default:
		return fmt.Sprint(value)
	}
}

func (a *Adaptor) convertOpenAICompatibleResponsesRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.OpenAIResponsesRequest) (any, error) {
	old := info.ChannelType
	info.ChannelType = constant.ChannelTypeOpenAI
	converted, err := a.openaiAdaptor.ConvertOpenAIResponsesRequest(c, info, request)
	info.ChannelType = old
	return converted, err
}

func (a *Adaptor) convertOpenAICompatibleEmbeddingRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.EmbeddingRequest) (any, error) {
	old := info.ChannelType
	info.ChannelType = constant.ChannelTypeOpenAI
	converted, err := a.openaiAdaptor.ConvertEmbeddingRequest(c, info, request)
	info.ChannelType = old
	return converted, err
}

func (a *Adaptor) convertOpenAICompatibleAudioRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.AudioRequest) (io.Reader, error) {
	old := info.ChannelType
	info.ChannelType = constant.ChannelTypeOpenAI
	converted, err := a.openaiAdaptor.ConvertAudioRequest(c, info, request)
	info.ChannelType = old
	return converted, err
}

func (a *Adaptor) convertOpenAICompatibleImageRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.ImageRequest) (any, error) {
	old := info.ChannelType
	info.ChannelType = constant.ChannelTypeOpenAI
	converted, err := a.openaiAdaptor.ConvertImageRequest(c, info, request)
	info.ChannelType = old
	return converted, err
}
