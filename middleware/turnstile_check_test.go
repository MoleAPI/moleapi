package middleware

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type turnstileTestTransport func(*http.Request) (*http.Response, error)

func (f turnstileTestTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func TestTurnstileCheck(t *testing.T) {
	gin.SetMode(gin.TestMode)
	previousEnabled, previousSecret := common.TurnstileCheckEnabled, common.TurnstileSecretKey
	previousTransport, previousWriter := http.DefaultTransport, gin.DefaultErrorWriter
	previousLanguage := common.DefaultLanguage
	t.Cleanup(func() {
		common.TurnstileCheckEnabled, common.TurnstileSecretKey = previousEnabled, previousSecret
		http.DefaultTransport, gin.DefaultErrorWriter = previousTransport, previousWriter
		common.DefaultLanguage = previousLanguage
	})
	common.TurnstileSecretKey = "turnstile-secret-never-log"
	common.DefaultLanguage = "en"
	t.Setenv("TRUSTED_PROXIES", "")

	for _, tc := range []struct {
		name       string
		peer       string
		forwarded  string
		remoteIP   string
		response   string
		status     int
		token      string
		missing    bool
		disabled   bool
		cancelled  bool
		reason     string
		loggedCode string
	}{
		{name: "public IPv4", peer: "203.0.113.10:1234", remoteIP: "203.0.113.10"},
		{name: "public IPv6", peer: "[2001:db8::10]:1234", remoteIP: "2001:db8::10"},
		{name: "IPv4 mapped loopback", peer: "[::ffff:127.0.0.1]:1234"},
		{name: "loopback proxy", peer: "127.0.0.1:1234"},
		{name: "IPv6 loopback proxy", peer: "[::1]:1234"},
		{name: "private proxy", peer: "10.20.30.40:1234"},
		{name: "IPv6 private proxy", peer: "[fd00::10]:1234"},
		{name: "link local proxy", peer: "169.254.1.1:1234"},
		{name: "trusted forwarded client", peer: "127.0.0.1:1234", forwarded: "192.0.2.99, 203.0.113.10", remoteIP: "203.0.113.10"},
		{name: "untrusted forwarded client", peer: "198.51.100.10:1234", forwarded: "203.0.113.10", remoteIP: "198.51.100.10"},
		{name: "expired or replayed token", response: `{"success":false,"error-codes":["timeout-or-duplicate"]}`, reason: "turnstile_verification_failed", loggedCode: "timeout-or-duplicate"},
		{name: "invalid secret", response: `{"success":false,"error-codes":["invalid-input-secret"]}`, reason: "turnstile_verification_failed", loggedCode: "invalid-input-secret"},
		{name: "invalid secret HTTP400 retains rejection code", status: http.StatusBadRequest, response: `{"success":false,"error-codes":["invalid-input-secret"]}`, reason: "turnstile_unavailable", loggedCode: "invalid-input-secret"},
		{name: "unknown error is sanitized", response: `{"success":false,"error-codes":["turnstile-secret-never-log\nforged-log-entry"]}`, reason: "turnstile_verification_failed", loggedCode: "unknown-error"},
		{name: "no rejection code", response: `{"success":false}`, reason: "turnstile_verification_failed", loggedCode: "unknown-error"},
		{name: "non JSON response", response: "turnstile-secret-never-log", reason: "turnstile_invalid_response"},
		{name: "oversized response", response: `{"padding":"` + strings.Repeat("x", 16<<10) + `","success":true}`, reason: "turnstile_invalid_response"},
		{name: "HTTP failure cannot pass", status: http.StatusServiceUnavailable, reason: "turnstile_unavailable"},
		{name: "redirect cannot receive secret", status: http.StatusTemporaryRedirect, reason: "turnstile_unavailable"},
		{name: "cancelled request", cancelled: true, reason: "turnstile_unavailable"},
		{name: "missing token", missing: true, reason: "turnstile_token_missing"},
		{name: "oversized token", token: strings.Repeat("x", 2049), reason: "turnstile_verification_failed"},
		{name: "disabled verification", disabled: true, missing: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			common.TurnstileCheckEnabled = !tc.disabled
			var logs bytes.Buffer
			gin.DefaultErrorWriter = &logs
			calls := 0
			http.DefaultTransport = turnstileTestTransport(func(r *http.Request) (*http.Response, error) {
				calls++
				assert.Equal(t, "https://challenges.cloudflare.com/turnstile/v0/siteverify", r.URL.String())
				assert.Equal(t, http.MethodPost, r.Method)
				_, hasDeadline := r.Context().Deadline()
				assert.True(t, hasDeadline, "verification must have a bounded timeout")
				require.NoError(t, r.ParseForm())
				assert.Equal(t, common.TurnstileSecretKey, r.PostForm.Get("secret"))
				assert.Equal(t, "turnstile-token-never-log", r.PostForm.Get("response"))
				assert.Equal(t, tc.remoteIP, r.PostForm.Get("remoteip"))
				if tc.remoteIP == "" {
					assert.NotContains(t, r.PostForm, "remoteip")
				}
				if tc.cancelled {
					require.ErrorIs(t, r.Context().Err(), context.Canceled)
					return nil, r.Context().Err()
				}
				status, body := tc.status, tc.response
				if status == 0 {
					status = http.StatusOK
				}
				if body == "" {
					body = `{"success":true}`
				}
				return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body)),
					Header: http.Header{"Location": {"https://unexpected.example/"}}, Request: r}, nil
			})
			router := gin.New()
			require.NoError(t, ConfigureTrustedProxies(router))
			var reason string
			router.Use(func(c *gin.Context) {
				c.Set(passwordLoginAuditEnabledKey, true)
				c.Set(common.RequestIdKey, "turnstile-regression-request")
				c.Next()
				reason = c.GetString(passwordLoginFailureKey)
			})
			router.POST("/login", TurnstileCheck(), func(c *gin.Context) { c.Status(http.StatusNoContent) })
			token := tc.token
			if token == "" && !tc.missing {
				token = "turnstile-token-never-log"
			}
			request := httptest.NewRequest(http.MethodPost, "/login?turnstile="+token, nil)
			request.RemoteAddr = tc.peer
			if request.RemoteAddr == "" {
				request.RemoteAddr = "127.0.0.1:1234"
			}
			request.Header.Set("X-Forwarded-For", tc.forwarded)
			request.Header.Set("CF-Connecting-IP", "192.0.2.123")
			if tc.cancelled {
				ctx, cancel := context.WithCancel(request.Context())
				cancel()
				request = request.WithContext(ctx)
			}
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, request)
			assert.Equal(t, tc.reason, reason)
			if tc.reason == "" {
				assert.Equal(t, http.StatusNoContent, recorder.Code)
			} else {
				assert.Equal(t, http.StatusOK, recorder.Code)
				var result struct {
					Success bool `json:"success"`
				}
				require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &result))
				assert.False(t, result.Success)
			}
			if tc.disabled || tc.missing || len(tc.token) > 2048 {
				assert.Zero(t, calls)
			} else {
				assert.Equal(t, 1, calls, "never retry or redirect a single-use token")
			}
			if tc.loggedCode != "" {
				assert.Contains(t, logs.String(), "error_codes="+tc.loggedCode)
				assert.Contains(t, logs.String(), "turnstile-regression-request")
			}
			for _, secret := range []string{common.TurnstileSecretKey, "turnstile-token-never-log", "forged-log-entry"} {
				assert.NotContains(t, logs.String(), secret)
				assert.NotContains(t, recorder.Body.String(), secret)
			}
		})
	}
}
