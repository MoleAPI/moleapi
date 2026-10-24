package channel

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAPIRequestDropsLegacyModelParameters(t *testing.T) {
	const input = `{"model":"public-alias","temperature":0,"temperature":0.7,"top_p":0,"top_k":40,"frequency_penalty":0,"presence_penalty":1,"repetition_penalty":1.1,"seed":9007199254740993,"logit_bias":{"123":5},"logprobs":true,"top_logprobs":3,"stop":["end"],"max_tokens":512,"max_output_tokens":1024,"reasoning":{"effort":"high"},"messages":[{"role":"user","content":"hi"}],"metadata":{"temperature":0.4,"id":9007199254740993}}`
	for _, tc := range []struct {
		model  string
		drop   bool
		stop   bool
		origin string
	}{
		{"claude-sonnet-4-6", true, false, ""},
		{"claude-opus-4-7", true, false, ""},
		{"anthropic/claude-3-5-sonnet", true, false, ""},
		{"us.anthropic.claude-opus-4-7-v1:0", true, false, ""},
		{"gpt-5", true, true, ""},
		{"gpt-5.1-chat-latest", true, true, ""},
		{"openai/gpt-5.2-codex", true, true, ""},
		{"GPT-6", true, true, ""},
		{"gpt-10-mini", true, true, ""},
		{"gpt-4.1", false, false, ""},
		{"gpt-4o", false, false, ""},
		{"gemini-3-pro", false, false, ""},
		{"gpt-5fake", false, false, ""},
		{"myclaude-model", false, false, ""},
		{"azure-deployment", true, true, "gpt-5.2"},
		{"private-alias", true, false, "claude-opus-4-7"},
	} {
		for _, path := range []string{"/v1/chat/completions", "/v1/messages", "/v1/responses"} {
			t.Run(tc.model+path, func(t *testing.T) {
				receivedBodies := make(chan []byte, 1)
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					received, err := io.ReadAll(r.Body)
					assert.NoError(t, err)
					assert.Equal(t, int64(len(received)), r.ContentLength)
					receivedBodies <- received
					w.WriteHeader(http.StatusOK)
				}))
				defer server.Close()
				c, _ := gin.CreateTestContext(httptest.NewRecorder())
				c.Request = httptest.NewRequest(http.MethodPost, path, strings.NewReader(input))
				info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: tc.model,
					ChannelSetting: dto.ChannelSettings{PassThroughBodyEnabled: true}}}
				info.OriginModelName = tc.origin
				resp, err := DoApiRequest(requestContextAdaptor{url: server.URL}, c, info, strings.NewReader(input))
				require.NoError(t, err)
				defer resp.Body.Close()
				received := <-receivedBodies
				var fields map[string]common.RawMessage
				require.NoError(t, common.Unmarshal(received, &fields))
				for _, key := range []string{"temperature", "top_p", "top_k", "frequency_penalty", "presence_penalty", "repetition_penalty", "seed", "logit_bias", "logprobs", "top_logprobs"} {
					_, exists := fields[key]
					assert.Equal(t, !tc.drop, exists, key)
				}
				_, hasStop := fields["stop"]
				assert.Equal(t, !tc.stop, hasStop)
				assert.JSONEq(t, `{"temperature":0.4,"id":9007199254740993}`, string(fields["metadata"]))
				assert.Equal(t, "512", string(fields["max_tokens"]))
				assert.Equal(t, "1024", string(fields["max_output_tokens"]))
				assert.JSONEq(t, `{"effort":"high"}`, string(fields["reasoning"]))
				assert.JSONEq(t, `[{"role":"user","content":"hi"}]`, string(fields["messages"]))
				if !tc.drop {
					assert.Equal(t, input, string(received))
				}
			})
		}
	}
}

func TestAPIRequestRejectsInvalidSamplingBodyBeforeSending(t *testing.T) {
	for _, body := range []string{`{"temperature":`, `null`, `[]`} {
		t.Run(body, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(body))
			info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "claude-opus-4-7"}}
			// An empty adaptor URL proves rejection precedes request construction.
			_, err := DoApiRequest(requestContextAdaptor{}, c, info, strings.NewReader(body))
			require.ErrorContains(t, err, "filter model sampling parameters")
		})
	}
}

type requestContextAdaptor struct {
	Adaptor
	url string
}

type openCodeHeaderAdaptor struct{ requestContextAdaptor }

func (openCodeHeaderAdaptor) SetupRequestHeader(c *gin.Context, headers *http.Header, info *relaycommon.RelayInfo) error {
	SetupApiRequestHeader(info, c, headers)
	return nil
}

func (a requestContextAdaptor) GetRequestURL(_ *relaycommon.RelayInfo) (string, error) {
	return a.url, nil
}

func (requestContextAdaptor) SetupRequestHeader(_ *gin.Context, _ *http.Header, _ *relaycommon.RelayInfo) error {
	return nil
}

func TestAPIRequestInheritsClientCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	requestContext, cancel := context.WithCancel(context.Background())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil).WithContext(requestContext)
	cancel()

	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{}}
	_, err := DoApiRequest(requestContextAdaptor{url: server.URL}, c, info, strings.NewReader("{}"))
	require.Error(t, err)
	var apiErr *types.NewAPIError
	require.ErrorAs(t, err, &apiErr)
	assert.True(t, types.IsSkipRetryError(apiErr))
}

func TestAPIRequestForwardsOpenCodeSessionHeaders(t *testing.T) {
	received := make(chan http.Header, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received <- r.Header.Clone()
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader("{}"))
	c.Request.Header.Set("x-opencode-session", "session-123")
	c.Request.Header.Set("x-opencode-request", "request-456")
	c.Request.Header.Set("x-opencode-project", "project-789")
	c.Request.Header.Set("x-opencode-client", "client-test")

	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ChannelOtherSettings: dto.ChannelOtherSettings{CodingPlanProvider: dto.CodingPlanProviderOpenCodeGo}}}
	_, err := DoApiRequest(openCodeHeaderAdaptor{requestContextAdaptor{url: server.URL}}, c, info, strings.NewReader("{}"))
	require.NoError(t, err)
	headers := <-received
	require.Equal(t, "session-123", headers.Get("x-opencode-session"))
	require.Equal(t, "request-456", headers.Get("x-opencode-request"))
	require.Equal(t, "project-789", headers.Get("x-opencode-project"))
	require.Equal(t, "client-test", headers.Get("x-opencode-client"))
}

func TestAPIRequestAddsOpaqueStableOpenCodeSessionHeader(t *testing.T) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	info := &relaycommon.RelayInfo{UserId: 7, TokenId: 11, ChannelMeta: &relaycommon.ChannelMeta{ChannelOtherSettings: dto.ChannelOtherSettings{CodingPlanProvider: dto.CodingPlanProviderOpenCodeGo}}}

	first := http.Header{}
	SetupApiRequestHeader(info, c, &first)
	second := http.Header{}
	SetupApiRequestHeader(info, c, &second)

	require.Regexp(t, `^[0-9a-f]{32}$`, first.Get("x-opencode-session"))
	require.Equal(t, first.Get("x-opencode-session"), second.Get("x-opencode-session"))
	info.TokenId = 12
	differentToken := http.Header{}
	SetupApiRequestHeader(info, c, &differentToken)
	assert.NotEqual(t, first.Get("x-opencode-session"), differentToken.Get("x-opencode-session"))

	c.Request.Header.Set("x-opencode-session", "client-session")
	c.Request.Header.Set("x-opencode-request", "client-request")
	c.Request.Header.Set("x-opencode-project", "client-project")
	c.Request.Header.Set("x-opencode-client", "client-name")
	otherProvider := http.Header{}
	info.ChannelOtherSettings.CodingPlanProvider = dto.CodingPlanProviderKimi
	SetupApiRequestHeader(info, c, &otherProvider)
	for _, name := range []string{"x-opencode-session", "x-opencode-request", "x-opencode-project", "x-opencode-client"} {
		require.Empty(t, otherProvider.Get(name))
	}
}

func TestNewTaskAPIRequestInheritsClientCancellation(t *testing.T) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	requestContext, cancel := context.WithCancel(context.Background())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil).WithContext(requestContext)

	upstream, err := newTaskAPIRequest(c, "https://provider.example/tasks", nil)
	require.NoError(t, err)
	cancel()

	require.ErrorIs(t, upstream.Context().Err(), context.Canceled)
}

func TestProcessHeaderOverride_ChannelTestSkipsPassthroughRules(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	ctx.Request.Header.Set("X-Trace-Id", "trace-123")

	info := &relaycommon.RelayInfo{
		IsChannelTest: true,
		ChannelMeta: &relaycommon.ChannelMeta{
			HeadersOverride: map[string]any{
				"*": "",
			},
		},
	}

	headers, err := processHeaderOverride(info, ctx)
	require.NoError(t, err)
	require.Empty(t, headers)
}

func TestProcessHeaderOverride_ChannelTestSkipsClientHeaderPlaceholder(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	ctx.Request.Header.Set("X-Trace-Id", "trace-123")

	info := &relaycommon.RelayInfo{
		IsChannelTest: true,
		ChannelMeta: &relaycommon.ChannelMeta{
			HeadersOverride: map[string]any{
				"X-Upstream-Trace": "{client_header:X-Trace-Id}",
			},
		},
	}

	headers, err := processHeaderOverride(info, ctx)
	require.NoError(t, err)
	_, ok := headers["x-upstream-trace"]
	require.False(t, ok)
}

func TestProcessHeaderOverride_NonTestKeepsClientHeaderPlaceholder(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	ctx.Request.Header.Set("X-Trace-Id", "trace-123")

	info := &relaycommon.RelayInfo{
		IsChannelTest: false,
		ChannelMeta: &relaycommon.ChannelMeta{
			HeadersOverride: map[string]any{
				"X-Upstream-Trace": "{client_header:X-Trace-Id}",
			},
		},
	}

	headers, err := processHeaderOverride(info, ctx)
	require.NoError(t, err)
	require.Equal(t, "trace-123", headers["x-upstream-trace"])
}

func TestProcessHeaderOverride_RuntimeOverrideIsFinalHeaderMap(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	info := &relaycommon.RelayInfo{
		IsChannelTest:             false,
		UseRuntimeHeadersOverride: true,
		RuntimeHeadersOverride: map[string]any{
			"x-static":  "runtime-value",
			"x-runtime": "runtime-only",
		},
		ChannelMeta: &relaycommon.ChannelMeta{
			HeadersOverride: map[string]any{
				"X-Static": "legacy-value",
				"X-Legacy": "legacy-only",
			},
		},
	}

	headers, err := processHeaderOverride(info, ctx)
	require.NoError(t, err)
	require.Equal(t, "runtime-value", headers["x-static"])
	require.Equal(t, "runtime-only", headers["x-runtime"])
	_, exists := headers["x-legacy"]
	require.False(t, exists)
}

func TestProcessHeaderOverride_PassthroughSkipsAcceptEncoding(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	ctx.Request.Header.Set("X-Trace-Id", "trace-123")
	ctx.Request.Header.Set("Accept-Encoding", "gzip")

	info := &relaycommon.RelayInfo{
		IsChannelTest: false,
		ChannelMeta: &relaycommon.ChannelMeta{
			HeadersOverride: map[string]any{
				"*": "",
			},
		},
	}

	headers, err := processHeaderOverride(info, ctx)
	require.NoError(t, err)
	require.Equal(t, "trace-123", headers["x-trace-id"])

	_, hasAcceptEncoding := headers["accept-encoding"]
	require.False(t, hasAcceptEncoding)
}

func TestProcessHeaderOverride_PassHeadersTemplateSetsRuntimeHeaders(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	ctx.Request.Header.Set("Originator", "Codex CLI")
	ctx.Request.Header.Set("Session_id", "sess-123")

	info := &relaycommon.RelayInfo{
		IsChannelTest: false,
		RequestHeaders: map[string]string{
			"Originator": "Codex CLI",
			"Session_id": "sess-123",
		},
		ChannelMeta: &relaycommon.ChannelMeta{
			ParamOverride: map[string]any{
				"operations": []any{
					map[string]any{
						"mode":  "pass_headers",
						"value": []any{"Originator", "Session_id", "X-Codex-Beta-Features"},
					},
				},
			},
			HeadersOverride: map[string]any{
				"X-Static": "legacy-value",
			},
		},
	}

	_, err := relaycommon.ApplyParamOverrideWithRelayInfo([]byte(`{"model":"gpt-4.1"}`), info)
	require.NoError(t, err)
	require.True(t, info.UseRuntimeHeadersOverride)
	require.Equal(t, "Codex CLI", info.RuntimeHeadersOverride["originator"])
	require.Equal(t, "sess-123", info.RuntimeHeadersOverride["session_id"])
	_, exists := info.RuntimeHeadersOverride["x-codex-beta-features"]
	require.False(t, exists)
	require.Equal(t, "legacy-value", info.RuntimeHeadersOverride["x-static"])

	headers, err := processHeaderOverride(info, ctx)
	require.NoError(t, err)
	require.Equal(t, "Codex CLI", headers["originator"])
	require.Equal(t, "sess-123", headers["session_id"])
	_, exists = headers["x-codex-beta-features"]
	require.False(t, exists)

	upstreamReq := httptest.NewRequest(http.MethodPost, "https://example.com/v1/responses", nil)
	applyHeaderOverrideToRequest(upstreamReq, headers)
	require.Equal(t, "Codex CLI", upstreamReq.Header.Get("Originator"))
	require.Equal(t, "sess-123", upstreamReq.Header.Get("Session_id"))
	require.Empty(t, upstreamReq.Header.Get("X-Codex-Beta-Features"))
}
