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

func TestAPIRequestNormalizesModelReasoning(t *testing.T) {
	for _, tc := range []struct {
		name, model string
		format      types.RelayFormat
		input, want string
	}{
		{"gpt unknown", "gpt-5", types.RelayFormatOpenAI, `{"reasoning_effort":"unusual"}`, `{"reasoning_effort":"medium"}`},
		{"gpt5 ceiling", "gpt-5", types.RelayFormatOpenAI, `{"reasoning_effort":"max"}`, `{"reasoning_effort":"high"}`},
		{"gpt5 off floor", "gpt-5", types.RelayFormatOpenAI, `{"reasoning_effort":"off"}`, `{"reasoning_effort":"minimal"}`},
		{"gpt51 ceiling", "gpt-5.1", types.RelayFormatOpenAI, `{"reasoning_effort":"xhigh"}`, `{"reasoning_effort":"high"}`},
		{"gpt52 ceiling", "openai/gpt-5.2", types.RelayFormatOpenAIResponses, `{"reasoning":{"effort":"max","summary":"auto","context":{"id":9007199254740993}}}`, `{"reasoning":{"effort":"xhigh","summary":"auto","context":{"id":9007199254740993}}}`},
		{"gpt pro floor", "gpt-5.5-pro", types.RelayFormatOpenAIResponses, `{"reasoning":{"effort":"low"}}`, `{"reasoning":{"effort":"medium"}}`},
		{"gpt6 off floor", "gpt-6-astra", types.RelayFormatOpenAIResponses, `{"reasoning":{"effort":"none"}}`, `{"reasoning":{"effort":"low"}}`},
		{"gpt6 none supported", "gpt-6-sol", types.RelayFormatOpenAI, `{"reasoning_effort":"none"}`, `{"reasoning_effort":"none"}`},
		{"codex model identity", "gpt-5.1-codex-max", types.RelayFormatOpenAI, `{"reasoning_effort":"max"}`, `{"reasoning_effort":"xhigh"}`},
		{"nonreasoning gpt", "gpt-4o", types.RelayFormatOpenAI, `{"reasoning_effort":"high"}`, `{}`},
		{"default omitted", "gpt-5.2", types.RelayFormatOpenAIResponses, `{"reasoning":{"effort":"default"}}`, `{}`},
		{"summary hidden", "gpt-5.2", types.RelayFormatOpenAIResponses, `{"reasoning":{"effort":"high","summary":"none"}}`, `{"reasoning":{"effort":"high"}}`},
		{"summary alias", "gpt-5.2", types.RelayFormatOpenAIResponses, `{"reasoning":{"effort":"normal","summary":"full"}}`, `{"reasoning":{"effort":"medium","summary":"auto"}}`},
		{"deepseek medium", "deepseek-v4.1-flash", types.RelayFormatOpenAI, `{"reasoning_effort":"medium"}`, `{"reasoning_effort":"high","thinking":{"type":"enabled"}}`},
		{"deepseek ultra", "deepseek-v4-pro", types.RelayFormatOpenAI, `{"reasoning_effort":"ultra"}`, `{"reasoning_effort":"max","thinking":{"type":"enabled"}}`},
		{"deepseek xhigh", "deepseek-flash", types.RelayFormatOpenAIResponses, `{"reasoning":{"effort":"xhigh"}}`, `{"reasoning":{"effort":"high"}}`},
		{"deepseek off", "deepseek-flash", types.RelayFormatOpenAI, `{"reasoning_effort":"off"}`, `{"thinking":{"type":"disabled"}}`},
		{"glm legacy toggle", "glm-4.7", types.RelayFormatOpenAI, `{"reasoning_effort":"high"}`, `{"thinking":{"type":"enabled"}}`},
		{"glm legacy off", "glm-5.1", types.RelayFormatOpenAI, `{"reasoning_effort":"none"}`, `{"thinking":{"type":"disabled"}}`},
		{"glm enabled flag", "glm-5.1", types.RelayFormatOpenAI, `{"enable_thinking":true}`, `{"thinking":{"type":"enabled"}}`},
		{"glm52 levels", "glm-5.2", types.RelayFormatOpenAI, `{"reasoning_effort":"balanced"}`, `{"reasoning_effort":"high","thinking":{"type":"enabled"}}`},
		{"glm53 forced thinking", "glm-5.3", types.RelayFormatOpenAI, `{"thinking":{"type":"disabled"}}`, `{"reasoning_effort":"low","thinking":{"type":"enabled"}}`},
		{"kimi hybrid", "kimi-k2.6", types.RelayFormatOpenAI, `{"reasoning_effort":"medium","thinking":{"type":"enabled","keep":"all"}}`, `{"thinking":{"type":"enabled","keep":"all"}}`},
		{"kimi forced", "kimi-k3", types.RelayFormatOpenAI, `{"reasoning_effort":"unknown"}`, `{"reasoning_effort":"high"}`},
		{"kimi code fixed", "kimi-k2.7-code", types.RelayFormatOpenAI, `{"reasoning_effort":"high","thinking":{"type":"enabled"}}`, `{}`},
		{"kimi response off", "kimi-k3.1-pro", types.RelayFormatOpenAIResponses, `{"reasoning":{"effort":"none"}}`, `{"reasoning":{"effort":"low"}}`},
		{"deepseek messages", "deepseek-flash", types.RelayFormatClaude, `{"thinking":{"type":"adaptive","display":"summarized"},"output_config":{"effort":"unknown","format":{"z":1,"a":2}}}`, `{"thinking":{"type":"enabled"},"output_config":{"effort":"high","format":{"z":1,"a":2}}}`},
		{"claude adaptive", "claude-opus-4-8", types.RelayFormatClaude, `{"max_tokens":65536,"thinking":{"type":"enabled","budget_tokens":2048,"future_field":{"id":9007199254740993}},"output_config":{"effort":"unusual","format":{"z":1,"a":2}}}`, `{"max_tokens":65536,"thinking":{"type":"adaptive","display":"summarized","future_field":{"id":9007199254740993}},"output_config":{"effort":"medium","format":{"z":1,"a":2}}}`},
		{"claude old manual", "claude-sonnet-4-5", types.RelayFormatClaude, `{"max_tokens":4096,"thinking":{"type":"enabled","budget_tokens":2048},"output_config":{"effort":"xhigh"}}`, `{"max_tokens":4096,"thinking":{"type":"enabled","budget_tokens":2048,"display":"summarized"}}`},
		{"claude no summary", "claude-sonnet-4-6", types.RelayFormatClaude, `{"max_tokens":4096,"thinking":{"type":"adaptive","display":"omitted"},"output_config":{"effort":"minimal"}}`, `{"max_tokens":4096,"thinking":{"type":"adaptive","display":"omitted"},"output_config":{"effort":"low"}}`},
		{"gemini unknown", "gemini-4-argon", types.RelayFormatGemini, `{"generationConfig":{"thinkingConfig":{"thinkingLevel":"unusual"},"maxOutputTokens":4096,"vendor_id":9007199254740993}}`, `{"generationConfig":{"thinkingConfig":{"thinkingLevel":"medium","includeThoughts":true},"maxOutputTokens":4096,"vendor_id":9007199254740993}}`},
		{"gemini default summary", "gemini-4-argon", types.RelayFormatGemini, `{"generationConfig":{"maxOutputTokens":4096}}`, `{"generationConfig":{"thinkingConfig":{"includeThoughts":true},"maxOutputTokens":4096}}`},
		{"gemini hidden", "gemini-3.8-flash", types.RelayFormatGemini, `{"generationConfig":{"thinkingConfig":{"thinkingLevel":"HIGH","includeThoughts":false}}}`, `{"generationConfig":{"thinkingConfig":{"thinkingLevel":"high","includeThoughts":false}}}`},
		{"gemini old budget", "gemini-2.5-flash", types.RelayFormatGemini, `{"generationConfig":{"thinkingConfig":{"thinkingLevel":"balanced"}}}`, `{"generationConfig":{"thinkingConfig":{"thinkingBudget":8192,"includeThoughts":true}}}`},
		{"gemini old default", "gemini-2.5-flash-lite", types.RelayFormatGemini, `{"generationConfig":{"thinkingConfig":{"thinkingLevel":"default"}}}`, `{"generationConfig":{"thinkingConfig":{"includeThoughts":true}}}`},
		{"gemini legacy budget", "gemini-4-argon", types.RelayFormatGemini, `{"generationConfig":{"thinkingConfig":{"thinkingBudget":2048}}}`, `{"generationConfig":{"thinkingConfig":{"thinkingLevel":"medium","includeThoughts":true}}}`},
		{"gemini cannot turn off", "gemini-4-argon", types.RelayFormatGemini, `{"generationConfig":{"thinkingConfig":{"thinkingLevel":"off"}}}`, `{"generationConfig":{"thinkingConfig":{"thinkingLevel":"low","includeThoughts":false}}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var input map[string]common.RawMessage
			require.NoError(t, common.Unmarshal([]byte(tc.input), &input))
			const preserved = `{"signature":"opaque-signature","id":9007199254740993,"schema":{"z":1,"a":2}}`
			input["metadata"] = common.RawMessage(preserved)
			input["model"], _ = common.Marshal(tc.model)
			body, err := common.Marshal(input)
			require.NoError(t, err)
			received := make(chan []byte, 1)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				data, err := io.ReadAll(r.Body)
				assert.NoError(t, err)
				assert.Equal(t, int64(len(data)), r.ContentLength)
				received <- data
				w.WriteHeader(http.StatusOK)
			}))
			defer server.Close()
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/fixture", strings.NewReader(string(body)))
			info := &relaycommon.RelayInfo{RelayFormat: tc.format, ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: tc.model}}
			response, err := DoApiRequest(requestContextAdaptor{url: server.URL}, c, info, strings.NewReader(string(body)))
			require.NoError(t, err)
			t.Cleanup(func() { response.Body.Close() })
			var fields map[string]common.RawMessage
			require.NoError(t, common.Unmarshal(<-received, &fields))
			assert.Equal(t, preserved, string(fields["metadata"]))
			assert.Equal(t, string(input["model"]), string(fields["model"]))
			delete(fields, "metadata")
			delete(fields, "model")
			actual, err := common.Marshal(fields)
			require.NoError(t, err)
			assert.JSONEq(t, tc.want, string(actual))
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
