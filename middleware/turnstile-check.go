package middleware

import (
	"io"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/gin-gonic/gin"
)

type turnstileCheckResponse struct {
	Success    bool     `json:"success"`
	ErrorCodes []string `json:"error-codes"`
}

func TurnstileCheck() gin.HandlerFunc {
	return func(c *gin.Context) {
		if common.TurnstileCheckEnabled {
			response := c.Query("turnstile")
			if response == "" {
				MarkPasswordLoginFailure(c, "turnstile_token_missing")
				common.ApiErrorT(c, "Turnstile token is empty")
				c.Abort()
				return
			}
			if len(response) > 2048 {
				MarkPasswordLoginFailure(c, "turnstile_verification_failed")
				common.ApiErrorT(c, "Turnstile verification failed, please refresh and try again!")
				c.Abort()
				return
			}
			form := url.Values{
				"secret":   {common.TurnstileSecretKey},
				"response": {response},
			}
			// ponytail: remoteip is optional; omit proxy-local addresses instead of guessing from untrusted headers.
			if ip, err := netip.ParseAddr(c.ClientIP()); err == nil {
				ip = ip.Unmap()
				if ip.IsGlobalUnicast() && !ip.IsPrivate() {
					form.Set("remoteip", ip.String())
				}
			}
			request, err := http.NewRequestWithContext(c.Request.Context(), http.MethodPost,
				"https://challenges.cloudflare.com/turnstile/v0/siteverify", strings.NewReader(form.Encode()))
			if err == nil {
				request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			}
			var rawRes *http.Response
			if err == nil {
				client := &http.Client{
					Timeout: 10 * time.Second,
					CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
						return http.ErrUseLastResponse
					},
				}
				rawRes, err = client.Do(request)
			}
			if err != nil {
				MarkPasswordLoginFailure(c, "turnstile_unavailable")
				logger.LogWarn(c, common.LogText("Turnstile verification unavailable (error_type=%T)", err))
				common.ApiErrorT(c, "Turnstile verification failed, please refresh and try again!")
				c.Abort()
				return
			}
			defer rawRes.Body.Close()
			if rawRes.StatusCode != http.StatusOK {
				MarkPasswordLoginFailure(c, "turnstile_unavailable")
				logger.LogWarn(c, common.LogText("Turnstile verification unavailable (http_status=%d)", rawRes.StatusCode))
				common.ApiErrorT(c, "Turnstile verification failed, please refresh and try again!")
				c.Abort()
				return
			}
			var res turnstileCheckResponse
			err = common.DecodeJson(io.LimitReader(rawRes.Body, 16<<10), &res)
			if err != nil {
				MarkPasswordLoginFailure(c, "turnstile_invalid_response")
				logger.LogWarn(c, common.LogText("Turnstile verification returned invalid JSON"))
				common.ApiErrorT(c, "Turnstile verification failed, please refresh and try again!")
				c.Abort()
				return
			}
			if !res.Success {
				MarkPasswordLoginFailure(c, "turnstile_verification_failed")
				if len(res.ErrorCodes) == 0 {
					res.ErrorCodes = []string{"unknown-error"}
				}
				for i, code := range res.ErrorCodes {
					switch code {
					case "missing-input-secret", "invalid-input-secret", "missing-input-response", "invalid-input-response",
						"bad-request", "timeout-or-duplicate", "internal-error":
					default:
						res.ErrorCodes[i] = "unknown-error"
					}
				}
				logger.LogWarn(c, common.LogText("Turnstile verification rejected (error_codes=%s)", strings.Join(res.ErrorCodes, ",")))
				common.ApiErrorT(c, "Turnstile verification failed, please refresh and try again!")
				c.Abort()
				return
			}
		}
		c.Next()
	}
}
