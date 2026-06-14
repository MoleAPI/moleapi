package relay

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/relay/channel"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

func chatCompletionsViaImageGeneration(c *gin.Context, info *relaycommon.RelayInfo, adaptor channel.Adaptor, request *dto.GeneralOpenAIRequest) (*dto.Usage, *types.NewAPIError) {
	imageReq, err := service.ChatCompletionsRequestToImageRequest(request)
	if err != nil {
		return nil, types.NewErrorWithStatusCode(err, types.ErrorCodeInvalidRequest, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
	}
	applyImageOptionsFromRequestBody(c, imageReq)
	updateImageCompatPromptEstimate(c, info, imageReq)
	if imageReq.Stream != nil && *imageReq.Stream {
		// Chat Completions is append-only text streaming, so partial images would
		// become permanent extra images in the final message. Stream only the final image.
		imageReq.PartialImages = []byte("0")
	}

	httpResp, newAPIError := doImageGenerationRequest(c, info, adaptor, imageReq)
	if newAPIError != nil {
		return nil, newAPIError
	}
	if isEventStreamImageResponse(httpResp) {
		return imageStreamToChatCompletions(c, info, httpResp, imageReq.Model, imageReq)
	}
	if imageReq.IsStream(c) {
		return imageResponseToChatCompletionsStream(c, info, httpResp, imageReq.Model, adaptor, imageReq)
	}
	return imageResponseToChatCompletions(c, info, httpResp, imageReq.Model, adaptor, imageReq)
}

func responsesViaImageGeneration(c *gin.Context, info *relaycommon.RelayInfo, adaptor channel.Adaptor, request *dto.OpenAIResponsesRequest) (*dto.Usage, *types.NewAPIError) {
	imageReq, err := service.ResponsesRequestToImageRequest(request)
	if err != nil {
		return nil, types.NewErrorWithStatusCode(err, types.ErrorCodeInvalidRequest, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
	}
	applyImageOptionsFromRequestBody(c, imageReq)
	service.ApplyImageGenerationToolOptions(request.Tools, imageReq)
	updateImageCompatPromptEstimate(c, info, imageReq)

	httpResp, newAPIError := doImageGenerationRequest(c, info, adaptor, imageReq)
	if newAPIError != nil {
		return nil, newAPIError
	}
	if isEventStreamImageResponse(httpResp) {
		return imageStreamToResponses(c, info, httpResp, imageReq.Model, imageReq)
	}
	if imageReq.IsStream(c) {
		return imageResponseToResponsesStream(c, info, httpResp, imageReq.Model, adaptor, imageReq)
	}
	return imageResponseToResponses(c, info, httpResp, imageReq.Model, adaptor, imageReq)
}

func doImageGenerationRequest(c *gin.Context, info *relaycommon.RelayInfo, adaptor channel.Adaptor, imageReq *dto.ImageRequest) (*http.Response, *types.NewAPIError) {
	if imageReq == nil {
		return nil, types.NewErrorWithStatusCode(fmt.Errorf("image request is nil"), types.ErrorCodeInvalidRequest, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
	}

	savedRelayMode := info.RelayMode
	savedRequestURLPath := info.RequestURLPath
	savedRequest := info.Request
	defer func() {
		info.RelayMode = savedRelayMode
		info.RequestURLPath = savedRequestURLPath
		info.Request = savedRequest
	}()

	info.RelayMode = relayconstant.RelayModeImagesGenerations
	info.RequestURLPath = "/v1/images/generations"
	info.Request = imageReq
	service.NormalizeGPTImage2GenerationImageInputs(imageReq)

	convertedRequest, err := adaptor.ConvertImageRequest(c, info, *imageReq)
	if err != nil {
		return nil, types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
	}
	relaycommon.AppendRequestConversionFromRequest(info, convertedRequest)

	var requestBody io.Reader
	switch v := convertedRequest.(type) {
	case io.Reader:
		requestBody = v
	default:
		jsonData, err := common.Marshal(convertedRequest)
		if err != nil {
			return nil, types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
		}
		jsonData, err = relaycommon.RemoveDisabledFields(jsonData, info.ChannelOtherSettings, info.ChannelSetting.PassThroughBodyEnabled)
		if err != nil {
			return nil, types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
		}
		if len(info.ParamOverride) > 0 {
			jsonData, err = relaycommon.ApplyParamOverrideWithRelayInfo(jsonData, info)
			if err != nil {
				return nil, newAPIErrorFromParamOverride(err)
			}
		}
		requestBody = bytes.NewBuffer(jsonData)
	}

	resp, err := adaptor.DoRequest(c, info, requestBody)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeDoRequestFailed, http.StatusInternalServerError)
	}
	if resp == nil {
		return nil, types.NewOpenAIError(nil, types.ErrorCodeBadResponse, http.StatusInternalServerError)
	}

	httpResp := resp.(*http.Response)
	info.IsStream = info.IsStream || imageReq.IsStream(c) || strings.HasPrefix(httpResp.Header.Get("Content-Type"), "text/event-stream")
	if httpResp.StatusCode != http.StatusOK {
		newAPIError := service.RelayErrorHandler(c.Request.Context(), httpResp, false)
		service.ResetStatusCode(newAPIError, c.GetString("status_code_mapping"))
		return nil, newAPIError
	}
	return httpResp, nil
}

func imageResponseToChatCompletions(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response, model string, adaptor channel.Adaptor, imageReq *dto.ImageRequest) (*dto.Usage, *types.NewAPIError) {
	defer service.CloseResponseBodyGracefully(resp)

	imageResp, body, newAPIError := readImageResponse(c, info, resp, adaptor, imageReq)
	if newAPIError != nil {
		return nil, newAPIError
	}

	chatResp, usage, err := service.ImageResponseToChatResponse(imageResp, model, helper.GetResponseID(c), info.GetEstimatePromptTokens())
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}
	body, err = common.Marshal(chatResp)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeJsonMarshalFailed, http.StatusInternalServerError)
	}
	service.IOCopyBytesGracefully(c, resp, body)
	return usage, nil
}

func imageResponseToResponses(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response, model string, adaptor channel.Adaptor, imageReq *dto.ImageRequest) (*dto.Usage, *types.NewAPIError) {
	defer service.CloseResponseBodyGracefully(resp)

	imageResp, body, newAPIError := readImageResponse(c, info, resp, adaptor, imageReq)
	if newAPIError != nil {
		return nil, newAPIError
	}

	response, usage, err := service.ImageResponseToResponsesResponse(imageResp, model, getResponsesCompatID(c), getImageCompatItemID(c), info.GetEstimatePromptTokens())
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}
	body, err = common.Marshal(response)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeJsonMarshalFailed, http.StatusInternalServerError)
	}
	service.IOCopyBytesGracefully(c, resp, body)
	return usage, nil
}

func imageResponseToChatCompletionsStream(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response, model string, adaptor channel.Adaptor, imageReq *dto.ImageRequest) (*dto.Usage, *types.NewAPIError) {
	defer service.CloseResponseBodyGracefully(resp)

	imageResp, _, newAPIError := readImageResponse(c, info, resp, adaptor, imageReq)
	if newAPIError != nil {
		return nil, newAPIError
	}
	images := service.ImageDataItems(imageResp)
	if len(images) == 0 {
		return nil, types.NewOpenAIError(fmt.Errorf("image response does not contain generated image data"), types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}

	helper.SetEventStreamHeaders(c)
	responseID := helper.GetResponseID(c)
	created := imageResp.Created
	if created == 0 {
		created = time.Now().Unix()
	}
	usage := service.ImageUsageToUsage(imageResp.Usage, info.GetEstimatePromptTokens())

	if err := helper.ObjectData(c, helper.GenerateStartEmptyResponse(responseID, created, model, nil)); err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponse, http.StatusInternalServerError)
	}
	content := strings.Join(service.ImageMarkdownContents(images, imageResp.OutputFormat), "\n\n")
	chunk := &dto.ChatCompletionsStreamResponse{
		Id:      responseID,
		Object:  "chat.completion.chunk",
		Created: created,
		Model:   model,
		Choices: []dto.ChatCompletionsStreamResponseChoice{
			{
				Index: 0,
				Delta: dto.ChatCompletionsStreamResponseChoiceDelta{
					Content: &content,
				},
			},
		},
	}
	if err := helper.ObjectData(c, chunk); err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponse, http.StatusInternalServerError)
	}
	if err := helper.ObjectData(c, helper.GenerateStopResponse(responseID, created, model, "stop")); err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponse, http.StatusInternalServerError)
	}
	if info.ShouldIncludeUsage {
		if err := helper.ObjectData(c, helper.GenerateFinalUsageResponse(responseID, created, model, *usage)); err != nil {
			return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponse, http.StatusInternalServerError)
		}
	}
	helper.Done(c)
	return usage, nil
}

func imageResponseToResponsesStream(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response, model string, adaptor channel.Adaptor, imageReq *dto.ImageRequest) (*dto.Usage, *types.NewAPIError) {
	defer service.CloseResponseBodyGracefully(resp)

	imageResp, _, newAPIError := readImageResponse(c, info, resp, adaptor, imageReq)
	if newAPIError != nil {
		return nil, newAPIError
	}

	responseID := getResponsesCompatID(c)
	itemID := getImageCompatItemID(c)
	response, usage, err := service.ImageResponseToResponsesResponse(imageResp, model, responseID, itemID, info.GetEstimatePromptTokens())
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}
	outputItems, ok := response["output"].([]map[string]any)
	if !ok || len(outputItems) == 0 {
		return nil, types.NewOpenAIError(fmt.Errorf("image response does not contain generated image data"), types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}

	helper.SetEventStreamHeaders(c)
	sendEvent := func(eventType string, payload map[string]any) *types.NewAPIError {
		payload["type"] = eventType
		data, err := common.Marshal(payload)
		if err != nil {
			return types.NewOpenAIError(err, types.ErrorCodeJsonMarshalFailed, http.StatusInternalServerError)
		}
		helper.ResponseChunkData(c, dto.ResponsesStreamResponse{Type: eventType}, string(data))
		return nil
	}
	if newAPIError := sendEvent("response.created", map[string]any{
		"response": map[string]any{
			"id":         responseID,
			"object":     "response",
			"created_at": response["created_at"],
			"status":     "in_progress",
			"model":      model,
			"output":     []any{},
		},
	}); newAPIError != nil {
		return nil, newAPIError
	}
	for index, outputItem := range outputItems {
		if newAPIError := sendEvent(dto.ResponsesOutputTypeItemAdded, map[string]any{
			"output_index": index,
			"item": map[string]any{
				"id":     outputItem["id"],
				"type":   outputItem["type"],
				"status": "in_progress",
			},
		}); newAPIError != nil {
			return nil, newAPIError
		}
		if newAPIError := sendEvent(dto.ResponsesOutputTypeItemDone, map[string]any{
			"output_index": index,
			"item":         outputItem,
		}); newAPIError != nil {
			return nil, newAPIError
		}
	}
	if newAPIError := sendEvent("response.completed", map[string]any{"response": response}); newAPIError != nil {
		return nil, newAPIError
	}
	return usage, nil
}

func imageStreamToChatCompletions(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response, model string, imageReq *dto.ImageRequest) (*dto.Usage, *types.NewAPIError) {
	if resp == nil || resp.Body == nil {
		return nil, types.NewOpenAIError(fmt.Errorf("invalid response"), types.ErrorCodeBadResponse, http.StatusInternalServerError)
	}

	responseID := helper.GetResponseID(c)
	created := time.Now().Unix()
	usage := &dto.Usage{}
	var streamErr *types.NewAPIError
	sentStart := false
	sentStop := false
	sentFinalImage := false
	completedImageCount := 0
	completedImageOutputTokens := 0

	sendStart := func() bool {
		if sentStart {
			return true
		}
		if err := helper.ObjectData(c, helper.GenerateStartEmptyResponse(responseID, created, model, nil)); err != nil {
			streamErr = types.NewOpenAIError(err, types.ErrorCodeBadResponse, http.StatusInternalServerError)
			return false
		}
		sentStart = true
		return true
	}
	sendContent := func(content string) bool {
		if content == "" {
			return true
		}
		if !sendStart() {
			return false
		}
		chunk := &dto.ChatCompletionsStreamResponse{
			Id:      responseID,
			Object:  "chat.completion.chunk",
			Created: created,
			Model:   model,
			Choices: []dto.ChatCompletionsStreamResponseChoice{
				{
					Index: 0,
					Delta: dto.ChatCompletionsStreamResponseChoiceDelta{
						Content: &content,
					},
				},
			},
		}
		if err := helper.ObjectData(c, chunk); err != nil {
			streamErr = types.NewOpenAIError(err, types.ErrorCodeBadResponse, http.StatusInternalServerError)
			return false
		}
		return true
	}
	sendStop := func() bool {
		if sentStop {
			return true
		}
		if !sendStart() {
			return false
		}
		if err := helper.ObjectData(c, helper.GenerateStopResponse(responseID, created, model, "stop")); err != nil {
			streamErr = types.NewOpenAIError(err, types.ErrorCodeBadResponse, http.StatusInternalServerError)
			return false
		}
		sentStop = true
		return true
	}

	helper.StreamScannerHandler(c, resp, info, func(data string, sr *helper.StreamResult) {
		if streamErr != nil {
			sr.Stop(streamErr)
			return
		}
		var event dto.ImageStreamEvent
		if err := common.UnmarshalJsonStr(data, &event); err != nil {
			streamErr = types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
			sr.Stop(streamErr)
			return
		}
		if event.Type != "image_generation.completed" {
			return
		}
		if strings.TrimSpace(event.B64Json) == "" {
			streamErr = types.NewOpenAIError(fmt.Errorf("image stream completed without image data"), types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
			sr.Stop(streamErr)
			return
		}
		completedImageCount++
		if tokens, ok := service.GPTImage2OutputTokensForImageData(imageReq, dto.ImageData{B64Json: event.B64Json}, event.Quality); ok {
			completedImageOutputTokens += tokens
		}
		outputFallback := completedImageOutputTokens
		if outputFallback == 0 {
			outputFallback = imageOutputTokenFallback(imageReq, completedImageCount)
		}
		usage = service.ImageUsageToUsageWithOutputFallback(event.Usage, info.GetEstimatePromptTokens(), outputFallback)
		content := service.ImageMarkdownContent(dto.ImageData{B64Json: event.B64Json}, event.OutputFormat)
		if !sendContent(content) {
			sr.Stop(streamErr)
			return
		}
		sentFinalImage = true
		if !sendStop() {
			sr.Stop(streamErr)
			return
		}
		sr.Done()
	})

	if streamErr != nil {
		return nil, streamErr
	}
	if !sentFinalImage {
		return nil, types.NewOpenAIError(fmt.Errorf("image stream ended without completed image data"), types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}
	service.ApplyImageResultCountPricingFromCount(info, completedImageCount)
	if !sentStop && !sendStop() {
		return nil, streamErr
	}
	if info.ShouldIncludeUsage {
		if err := helper.ObjectData(c, helper.GenerateFinalUsageResponse(responseID, created, model, *usage)); err != nil {
			return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponse, http.StatusInternalServerError)
		}
	}
	helper.Done(c)
	return usage, nil
}

func imageStreamToResponses(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response, model string, imageReq *dto.ImageRequest) (*dto.Usage, *types.NewAPIError) {
	if resp == nil || resp.Body == nil {
		return nil, types.NewOpenAIError(fmt.Errorf("invalid response"), types.ErrorCodeBadResponse, http.StatusInternalServerError)
	}

	responseID := getResponsesCompatID(c)
	itemID := getImageCompatItemID(c)
	created := time.Now().Unix()
	usage := &dto.Usage{}
	var streamErr *types.NewAPIError
	sentStart := false
	sentCompleted := false
	completedImageCount := 0
	completedImageOutputTokens := 0

	sendEvent := func(eventType string, payload map[string]any) bool {
		payload["type"] = eventType
		data, err := common.Marshal(payload)
		if err != nil {
			streamErr = types.NewOpenAIError(err, types.ErrorCodeJsonMarshalFailed, http.StatusInternalServerError)
			return false
		}
		helper.ResponseChunkData(c, dto.ResponsesStreamResponse{Type: eventType}, string(data))
		return true
	}
	sendStart := func() bool {
		if sentStart {
			return true
		}
		if !sendEvent("response.created", map[string]any{
			"response": map[string]any{
				"id":         responseID,
				"object":     "response",
				"created_at": created,
				"status":     "in_progress",
				"model":      model,
				"output":     []any{},
			},
		}) {
			return false
		}
		if !sendEvent(dto.ResponsesOutputTypeItemAdded, map[string]any{
			"output_index": 0,
			"item": map[string]any{
				"id":     itemID,
				"type":   dto.ResponsesOutputTypeImageGenerationCall,
				"status": "in_progress",
			},
		}) {
			return false
		}
		sentStart = true
		return true
	}

	helper.StreamScannerHandler(c, resp, info, func(data string, sr *helper.StreamResult) {
		if streamErr != nil {
			sr.Stop(streamErr)
			return
		}
		var event dto.ImageStreamEvent
		if err := common.UnmarshalJsonStr(data, &event); err != nil {
			streamErr = types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
			sr.Stop(streamErr)
			return
		}
		if !sendStart() {
			sr.Stop(streamErr)
			return
		}
		switch event.Type {
		case "image_generation.partial_image":
			payload := map[string]any{
				"output_index":      0,
				"item_id":           itemID,
				"partial_image_b64": event.B64Json,
			}
			if event.PartialImageIndex != nil {
				payload["partial_image_index"] = *event.PartialImageIndex
			}
			if event.Size != "" {
				payload["size"] = event.Size
			}
			if event.Quality != "" {
				payload["quality"] = event.Quality
			}
			if event.Background != "" {
				payload["background"] = event.Background
			}
			if event.OutputFormat != "" {
				payload["output_format"] = event.OutputFormat
			}
			if !sendEvent("response.image_generation_call.partial_image", payload) {
				sr.Stop(streamErr)
				return
			}
		case "image_generation.completed":
			if strings.TrimSpace(event.B64Json) == "" {
				streamErr = types.NewOpenAIError(fmt.Errorf("image stream completed without image data"), types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
				sr.Stop(streamErr)
				return
			}
			completedImageCount++
			if tokens, ok := service.GPTImage2OutputTokensForImageData(imageReq, dto.ImageData{B64Json: event.B64Json}, event.Quality); ok {
				completedImageOutputTokens += tokens
			}
			outputFallback := completedImageOutputTokens
			if outputFallback == 0 {
				outputFallback = imageOutputTokenFallback(imageReq, completedImageCount)
			}
			usage = service.ImageUsageToUsageWithOutputFallback(event.Usage, info.GetEstimatePromptTokens(), outputFallback)
			outputItem := service.ImageOutputItemFromStream(event, itemID, "completed")
			if !sendEvent(dto.ResponsesOutputTypeItemDone, map[string]any{
				"output_index": 0,
				"item":         outputItem,
			}) {
				sr.Stop(streamErr)
				return
			}
			if !sendEvent("response.completed", map[string]any{
				"response": map[string]any{
					"id":                  responseID,
					"object":              "response",
					"created_at":          created,
					"status":              "completed",
					"model":               model,
					"output":              []map[string]any{outputItem},
					"parallel_tool_calls": false,
					"usage":               service.ResponsesUsageMap(usage),
				},
			}) {
				sr.Stop(streamErr)
				return
			}
			sentCompleted = true
			sr.Done()
		}
	})

	if streamErr != nil {
		return nil, streamErr
	}
	if !sentCompleted {
		return nil, types.NewOpenAIError(fmt.Errorf("image stream ended without completed image data"), types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}
	service.ApplyImageResultCountPricingFromCount(info, completedImageCount)
	return usage, nil
}

func updateImageCompatPromptEstimate(c *gin.Context, info *relaycommon.RelayInfo, imageReq *dto.ImageRequest) {
	if c == nil || info == nil || imageReq == nil {
		return
	}
	tokens, err := service.EstimateRequestToken(c, imageReq.GetTokenCountMeta(), info)
	if err != nil {
		return
	}
	info.SetEstimatePromptTokens(tokens)
}

func readImageResponse(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response, adaptor channel.Adaptor, imageReq *dto.ImageRequest) (*dto.ImageResponse, []byte, *types.NewAPIError) {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, types.NewOpenAIError(err, types.ErrorCodeReadResponseBodyFailed, http.StatusInternalServerError)
	}
	if info != nil {
		submitBodies := [][]byte{body}
		if _, isAsync, err := service.ExtractAsyncImageTaskID(body); err != nil {
			return nil, nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
		} else if isAsync {
			additionalBodies, newAPIError := submitAdditionalAsyncImageTaskBodies(c, info, adaptor, imageReq)
			if newAPIError != nil {
				return nil, nil, newAPIError
			}
			submitBodies = append(submitBodies, additionalBodies...)
		}
		imageResp, _, ok, err := service.ResolveAsyncImageTaskResponses(c.Request.Context(), submitBodies, service.AsyncImageTaskPollOptions{
			BaseURL: info.ChannelBaseUrl,
			APIKey:  info.ApiKey,
			Proxy:   info.ChannelSetting.Proxy,
		})
		if ok {
			if err != nil {
				return nil, nil, asyncImageTaskNewAPIError(err)
			}
			service.ApplyImageUsageOutputTokenFallback(imageResp, imageReq, info.GetEstimatePromptTokens())
			service.ApplyImageResultCountPricing(info, imageResp)
			body, err := common.Marshal(imageResp)
			if err != nil {
				return nil, nil, types.NewOpenAIError(err, types.ErrorCodeJsonMarshalFailed, http.StatusInternalServerError)
			}
			return imageResp, body, nil
		}
	}
	var imageResp dto.ImageResponse
	if err := common.Unmarshal(body, &imageResp); err != nil {
		return nil, nil, imageBadResponseBodyError(body, err)
	}
	if info != nil {
		service.ApplyImageUsageOutputTokenFallback(&imageResp, imageReq, info.GetEstimatePromptTokens())
		body, err = common.Marshal(&imageResp)
		if err != nil {
			return nil, nil, types.NewOpenAIError(err, types.ErrorCodeJsonMarshalFailed, http.StatusInternalServerError)
		}
	}
	service.ApplyImageResultCountPricing(info, &imageResp)
	return &imageResp, body, nil
}

func imageBadResponseBodyError(body []byte, err error) *types.NewAPIError {
	if strings.HasPrefix(strings.TrimSpace(string(body)), "<") {
		return types.NewOpenAIError(fmt.Errorf("upstream returned non-JSON image response"), types.ErrorCodeBadResponseBody, http.StatusBadGateway)
	}
	return types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
}

func submitAdditionalAsyncImageTaskBodies(c *gin.Context, info *relaycommon.RelayInfo, adaptor channel.Adaptor, imageReq *dto.ImageRequest) ([][]byte, *types.NewAPIError) {
	if adaptor == nil || imageReq == nil {
		return nil, nil
	}
	additionalCount := service.RequestedImageCount(imageReq) - 1
	if additionalCount <= 0 {
		return nil, nil
	}

	bodies := make([][]byte, 0, additionalCount)
	for i := 0; i < additionalCount; i++ {
		httpResp, newAPIError := doImageGenerationRequest(c, info, adaptor, service.ImageRequestWithCount(imageReq, 1))
		if newAPIError != nil {
			return nil, newAPIError
		}
		body, err := io.ReadAll(httpResp.Body)
		service.CloseResponseBodyGracefully(httpResp)
		if err != nil {
			return nil, types.NewOpenAIError(err, types.ErrorCodeReadResponseBodyFailed, http.StatusInternalServerError)
		}
		bodies = append(bodies, body)
	}
	return bodies, nil
}

func imageOutputTokenFallback(imageReq *dto.ImageRequest, imageCount int) int {
	outputTokens, ok := service.GPTImage2OutputTokensForRequest(imageReq, imageCount)
	if !ok {
		return 0
	}
	return outputTokens
}

func isEventStreamImageResponse(resp *http.Response) bool {
	if resp == nil {
		return false
	}
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(resp.Header.Get("Content-Type"))), "text/event-stream")
}

func asyncImageTaskNewAPIError(err error) *types.NewAPIError {
	var taskErr *service.AsyncImageTaskError
	if errors.As(err, &taskErr) {
		statusCode := taskErr.StatusCode
		if statusCode == 0 {
			statusCode = http.StatusBadGateway
		}
		return types.WithOpenAIError(taskErr.OpenAIError, statusCode, types.ErrOptionWithSkipRetry())
	}
	return types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError, types.ErrOptionWithSkipRetry())
}

func applyImageOptionsFromRequestBody(c *gin.Context, imageReq *dto.ImageRequest) {
	storage, err := common.GetBodyStorage(c)
	if err != nil {
		return
	}
	body, err := storage.Bytes()
	if err != nil {
		return
	}
	service.ApplyImageOptionsFromRaw(body, imageReq)
}

func getResponsesCompatID(c *gin.Context) string {
	return fmt.Sprintf("resp_%s", c.GetString(common.RequestIdKey))
}

func getImageCompatItemID(c *gin.Context) string {
	return fmt.Sprintf("igc_%s", c.GetString(common.RequestIdKey))
}
