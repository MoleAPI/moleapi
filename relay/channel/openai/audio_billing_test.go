package openai

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAudioHandlersPreserveBillableModalities(t *testing.T) {
	previousTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = previousTimeout })
	for _, tc := range []struct {
		name, body                           string
		speech                               bool
		input, output, text, audio, audioOut int
	}{
		{"speech stream", "data: {\"type\":\"speech.audio.done\",\"usage\":{\"input_tokens\":14,\"output_tokens\":100,\"total_tokens\":114}}\n\n", true, 14, 100, 14, 0, 100},
		{"transcription", `{"text":"hello","usage":{"type":"tokens","input_tokens":14,"input_token_details":{"text_tokens":10,"audio_tokens":4},"output_tokens":101,"total_tokens":115}}`, false, 14, 101, 10, 4, 0},
		{"transcription stream", "data: {\"type\":\"transcript.text.done\",\"usage\":{\"input_tokens\":14,\"input_token_details\":{\"text_tokens\":10,\"audio_tokens\":4},\"output_tokens\":101,\"total_tokens\":115}}\n\n", false, 14, 101, 10, 4, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			writer := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(writer)
			ctx.Request = httptest.NewRequest("POST", "/v1/audio", nil)
			info := &relaycommon.RelayInfo{StartTime: time.Now(), IsStream: tc.speech}
			response := &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(tc.body))}
			if strings.HasPrefix(tc.body, "data:") {
				response.Header.Set("Content-Type", "text/event-stream")
			}
			var usage *dto.Usage
			if tc.speech {
				usage = OpenaiTTSHandler(ctx, response, info)
			} else {
				apiErr, got := OpenaiSTTHandler(ctx, response, info, "json")
				require.Nil(t, apiErr)
				usage = got
			}
			require.NotNil(t, usage)
			assert.Equal(t, tc.input, usage.PromptTokens)
			assert.Equal(t, tc.output, usage.CompletionTokens)
			assert.Equal(t, tc.text, usage.PromptTokensDetails.TextTokens)
			assert.Equal(t, tc.audio, usage.PromptTokensDetails.AudioTokens)
			assert.Equal(t, tc.audioOut, usage.CompletionTokenDetails.AudioTokens)
			assert.Equal(t, tc.output-tc.audioOut, usage.CompletionTokenDetails.TextTokens)
			assert.Contains(t, writer.Body.String(), "usage")
		})
	}
}

type audioReservationRecorder struct {
	relaycommon.BillingSettler
	quotas []int
}

func (r *audioReservationRecorder) Reserve(quota int) error {
	r.quotas = append(r.quotas, quota)
	return nil
}

func TestRealtimeConnectionReservesCumulativeAudioUsage(t *testing.T) {
	upgrader := websocket.Upgrader{}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		for range 2 {
			if conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"response.done","response":{"usage":{"input_tokens":14,"output_tokens":100,"total_tokens":114,"input_token_details":{"text_tokens":10,"audio_tokens":4,"cached_tokens":3,"cached_tokens_details":{"text_tokens":1,"audio_tokens":2}},"output_token_details":{"text_tokens":10,"audio_tokens":90}}}}`)) != nil {
				return
			}
		}
		_, _, _ = conn.ReadMessage()
	}))
	defer upstream.Close()
	reservations := &audioReservationRecorder{}
	result := make(chan *dto.RealtimeUsage, 1)
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		client, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			result <- nil
			return
		}
		defer client.Close()
		target, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(upstream.URL, "http"), nil)
		if err != nil {
			result <- nil
			return
		}
		defer target.Close()
		ctx, _ := gin.CreateTestContext(w)
		ctx.Request = r
		info := &relaycommon.RelayInfo{ClientWs: client, TargetWs: target, Billing: reservations, StartTime: time.Now(),
			TieredBillingSnapshot: &billingexpr.BillingSnapshot{BillingMode: "tiered_expr", ExprString: `p * 2 + c * 4 + ai * 10 + ao * 20 + cr * 0.5`, GroupRatio: 1, QuotaPerUnit: 500000}}
		_, usage := OpenaiRealtimeHandler(ctx, info)
		result <- usage
	}))
	defer proxy.Close()
	client, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(proxy.URL, "http"), nil)
	require.NoError(t, err)
	defer client.Close()
	require.NoError(t, client.SetReadDeadline(time.Now().Add(5*time.Second)))
	for range 2 {
		_, message, err := client.ReadMessage()
		require.NoError(t, err)
		assert.Contains(t, string(message), "response.done")
	}
	require.NoError(t, client.Close())
	select {
	case usage := <-result:
		require.NotNil(t, usage)
		assert.Equal(t, 228, usage.TotalTokens)
		assert.Equal(t, 8, usage.InputTokenDetails.AudioTokens)
		assert.Equal(t, 180, usage.OutputTokenDetails.AudioTokens)
		require.NotNil(t, usage.InputTokenDetails.CachedTokensDetails)
		assert.Equal(t, 4, *usage.InputTokenDetails.CachedTokensDetails.AudioTokens)
		assert.Equal(t, []int{940, 1880}, reservations.quotas)
	case <-time.After(5 * time.Second):
		t.Fatal("closing the client must stop both readers before settlement")
	}
}
