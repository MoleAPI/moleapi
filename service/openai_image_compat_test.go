package service

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/png"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
)

func testPNGBase64(t *testing.T, width int, height int) string {
	t.Helper()
	img := image.NewGray(image.Rect(0, 0, width, height))
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("failed to encode test png: %v", err)
	}
	return base64.StdEncoding.EncodeToString(buf.Bytes())
}

func TestChatCompletionsRequestToImageRequest(t *testing.T) {
	stream := true
	n := 2
	req := &dto.GeneralOpenAIRequest{
		Model:  "gpt-image-2",
		Stream: &stream,
		N:      &n,
		Size:   "1024x1024",
		Messages: []dto.Message{
			{Role: "system", Content: "Use a clean editorial style."},
			{
				Role: "user",
				Content: []any{
					map[string]any{"type": "text", "text": "Draw a red door"},
					map[string]any{"type": "image_url", "image_url": "https://example.com/input.png"},
				},
			},
		},
		ExtraBody: []byte(`{"quality":"high","background":"transparent","partial_images":3}`),
	}

	imageReq, err := ChatCompletionsRequestToImageRequest(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if imageReq.Model != "gpt-image-2" {
		t.Fatalf("unexpected model: %s", imageReq.Model)
	}
	if imageReq.Prompt != "Use a clean editorial style.\n\nDraw a red door" {
		t.Fatalf("unexpected prompt: %q", imageReq.Prompt)
	}
	if imageReq.Stream == nil || !*imageReq.Stream {
		t.Fatal("expected stream to be preserved")
	}
	if imageReq.N == nil || *imageReq.N != 2 {
		t.Fatalf("unexpected n: %v", imageReq.N)
	}
	if imageReq.Size != "1024x1024" || imageReq.Quality != "high" {
		t.Fatalf("unexpected size or quality: %s / %s", imageReq.Size, imageReq.Quality)
	}
	if string(imageReq.Background) != `"transparent"` {
		t.Fatalf("unexpected background: %s", string(imageReq.Background))
	}
	if string(imageReq.PartialImages) != "3" {
		t.Fatalf("unexpected partial_images: %s", string(imageReq.PartialImages))
	}
	if !strings.Contains(string(imageReq.ImageUrls), "https://example.com/input.png") {
		t.Fatalf("expected image_url to be forwarded as image_urls, got %s", string(imageReq.ImageUrls))
	}
}

func TestChatCompletionsRequestToImageRequestStripsGeneratedImagePayloads(t *testing.T) {
	generatedImage := "![generated image](data:image/png;base64," + strings.Repeat("a", 12000) + ")"
	req := &dto.GeneralOpenAIRequest{
		Model: "gpt-image-2",
		Messages: []dto.Message{
			{Role: "system", Content: "Use a clean editorial style."},
			{Role: "assistant", Content: generatedImage},
			{Role: "user", Content: "Make the next image use a blue background."},
		},
	}

	imageReq, err := ChatCompletionsRequestToImageRequest(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if strings.Contains(imageReq.Prompt, "data:image") || strings.Contains(imageReq.Prompt, "base64") {
		t.Fatalf("prompt should not contain generated image payload: %q", imageReq.Prompt)
	}
	if !strings.Contains(imageReq.Prompt, "Use a clean editorial style.") || !strings.Contains(imageReq.Prompt, "Make the next image use a blue background.") {
		t.Fatalf("prompt should retain useful text: %q", imageReq.Prompt)
	}
}

func TestImageCompatTokenCountMetaUsesConvertedPromptOnly(t *testing.T) {
	req := &dto.GeneralOpenAIRequest{
		Model: "gpt-image-2",
		Messages: []dto.Message{
			{Role: "assistant", Content: "data:image/webp;base64," + strings.Repeat("b", 10000)},
			{Role: "user", Content: "Draw a small red cube."},
		},
	}

	meta, ok := ImageCompatTokenCountMeta(req, "gpt-image-2")
	if !ok {
		t.Fatal("expected image compatibility token meta")
	}
	if strings.Contains(meta.CombineText, "data:image") || strings.Contains(meta.CombineText, strings.Repeat("b", 100)) {
		t.Fatalf("token meta should not contain generated image payload: %q", meta.CombineText)
	}
	if meta.CombineText != "Draw a small red cube." {
		t.Fatalf("unexpected token meta text: %q", meta.CombineText)
	}
}

func TestResponsesRequestToImageRequest(t *testing.T) {
	stream := true
	req := &dto.OpenAIResponsesRequest{
		Model:  "gpt-image-2",
		Stream: &stream,
		Input:  []byte(`[{"role":"user","content":[{"type":"input_text","text":"Make a launch poster"},{"type":"input_image","image_url":"data:image/png;base64,abc123"}]}]`),
		Tools:  []byte(`[{"type":"web_search_preview"},{"type":"image_generation","size":"1536x1024","quality":"medium","partial_images":2,"output_format":"webp"}]`),
	}

	imageReq, err := ResponsesRequestToImageRequest(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if imageReq.Prompt != "Make a launch poster" {
		t.Fatalf("unexpected prompt: %q", imageReq.Prompt)
	}
	if imageReq.Stream == nil || !*imageReq.Stream {
		t.Fatal("expected stream to be preserved")
	}
	if imageReq.Size != "1536x1024" || imageReq.Quality != "medium" {
		t.Fatalf("unexpected size or quality: %s / %s", imageReq.Size, imageReq.Quality)
	}
	if string(imageReq.PartialImages) != "2" {
		t.Fatalf("unexpected partial_images: %s", string(imageReq.PartialImages))
	}
	if string(imageReq.OutputFormat) != `"webp"` {
		t.Fatalf("unexpected output_format: %s", string(imageReq.OutputFormat))
	}
	if !strings.Contains(string(imageReq.ImageUrls), "data:image/png;base64,abc123") {
		t.Fatalf("expected input_image to be forwarded as image_urls, got %s", string(imageReq.ImageUrls))
	}
}

func TestApplyImageOptionsFromRawPreservesApimartFields(t *testing.T) {
	imageReq := &dto.ImageRequest{}
	ApplyImageOptionsFromRaw([]byte(`{"resolution":"2k","image_urls":["https://example.com/ref.png"],"image":"https://example.com/ignored.png","official_fallback":false}`), imageReq)

	if string(imageReq.Resolution) != `"2k"` {
		t.Fatalf("unexpected resolution: %s", string(imageReq.Resolution))
	}
	if !strings.Contains(string(imageReq.ImageUrls), "https://example.com/ref.png") {
		t.Fatalf("unexpected image_urls: %s", string(imageReq.ImageUrls))
	}
	if imageReq.OfficialFallback == nil || *imageReq.OfficialFallback {
		t.Fatalf("expected explicit false official_fallback, got %v", imageReq.OfficialFallback)
	}
	if !strings.Contains(string(imageReq.Image), "https://example.com/ignored.png") {
		t.Fatalf("unexpected image: %s", string(imageReq.Image))
	}
	body, err := common.Marshal(imageReq)
	if err != nil {
		t.Fatalf("marshal image request: %v", err)
	}
	for _, want := range []string{`"resolution":"2k"`, `"image_urls":["https://example.com/ref.png"]`, `"official_fallback":false`} {
		if !strings.Contains(string(body), want) {
			t.Fatalf("expected %s in marshaled request, got %s", want, string(body))
		}
	}
}

func TestNormalizeGPTImage2GenerationImageInputsConvertsImageToImageURLs(t *testing.T) {
	imageReq := &dto.ImageRequest{
		Model: "gpt-image-2",
		Image: []byte(`[
			{"image_url":{"url":"https://example.com/a.png"}},
			"data:image/png;base64,abc123"
		]`),
	}

	NormalizeGPTImage2GenerationImageInputs(imageReq)

	if len(imageReq.Image) != 0 {
		t.Fatalf("expected image field to be removed, got %s", string(imageReq.Image))
	}
	if !strings.Contains(string(imageReq.ImageUrls), "https://example.com/a.png") ||
		!strings.Contains(string(imageReq.ImageUrls), "data:image/png;base64,abc123") {
		t.Fatalf("unexpected image_urls: %s", string(imageReq.ImageUrls))
	}
}

func TestNormalizeGPTImage2GenerationImageInputsPreservesExistingImageURLs(t *testing.T) {
	imageReq := &dto.ImageRequest{
		Model:     "gpt-image-2",
		ImageUrls: []byte(`["https://example.com/ref.png"]`),
		Image:     []byte(`"https://example.com/ignored.png"`),
	}

	NormalizeGPTImage2GenerationImageInputs(imageReq)

	if len(imageReq.Image) != 0 {
		t.Fatalf("expected image field to be removed, got %s", string(imageReq.Image))
	}
	if string(imageReq.ImageUrls) != `["https://example.com/ref.png"]` {
		t.Fatalf("unexpected image_urls: %s", string(imageReq.ImageUrls))
	}
}

func TestNormalizeGPTImage2GenerationImageInputsKeepsUnsupportedImageShape(t *testing.T) {
	imageReq := &dto.ImageRequest{
		Model: "gpt-image-2",
		Image: []byte(`"file-abc123"`),
	}

	NormalizeGPTImage2GenerationImageInputs(imageReq)

	if len(imageReq.ImageUrls) != 0 {
		t.Fatalf("expected no image_urls for unsupported image value, got %s", string(imageReq.ImageUrls))
	}
	if string(imageReq.Image) != `"file-abc123"` {
		t.Fatalf("expected image to remain for upstream validation, got %s", string(imageReq.Image))
	}
}

func TestGPTImage2OutputTokensForRequestUsesOfficialCalculatorValues(t *testing.T) {
	tests := []struct {
		name   string
		req    *dto.ImageRequest
		count  int
		want   int
		wantOK bool
	}{
		{
			name:   "low square",
			req:    &dto.ImageRequest{Model: "gpt-image-2", Size: "1024x1024", Quality: "low"},
			count:  1,
			want:   196,
			wantOK: true,
		},
		{
			name:   "medium portrait",
			req:    &dto.ImageRequest{Model: "gpt-image-2", Size: "1024x1536", Quality: "medium"},
			count:  1,
			want:   1372,
			wantOK: true,
		},
		{
			name:   "high landscape two images",
			req:    &dto.ImageRequest{Model: "gpt-image-2", Size: "1536x1024", Quality: "high"},
			count:  2,
			want:   10976,
			wantOK: true,
		},
		{
			name:   "apimart ratio resolution",
			req:    &dto.ImageRequest{Model: "gpt-image-2", Size: "16:9", Quality: "medium", Resolution: []byte(`"2k"`)},
			count:  1,
			want:   1413,
			wantOK: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := GPTImage2OutputTokensForRequest(tt.req, tt.count)
			if ok != tt.wantOK || got != tt.want {
				t.Fatalf("expected %d/%v, got %d/%v", tt.want, tt.wantOK, got, ok)
			}
		})
	}
}

func TestGeminiImageOutputTokensForDimensionsUsesResolutionTiers(t *testing.T) {
	tests := []struct {
		name   string
		model  string
		width  int
		height int
		want   int
	}{
		{
			name:   "flash image 512",
			model:  "gemini-3.1-flash-image-preview",
			width:  512,
			height: 512,
			want:   747,
		},
		{
			name:   "flash image 1k landscape",
			model:  "gemini-3.1-flash-image-preview",
			width:  1536,
			height: 864,
			want:   1120,
		},
		{
			name:   "flash image 2k square",
			model:  "gemini-3.1-flash-image-preview",
			width:  2048,
			height: 2048,
			want:   1680,
		},
		{
			name:   "flash image 4k landscape",
			model:  "gemini-3.1-flash-image-preview",
			width:  3840,
			height: 2160,
			want:   2520,
		},
		{
			name:   "pro image 2k",
			model:  "gemini-3-pro-image-preview",
			width:  2048,
			height: 2048,
			want:   1120,
		},
		{
			name:   "pro image 4k",
			model:  "gemini-3-pro-image-preview",
			width:  3840,
			height: 2160,
			want:   2000,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := GeminiImageOutputTokensForDimensions(tt.model, tt.width, tt.height)
			if !ok || got != tt.want {
				t.Fatalf("expected %d/true, got %d/%v", tt.want, got, ok)
			}
		})
	}
}

func TestImageResponseOutputTokensForRequestPrefersActualImagePixels(t *testing.T) {
	req := &dto.ImageRequest{Model: "gpt-image-2", Size: "1024x1024", Quality: "medium"}
	resp := &dto.ImageResponse{
		Data: []dto.ImageData{
			{B64Json: testPNGBase64(t, 1024, 1536)},
		},
	}

	got, ok := ImageResponseOutputTokensForRequest(resp, req)
	if !ok || got != 1372 {
		t.Fatalf("expected actual 1024x1536 medium output tokens 1372, got %d/%v", got, ok)
	}
}

func TestApplyImageUsageOutputTokenFallbackUsesActualResultCount(t *testing.T) {
	req := &dto.ImageRequest{Model: "gpt-image-2", Size: "1024x1024", Quality: "low"}
	resp := &dto.ImageResponse{
		Data: []dto.ImageData{
			{Url: "https://example.com/a.png"},
			{Url: "https://example.com/b.png"},
		},
		Usage: &dto.Usage{InputTokens: 7, OutputTokens: 1, TotalTokens: 8},
	}

	ApplyImageUsageOutputTokenFallback(resp, req, 0)

	if resp.Usage == nil {
		t.Fatal("expected usage")
	}
	if resp.Usage.PromptTokens != 7 || resp.Usage.CompletionTokens != 392 || resp.Usage.OutputTokens != 392 {
		t.Fatalf("unexpected usage: %+v", resp.Usage)
	}
	if resp.Usage.CompletionTokenDetails.ImageTokens != 392 || resp.Usage.TotalTokens != 399 {
		t.Fatalf("unexpected token details: %+v", resp.Usage)
	}
}

func TestImageResponseToChatResponse(t *testing.T) {
	resp := &dto.ImageResponse{
		Created:      123,
		OutputFormat: "webp",
		Data:         []dto.ImageData{{B64Json: "abc123"}},
		Usage:        &dto.Usage{PromptTokens: 5, CompletionTokens: 7, TotalTokens: 12},
	}

	chatResp, usage, err := ImageResponseToChatResponse(resp, "gpt-image-2", "chatcmpl_test", 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if chatResp.Id != "chatcmpl_test" || chatResp.Model != "gpt-image-2" {
		t.Fatalf("unexpected response identity: %s / %s", chatResp.Id, chatResp.Model)
	}
	if len(chatResp.Choices) != 1 {
		t.Fatalf("unexpected choice count: %d", len(chatResp.Choices))
	}
	if chatResp.Choices[0].Message.Content != "![generated image](data:image/webp;base64,abc123)" {
		t.Fatalf("unexpected content: %v", chatResp.Choices[0].Message.Content)
	}
	if usage.TotalTokens != 12 || chatResp.Usage.TotalTokens != 12 {
		t.Fatalf("unexpected usage: %+v / %+v", usage, chatResp.Usage)
	}
}

func TestImageResponseToChatResponseIncludesAllImages(t *testing.T) {
	resp := &dto.ImageResponse{
		Created:      123,
		OutputFormat: "png",
		Data: []dto.ImageData{
			{Url: "https://example.com/a.png"},
			{Url: "https://example.com/b.png"},
		},
		Usage: &dto.Usage{PromptTokens: 5, CompletionTokens: 7, TotalTokens: 12},
	}

	chatResp, _, err := ImageResponseToChatResponse(resp, "gpt-image-2", "chatcmpl_test", 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	content, ok := chatResp.Choices[0].Message.Content.(string)
	if !ok {
		t.Fatalf("expected string content, got %T", chatResp.Choices[0].Message.Content)
	}
	if !strings.Contains(content, "https://example.com/a.png") || !strings.Contains(content, "https://example.com/b.png") {
		t.Fatalf("expected both images in chat content, got %q", content)
	}
}

func TestImageResponseToResponsesResponseIncludesAllImages(t *testing.T) {
	resp := &dto.ImageResponse{
		Created: 123,
		Data: []dto.ImageData{
			{Url: "https://example.com/a.png"},
			{Url: "https://example.com/b.png"},
		},
		Usage: &dto.Usage{PromptTokens: 5, CompletionTokens: 7, TotalTokens: 12},
	}

	response, _, err := ImageResponseToResponsesResponse(resp, "gpt-image-2", "resp_test", "igc_test", 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output, ok := response["output"].([]map[string]any)
	if !ok || len(output) != 2 {
		t.Fatalf("expected two output items, got %#v", response["output"])
	}
	if output[0]["result"] != "https://example.com/a.png" || output[1]["result"] != "https://example.com/b.png" {
		t.Fatalf("unexpected output items: %#v", output)
	}
}

func TestImageDataCountIgnoresEmptyItems(t *testing.T) {
	resp := &dto.ImageResponse{
		Data: []dto.ImageData{
			{},
			{Url: "https://example.com/a.png"},
			{B64Json: "abc123"},
		},
	}

	if got := ImageDataCount(resp); got != 2 {
		t.Fatalf("expected 2 images, got %d", got)
	}
}

func TestImageResponseToResponsesResponse(t *testing.T) {
	resp := &dto.ImageResponse{
		Created:      456,
		Background:   "transparent",
		OutputFormat: "png",
		Quality:      "high",
		Size:         "1024x1024",
		Data: []dto.ImageData{{
			B64Json:       "image-b64",
			RevisedPrompt: "A revised prompt",
		}},
		Usage: &dto.Usage{PromptTokens: 3, CompletionTokens: 9, TotalTokens: 12, OutputTokens: 9},
	}

	response, usage, err := ImageResponseToResponsesResponse(resp, "gpt-image-2", "resp_test", "igc_test", 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output, ok := response["output"].([]map[string]any)
	if !ok || len(output) != 1 {
		t.Fatalf("unexpected output: %#v", response["output"])
	}
	item := output[0]
	if item["type"] != dto.ResponsesOutputTypeImageGenerationCall || item["result"] != "image-b64" {
		t.Fatalf("unexpected output item: %#v", item)
	}
	if item["revised_prompt"] != "A revised prompt" || item["quality"] != "high" || item["size"] != "1024x1024" {
		t.Fatalf("unexpected image metadata: %#v", item)
	}
	if usage.TotalTokens != 12 {
		t.Fatalf("unexpected usage: %+v", usage)
	}
	usageMap, ok := response["usage"].(map[string]any)
	if !ok {
		t.Fatalf("unexpected usage map: %#v", response["usage"])
	}
	if usageMap["total_tokens"] != 12 {
		t.Fatalf("unexpected usage total: %#v", usageMap)
	}
}
