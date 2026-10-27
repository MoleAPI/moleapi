package middleware

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

const (
	passwordLoginAuditEnabledKey = "password_login_audit_enabled"
	passwordLoginFailureKey      = "password_login_failure_reason"
	passwordLoginFailureWindow   = 10 * time.Minute
	passwordLoginPairLimit       = 5
	passwordLoginPairCooldown    = 15 * time.Minute
	passwordLoginIPLimit         = 20
	passwordLoginIPCooldown      = 30 * time.Minute
)

type passwordLoginFailureState struct {
	count         int
	windowUntil   time.Time
	cooldownUntil time.Time
}

var passwordLoginFailures = struct {
	sync.Mutex
	items       map[string]passwordLoginFailureState
	lastCleanup time.Time
}{items: make(map[string]passwordLoginFailureState)}

func passwordLoginRateKeys(username, ip string) (string, string) {
	pair := common.GenerateHMAC(strings.ToLower(strings.TrimSpace(username)) + "\x00" + ip)
	return fmt.Sprintf("%s:login:pair:%s", redisRateLimitNamespace, pair), fmt.Sprintf("%s:login:ip:%s", redisRateLimitNamespace, ip)
}

func memoryPasswordLoginCooldown(pairKey, ipKey string) time.Duration {
	passwordLoginFailures.Lock()
	defer passwordLoginFailures.Unlock()
	now := time.Now()
	remaining := time.Duration(0)
	for _, key := range []string{pairKey, ipKey} {
		state, ok := passwordLoginFailures.items[key]
		if !ok {
			continue
		}
		if state.cooldownUntil.After(now) && time.Until(state.cooldownUntil) > remaining {
			remaining = time.Until(state.cooldownUntil)
		}
	}
	return remaining
}

func recordMemoryPasswordLoginFailure(pairKey, ipKey string) {
	passwordLoginFailures.Lock()
	defer passwordLoginFailures.Unlock()
	now := time.Now()
	// ponytail: one opportunistic cleanup is enough for the Redis-outage fallback.
	if passwordLoginFailures.lastCleanup.IsZero() || now.Sub(passwordLoginFailures.lastCleanup) >= time.Minute {
		for key, state := range passwordLoginFailures.items {
			if state.windowUntil.Before(now) && state.cooldownUntil.Before(now) {
				delete(passwordLoginFailures.items, key)
			}
		}
		passwordLoginFailures.lastCleanup = now
	}
	for _, limit := range []struct {
		key      string
		maximum  int
		cooldown time.Duration
	}{{pairKey, passwordLoginPairLimit, passwordLoginPairCooldown}, {ipKey, passwordLoginIPLimit, passwordLoginIPCooldown}} {
		state := passwordLoginFailures.items[limit.key]
		if !state.windowUntil.After(now) {
			state.count = 0
			state.windowUntil = now.Add(passwordLoginFailureWindow)
		}
		state.count++
		if state.count >= limit.maximum {
			state.cooldownUntil = now.Add(limit.cooldown)
		}
		passwordLoginFailures.items[limit.key] = state
	}
}

func clearMemoryPasswordLoginPair(pairKey string) {
	passwordLoginFailures.Lock()
	delete(passwordLoginFailures.items, pairKey)
	passwordLoginFailures.Unlock()
}

func redisPasswordLoginCooldown(ctx context.Context, pairKey, ipKey string) (time.Duration, error) {
	if common.RDB == nil {
		return 0, fmt.Errorf("Redis client is not initialized")
	}
	pairTTL, err := common.RDB.TTL(ctx, pairKey+":cooldown").Result()
	if err != nil {
		return 0, err
	}
	ipTTL, err := common.RDB.TTL(ctx, ipKey+":cooldown").Result()
	if err != nil {
		return 0, err
	}
	if ipTTL > pairTTL {
		return ipTTL, nil
	}
	return pairTTL, nil
}

func recordRedisPasswordLoginFailure(ctx context.Context, pairKey, ipKey string) error {
	_, pairCount, _, err := redisFixedWindowTake(ctx, pairKey, passwordLoginPairLimit, int64(passwordLoginFailureWindow.Seconds()))
	if err != nil {
		return err
	}
	_, ipCount, _, err := redisFixedWindowTake(ctx, ipKey, passwordLoginIPLimit, int64(passwordLoginFailureWindow.Seconds()))
	if err != nil {
		return err
	}
	if pairCount >= passwordLoginPairLimit {
		if err := common.RDB.Set(ctx, pairKey+":cooldown", "1", passwordLoginPairCooldown).Err(); err != nil {
			return err
		}
	}
	if ipCount >= passwordLoginIPLimit {
		if err := common.RDB.Set(ctx, ipKey+":cooldown", "1", passwordLoginIPCooldown).Err(); err != nil {
			return err
		}
	}
	return nil
}

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

		pairKey, ipKey := passwordLoginRateKeys(request.Username, c.ClientIP())
		remaining := time.Duration(0)
		if common.RedisEnabled {
			var err error
			remaining, err = redisPasswordLoginCooldown(c.Request.Context(), pairKey, ipKey)
			if err != nil {
				logger.LogError(c.Request.Context(), common.LogText("password login cooldown check failed: %v", err))
				remaining = memoryPasswordLoginCooldown(pairKey, ipKey)
			}
		} else {
			remaining = memoryPasswordLoginCooldown(pairKey, ipKey)
		}
		if remaining > 0 {
			seconds := int64((remaining + time.Second - 1) / time.Second)
			c.Header("Retry-After", strconv.FormatInt(seconds, 10))
			MarkPasswordLoginFailure(c, "login_cooldown")
			common.ApiErrorStatus(c, http.StatusTooManyRequests, common.NewMessage("Please try again later."))
			c.Abort()
		} else {
			c.Next()
		}
		reason := c.GetString(passwordLoginFailureKey)
		if reason == "invalid_credentials" {
			if common.RedisEnabled {
				if err := recordRedisPasswordLoginFailure(c.Request.Context(), pairKey, ipKey); err != nil {
					logger.LogError(c.Request.Context(), common.LogText("password login failure tracking failed: %v", err))
					recordMemoryPasswordLoginFailure(pairKey, ipKey)
				}
			} else {
				recordMemoryPasswordLoginFailure(pairKey, ipKey)
			}
		} else if reason == "" {
			if common.RedisEnabled && common.RDB != nil {
				if err := common.RDB.Del(c.Request.Context(), pairKey, pairKey+":cooldown").Err(); err != nil {
					logger.LogError(c.Request.Context(), common.LogText("password login cooldown reset failed: %v", err))
				}
			}
			clearMemoryPasswordLoginPair(pairKey)
		}
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
