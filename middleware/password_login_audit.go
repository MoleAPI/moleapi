package middleware

import (
	"bytes"
	"io"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

const (
	passwordLoginAuditEnabledKey = "password_login_audit_enabled"
	passwordLoginFailureKey      = "password_login_failure_reason"
)

// PasswordLoginAudit records failed password-login attempts without retaining
// the password, encrypted password, encryption key ID, or raw request body.
func PasswordLoginAudit() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set(passwordLoginAuditEnabledKey, true)
		var request struct {
			Username string `json:"username"`
		}
		if c.Request.Body != nil {
			body, err := io.ReadAll(c.Request.Body)
			c.Request.Body = io.NopCloser(bytes.NewReader(body))
			if err == nil {
				_ = common.Unmarshal(body, &request)
			}
		}

		c.Next()
		reason := c.GetString(passwordLoginFailureKey)
		if reason == "" {
			return
		}
		username := strings.TrimSpace(request.Username)
		if runes := []rune(username); len(runes) > 64 {
			username = string(runes[:64])
		}
		model.RecordAuditLog(c, model.AuditLog{
			Username: username, Category: model.AuditCategoryLogin, Action: "login.failed",
			Content: "Password login failed", Success: false,
			Other: model.AuditOther{LoginMethod: "password", Op: &model.AuditOperation{
				Action: "login.failed", Params: model.AuditFields{"method": "password", "failure_reason": reason},
			}},
		})
	}
}

// MarkPasswordLoginFailure attaches a safe reason code to the current audit.
func MarkPasswordLoginFailure(c *gin.Context, reason string) {
	if c.GetBool(passwordLoginAuditEnabledKey) {
		c.Set(passwordLoginFailureKey, reason)
	}
}
