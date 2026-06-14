package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"
)

const gptImage2ModelPrefix = "gpt-image-2"

type gptImage2Dimensions struct {
	Width  int
	Height int
}

var (
	imagePromptMarkdownDataURLPattern = regexp.MustCompile(`!\[[^\]\r\n]*\]\(\s*data:image/[A-Za-z0-9.+-]+;base64,[^)]+\)`)
	imagePromptDataURLPattern         = regexp.MustCompile(`data:image/[A-Za-z0-9.+-]+;base64,[-A-Za-z0-9+/_=]+`)

	gptImage2QualityPatchColumns = map[string]int{
		"low":    16,
		"medium": 48,
		"high":   96,
	}

	apimartGPTImage2Dimensions = map[string]map[string]gptImage2Dimensions{
		"1:1": {
			"1k": {Width: 1024, Height: 1024},
			"2k": {Width: 2048, Height: 2048},
		},
		"3:2": {
			"1k": {Width: 1536, Height: 1024},
			"2k": {Width: 2048, Height: 1360},
		},
		"2:3": {
			"1k": {Width: 1024, Height: 1536},
			"2k": {Width: 1360, Height: 2048},
		},
		"4:3": {
			"1k": {Width: 1024, Height: 768},
			"2k": {Width: 2048, Height: 1536},
		},
		"3:4": {
			"1k": {Width: 768, Height: 1024},
			"2k": {Width: 1536, Height: 2048},
		},
		"5:4": {
			"1k": {Width: 1280, Height: 1024},
			"2k": {Width: 2560, Height: 2048},
		},
		"4:5": {
			"1k": {Width: 1024, Height: 1280},
			"2k": {Width: 2048, Height: 2560},
		},
		"16:9": {
			"1k": {Width: 1536, Height: 864},
			"2k": {Width: 2048, Height: 1152},
			"4k": {Width: 3840, Height: 2160},
		},
		"9:16": {
			"1k": {Width: 864, Height: 1536},
			"2k": {Width: 1152, Height: 2048},
			"4k": {Width: 2160, Height: 3840},
		},
		"2:1": {
			"1k": {Width: 2048, Height: 1024},
			"2k": {Width: 2688, Height: 1344},
			"4k": {Width: 3840, Height: 1920},
		},
		"1:2": {
			"1k": {Width: 1024, Height: 2048},
			"2k": {Width: 1344, Height: 2688},
			"4k": {Width: 1920, Height: 3840},
		},
		"21:9": {
			"1k": {Width: 2016, Height: 864},
			"2k": {Width: 2688, Height: 1152},
			"4k": {Width: 3840, Height: 1648},
		},
		"9:21": {
			"1k": {Width: 864, Height: 2016},
			"2k": {Width: 1152, Height: 2688},
			"4k": {Width: 1648, Height: 3840},
		},
	}
)

func IsGPTImage2Model(model string) bool {
	model = strings.ToLower(strings.TrimSpace(model))
	return model == gptImage2ModelPrefix || strings.HasPrefix(model, gptImage2ModelPrefix+"-")
}

func ImageCompatTokenCountMeta(req dto.Request, model string) (*types.TokenCountMeta, bool) {
	if req == nil {
		return nil, false
	}

	switch r := req.(type) {
	case *dto.GeneralOpenAIRequest:
		if !IsGPTImage2Model(model) && !IsGPTImage2Model(r.Model) {
			return nil, false
		}
		imageReq, err := ChatCompletionsRequestToImageRequest(r)
		if err != nil {
			return nil, false
		}
		return imageReq.GetTokenCountMeta(), true
	case *dto.OpenAIResponsesRequest:
		if !IsGPTImage2Model(model) && !IsGPTImage2Model(r.Model) {
			return nil, false
		}
		imageReq, err := ResponsesRequestToImageRequest(r)
		if err != nil {
			return nil, false
		}
		return imageReq.GetTokenCountMeta(), true
	default:
		return nil, false
	}
}

func RequestedImageCount(req *dto.ImageRequest) int {
	if req == nil || req.N == nil || *req.N == 0 {
		return 1
	}
	return int(*req.N)
}

func ImageRequestWithCount(req *dto.ImageRequest, count uint) *dto.ImageRequest {
	if req == nil {
		return nil
	}
	copied := *req
	copied.N = common.GetPointer(count)
	return &copied
}

func ChatCompletionsRequestToImageRequest(req *dto.GeneralOpenAIRequest) (*dto.ImageRequest, error) {
	if req == nil {
		return nil, errors.New("request is nil")
	}
	if strings.TrimSpace(req.Model) == "" {
		return nil, errors.New("model is required")
	}

	prompt := extractPromptFromChatRequest(req)
	if prompt == "" {
		return nil, errors.New("prompt is required")
	}

	imageReq := &dto.ImageRequest{
		Model:  req.Model,
		Prompt: prompt,
		Size:   req.Size,
		Stream: req.Stream,
		User:   req.User,
	}
	if req.N != nil && *req.N > 0 {
		imageReq.N = common.GetPointer(uint(*req.N))
	}
	ApplyImageOptionsFromRaw(req.ExtraBody, imageReq)
	appendImageURLsToRequest(imageReq, imageURLsFromChatRequest(req))
	return imageReq, nil
}

func ResponsesRequestToImageRequest(req *dto.OpenAIResponsesRequest) (*dto.ImageRequest, error) {
	if req == nil {
		return nil, errors.New("request is nil")
	}
	if strings.TrimSpace(req.Model) == "" {
		return nil, errors.New("model is required")
	}

	prompt := extractPromptFromResponsesRequest(req)
	if prompt == "" {
		return nil, errors.New("prompt is required")
	}

	imageReq := &dto.ImageRequest{
		Model:  req.Model,
		Prompt: prompt,
		Stream: req.Stream,
		User:   req.User,
	}
	ApplyImageGenerationToolOptions(req.Tools, imageReq)
	appendImageURLsToRequest(imageReq, imageURLsFromResponsesRequest(req))
	return imageReq, nil
}

func ImageUsageToUsage(imageUsage *dto.Usage, fallbackPromptTokens int) *dto.Usage {
	return ImageUsageToUsageWithOutputFallback(imageUsage, fallbackPromptTokens, 0)
}

func ImageUsageToUsageWithOutputFallback(imageUsage *dto.Usage, fallbackPromptTokens int, fallbackOutputTokens int) *dto.Usage {
	usage := &dto.Usage{}
	if imageUsage != nil {
		*usage = *imageUsage
	}
	outputFallbackApplied := false

	if usage.PromptTokens == 0 && usage.InputTokens != 0 {
		usage.PromptTokens = usage.InputTokens
	}
	if usage.CompletionTokens == 0 && usage.OutputTokens != 0 {
		usage.CompletionTokens = usage.OutputTokens
	}
	if usage.InputTokens == 0 && usage.PromptTokens != 0 {
		usage.InputTokens = usage.PromptTokens
	}
	if usage.OutputTokens == 0 && usage.CompletionTokens != 0 {
		usage.OutputTokens = usage.CompletionTokens
	}
	if usage.InputTokensDetails != nil {
		usage.PromptTokensDetails = *usage.InputTokensDetails
	}
	if usage.CompletionTokenDetails.ImageTokens == 0 && usage.OutputTokens != 0 {
		usage.CompletionTokenDetails.ImageTokens = usage.OutputTokens
	}
	if usage.PromptTokens == 0 && fallbackPromptTokens > 0 {
		usage.PromptTokens = fallbackPromptTokens
		usage.InputTokens = fallbackPromptTokens
	}
	if fallbackOutputTokens > 0 && shouldApplyImageOutputTokenFallback(usage) {
		usage.CompletionTokens = fallbackOutputTokens
		usage.OutputTokens = fallbackOutputTokens
		usage.CompletionTokenDetails.ImageTokens = fallbackOutputTokens
		outputFallbackApplied = true
	}
	if usage.CompletionTokens == 0 && imageUsage == nil {
		usage.CompletionTokens = 1
		usage.OutputTokens = 1
		usage.CompletionTokenDetails.ImageTokens = 1
	}
	if usage.TotalTokens == 0 || outputFallbackApplied {
		usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
	}
	return usage
}

func ApplyImageUsageOutputTokenFallback(resp *dto.ImageResponse, req *dto.ImageRequest, fallbackPromptTokens int) {
	if resp == nil || req == nil {
		return
	}
	outputTokens, ok := ImageResponseOutputTokensForRequest(resp, req)
	if !ok {
		return
	}
	resp.Usage = ImageUsageToUsageWithOutputFallback(resp.Usage, fallbackPromptTokens, outputTokens)
}

func ImageResponseOutputTokensForRequest(resp *dto.ImageResponse, req *dto.ImageRequest) (int, bool) {
	if resp == nil || req == nil {
		return 0, false
	}
	images := ImageDataItems(resp)
	if len(images) == 0 {
		return 0, false
	}
	if !IsGPTImage2Model(req.Model) {
		return 0, false
	}

	quality := req.Quality
	if strings.TrimSpace(resp.Quality) != "" {
		quality = resp.Quality
	}

	total := 0
	decodedCount := 0
	for _, image := range images {
		tokens, ok := GPTImage2OutputTokensForImageData(req, image, quality)
		if !ok {
			continue
		}
		total += tokens
		decodedCount++
	}

	missingCount := len(images) - decodedCount
	if missingCount > 0 {
		fallbackTokens, ok := GPTImage2OutputTokensForRequest(req, missingCount)
		if ok {
			total += fallbackTokens
		}
	}

	if total > 0 {
		return total, true
	}
	return GPTImage2OutputTokensForRequest(req, len(images))
}

func GPTImage2OutputTokensForImageData(req *dto.ImageRequest, image dto.ImageData, quality string) (int, bool) {
	if req == nil || !IsGPTImage2Model(req.Model) {
		return 0, false
	}
	dimensions, ok := imageDataDimensions(image)
	if !ok {
		return 0, false
	}
	return gptImage2OutputTokens(dimensions.Width, dimensions.Height, normalizeGPTImage2Quality(quality))
}

func GPTImage2OutputTokensForRequest(req *dto.ImageRequest, imageCount int) (int, bool) {
	if req == nil || imageCount <= 0 || !IsGPTImage2Model(req.Model) {
		return 0, false
	}
	dimensions, ok := resolveGPTImage2Dimensions(req)
	if !ok {
		return 0, false
	}
	tokens, ok := gptImage2OutputTokens(dimensions.Width, dimensions.Height, normalizeGPTImage2Quality(req.Quality))
	if !ok {
		return 0, false
	}
	return tokens * imageCount, true
}

func GeminiImageOutputTokensForBase64(model string, base64Data string) (int, bool) {
	config, _, _, err := DecodeBase64ImageData(base64Data)
	if err != nil {
		return 0, false
	}
	return GeminiImageOutputTokensForDimensions(model, config.Width, config.Height)
}

func GeminiImageOutputTokensForImageData(model string, image dto.ImageData) (int, bool) {
	dimensions, ok := imageDataDimensions(image)
	if !ok {
		return 0, false
	}
	return GeminiImageOutputTokensForDimensions(model, dimensions.Width, dimensions.Height)
}

func GeminiDefaultImageOutputTokens(model string) (int, bool) {
	if isGemini31FlashImageModel(model) || isGemini3ProImageModel(model) {
		return 1120, true
	}
	return 0, false
}

func GeminiImageOutputTokensForDimensions(model string, width int, height int) (int, bool) {
	if width <= 0 || height <= 0 {
		return 0, false
	}
	pixels := width * height
	switch {
	case isGemini31FlashImageModel(model):
		switch {
		case pixels <= 512*512*2:
			return 747, true
		case pixels <= 1024*1024*2:
			return 1120, true
		case pixels <= 2048*2048:
			return 1680, true
		default:
			return 2520, true
		}
	case isGemini3ProImageModel(model):
		if pixels <= 2048*2048 {
			return 1120, true
		}
		return 2000, true
	default:
		return 0, false
	}
}

func shouldApplyImageOutputTokenFallback(usage *dto.Usage) bool {
	if usage == nil {
		return true
	}
	maxTokens := usage.CompletionTokens
	if usage.OutputTokens > maxTokens {
		maxTokens = usage.OutputTokens
	}
	if usage.CompletionTokenDetails.ImageTokens > maxTokens {
		maxTokens = usage.CompletionTokenDetails.ImageTokens
	}
	return maxTokens <= 1
}

func imageDataDimensions(image dto.ImageData) (gptImage2Dimensions, bool) {
	data := strings.TrimSpace(image.B64Json)
	if data == "" && strings.HasPrefix(strings.ToLower(strings.TrimSpace(image.Url)), "data:image/") {
		data = image.Url
	}
	if data == "" {
		return gptImage2Dimensions{}, false
	}
	config, _, _, err := DecodeBase64ImageData(data)
	if err != nil {
		return gptImage2Dimensions{}, false
	}
	return gptImage2Dimensions{Width: config.Width, Height: config.Height}, true
}

func isGemini31FlashImageModel(model string) bool {
	model = strings.ToLower(strings.TrimSpace(model))
	return strings.Contains(model, "gemini-3.1-flash-image") || strings.Contains(model, "nano-banana-2")
}

func isGemini3ProImageModel(model string) bool {
	model = strings.ToLower(strings.TrimSpace(model))
	return strings.Contains(model, "gemini-3-pro-image") || strings.Contains(model, "nano-banana-pro")
}

func resolveGPTImage2Dimensions(req *dto.ImageRequest) (gptImage2Dimensions, bool) {
	if req == nil {
		return gptImage2Dimensions{}, false
	}
	size := strings.ToLower(strings.TrimSpace(req.Size))
	if size == "" || size == "auto" {
		size = "1024x1024"
	}
	if dimensions, ok := parseGPTImage2ExplicitDimensions(size); ok {
		return dimensions, true
	}

	resolution := strings.ToLower(strings.TrimSpace(rawJSONOptionString(req.Resolution)))
	if resolution == "" || resolution == "auto" {
		resolution = "1k"
	}
	byResolution, ok := apimartGPTImage2Dimensions[size]
	if !ok {
		return gptImage2Dimensions{}, false
	}
	dimensions, ok := byResolution[resolution]
	return dimensions, ok
}

func parseGPTImage2ExplicitDimensions(size string) (gptImage2Dimensions, bool) {
	normalized := strings.ToLower(strings.TrimSpace(size))
	normalized = strings.ReplaceAll(normalized, "×", "x")
	normalized = strings.ReplaceAll(normalized, " ", "")
	parts := strings.Split(normalized, "x")
	if len(parts) != 2 {
		return gptImage2Dimensions{}, false
	}
	width, err := strconv.Atoi(parts[0])
	if err != nil {
		return gptImage2Dimensions{}, false
	}
	height, err := strconv.Atoi(parts[1])
	if err != nil {
		return gptImage2Dimensions{}, false
	}
	return gptImage2Dimensions{Width: width, Height: height}, true
}

func normalizeGPTImage2Quality(quality string) string {
	switch strings.ToLower(strings.TrimSpace(quality)) {
	case "low":
		return "low"
	case "high", "hd":
		return "high"
	case "medium", "standard":
		return "medium"
	default:
		return "medium"
	}
}

func gptImage2OutputTokens(width int, height int, quality string) (int, bool) {
	if !validGPTImage2Dimensions(width, height) {
		return 0, false
	}
	factor, ok := gptImage2QualityPatchColumns[quality]
	if !ok {
		return 0, false
	}
	longSide := maxInt(width, height)
	shortSide := minInt(width, height)
	scaledShort := int(math.Round(float64(factor) * float64(shortSide) / float64(longSide)))
	columns := factor
	rows := scaledShort
	if height > width {
		columns = scaledShort
		rows = factor
	}
	patches := columns * rows
	pixels := width * height
	return int(math.Ceil(float64(patches) * float64(2000000+pixels) / 4000000.0)), true
}

func validGPTImage2Dimensions(width int, height int) bool {
	if width <= 0 || height <= 0 {
		return false
	}
	if width%16 != 0 || height%16 != 0 {
		return false
	}
	pixels := width * height
	if pixels < 655360 || pixels > 8294400 {
		return false
	}
	longSide := maxInt(width, height)
	shortSide := minInt(width, height)
	if longSide > 3840 || float64(longSide)/float64(shortSide) > 3 {
		return false
	}
	return true
}

func rawJSONOptionString(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var value string
	if err := common.Unmarshal(raw, &value); err == nil {
		return value
	}
	return strings.Trim(string(raw), `"`)
}

func maxInt(a int, b int) int {
	if a > b {
		return a
	}
	return b
}

func minInt(a int, b int) int {
	if a < b {
		return a
	}
	return b
}

func FirstImageData(resp *dto.ImageResponse) (dto.ImageData, bool) {
	items := ImageDataItems(resp)
	if len(items) == 0 {
		return dto.ImageData{}, false
	}
	return items[0], true
}

func ImageDataItems(resp *dto.ImageResponse) []dto.ImageData {
	if resp == nil || len(resp.Data) == 0 {
		return nil
	}
	items := make([]dto.ImageData, 0, len(resp.Data))
	for _, item := range resp.Data {
		if item.B64Json != "" || item.Url != "" {
			items = append(items, item)
		}
	}
	return items
}

func ImageDataCount(resp *dto.ImageResponse) int {
	return len(ImageDataItems(resp))
}

func ApplyImageResultCountPricing(info *relaycommon.RelayInfo, resp *dto.ImageResponse) {
	if info == nil || !info.PriceData.UsePrice {
		return
	}
	ApplyImageResultCountPricingFromCount(info, ImageDataCount(resp))
}

func ApplyImageResultCountPricingFromCount(info *relaycommon.RelayInfo, count int) {
	if info == nil || !info.PriceData.UsePrice || count <= 0 {
		return
	}
	info.PriceData.AddOtherRatio("n", float64(count))
}

func ImageMarkdownContent(image dto.ImageData, outputFormat string) string {
	if image.B64Json != "" {
		return fmt.Sprintf("![generated image](data:%s;base64,%s)", imageMimeType(outputFormat), image.B64Json)
	}
	if image.Url != "" {
		return fmt.Sprintf("![generated image](%s)", image.Url)
	}
	return ""
}

func ImageMarkdownContents(images []dto.ImageData, outputFormat string) []string {
	contents := make([]string, 0, len(images))
	for _, image := range images {
		if content := ImageMarkdownContent(image, outputFormat); content != "" {
			contents = append(contents, content)
		}
	}
	return contents
}

func ImageResponseToChatResponse(resp *dto.ImageResponse, model string, id string, fallbackPromptTokens int) (*dto.OpenAITextResponse, *dto.Usage, error) {
	images := ImageDataItems(resp)
	if len(images) == 0 {
		return nil, nil, errors.New("image response does not contain generated image data")
	}
	content := strings.Join(ImageMarkdownContents(images, resp.OutputFormat), "\n\n")

	usage := ImageUsageToUsage(resp.Usage, fallbackPromptTokens)
	created := resp.Created
	if created == 0 {
		created = time.Now().Unix()
	}

	chatResp := &dto.OpenAITextResponse{
		Id:      id,
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
				FinishReason: "stop",
			},
		},
		Usage: *usage,
	}
	return chatResp, usage, nil
}

func ImageResponseToResponsesResponse(resp *dto.ImageResponse, model string, id string, itemID string, fallbackPromptTokens int) (map[string]any, *dto.Usage, error) {
	images := ImageDataItems(resp)
	if len(images) == 0 {
		return nil, nil, errors.New("image response does not contain generated image data")
	}

	usage := ImageUsageToUsage(resp.Usage, fallbackPromptTokens)
	created := resp.Created
	if created == 0 {
		created = time.Now().Unix()
	}

	output := make([]map[string]any, 0, len(images))
	for i, image := range images {
		currentItemID := itemID
		if len(images) > 1 {
			currentItemID = fmt.Sprintf("%s_%d", itemID, i)
		}
		output = append(output, imageOutputItem(image, resp, currentItemID, "completed"))
	}
	response := map[string]any{
		"id":                  id,
		"object":              "response",
		"created_at":          created,
		"status":              "completed",
		"model":               model,
		"output":              output,
		"parallel_tool_calls": false,
		"usage":               ResponsesUsageMap(usage),
	}
	return response, usage, nil
}

func ResponsesUsageMap(usage *dto.Usage) map[string]any {
	if usage == nil {
		return nil
	}
	inputDetails := usage.PromptTokensDetails
	if usage.InputTokensDetails != nil {
		inputDetails = *usage.InputTokensDetails
	}
	outputDetails := usage.CompletionTokenDetails
	if outputDetails.ImageTokens == 0 && usage.OutputTokens != 0 {
		outputDetails.ImageTokens = usage.OutputTokens
	}
	return map[string]any{
		"input_tokens":  usage.PromptTokens,
		"output_tokens": usage.CompletionTokens,
		"total_tokens":  usage.TotalTokens,
		"input_tokens_details": map[string]any{
			"cached_tokens": inputDetails.CachedTokens,
			"text_tokens":   inputDetails.TextTokens,
			"image_tokens":  inputDetails.ImageTokens,
			"audio_tokens":  inputDetails.AudioTokens,
		},
		"output_tokens_details": map[string]any{
			"text_tokens":      outputDetails.TextTokens,
			"image_tokens":     outputDetails.ImageTokens,
			"audio_tokens":     outputDetails.AudioTokens,
			"reasoning_tokens": outputDetails.ReasoningTokens,
		},
	}
}

func ImageOutputItemFromStream(event dto.ImageStreamEvent, itemID string, status string) map[string]any {
	image := dto.ImageData{B64Json: event.B64Json}
	resp := &dto.ImageResponse{
		Background:   event.Background,
		OutputFormat: event.OutputFormat,
		Quality:      event.Quality,
		Size:         event.Size,
	}
	return imageOutputItem(image, resp, itemID, status)
}

func imageOutputItem(image dto.ImageData, resp *dto.ImageResponse, itemID string, status string) map[string]any {
	item := map[string]any{
		"id":     itemID,
		"type":   dto.ResponsesOutputTypeImageGenerationCall,
		"status": status,
	}
	if image.B64Json != "" {
		item["result"] = image.B64Json
	} else if image.Url != "" {
		item["result"] = image.Url
	}
	if image.RevisedPrompt != "" {
		item["revised_prompt"] = image.RevisedPrompt
	}
	if resp != nil {
		if resp.Quality != "" {
			item["quality"] = resp.Quality
		}
		if resp.Size != "" {
			item["size"] = resp.Size
		}
		if resp.Background != "" {
			item["background"] = resp.Background
		}
		if resp.OutputFormat != "" {
			item["output_format"] = resp.OutputFormat
		}
	}
	return item
}

func extractPromptFromChatRequest(req *dto.GeneralOpenAIRequest) string {
	var parts []string
	for _, msg := range req.Messages {
		parts = append(parts, textPartsFromChatMessage(msg)...)
	}
	if len(parts) == 0 {
		parts = append(parts, textPartsFromAny(req.Prompt)...)
		parts = append(parts, textPartsFromAny(req.Input)...)
	}
	if req.Instruction != "" {
		parts = append([]string{req.Instruction}, parts...)
	}
	return strings.TrimSpace(strings.Join(compactStrings(parts), "\n\n"))
}

func extractPromptFromResponsesRequest(req *dto.OpenAIResponsesRequest) string {
	var parts []string
	if len(req.Instructions) > 0 {
		parts = append(parts, textPartsFromRaw(req.Instructions)...)
	}
	parts = append(parts, textPartsFromRaw(req.Input)...)
	if len(req.Prompt) > 0 {
		parts = append(parts, textPartsFromRaw(req.Prompt)...)
	}
	return strings.TrimSpace(strings.Join(compactStrings(parts), "\n\n"))
}

func imageURLsFromChatRequest(req *dto.GeneralOpenAIRequest) []string {
	if req == nil {
		return nil
	}
	var urls []string
	for _, msg := range req.Messages {
		for _, part := range msg.ParseContent() {
			if part.Type != dto.ContentTypeImageURL {
				continue
			}
			image := part.GetImageMedia()
			if image == nil {
				continue
			}
			if url, ok := normalizeSupportedImageURL(image.Url); ok {
				urls = append(urls, url)
			}
		}
	}
	urls = append(urls, imageURLsFromAny(req.Input)...)
	return dedupeStrings(urls)
}

func imageURLsFromResponsesRequest(req *dto.OpenAIResponsesRequest) []string {
	if req == nil {
		return nil
	}
	var urls []string
	for _, input := range req.ParseInput() {
		if input.Type != "input_image" {
			continue
		}
		if url, ok := normalizeSupportedImageURL(input.ImageUrl); ok {
			urls = append(urls, url)
		}
	}
	return dedupeStrings(urls)
}

func textPartsFromChatMessage(msg dto.Message) []string {
	if msg.Content == nil {
		return nil
	}
	if msg.IsStringContent() {
		return []string{msg.StringContent()}
	}
	var parts []string
	for _, part := range msg.ParseContent() {
		if part.Type == dto.ContentTypeText && strings.TrimSpace(part.Text) != "" {
			parts = append(parts, part.Text)
		}
	}
	return parts
}

func textPartsFromRaw(raw json.RawMessage) []string {
	if len(raw) == 0 {
		return nil
	}
	var value any
	if err := common.Unmarshal(raw, &value); err != nil {
		return nil
	}
	return textPartsFromAny(value)
}

func textPartsFromAny(value any) []string {
	switch v := value.(type) {
	case nil:
		return nil
	case string:
		if strings.TrimSpace(v) == "" {
			return nil
		}
		return []string{v}
	case []any:
		var parts []string
		for _, item := range v {
			parts = append(parts, textPartsFromAny(item)...)
		}
		return parts
	case map[string]any:
		var parts []string
		itemType := common.Interface2String(v["type"])
		if text := common.Interface2String(v["text"]); text != "" && (itemType == "" || itemType == "text" || itemType == "input_text" || itemType == "output_text") {
			parts = append(parts, text)
		}
		if content, ok := v["content"]; ok {
			parts = append(parts, textPartsFromAny(content)...)
		}
		if input, ok := v["input"]; ok {
			parts = append(parts, textPartsFromAny(input)...)
		}
		return parts
	default:
		return nil
	}
}

func compactStrings(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = stripImagePromptGeneratedPayloads(value)
		value = strings.TrimSpace(value)
		if value != "" {
			out = append(out, value)
		}
	}
	return out
}

func stripImagePromptGeneratedPayloads(value string) string {
	if value == "" {
		return value
	}
	value = imagePromptMarkdownDataURLPattern.ReplaceAllString(value, "")
	return imagePromptDataURLPattern.ReplaceAllString(value, "")
}

func ApplyImageGenerationToolOptions(raw []byte, imageReq *dto.ImageRequest) {
	if len(raw) == 0 || imageReq == nil {
		return
	}
	var tools []map[string]any
	if err := common.Unmarshal(raw, &tools); err != nil {
		return
	}
	for _, tool := range tools {
		if common.Interface2String(tool["type"]) != dto.BuildInToolImageGeneration {
			continue
		}
		applyImageOptionsFromMap(tool, imageReq)
	}
}

func ApplyImageOptionsFromRaw(raw []byte, imageReq *dto.ImageRequest) {
	if len(raw) == 0 || imageReq == nil {
		return
	}
	var values map[string]any
	if err := common.Unmarshal(raw, &values); err != nil {
		return
	}
	applyImageOptionsFromMap(values, imageReq)
}

func applyImageOptionsFromMap(values map[string]any, imageReq *dto.ImageRequest) {
	if imageReq == nil {
		return
	}
	if size := common.Interface2String(values["size"]); size != "" {
		imageReq.Size = size
	}
	if quality := common.Interface2String(values["quality"]); quality != "" {
		imageReq.Quality = quality
	}
	assignRawOption(values, "background", &imageReq.Background)
	assignRawOption(values, "output_format", &imageReq.OutputFormat)
	assignRawOption(values, "output_compression", &imageReq.OutputCompression)
	assignRawOption(values, "partial_images", &imageReq.PartialImages)
	assignRawOption(values, "moderation", &imageReq.Moderation)
	assignRawOption(values, "resolution", &imageReq.Resolution)
	assignRawOption(values, "image_urls", &imageReq.ImageUrls)
	assignRawOption(values, "image", &imageReq.Image)
	if raw, ok := values["official_fallback"]; ok {
		if enabled, ok := raw.(bool); ok {
			imageReq.OfficialFallback = &enabled
		}
	}
}

func NormalizeGPTImage2GenerationImageInputs(imageReq *dto.ImageRequest) {
	if imageReq == nil || !IsGPTImage2Model(imageReq.Model) {
		return
	}
	if len(imageReq.ImageUrls) > 0 {
		imageReq.Image = nil
		return
	}
	urls := imageURLsFromRawMessage(imageReq.Image)
	if len(urls) == 0 {
		return
	}
	raw, err := common.Marshal(urls)
	if err != nil {
		return
	}
	imageReq.ImageUrls = raw
	imageReq.Image = nil
}

func appendImageURLsToRequest(imageReq *dto.ImageRequest, urls []string) {
	if imageReq == nil || len(urls) == 0 {
		return
	}
	combined := append(imageURLsFromRawMessage(imageReq.ImageUrls), urls...)
	combined = dedupeStrings(combined)
	if len(combined) == 0 {
		return
	}
	raw, err := common.Marshal(combined)
	if err != nil {
		return
	}
	imageReq.ImageUrls = raw
}

func imageURLsFromRawMessage(raw json.RawMessage) []string {
	if len(raw) == 0 {
		return nil
	}
	var value any
	if err := common.Unmarshal(raw, &value); err != nil {
		return nil
	}
	return imageURLsFromAny(value)
}

func imageURLsFromAny(value any) []string {
	switch v := value.(type) {
	case string:
		if url, ok := normalizeSupportedImageURL(v); ok {
			return []string{url}
		}
	case []any:
		var urls []string
		for _, item := range v {
			urls = append(urls, imageURLsFromAny(item)...)
		}
		return urls
	case map[string]any:
		for _, key := range []string{"url", "image_url", "image"} {
			if nested, ok := v[key]; ok {
				if urls := imageURLsFromAny(nested); len(urls) > 0 {
					return urls
				}
			}
		}
	}
	return nil
}

func normalizeSupportedImageURL(value string) (string, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", false
	}
	lower := strings.ToLower(value)
	if strings.HasPrefix(lower, "http://") ||
		strings.HasPrefix(lower, "https://") ||
		strings.HasPrefix(lower, "data:image/") {
		return value, true
	}
	return "", false
}

func dedupeStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func assignRawOption(values map[string]any, key string, target *json.RawMessage) {
	if values == nil || target == nil {
		return
	}
	value, ok := values[key]
	if !ok {
		return
	}
	raw, err := common.Marshal(value)
	if err != nil {
		return
	}
	*target = raw
}

func imageMimeType(outputFormat string) string {
	switch strings.ToLower(strings.TrimSpace(outputFormat)) {
	case "jpeg", "jpg":
		return "image/jpeg"
	case "webp":
		return "image/webp"
	default:
		return "image/png"
	}
}
