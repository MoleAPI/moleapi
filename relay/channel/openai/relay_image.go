package openai

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

func updateOpenAIImageCount(info *relaycommon.RelayInfo, count int64) {
	if info == nil || !info.PriceData.UsePrice || count <= 0 || count > int64(dto.MaxImageN) {
		return
	}
	info.PriceData.AddOtherRatio("n", float64(count))
}

func shouldWrapImageAsChatCompletion(c *gin.Context, info *relaycommon.RelayInfo) bool {
	return info != nil &&
		info.RelayFormat == types.RelayFormatOpenAI &&
		((c != nil && c.GetBool("chat_image_completion_bridge")) ||
			strings.HasPrefix(info.RequestURLPath, "/v1/chat/completions"))
}

func shouldWrapImageAsResponses(c *gin.Context, info *relaycommon.RelayInfo) bool {
	return c != nil &&
		info != nil &&
		info.RelayFormat == types.RelayFormatOpenAIResponses &&
		c.GetBool("responses_image_generation_bridge")
}

// OpenaiImageHandler handles non-streaming OpenAI image responses
// (generations/edits), returning the parsed usage for billing.
func OpenaiImageHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	defer service.CloseResponseBodyGracefully(resp)

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeReadResponseBodyFailed, http.StatusInternalServerError)
	}

	var usageResp dto.SimpleResponse
	err = common.Unmarshal(responseBody, &usageResp)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}

	if oaiError := usageResp.GetOpenAIError(); oaiError != nil && oaiError.Type != "" {
		return nil, types.WithOpenAIError(*oaiError, resp.StatusCode)
	}

	updateOpenAIImageCount(info, gjson.GetBytes(responseBody, "data.#").Int())
	normalizeOpenAIUsage(&usageResp.Usage)
	applyUsagePostProcessing(info, &usageResp.Usage, responseBody)

	if shouldWrapImageAsChatCompletion(c, info) {
		chatBody, err := buildImageChatCompletionResponse(c, info, responseBody, &usageResp.Usage)
		if err != nil {
			return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
		}
		service.IOCopyBytesGracefully(c, resp, chatBody)
		return &usageResp.Usage, nil
	}
	if shouldWrapImageAsResponses(c, info) {
		responsesBody, err := buildImageResponsesResponse(c, info, responseBody, &usageResp.Usage)
		if err != nil {
			return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
		}
		service.IOCopyBytesGracefully(c, resp, responsesBody)
		return &usageResp.Usage, nil
	}

	// 写入新的 response body
	service.IOCopyBytesGracefully(c, resp, responseBody)

	return &usageResp.Usage, nil
}

// normalizeOpenAIUsage maps the OpenAI Images usage shape (input_tokens /
// output_tokens / input_tokens_details) onto the canonical prompt/completion
// fields. It is used only on the OpenAI image relay paths (generations/edits,
// streaming and non-streaming): the image API never returns prompt_tokens /
// completion_tokens, so the overwrite (=) semantics here are equivalent to the
// previous additive (+=) behavior while avoiding any future double-counting if
// both field sets are ever populated. Do not reuse this on chat/embedding paths
// without revisiting the overwrite semantics.
func normalizeOpenAIUsage(usage *dto.Usage) {
	if usage == nil {
		return
	}
	if usage.InputTokens != 0 {
		usage.PromptTokens = usage.InputTokens
	}
	if usage.OutputTokens != 0 {
		usage.CompletionTokens = usage.OutputTokens
	}
	if usage.OutputTokens > 0 &&
		usage.CompletionTokenDetails.TextTokens == 0 &&
		usage.CompletionTokenDetails.AudioTokens == 0 &&
		usage.CompletionTokenDetails.ImageTokens == 0 &&
		usage.CompletionTokenDetails.ReasoningTokens == 0 {
		usage.CompletionTokenDetails.ImageTokens = usage.OutputTokens
	}
	if usage.InputTokensDetails != nil {
		usage.PromptTokensDetails.CachedTokens = usage.InputTokensDetails.CachedTokens
		usage.PromptTokensDetails.CachedCreationTokens = usage.InputTokensDetails.CachedCreationTokens
		usage.PromptTokensDetails.CacheWriteTokens = usage.InputTokensDetails.CacheWriteTokens
		usage.PromptTokensDetails.ImageTokens = usage.InputTokensDetails.ImageTokens
		usage.PromptTokensDetails.TextTokens = usage.InputTokensDetails.TextTokens
		usage.PromptTokensDetails.AudioTokens = usage.InputTokensDetails.AudioTokens
	}
	if usage.TotalTokens == 0 {
		usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
	}
}

func buildImageChatCompletionResponse(c *gin.Context, info *relaycommon.RelayInfo, responseBody []byte, usage *dto.Usage) ([]byte, error) {
	content := imageMarkdownContentFromResponse(responseBody)
	created := gjson.GetBytes(responseBody, "created").Int()
	if created == 0 {
		created = time.Now().Unix()
	}
	model := info.OriginModelName
	if model == "" {
		model = gjson.GetBytes(responseBody, "model").String()
	}
	finishReason := "stop"
	response := dto.OpenAITextResponse{
		Id:      helper.GetResponseID(c),
		Object:  "chat.completion",
		Created: created,
		Model:   model,
		Choices: []dto.OpenAITextResponseChoice{
			{
				Index: 0,
				Message: dto.Message{
					Role:    "assistant",
					Content: content,
				},
				FinishReason: finishReason,
			},
		},
		Usage: *usage,
	}
	return common.Marshal(response)
}

func imageMarkdownContentFromResponse(responseBody []byte) string {
	imageCount := gjson.GetBytes(responseBody, "data.#").Int()
	parts := make([]string, 0, imageCount)
	for i := int64(0); i < imageCount; i++ {
		if markdown := imageMarkdownFromResult(gjson.GetBytes(responseBody, "data."+strconv.FormatInt(i, 10))); markdown != "" {
			parts = append(parts, markdown)
		}
	}
	return strings.Join(parts, "\n\n")
}

func imageMarkdownFromResult(image gjson.Result) string {
	if url := strings.TrimSpace(image.Get("url").String()); url != "" {
		return fmt.Sprintf("![generated image](%s)", url)
	}
	if b64 := strings.TrimSpace(image.Get("b64_json").String()); b64 != "" {
		return fmt.Sprintf("![generated image](data:image/png;base64,%s)", b64)
	}
	return ""
}

func buildImageResponsesResponse(c *gin.Context, info *relaycommon.RelayInfo, responseBody []byte, usage *dto.Usage) ([]byte, error) {
	created := gjson.GetBytes(responseBody, "created").Int()
	if created == 0 {
		created = time.Now().Unix()
	}
	response := map[string]any{
		"id":         imageResponsesID(c),
		"object":     "response",
		"created_at": created,
		"status":     "completed",
		"model":      imageChatCompletionModel(info, responseBody),
		"output":     imageResponsesOutputItems(c, responseBody),
		"usage":      usage,
	}
	return common.Marshal(response)
}

func imageResponsesID(c *gin.Context) string {
	return "resp_" + strings.TrimPrefix(helper.GetResponseID(c), "chatcmpl-")
}

func imageResponsesOutputItems(c *gin.Context, responseBody []byte) []map[string]any {
	imageCount := gjson.GetBytes(responseBody, "data.#").Int()
	items := make([]map[string]any, 0, imageCount)
	for i := int64(0); i < imageCount; i++ {
		items = append(items, imageResponsesOutputItem(c, i, gjson.GetBytes(responseBody, "data."+strconv.FormatInt(i, 10))))
	}
	return items
}

func imageResponsesOutputItem(c *gin.Context, index int64, image gjson.Result) map[string]any {
	item := map[string]any{
		"id":     imageResponsesOutputItemID(c, index),
		"type":   dto.ResponsesOutputTypeImageGenerationCall,
		"status": "completed",
	}
	if b64 := strings.TrimSpace(image.Get("b64_json").String()); b64 != "" {
		item["result"] = b64
	}
	if url := strings.TrimSpace(image.Get("url").String()); url != "" {
		item["url"] = url
		if _, ok := item["result"]; !ok {
			item["result"] = url
		}
	}
	if revisedPrompt := strings.TrimSpace(image.Get("revised_prompt").String()); revisedPrompt != "" {
		item["revised_prompt"] = revisedPrompt
	}
	return item
}

func imageResponsesOutputItemID(c *gin.Context, index int64) string {
	return fmt.Sprintf("ig_%s_%d", strings.TrimPrefix(helper.GetResponseID(c), "chatcmpl-"), index)
}

func OpenaiImageStreamHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	if resp == nil || resp.Body == nil {
		logger.LogError(c, "invalid image stream response")
		return nil, types.NewOpenAIError(fmt.Errorf("invalid response"), types.ErrorCodeBadResponse, http.StatusInternalServerError)
	}
	if shouldWrapImageAsChatCompletion(c, info) {
		return openaiImageChatStreamHandler(c, info, resp)
	}
	if shouldWrapImageAsResponses(c, info) {
		return openaiImageResponsesStreamHandler(c, info, resp)
	}

	contentType := strings.ToLower(resp.Header.Get("Content-Type"))
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return OpenaiImageHandler(c, info, resp)
	}
	if !strings.Contains(contentType, "text/event-stream") {
		return openaiImageJSONAsStreamHandler(c, info, resp)
	}
	// Reuse the shared streaming engine (helper.StreamScannerHandler) so the
	// image streaming path gets the same ping keepalive, streaming-timeout
	// watchdog, client-disconnect detection, panic recovery and goroutine
	// cleanup as every other relay stream. The scanner delivers only the
	// "data:" payload, so the SSE "event:" line is rebuilt from the JSON "type"
	// field (real OpenAI image events keep event == type).
	usage := &dto.Usage{}
	var lastStreamData []byte
	var completedImages int64

	helper.StreamScannerHandler(c, resp, info, func(data string, sr *helper.StreamResult) {
		raw := common.StringToByteSlice(data)
		lastStreamData = raw
		if isOpenAIImageStreamErrorEvent(raw) {
			// Record the error as a soft error; the scanner drives the final
			// EndReason. HasErrors() flags the failure for logging/handling.
			sr.Error(fmt.Errorf("%s", extractOpenAIImageStreamErrorMessage(raw)))
		}
		var chunk struct {
			Type  string    `json:"type"`
			Usage dto.Usage `json:"usage"`
		}
		if err := common.Unmarshal(raw, &chunk); err == nil {
			normalizeOpenAIUsage(&chunk.Usage)
			if service.ValidUsage(&chunk.Usage) {
				usage = &chunk.Usage
			}
			if chunk.Type == "image_generation.completed" || chunk.Type == "image_edit.completed" {
				completedImages++
			}
		}
		if err := writeOpenaiImageStreamChunk(c, raw); err != nil {
			sr.Stop(err)
		}
	})

	// StreamScannerHandler consumes the upstream [DONE]; re-emit it so the
	// client still receives a terminal data: [DONE].
	if info.StreamStatus != nil && info.StreamStatus.EndReason == relaycommon.StreamEndReasonDone {
		helper.Done(c)
	}

	applyUsagePostProcessing(info, usage, lastStreamData)
	// Only trust completedImages when upstream finished the stream (done/eof).
	// On client-side aborts (client_gone, or handler_stop from a failed client
	// write) the counter undercounts what upstream actually generated and
	// charged, so keep the requested n — otherwise a client could pay for one
	// image by disconnecting right after the first completed event. The abort
	// guard only blocks lowering the charge: if completed events already
	// exceed the recorded n, bill the higher actual count regardless.
	if info.StreamStatus != nil {
		upstreamFinished := info.StreamStatus.EndReason == relaycommon.StreamEndReasonDone ||
			info.StreamStatus.EndReason == relaycommon.StreamEndReasonEOF
		requestedN := 1.0
		if n, ok := info.PriceData.OtherRatios()["n"]; ok {
			requestedN = n
		}
		if upstreamFinished || float64(completedImages) > requestedN {
			updateOpenAIImageCount(info, completedImages)
		}
	}
	return usage, nil
}

func openaiImageChatStreamHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	contentType := strings.ToLower(resp.Header.Get("Content-Type"))
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return OpenaiImageHandler(c, info, resp)
	}
	if !strings.Contains(contentType, "text/event-stream") {
		return openaiImageJSONAsChatStreamHandler(c, info, resp)
	}

	usage := &dto.Usage{}
	var lastStreamData []byte
	var completedImages int64
	id := helper.GetResponseID(c)
	created := time.Now().Unix()
	model := imageChatCompletionModel(info, nil)

	helper.SetEventStreamHeaders(c)
	c.Status(http.StatusOK)
	if err := helper.ObjectData(c, helper.GenerateStartEmptyResponse(id, created, model, nil)); err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponse, http.StatusInternalServerError)
	}

	helper.StreamScannerHandler(c, resp, info, func(data string, sr *helper.StreamResult) {
		raw := common.StringToByteSlice(data)
		lastStreamData = raw
		if isOpenAIImageStreamErrorEvent(raw) {
			sr.Error(fmt.Errorf("%s", extractOpenAIImageStreamErrorMessage(raw)))
		}
		var chunk struct {
			Type  string    `json:"type"`
			Usage dto.Usage `json:"usage"`
		}
		if err := common.Unmarshal(raw, &chunk); err == nil {
			normalizeOpenAIUsage(&chunk.Usage)
			if service.ValidUsage(&chunk.Usage) {
				usage = &chunk.Usage
			}
			if chunk.Type == "image_generation.completed" || chunk.Type == "image_edit.completed" {
				markdown := imageMarkdownFromResult(gjson.ParseBytes(raw))
				if markdown != "" {
					if completedImages > 0 {
						markdown = "\n\n" + markdown
					}
					if err := writeChatImageContentChunk(c, id, created, model, markdown); err != nil {
						sr.Stop(err)
						return
					}
				}
				completedImages++
			}
		}
	})

	if info.StreamStatus != nil &&
		(info.StreamStatus.EndReason == relaycommon.StreamEndReasonDone ||
			info.StreamStatus.EndReason == relaycommon.StreamEndReasonEOF) {
		if err := helper.ObjectData(c, helper.GenerateStopResponse(id, created, model, "stop")); err != nil {
			return usage, nil
		}
		helper.Done(c)
	}

	applyUsagePostProcessing(info, usage, lastStreamData)
	if info.StreamStatus != nil {
		upstreamFinished := info.StreamStatus.EndReason == relaycommon.StreamEndReasonDone ||
			info.StreamStatus.EndReason == relaycommon.StreamEndReasonEOF
		requestedN := 1.0
		if n, ok := info.PriceData.OtherRatios()["n"]; ok {
			requestedN = n
		}
		if upstreamFinished || float64(completedImages) > requestedN {
			updateOpenAIImageCount(info, completedImages)
		}
	}
	return usage, nil
}

func openaiImageResponsesStreamHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	contentType := strings.ToLower(resp.Header.Get("Content-Type"))
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return OpenaiImageHandler(c, info, resp)
	}
	if !strings.Contains(contentType, "text/event-stream") {
		return openaiImageJSONAsResponsesStreamHandler(c, info, resp)
	}

	usage := &dto.Usage{}
	var lastStreamData []byte
	var completedImages int64
	items := make([]map[string]any, 0)
	id := imageResponsesID(c)
	created := time.Now().Unix()
	model := imageChatCompletionModel(info, nil)

	helper.SetEventStreamHeaders(c)
	c.Status(http.StatusOK)
	if err := writeResponsesImageEvent(c, "response.created", map[string]any{
		"type": "response.created",
		"response": map[string]any{
			"id":         id,
			"object":     "response",
			"created_at": created,
			"status":     "in_progress",
			"model":      model,
		},
	}); err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponse, http.StatusInternalServerError)
	}

	helper.StreamScannerHandler(c, resp, info, func(data string, sr *helper.StreamResult) {
		raw := common.StringToByteSlice(data)
		lastStreamData = raw
		if isOpenAIImageStreamErrorEvent(raw) {
			sr.Error(fmt.Errorf("%s", extractOpenAIImageStreamErrorMessage(raw)))
		}

		chunk := gjson.ParseBytes(raw)
		chunkType := chunk.Get("type").String()
		var chunkUsage dto.Usage
		if err := common.UnmarshalJsonStr(chunk.Get("usage").Raw, &chunkUsage); err == nil {
			normalizeOpenAIUsage(&chunkUsage)
			if service.ValidUsage(&chunkUsage) {
				usage = &chunkUsage
			}
		}

		switch chunkType {
		case "image_generation.partial_image":
			payload := map[string]any{
				"type":                "response.image_generation_call.partial_image",
				"item_id":             imageResponsesOutputItemID(c, completedImages),
				"output_index":        completedImages,
				"partial_image_index": chunk.Get("partial_image_index").Int(),
				"partial_image_b64":   chunk.Get("b64_json").String(),
			}
			if err := writeResponsesImageEvent(c, "response.image_generation_call.partial_image", payload); err != nil {
				sr.Stop(err)
			}
		case "image_generation.completed", "image_edit.completed":
			item := imageResponsesOutputItem(c, completedImages, chunk)
			items = append(items, item)
			if err := writeResponsesImageEvent(c, dto.ResponsesOutputTypeItemDone, map[string]any{
				"type":         dto.ResponsesOutputTypeItemDone,
				"output_index": completedImages,
				"item":         item,
			}); err != nil {
				sr.Stop(err)
				return
			}
			if err := writeResponsesImageEvent(c, "response.image_generation_call.completed", map[string]any{
				"type":         "response.image_generation_call.completed",
				"item_id":      item["id"],
				"output_index": completedImages,
			}); err != nil {
				sr.Stop(err)
				return
			}
			completedImages++
		}
	})

	applyUsagePostProcessing(info, usage, lastStreamData)
	if info.StreamStatus != nil &&
		(info.StreamStatus.EndReason == relaycommon.StreamEndReasonDone ||
			info.StreamStatus.EndReason == relaycommon.StreamEndReasonEOF) {
		if err := writeResponsesImageEvent(c, "response.completed", map[string]any{
			"type": "response.completed",
			"response": map[string]any{
				"id":         id,
				"object":     "response",
				"created_at": created,
				"status":     "completed",
				"model":      model,
				"output":     items,
				"usage":      usage,
			},
		}); err != nil {
			return usage, nil
		}
		helper.Done(c)
	}

	if info.StreamStatus != nil {
		upstreamFinished := info.StreamStatus.EndReason == relaycommon.StreamEndReasonDone ||
			info.StreamStatus.EndReason == relaycommon.StreamEndReasonEOF
		requestedN := 1.0
		if n, ok := info.PriceData.OtherRatios()["n"]; ok {
			requestedN = n
		}
		if upstreamFinished || float64(completedImages) > requestedN {
			updateOpenAIImageCount(info, completedImages)
		}
	}
	return usage, nil
}

func openaiImageJSONAsChatStreamHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	defer service.CloseResponseBodyGracefully(resp)

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeReadResponseBodyFailed, http.StatusInternalServerError)
	}

	var usageResp dto.SimpleResponse
	if err := common.Unmarshal(responseBody, &usageResp); err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}
	if oaiError := usageResp.GetOpenAIError(); oaiError != nil && oaiError.Type != "" {
		return nil, types.WithOpenAIError(*oaiError, resp.StatusCode)
	}
	normalizeOpenAIUsage(&usageResp.Usage)
	applyUsagePostProcessing(info, &usageResp.Usage, responseBody)

	imageCount := gjson.GetBytes(responseBody, "data.#").Int()
	updateOpenAIImageCount(info, imageCount)

	helper.SetEventStreamHeaders(c)
	c.Status(http.StatusOK)

	created := gjson.GetBytes(responseBody, "created").Int()
	if created == 0 {
		created = time.Now().Unix()
	}
	if info != nil {
		info.SetFirstResponseTime()
	}
	id := helper.GetResponseID(c)
	model := imageChatCompletionModel(info, responseBody)
	if err := helper.ObjectData(c, helper.GenerateStartEmptyResponse(id, created, model, nil)); err != nil {
		return &usageResp.Usage, nil
	}

	for i := int64(0); i < imageCount; i++ {
		markdown := imageMarkdownFromResult(gjson.GetBytes(responseBody, "data."+strconv.FormatInt(i, 10)))
		if markdown == "" {
			continue
		}
		if i > 0 {
			markdown = "\n\n" + markdown
		}
		if err := writeChatImageContentChunk(c, id, created, model, markdown); err != nil {
			if info != nil && info.StreamStatus != nil {
				info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonClientGone, err)
			}
			return &usageResp.Usage, nil
		}
	}
	if err := helper.ObjectData(c, helper.GenerateStopResponse(id, created, model, "stop")); err != nil {
		return &usageResp.Usage, nil
	}
	helper.Done(c)
	if info != nil {
		info.ReceivedResponseCount += int(imageCount)
		if info.StreamStatus == nil {
			info.StreamStatus = relaycommon.NewStreamStatus()
		}
		info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonDone, nil)
	}
	return &usageResp.Usage, nil
}

func openaiImageJSONAsResponsesStreamHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	defer service.CloseResponseBodyGracefully(resp)

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeReadResponseBodyFailed, http.StatusInternalServerError)
	}

	var usageResp dto.SimpleResponse
	if err := common.Unmarshal(responseBody, &usageResp); err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}
	if oaiError := usageResp.GetOpenAIError(); oaiError != nil && oaiError.Type != "" {
		return nil, types.WithOpenAIError(*oaiError, resp.StatusCode)
	}
	normalizeOpenAIUsage(&usageResp.Usage)
	applyUsagePostProcessing(info, &usageResp.Usage, responseBody)

	imageCount := gjson.GetBytes(responseBody, "data.#").Int()
	updateOpenAIImageCount(info, imageCount)

	helper.SetEventStreamHeaders(c)
	c.Status(http.StatusOK)

	created := gjson.GetBytes(responseBody, "created").Int()
	if created == 0 {
		created = time.Now().Unix()
	}
	if info != nil {
		info.SetFirstResponseTime()
	}
	id := imageResponsesID(c)
	model := imageChatCompletionModel(info, responseBody)
	if err := writeResponsesImageEvent(c, "response.created", map[string]any{
		"type": "response.created",
		"response": map[string]any{
			"id":         id,
			"object":     "response",
			"created_at": created,
			"status":     "in_progress",
			"model":      model,
		},
	}); err != nil {
		return &usageResp.Usage, nil
	}

	items := imageResponsesOutputItems(c, responseBody)
	for index, item := range items {
		if err := writeResponsesImageEvent(c, dto.ResponsesOutputTypeItemDone, map[string]any{
			"type":         dto.ResponsesOutputTypeItemDone,
			"output_index": index,
			"item":         item,
		}); err != nil {
			return &usageResp.Usage, nil
		}
		if err := writeResponsesImageEvent(c, "response.image_generation_call.completed", map[string]any{
			"type":         "response.image_generation_call.completed",
			"item_id":      item["id"],
			"output_index": index,
		}); err != nil {
			return &usageResp.Usage, nil
		}
	}
	if err := writeResponsesImageEvent(c, "response.completed", map[string]any{
		"type": "response.completed",
		"response": map[string]any{
			"id":         id,
			"object":     "response",
			"created_at": created,
			"status":     "completed",
			"model":      model,
			"output":     items,
			"usage":      &usageResp.Usage,
		},
	}); err != nil {
		return &usageResp.Usage, nil
	}
	helper.Done(c)
	if info != nil {
		info.ReceivedResponseCount += int(imageCount)
		if info.StreamStatus == nil {
			info.StreamStatus = relaycommon.NewStreamStatus()
		}
		info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonDone, nil)
	}
	return &usageResp.Usage, nil
}

func imageChatCompletionModel(info *relaycommon.RelayInfo, responseBody []byte) string {
	if info != nil && info.OriginModelName != "" {
		return info.OriginModelName
	}
	if len(responseBody) > 0 {
		return gjson.GetBytes(responseBody, "model").String()
	}
	return ""
}

func writeChatImageContentChunk(c *gin.Context, id string, created int64, model string, content string) error {
	return helper.ObjectData(c, dto.ChatCompletionsStreamResponse{
		Id:      id,
		Object:  "chat.completion.chunk",
		Created: created,
		Model:   model,
		Choices: []dto.ChatCompletionsStreamResponseChoice{
			{
				Index: 0,
				Delta: dto.ChatCompletionsStreamResponseChoiceDelta{
					Content: common.GetPointer(content),
				},
			},
		},
	})
}

func writeResponsesImageEvent(c *gin.Context, eventType string, payload map[string]any) error {
	data, err := common.Marshal(payload)
	if err != nil {
		return err
	}
	return helper.ResponseChunkData(c, dto.ResponsesStreamResponse{Type: eventType}, string(data))
}

// writeOpenaiImageStreamChunk rebuilds the SSE frame for an image stream chunk:
// it emits an "event:" line derived from the JSON "type" field (when present)
// followed by the verbatim "data:" payload, mirroring helper.ResponseChunkData.
func writeOpenaiImageStreamChunk(c *gin.Context, data []byte) error {
	var payload struct {
		Type string `json:"type"`
	}
	_ = common.Unmarshal(data, &payload)
	if eventName := strings.TrimSpace(payload.Type); eventName != "" {
		return helper.ResponseChunkData(c, dto.ResponsesStreamResponse{Type: eventName}, string(data))
	}
	return helper.StringData(c, string(data))
}

// isOpenAIImageStreamErrorEvent detects upstream error chunks by JSON content
// only ("type" of error/upstream_error, or a non-empty "error" field). The SSE
// "event:" line is not available here: StreamScannerHandler delivers only the
// "data:" payload. A payload carrying just a "message" key is deliberately NOT
// treated as an error to avoid false positives.
func isOpenAIImageStreamErrorEvent(data []byte) bool {
	if !json.Valid(data) {
		return false
	}
	var payload struct {
		Type  string          `json:"type"`
		Error json.RawMessage `json:"error"`
	}
	if err := common.Unmarshal(data, &payload); err != nil {
		return false
	}
	payloadType := strings.ToLower(strings.TrimSpace(payload.Type))
	return payloadType == "error" || payloadType == "upstream_error" || len(payload.Error) > 0
}

func extractOpenAIImageStreamErrorMessage(data []byte) string {
	if len(data) == 0 || !json.Valid(data) {
		return "upstream image stream returned error event"
	}
	var payload struct {
		Message string          `json:"message"`
		Error   json.RawMessage `json:"error"`
	}
	if err := common.Unmarshal(data, &payload); err != nil {
		return "upstream image stream returned error event"
	}
	if msg := strings.TrimSpace(payload.Message); msg != "" {
		return msg
	}
	if len(payload.Error) > 0 {
		var nested struct {
			Message string `json:"message"`
		}
		if err := common.Unmarshal(payload.Error, &nested); err == nil {
			if msg := strings.TrimSpace(nested.Message); msg != "" {
				return msg
			}
		}
		if msg := strings.TrimSpace(common.JsonRawMessageToString(payload.Error)); msg != "" {
			return msg
		}
	}
	return "upstream image stream returned error event"
}

func openaiImageJSONAsStreamHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	defer service.CloseResponseBodyGracefully(resp)

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeReadResponseBodyFailed, http.StatusInternalServerError)
	}

	// Only decode usage/error. Do not Unmarshal data[] into dto.ImageResponse —
	// b64_json values are large and would be copied into Go strings then
	// re-marshaled for each SSE event.
	var usageResp dto.SimpleResponse
	if err := common.Unmarshal(responseBody, &usageResp); err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}
	if oaiError := usageResp.GetOpenAIError(); oaiError != nil && oaiError.Type != "" {
		return nil, types.WithOpenAIError(*oaiError, resp.StatusCode)
	}
	normalizeOpenAIUsage(&usageResp.Usage)
	applyUsagePostProcessing(info, &usageResp.Usage, responseBody)

	imageCount := gjson.GetBytes(responseBody, "data.#").Int()
	updateOpenAIImageCount(info, imageCount)

	helper.SetEventStreamHeaders(c)
	c.Status(http.StatusOK)

	created := gjson.GetBytes(responseBody, "created").Int()
	if created == 0 {
		created = time.Now().Unix()
	}
	if info != nil {
		info.SetFirstResponseTime()
	}

	validUsage := service.ValidUsage(&usageResp.Usage)
	var usageJSON []byte
	if validUsage {
		usageJSON, err = common.Marshal(usageResp.Usage)
		if err != nil {
			return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
		}
	}

	for i := int64(0); i < imageCount; i++ {
		image := gjson.GetBytes(responseBody, "data."+strconv.FormatInt(i, 10))
		payload := []byte(`{"type":"image_generation.completed"}`)
		payload, err = sjson.SetBytes(payload, "created_at", created)
		if err != nil {
			return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
		}
		if validUsage {
			payload, err = sjson.SetRawBytes(payload, "usage", usageJSON)
			if err != nil {
				return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
			}
		}
		// b64_json goes last: every sjson.Set* reallocates the whole payload,
		// so inserting the large blob after all small fields avoids re-copying
		// multi-MB buffers.
		for _, field := range []string{"url", "revised_prompt", "b64_json"} {
			value := image.Get(field)
			if value.Type != gjson.String || value.Raw == `""` {
				continue
			}
			raw := []byte(value.Raw)
			if value.Index > 0 {
				raw = responseBody[value.Index : value.Index+len(value.Raw)]
			}
			payload, err = sjson.SetRawBytes(payload, field, raw)
			if err != nil {
				return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
			}
		}
		if writeErr := helper.ResponseChunkData(c, dto.ResponsesStreamResponse{Type: "image_generation.completed"}, string(payload)); writeErr != nil {
			if info != nil && info.StreamStatus != nil {
				info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonClientGone, writeErr)
			}
			return &usageResp.Usage, nil
		}
	}
	if err := writeOpenaiImageStreamDone(c); err != nil {
		if info != nil && info.StreamStatus != nil {
			info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonClientGone, err)
		}
		return &usageResp.Usage, nil
	}
	if info != nil {
		info.ReceivedResponseCount += int(imageCount)
		if info.StreamStatus == nil {
			info.StreamStatus = relaycommon.NewStreamStatus()
		}
		info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonDone, nil)
	}
	return &usageResp.Usage, nil
}

func writeOpenaiImageStreamDone(c *gin.Context) error {
	return helper.StringData(c, "[DONE]")
}
