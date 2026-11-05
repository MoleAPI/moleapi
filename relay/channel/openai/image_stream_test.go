package openai

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNormalizeOpenAIUsageImageOutputDetails(t *testing.T) {
	tests := []struct {
		name       string
		details    dto.OutputTokenDetails
		wantImage  int
		wantText   int
		wantReason int
	}{
		{
			name:      "infers image tokens without a breakdown",
			wantImage: 8,
		},
		{
			name:      "keeps an explicit image breakdown",
			details:   dto.OutputTokenDetails{ImageTokens: 6},
			wantImage: 6,
		},
		{
			name:       "does not replace a more specific text and reasoning breakdown",
			details:    dto.OutputTokenDetails{TextTokens: 5, ReasoningTokens: 3},
			wantText:   5,
			wantReason: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			usage := dto.Usage{
				OutputTokens:           8,
				CompletionTokenDetails: tt.details,
			}

			normalizeOpenAIUsage(&usage)

			require.Equal(t, 8, usage.CompletionTokens)
			require.Equal(t, tt.wantImage, usage.CompletionTokenDetails.ImageTokens)
			require.Equal(t, tt.wantText, usage.CompletionTokenDetails.TextTokens)
			require.Equal(t, tt.wantReason, usage.CompletionTokenDetails.ReasoningTokens)
		})
	}
}

func newImageTestContext(t *testing.T, body, contentType string, isStream bool) (*gin.Context, *httptest.ResponseRecorder, *http.Response, *relaycommon.RelayInfo) {
	t.Helper()

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     http.Header{"Content-Type": []string{contentType}},
	}
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{},
		IsStream:    isStream,
		Request:     &dto.ImageRequest{ResponseFormat: "url"},
	}
	return c, recorder, resp, info
}

func TestImageExpressionUsesCompletedCountAndProtectsAbortedStreams(t *testing.T) {
	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = oldTimeout })
	for _, tc := range []struct {
		name, body      string
		stream, abort   bool
		requested, want int
	}{
		{"JSON uses actual count", `{"data":[{"b64_json":"first"},{"b64_json":"second"}]}`, false, false, 3, 2},
		{"JSON wrapped as SSE uses actual count", `{"data":[{"b64_json":"first"}]}`, true, false, 3, 1},
		{"empty response retains request", `{"data":[]}`, false, false, 3, 3},
		{"completed stream refunds missing images", "data: {\"type\":\"image_generation.completed\",\"b64_json\":\"first\"}\n\ndata: [DONE]\n\n", true, false, 3, 1},
		{"client abort cannot reduce count", "data: {\"type\":\"image_generation.completed\",\"b64_json\":\"first\"}\n\n", true, true, 3, 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			contentType := "application/json"
			if strings.HasPrefix(tc.body, "data:") {
				contentType = "text/event-stream"
			}
			c, _, resp, info := newImageTestContext(t, tc.body, contentType, tc.stream)
			if tc.abort {
				c, _, resp, info = newDisconnectingImageStream(t, tc.body, "first")
			}
			info.TieredBillingSnapshot = &billingexpr.BillingSnapshot{EstimatedImageCount: &tc.requested}
			if tc.stream {
				_, err := OpenaiImageStreamHandler(c, info, resp)
				require.Nil(t, err)
			} else {
				_, err := OpenaiImageHandler(c, info, resp)
				require.Nil(t, err)
			}
			count := info.RequestedImageCount()
			if info.BillingImageCount != nil {
				count = *info.BillingImageCount
			}
			assert.Equal(t, tc.want, count)
			assert.Empty(t, info.PriceData.OtherRatios(), "expression quantities must not add a legacy multiplier")
		})
	}
}

func TestImageCacheUsageAcrossResponseFormats(t *testing.T) {
	previousTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = previousTimeout })
	// Contract fixture for compatible upstreams. Live OpenAI cache response
	// verification is a separate release check; this is not a captured response.
	const usageJSON = `{"input_tokens":1000,"output_tokens":100,"total_tokens":1100,"input_tokens_details":{"text_tokens":400,"image_tokens":600,"cached_tokens":300,"cached_tokens_details":{"text_tokens":100,"image_tokens":200}}}`
	for _, mode := range []string{"image_generation", "image_edit"} {
		for _, stream := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/stream=%t", mode, stream), func(t *testing.T) {
				body := `{"data":[{"b64_json":"image"}],"usage":` + usageJSON + `}`
				contentType := "application/json"
				if stream {
					body = "data: {\"type\":\"" + mode + ".completed\",\"usage\":" + usageJSON + "}\n\ndata: [DONE]\n\n"
					contentType = "text/event-stream"
				}
				ctx, recorder, response, info := newImageTestContext(t, body, contentType, stream)
				info.RelayMode = relayconstant.RelayModeImagesGenerations
				if mode == "image_edit" {
					info.RelayMode = relayconstant.RelayModeImagesEdits
					ctx.Request.URL.Path = "/v1/images/edits"
				}
				result, apiErr := (&Adaptor{}).DoResponse(ctx, response, info)
				require.Nil(t, apiErr)
				usage := result.(*dto.Usage)
				assert.Equal(t, 1000, usage.PromptTokens)
				assert.Equal(t, 300, usage.PromptTokensDetails.CachedTokens)
				require.NotNil(t, usage.PromptTokensDetails.CachedTokensDetails)
				assert.Equal(t, 200, *usage.PromptTokensDetails.CachedTokensDetails.ImageTokens)
				assert.Contains(t, recorder.Body.String(), `"cached_tokens_details":{"text_tokens":100,"image_tokens":200}`)
			})
		}
	}
}

func TestOpenaiImageDoResponseUsesInfoIsStream(t *testing.T) {
	oldMode := gin.Mode()
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() { gin.SetMode(oldMode) })

	body := `{"created":1710000000,"data":[{"b64_json":"image"}]}`

	t.Run("non-stream response stays JSON", func(t *testing.T) {
		c, recorder, resp, info := newImageTestContext(t, body, "application/json", false)
		info.RelayMode = relayconstant.RelayModeImagesGenerations

		usage, err := (&Adaptor{}).DoResponse(c, resp, info)

		require.Nil(t, err)
		require.NotNil(t, usage)
		require.Equal(t, body, recorder.Body.String())
	})

	t.Run("stream response converts JSON to SSE", func(t *testing.T) {
		c, recorder, resp, info := newImageTestContext(t, body, "application/json", true)
		info.RelayMode = relayconstant.RelayModeImagesGenerations

		usage, err := (&Adaptor{}).DoResponse(c, resp, info)

		require.Nil(t, err)
		require.NotNil(t, usage)
		require.Contains(t, recorder.Body.String(), `event: image_generation.completed`)
		require.Contains(t, recorder.Body.String(), `data: [DONE]`)
	})
}

func TestOpenaiImageChatCompletionWrapper(t *testing.T) {
	oldMode := gin.Mode()
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() { gin.SetMode(oldMode) })

	body := `{"created":1710000000,"data":[{"url":"https://example.test/cat.png"}],"usage":{"input_tokens":3,"output_tokens":4,"total_tokens":7}}`

	t.Run("non-stream image response is returned as chat completion", func(t *testing.T) {
		c, recorder, resp, info := newImageTestContext(t, body, "application/json", false)
		c.Set("chat_image_completion_bridge", true)
		info.RelayMode = relayconstant.RelayModeImagesGenerations
		info.RelayFormat = types.RelayFormatOpenAI
		info.RequestURLPath = "/v1/images/generations"
		info.OriginModelName = "gpt-image-2"

		usage, err := OpenaiImageHandler(c, info, resp)

		require.Nil(t, err)
		require.Equal(t, 3, usage.PromptTokens)
		require.Equal(t, 4, usage.CompletionTokens)
		require.Contains(t, recorder.Body.String(), `"object":"chat.completion"`)
		require.Contains(t, recorder.Body.String(), `![generated image](https://example.test/cat.png)`)
	})

	t.Run("stream image response is returned as chat completion chunks", func(t *testing.T) {
		c, recorder, resp, info := newImageTestContext(t, body, "application/json", true)
		c.Set("chat_image_completion_bridge", true)
		info.RelayMode = relayconstant.RelayModeImagesGenerations
		info.RelayFormat = types.RelayFormatOpenAI
		info.RequestURLPath = "/v1/images/generations"
		info.OriginModelName = "gpt-image-2"

		usage, err := OpenaiImageStreamHandler(c, info, resp)

		require.Nil(t, err)
		require.Equal(t, 3, usage.PromptTokens)
		require.Equal(t, 4, usage.CompletionTokens)
		require.Equal(t, "text/event-stream", recorder.Header().Get("Content-Type"))
		require.Contains(t, recorder.Body.String(), `"object":"chat.completion.chunk"`)
		require.Contains(t, recorder.Body.String(), `![generated image](https://example.test/cat.png)`)
		require.Contains(t, recorder.Body.String(), `data: [DONE]`)
		require.NotContains(t, recorder.Body.String(), `event: image_generation.completed`)
	})
}

func TestOpenaiImageResponsesWrapper(t *testing.T) {
	oldMode := gin.Mode()
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() { gin.SetMode(oldMode) })
	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = oldTimeout })

	body := `{"created":1710000000,"data":[{"b64_json":"image-b64"}],"usage":{"input_tokens":3,"output_tokens":4,"total_tokens":7}}`

	t.Run("non-stream image response is returned as responses object", func(t *testing.T) {
		c, recorder, resp, info := newImageTestContext(t, body, "application/json", false)
		c.Set("responses_image_generation_bridge", true)
		info.RelayMode = relayconstant.RelayModeImagesGenerations
		info.RelayFormat = types.RelayFormatOpenAIResponses
		info.RequestURLPath = "/v1/images/generations"
		info.OriginModelName = "gpt-image-2"

		usage, err := OpenaiImageHandler(c, info, resp)

		require.Nil(t, err)
		require.Equal(t, 3, usage.PromptTokens)
		require.Equal(t, 4, usage.CompletionTokens)
		require.Contains(t, recorder.Body.String(), `"object":"response"`)
		require.Contains(t, recorder.Body.String(), `"type":"image_generation_call"`)
		require.Contains(t, recorder.Body.String(), `"result":"image-b64"`)
	})

	t.Run("stream image response is returned as responses events", func(t *testing.T) {
		streamBody := strings.Join([]string{
			`event: image_generation.partial_image`,
			`data: {"type":"image_generation.partial_image","partial_image_index":0,"b64_json":"partial"}`,
			``,
			`event: image_generation.completed`,
			`data: {"type":"image_generation.completed","b64_json":"image-b64","usage":{"input_tokens":3,"output_tokens":4,"total_tokens":7}}`,
			``,
			`data: [DONE]`,
			``,
		}, "\n")
		c, recorder, resp, info := newImageTestContext(t, streamBody, "text/event-stream", true)
		c.Set("responses_image_generation_bridge", true)
		info.RelayMode = relayconstant.RelayModeImagesGenerations
		info.RelayFormat = types.RelayFormatOpenAIResponses
		info.RequestURLPath = "/v1/images/generations"
		info.OriginModelName = "gpt-image-2"

		usage, err := OpenaiImageStreamHandler(c, info, resp)

		require.Nil(t, err)
		require.Equal(t, 3, usage.PromptTokens)
		require.Equal(t, 4, usage.CompletionTokens)
		require.Contains(t, recorder.Body.String(), `event: response.created`)
		require.Contains(t, recorder.Body.String(), `event: response.image_generation_call.partial_image`)
		require.Contains(t, recorder.Body.String(), `"partial_image_b64":"partial"`)
		require.Contains(t, recorder.Body.String(), `event: response.image_generation_call.completed`)
		require.Contains(t, recorder.Body.String(), `event: response.completed`)
		require.Contains(t, recorder.Body.String(), `"result":"image-b64"`)
		require.NotContains(t, recorder.Body.String(), `event: image_generation.completed`)
	})
}

// TestOpenaiImageStreamHandlerForwardsSSEAndUsage covers the core SSE path:
// chunks are forwarded with rebuilt event lines, usage is extracted and
// normalized (input_tokens -> prompt_tokens with details), and [DONE] is
// re-emitted to the client.
func TestOpenaiImageStreamHandlerForwardsSSEAndUsage(t *testing.T) {
	oldMode := gin.Mode()
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() { gin.SetMode(oldMode) })

	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = oldTimeout })

	body := strings.Join([]string{
		`event: image_generation.partial_image`,
		`data: {"type":"image_generation.partial_image","b64_json":"partial"}`,
		``,
		`data: {"usage":{"input_tokens":3,"output_tokens":4,"total_tokens":7,"input_tokens_details":{"image_tokens":2,"text_tokens":1}}}`,
		``,
		`data: [DONE]`,
		``,
	}, "\n")

	c, recorder, resp, info := newImageTestContext(t, body, "text/event-stream", true)
	info.PriceData.UsePrice = true
	info.PriceData.AddOtherRatio("n", 3)

	usage, err := OpenaiImageStreamHandler(c, info, resp)
	require.Nil(t, err)
	require.Equal(t, 3, usage.PromptTokens)
	require.Equal(t, 4, usage.CompletionTokens)
	require.Equal(t, 7, usage.TotalTokens)
	require.Equal(t, 2, usage.PromptTokensDetails.ImageTokens)
	require.Equal(t, 1, usage.PromptTokensDetails.TextTokens)
	require.Contains(t, recorder.Body.String(), `event: image_generation.partial_image`)
	require.Contains(t, recorder.Body.String(), `data: {"type":"image_generation.partial_image","b64_json":"partial"}`)
	require.Contains(t, recorder.Body.String(), `data: {"usage":{"input_tokens":3,"output_tokens":4,"total_tokens":7,"input_tokens_details":{"image_tokens":2,"text_tokens":1}}}`)
	require.Contains(t, recorder.Body.String(), `data: [DONE]`)
	require.Equal(t, "text/event-stream", recorder.Header().Get("Content-Type"))
	require.Equal(t, 3.0, info.PriceData.OtherRatios()["n"], "streams without completed events keep the requested count")
}

func TestOpenaiImageStreamHandlerUsesCompletedEventCount(t *testing.T) {
	oldMode := gin.Mode()
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() { gin.SetMode(oldMode) })

	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = oldTimeout })

	body := strings.Join([]string{
		`data: {"type":"image_generation.partial_image","partial_image_index":0,"b64_json":"partial"}`,
		``,
		`data: {"type":"image_generation.completed","b64_json":"first"}`,
		``,
		`data: {"type":"image_edit.completed","b64_json":"second","usage":{"input_tokens":3,"output_tokens":4,"total_tokens":7}}`,
		``,
		`data: [DONE]`,
		``,
	}, "\n")

	c, _, resp, info := newImageTestContext(t, body, "text/event-stream", true)
	info.PriceData.UsePrice = true
	info.PriceData.AddOtherRatio("n", 3)

	usage, err := OpenaiImageStreamHandler(c, info, resp)

	require.Nil(t, err)
	require.Equal(t, 7, usage.TotalTokens)
	require.Equal(t, 2.0, info.PriceData.OtherRatios()["n"])
}

// blockingBody serves one SSE chunk, then blocks until Close (the scanner's
// cleanup) and returns EOF — keeping the upstream "open" while the client-side
// disconnect is simulated elsewhere.
type blockingBody struct {
	mu     sync.Mutex
	sent   bool
	chunk  []byte
	closed chan struct{}
}

func (b *blockingBody) Read(p []byte) (int, error) {
	b.mu.Lock()
	if !b.sent {
		b.sent = true
		n := copy(p, b.chunk)
		b.mu.Unlock()
		return n, nil
	}
	b.mu.Unlock()
	<-b.closed
	return 0, io.EOF
}

func (b *blockingBody) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	select {
	case <-b.closed:
	default:
		close(b.closed)
	}
	return nil
}

// cancelAfterWriter cancels the request context right after the payload
// containing needle has been written to the client, simulating a client that
// disconnects after receiving that event. Cancelling from the write side (not
// the upstream read side) makes the abort deterministic: the handler has
// already processed and counted the event when the disconnect fires.
type cancelAfterWriter struct {
	gin.ResponseWriter
	needle string
	cancel context.CancelFunc
	once   sync.Once
}

func (w *cancelAfterWriter) Write(p []byte) (int, error) {
	n, err := w.ResponseWriter.Write(p)
	if strings.Contains(string(p), w.needle) {
		w.once.Do(w.cancel)
	}
	return n, err
}

func (w *cancelAfterWriter) WriteString(s string) (int, error) {
	n, err := io.WriteString(w.ResponseWriter, s)
	if strings.Contains(s, w.needle) {
		w.once.Do(w.cancel)
	}
	return n, err
}

func newDisconnectingImageStream(t *testing.T, sseBody, disconnectAfter string) (*gin.Context, *httptest.ResponseRecorder, *http.Response, *relaycommon.RelayInfo) {
	t.Helper()
	c, recorder, resp, info := newImageTestContext(t, "", "text/event-stream", true)
	ctx, cancel := context.WithCancel(c.Request.Context())
	t.Cleanup(cancel)
	c.Request = c.Request.WithContext(ctx)
	c.Writer = &cancelAfterWriter{ResponseWriter: c.Writer, needle: disconnectAfter, cancel: cancel}
	resp.Body = &blockingBody{
		chunk:  []byte(sseBody),
		closed: make(chan struct{}),
	}
	return c, recorder, resp, info
}

// TestOpenaiImageStreamHandlerClientDisconnectKeepsRequestedCount guards the
// billing invariant: completed-event counting must not lower the charge when
// the client aborts the stream. Upstream already generated (and charged for)
// all requested images, so a disconnect after the first completed event keeps
// the requested n instead of dropping it to 1.
func TestOpenaiImageStreamHandlerClientDisconnectKeepsRequestedCount(t *testing.T) {
	oldMode := gin.Mode()
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() { gin.SetMode(oldMode) })

	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = oldTimeout })

	body := "data: {\"type\":\"image_generation.completed\",\"b64_json\":\"first\"}\n\n"
	c, recorder, resp, info := newDisconnectingImageStream(t, body, "first")
	info.PriceData.UsePrice = true
	info.PriceData.AddOtherRatio("n", 3)

	usage, err := OpenaiImageStreamHandler(c, info, resp)

	require.Nil(t, err)
	require.NotNil(t, usage)
	require.NotNil(t, info.StreamStatus)
	// A client abort surfaces as client_gone (main-loop ctx watch) or
	// handler_stop (failed client write); both must be treated as untrusted.
	require.Contains(t,
		[]relaycommon.StreamEndReason{relaycommon.StreamEndReasonClientGone, relaycommon.StreamEndReasonHandlerStop},
		info.StreamStatus.EndReason)
	require.Contains(t, recorder.Body.String(), `"b64_json":"first"`)
	require.Equal(t, 3.0, info.PriceData.OtherRatios()["n"], "client abort must not reduce the billed image count")
}

// TestOpenaiImageStreamHandlerClientDisconnectRaisesCount covers the other
// direction of the abort guard: when completed events already exceed the
// recorded n, the higher actual count is billed even though the client aborted.
func TestOpenaiImageStreamHandlerClientDisconnectRaisesCount(t *testing.T) {
	oldMode := gin.Mode()
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() { gin.SetMode(oldMode) })

	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = oldTimeout })

	body := strings.Join([]string{
		`data: {"type":"image_generation.completed","b64_json":"first"}`,
		``,
		`data: {"type":"image_generation.completed","b64_json":"second"}`,
		``,
		``,
	}, "\n")
	c, _, resp, info := newDisconnectingImageStream(t, body, "second")
	info.PriceData.UsePrice = true
	info.PriceData.AddOtherRatio("n", 1)

	usage, err := OpenaiImageStreamHandler(c, info, resp)

	require.Nil(t, err)
	require.NotNil(t, usage)
	require.NotNil(t, info.StreamStatus)
	require.Contains(t,
		[]relaycommon.StreamEndReason{relaycommon.StreamEndReasonClientGone, relaycommon.StreamEndReasonHandlerStop},
		info.StreamStatus.EndReason)
	require.Equal(t, 2.0, info.PriceData.OtherRatios()["n"], "completed events beyond the recorded n must raise the charge even on abort")
}

// TestOpenaiImageStreamHandlerWrapsJSONResponse covers the non-SSE fallback:
// a JSON upstream response is wrapped into pseudo-SSE completed events.
func TestOpenaiImageStreamHandlerWrapsJSONResponse(t *testing.T) {
	oldMode := gin.Mode()
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() { gin.SetMode(oldMode) })

	body := `{"created":1710000000,"data":[{"b64_json":"first","revised_prompt":"draw a cat"},{"b64_json":"second"}],"usage":{"input_tokens":3,"output_tokens":4,"total_tokens":7,"input_tokens_details":{"image_tokens":2,"text_tokens":1}}}`

	c, recorder, resp, info := newImageTestContext(t, body, "application/json", true)
	info.PriceData.UsePrice = true
	info.PriceData.AddOtherRatio("n", 3)

	usage, err := OpenaiImageStreamHandler(c, info, resp)
	require.Nil(t, err)
	require.Equal(t, 3, usage.PromptTokens)
	require.Equal(t, 4, usage.CompletionTokens)
	require.Equal(t, 7, usage.TotalTokens)
	require.Equal(t, 2, usage.PromptTokensDetails.ImageTokens)
	require.Equal(t, 1, usage.PromptTokensDetails.TextTokens)
	require.Equal(t, "text/event-stream", recorder.Header().Get("Content-Type"))
	require.Empty(t, recorder.Header().Get("Content-Length"))
	require.Contains(t, recorder.Body.String(), `event: image_generation.completed`)
	require.Contains(t, recorder.Body.String(), `"type":"image_generation.completed"`)
	require.Contains(t, recorder.Body.String(), `"b64_json":"first"`)
	require.Contains(t, recorder.Body.String(), `"b64_json":"second"`)
	require.Contains(t, recorder.Body.String(), `"revised_prompt":"draw a cat"`)
	require.Contains(t, recorder.Body.String(), `data: [DONE]`)
	require.Equal(t, 2, strings.Count(recorder.Body.String(), `event: image_generation.completed`))
	require.Equal(t, 2.0, info.PriceData.OtherRatios()["n"])
}

func TestOpenaiImageHandlerUsesPositiveActualCountForFixedPrice(t *testing.T) {
	oldMode := gin.Mode()
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() { gin.SetMode(oldMode) })
	longImage := strings.Repeat("a", 4096)

	tests := []struct {
		name      string
		body      string
		usePrice  bool
		wantCount float64
	}{
		{
			name:      "fixed price uses data length",
			body:      `{"data":[{"b64_json":"` + longImage + `"},{"b64_json":"second"}]}`,
			usePrice:  true,
			wantCount: 2,
		},
		{
			name:      "empty data keeps requested count",
			body:      `{"data":[]}`,
			usePrice:  true,
			wantCount: 3,
		},
		{
			name:      "ratio billing ignores data length",
			body:      `{"data":[{"b64_json":"first"},{"b64_json":"second"}]}`,
			usePrice:  false,
			wantCount: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, recorder, resp, info := newImageTestContext(t, tt.body, "application/json", false)
			info.PriceData.UsePrice = tt.usePrice
			info.PriceData.AddOtherRatio("n", 3)

			_, err := OpenaiImageHandler(c, info, resp)

			require.Nil(t, err)
			require.Equal(t, tt.wantCount, info.PriceData.OtherRatios()["n"])
			require.Equal(t, tt.body, recorder.Body.String())
		})
	}
}

func TestOpenaiImageHandlerReturnsBase64UnlessURLRequested(t *testing.T) {
	service.InitHttpClient()
	oldStreamingTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = oldStreamingTimeout })
	oldMaxFileDownloadMB := constant.MaxFileDownloadMB
	constant.MaxFileDownloadMB = 1
	t.Cleanup(func() { constant.MaxFileDownloadMB = oldMaxFileDownloadMB })
	oldFetchSetting := *system_setting.GetFetchSetting()
	system_setting.GetFetchSetting().EnableSSRFProtection = false
	t.Cleanup(func() { *system_setting.GetFetchSetting() = oldFetchSetting })

	imageServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte("image"))
	}))
	t.Cleanup(imageServer.Close)

	for _, tc := range []struct {
		name, responseFormat string
		wantURL              bool
	}{
		{name: "default response is base64"},
		{name: "explicit base64 response is base64", responseFormat: "b64_json"},
		{name: "explicit url response keeps url", responseFormat: "url", wantURL: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := fmt.Sprintf(`{"data":[{"url":%q,"revised_prompt":"draw a cat"}]}`, imageServer.URL+"/image.png")
			c, recorder, resp, info := newImageTestContext(t, body, "application/json", false)
			info.Request = &dto.ImageRequest{ResponseFormat: tc.responseFormat}

			_, apiErr := OpenaiImageHandler(c, info, resp)

			require.Nil(t, apiErr)
			if tc.wantURL {
				require.Contains(t, recorder.Body.String(), `"url"`)
				require.NotContains(t, recorder.Body.String(), `"b64_json"`)
				return
			}
			require.Contains(t, recorder.Body.String(), `"b64_json":"aW1hZ2U="`)
			require.NotContains(t, recorder.Body.String(), `"url"`)
			require.Contains(t, recorder.Body.String(), `"revised_prompt":"draw a cat"`)
		})
	}

	t.Run("streamed url is returned as base64", func(t *testing.T) {
		body := fmt.Sprintf("data: {\"type\":\"image_generation.completed\",\"url\":%q}\n\ndata: [DONE]\n\n", imageServer.URL+"/image.png")
		c, recorder, resp, info := newImageTestContext(t, body, "text/event-stream", true)
		info.Request = &dto.ImageRequest{ResponseFormat: "b64_json"}

		_, apiErr := OpenaiImageStreamHandler(c, info, resp)

		require.Nil(t, apiErr)
		require.Contains(t, recorder.Body.String(), `"b64_json":"aW1hZ2U="`)
		require.NotContains(t, recorder.Body.String(), `"url"`)
		require.Contains(t, recorder.Body.String(), `data: [DONE]`)
	})
}

// TestOpenaiImageHandlersReturnJSONError covers JSON error responses for both
// entry points: the non-streaming handler and the stream handler's non-SSE
// fallback. Neither must leak the error body to the client.
func TestOpenaiImageHandlersReturnJSONError(t *testing.T) {
	oldMode := gin.Mode()
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() { gin.SetMode(oldMode) })

	body := `{"error":{"message":"content moderation failed","type":"upstream_error","code":"content_moderation_failed","status":502}}`

	t.Run("non-streaming handler", func(t *testing.T) {
		c, recorder, resp, info := newImageTestContext(t, body, "application/json", false)

		usage, err := OpenaiImageHandler(c, info, resp)
		require.Nil(t, usage)
		require.NotNil(t, err)
		require.Equal(t, http.StatusOK, err.StatusCode)
		oaiError := err.ToOpenAIError()
		require.Equal(t, "content moderation failed", oaiError.Message)
		require.Equal(t, "upstream_error", oaiError.Type)
		require.Equal(t, "content_moderation_failed", oaiError.Code)
		require.Empty(t, recorder.Body.String())
	})

	t.Run("stream handler JSON fallback", func(t *testing.T) {
		c, recorder, resp, info := newImageTestContext(t, body, "application/json", true)

		usage, err := OpenaiImageStreamHandler(c, info, resp)
		require.Nil(t, usage)
		require.NotNil(t, err)
		require.Equal(t, http.StatusOK, err.StatusCode)
		require.Equal(t, "content moderation failed", err.ToOpenAIError().Message)
		require.Empty(t, recorder.Body.String())
	})

	t.Run("stream handler non-2xx stays JSON error", func(t *testing.T) {
		c, recorder, resp, info := newImageTestContext(t, body, "application/json", true)
		resp.StatusCode = http.StatusBadGateway

		usage, err := OpenaiImageStreamHandler(c, info, resp)
		require.Nil(t, usage)
		require.NotNil(t, err)
		require.Equal(t, http.StatusBadGateway, err.StatusCode)
		require.Equal(t, "content moderation failed", err.ToOpenAIError().Message)
		require.Empty(t, recorder.Body.String())
		require.NotContains(t, recorder.Header().Get("Content-Type"), "text/event-stream")
	})
}

// TestOpenaiImageStreamHandlerRecordsUpstreamErrorEvent verifies that an error
// event inside the SSE stream is retained for diagnostics without exposing the
// upstream payload to the client.
func TestOpenaiImageStreamHandlerRecordsUpstreamErrorEvent(t *testing.T) {
	oldMode := gin.Mode()
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() { gin.SetMode(oldMode) })

	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = oldTimeout })

	body := strings.Join([]string{
		`event: image_generation.partial_image`,
		`data: {"type":"image_generation.partial_image","b64_json":"partial"}`,
		``,
		`event: error`,
		`data: {"type":"upstream_error","error":{"message":"stream error: stream ID 77; INTERNAL_ERROR; received from peer"}}`,
		``,
	}, "\n")

	c, recorder, resp, info := newImageTestContext(t, body, "text/event-stream", true)

	usage, err := OpenaiImageStreamHandler(c, info, resp)
	require.Nil(t, err)
	require.NotNil(t, usage)
	require.NotNil(t, info.StreamStatus)
	require.Equal(t, relaycommon.StreamEndReasonEOF, info.StreamStatus.EndReason)
	require.True(t, info.StreamStatus.HasErrors())
	require.Equal(t, 1, info.StreamStatus.TotalErrorCount())
	require.Contains(t, info.StreamStatus.Errors[0].Message, "INTERNAL_ERROR")
	require.Contains(t, recorder.Body.String(), `event: error`)
	require.Contains(t, recorder.Body.String(), `The upstream service is temporarily unavailable`)
	require.NotContains(t, recorder.Body.String(), `stream ID 77`)
}

func TestOaiStreamHandlerDoesNotForwardUpstreamError(t *testing.T) {
	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = oldTimeout })

	body := "data: {\"error\":{\"message\":\"provider secret error\",\"type\":\"provider_error\",\"code\":\"secret_code\"}}\n\n"
	c, recorder, resp, info := newImageTestContext(t, body, "text/event-stream", true)
	info.RelayFormat = types.RelayFormatOpenAI

	usage, err := OaiStreamHandler(c, info, resp)

	require.Nil(t, err)
	require.NotNil(t, usage)
	require.Contains(t, recorder.Body.String(), "The upstream service is temporarily unavailable")
	require.NotContains(t, recorder.Body.String(), "provider secret error")
	require.NotContains(t, recorder.Body.String(), "secret_code")
}

func TestImageGIFRequestPreservesOfficialAnimationFields(t *testing.T) {
	for _, model := range []string{"gpt-image-2.6-flare", "gpt-image-2.5-sunburst"} {
		t.Run(model, func(t *testing.T) {
			body := fmt.Sprintf(`{"model":%q,"prompt":"a waving owl, seamless loop","output_format":"gif","frame_count":9,"frame_duration_ms":200,"complexity":"medium","n":1}`, model)
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", strings.NewReader(body))
			c.Request.Header.Set("Content-Type", "application/json")
			request, err := helper.GetAndValidOpenAIImageRequest(c, relayconstant.RelayModeImagesGenerations)
			require.NoError(t, err)
			request, err = common.DeepCopy(request)
			require.NoError(t, err)
			info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ChannelType: constant.ChannelTypeOpenAI, ChannelBaseUrl: "https://api.openai.com"}, RelayMode: relayconstant.RelayModeImagesGenerations}
			converted, err := (&Adaptor{}).ConvertImageRequest(c, info, *request)
			require.NoError(t, err)
			outbound, err := common.Marshal(converted)
			require.NoError(t, err)
			var fields map[string]any
			require.NoError(t, common.Unmarshal(outbound, &fields))
			assert.Equal(t, "gif", fields["output_format"])
			assert.Equal(t, float64(9), fields["frame_count"])
			assert.Equal(t, float64(200), fields["frame_duration_ms"])
			assert.Equal(t, "medium", fields["complexity"])
			assert.Equal(t, float64(1), fields["n"])
			assert.NotContains(t, fields, "response_format")
		})
	}
}

func TestImageGIFResponsesRetainAnimationAndFormat(t *testing.T) {
	var animation bytes.Buffer
	palette := color.Palette{color.Black, color.White}
	first := image.NewPaletted(image.Rect(0, 0, 2, 2), palette)
	second := image.NewPaletted(first.Rect, palette)
	second.Pix[0] = 1
	require.NoError(t, gif.EncodeAll(&animation, &gif.GIF{Image: []*image.Paletted{first, second}, Delay: []int{10, 20}, LoopCount: 0}))
	encoded := base64.StdEncoding.EncodeToString(animation.Bytes())
	service.InitHttpClient()
	previousLimit, previousTimeout := constant.MaxFileDownloadMB, constant.StreamingTimeout
	constant.MaxFileDownloadMB, constant.StreamingTimeout = 1, 30
	previousFetch := *system_setting.GetFetchSetting()
	system_setting.GetFetchSetting().EnableSSRFProtection = false
	t.Cleanup(func() {
		constant.MaxFileDownloadMB, constant.StreamingTimeout = previousLimit, previousTimeout
		*system_setting.GetFetchSetting() = previousFetch
	})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/gif")
		_, _ = w.Write(animation.Bytes())
	}))
	t.Cleanup(server.Close)
	for _, source := range []string{"base64", "url"} {
		for _, bridge := range []string{"images", "chat", "responses"} {
			for _, wire := range []string{"json", "json-as-sse", "sse", "object-as-sse"} {
				stream := wire != "json"
				t.Run(fmt.Sprintf("%s/%s/%s", source, bridge, wire), func(t *testing.T) {
					payload := fmt.Sprintf(`{"b64_json":%q}`, encoded)
					if source == "url" {
						payload = fmt.Sprintf(`{"url":%q}`, server.URL+"/animation.gif")
					}
					body := `{"output_format":"gif","data":[` + payload + `]}`
					contentType := "application/json"
					if wire == "sse" {
						body = "data: " + `{"type":"image_generation.completed","output_format":"gif",` + payload[1:] + "\n\ndata: [DONE]\n\n"
						contentType = "text/event-stream"
					}
					if wire == "object-as-sse" {
						body = `{"output_format":"gif","data":` + payload + `}`
					}
					c, recorder, resp, info := newImageTestContext(t, body, contentType, stream)
					info.Request = &dto.ImageRequest{ResponseFormat: "b64_json"}
					info.PriceData.UsePrice = true
					info.PriceData.AddOtherRatio("n", 1)
					if bridge == "chat" {
						info.RelayFormat = types.RelayFormatOpenAI
						c.Set("chat_image_completion_bridge", true)
					}
					if bridge == "responses" {
						info.RelayFormat = types.RelayFormatOpenAIResponses
						c.Set("responses_image_generation_bridge", true)
					}
					var apiErr *types.NewAPIError
					if stream {
						_, apiErr = OpenaiImageStreamHandler(c, info, resp)
					} else {
						_, apiErr = OpenaiImageHandler(c, info, resp)
					}
					require.Nil(t, apiErr)
					output := recorder.Body.String()
					require.Contains(t, output, encoded)
					if bridge == "chat" {
						assert.Contains(t, output, "data:image/gif;base64,")
					} else {
						assert.Contains(t, output, `"output_format":"gif"`)
					}
					assert.NotContains(t, output, "image/png")
					assert.NotContains(t, output, "image/jpeg")
					assert.Equal(t, float64(1), info.PriceData.OtherRatios()["n"], "animation frames are not separate images")
					start := strings.Index(output, encoded)
					decoded, err := base64.StdEncoding.DecodeString(output[start : start+len(encoded)])
					require.NoError(t, err)
					assert.Equal(t, animation.Bytes(), decoded)
					actual, err := gif.DecodeAll(bytes.NewReader(decoded))
					require.NoError(t, err)
					assert.Len(t, actual.Image, 2)
					assert.Equal(t, []int{10, 20}, actual.Delay)
				})
			}
		}
	}
}

func TestImageChatPreservesRasterFormats(t *testing.T) {
	for _, format := range []string{"png", "jpeg", "webp", "gif"} {
		t.Run(format, func(t *testing.T) {
			var payload bytes.Buffer
			frame := image.NewRGBA(image.Rect(0, 0, 2, 2))
			switch format {
			case "png":
				require.NoError(t, png.Encode(&payload, frame))
			case "jpeg":
				require.NoError(t, jpeg.Encode(&payload, frame, nil))
			case "gif":
				data, err := base64.StdEncoding.DecodeString("R0lGODlhAgACAAAAACH/C05FVFNDQVBFMi4wAwEAAAAh+QQACgAAACwAAAAAAgACAIAAAAD///8CAoRRACH5BAAUAAAALAAAAAACAAIAgAAAAP///wICDF4AOw==")
				require.NoError(t, err)
				_, err = payload.Write(data)
				require.NoError(t, err)
			case "webp":
				data, err := base64.StdEncoding.DecodeString("UklGRjgAAABXRUJQVlA4ICwAAADQAQCdASoCAAIAAUAmJaACdLoB+AADsAD+8iJf/LPn8NnHv/5BcsLriMAAAA==")
				require.NoError(t, err)
				_, err = payload.Write(data)
				require.NoError(t, err)
			}
			encoded := base64.StdEncoding.EncodeToString(payload.Bytes())
			if format == "gif" {
				encoded = encoded[:30] + "\n" + encoded[30:]
			}
			// A stale upstream label must not override the actual file signature.
			body := fmt.Sprintf(`{"output_format":"png","data":[{"b64_json":%q}]}`, encoded)
			c, recorder, resp, info := newImageTestContext(t, body, "application/json", false)
			info.RelayFormat = types.RelayFormatOpenAI
			c.Set("chat_image_completion_bridge", true)
			_, apiErr := OpenaiImageHandler(c, info, resp)
			require.Nil(t, apiErr)
			var result dto.OpenAITextResponse
			require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &result))
			require.Len(t, result.Choices, 1)
			assert.Equal(t, "![generated image](data:image/"+format+";base64,"+encoded+")", result.Choices[0].Message.Content)
		})
	}
}
