package middleware

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
)

const (
	EmailVerificationRateLimitMark = "EV"
	EmailVerificationMaxRequests   = 2  // 30秒内最多2次
	EmailVerificationDuration      = 30 // 30秒时间窗口
	EmailRecipientMaxRequests      = 3
	EmailRecipientDuration         = 10 * 60
)

func emailRecipientRateLimitKey(email string) string {
	return fmt.Sprintf("%s:email:%s:%s", redisRateLimitNamespace, EmailVerificationRateLimitMark, common.GenerateHMAC(model.NormalizeEmail(email)))
}

func redisEmailVerificationRateLimiter(c *gin.Context) {
	allowed, _, ttlSeconds, err := redisFixedWindowTake(
		c.Request.Context(),
		redisIPRateLimitKey(EmailVerificationRateLimitMark, c.ClientIP()),
		EmailVerificationMaxRequests,
		EmailVerificationDuration,
	)
	if err != nil {
		memoryEmailVerificationRateLimiter(c)
		return
	}
	if allowed {
		c.Next()
		return
	}

	waitSeconds := int64(EmailVerificationDuration)
	if ttlSeconds > 0 {
		waitSeconds = ttlSeconds
	}

	common.ApiErrorStatus(c, http.StatusTooManyRequests, common.NewMessage("Please wait {{seconds}} seconds before requesting another verification code.", map[string]any{"seconds": waitSeconds}))
	c.Abort()
}

func memoryEmailVerificationRateLimiter(c *gin.Context) {
	key := EmailVerificationRateLimitMark + ":" + c.ClientIP()

	if !inMemoryRateLimiter.Request(key, EmailVerificationMaxRequests, EmailVerificationDuration) {
		common.ApiErrorStatus(c, http.StatusTooManyRequests, common.NewMessage("Please wait before requesting another verification code."))
		c.Abort()
		return
	}

	c.Next()
}

func EmailVerificationRateLimit() gin.HandlerFunc {
	// Keep the fallback ready before requests arrive so a concurrent Redis
	// outage cannot race the in-memory limiter's first initialization.
	inMemoryRateLimiter.Init(common.RateLimitKeyExpirationDuration)
	return func(c *gin.Context) {
		if common.RedisEnabled {
			redisEmailVerificationRateLimiter(c)
		} else {
			memoryEmailVerificationRateLimiter(c)
		}
	}
}

// EmailRecipientRateLimit prevents distributed clients from repeatedly
// sending registration codes or password-reset messages to one mailbox.
func EmailRecipientRateLimit() gin.HandlerFunc {
	inMemoryRateLimiter.Init(common.RateLimitKeyExpirationDuration)
	return func(c *gin.Context) {
		email := model.NormalizeEmail(c.Query("email"))
		if email == "" {
			c.Next()
			return
		}
		key := emailRecipientRateLimitKey(email)
		if common.RedisEnabled {
			allowed, _, ttlSeconds, err := redisFixedWindowTake(c.Request.Context(), key, EmailRecipientMaxRequests, EmailRecipientDuration)
			if err == nil {
				if allowed {
					c.Next()
					return
				}
				if ttlSeconds > 0 {
					c.Header("Retry-After", strconv.FormatInt(ttlSeconds, 10))
				}
				common.ApiErrorStatus(c, http.StatusTooManyRequests, common.NewMessage("Please try again later."))
				c.Abort()
				return
			}
		}
		if !inMemoryRateLimiter.Request(key, EmailRecipientMaxRequests, EmailRecipientDuration) {
			c.Header("Retry-After", strconv.Itoa(EmailRecipientDuration))
			common.ApiErrorStatus(c, http.StatusTooManyRequests, common.NewMessage("Please try again later."))
			c.Abort()
			return
		}
		c.Next()
	}
}
