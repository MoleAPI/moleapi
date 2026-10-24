package common

import (
	"context"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVerificationSharedStorage(t *testing.T) {
	previousRDB, previousEnabled := RDB, RedisEnabled
	t.Cleanup(func() { RDB, RedisEnabled = previousRDB, previousEnabled })
	addr := os.Getenv("TEST_VERIFICATION_REDIS_ADDR")
	if addr == "" {
		addr = miniredis.RunT(t).Addr()
	}
	client := redis.NewClient(&redis.Options{Addr: addr})
	RDB = client
	t.Cleanup(func() { _ = client.Close() })
	RedisEnabled = true
	exe, err := os.Executable()
	require.NoError(t, err)
	key := GenerateVerificationCode(0) + "@example.invalid"
	ctx := context.Background()
	for _, purpose := range []string{EmailVerificationPurpose, PasswordResetPurpose} {
		t.Run(purpose, func(t *testing.T) {
			// Each operation runs in a fresh process: another node and a restart
			// must both use the stored code without inheriting any local memory.
			for _, operation := range []string{"register", "verify", "verify"} {
				child := exec.Command(exe, "-test.run=^TestVerificationApplicationProcess$")
				child.Env = append(os.Environ(), "TEST_VERIFICATION_REDIS_ADDR="+addr,
					"VERIFICATION_TEST_OPERATION="+operation, "VERIFICATION_TEST_KEY="+key, "VERIFICATION_TEST_PURPOSE="+purpose)
				output, err := child.CombinedOutput()
				require.NoError(t, err, string(output))
			}
			assert.False(t, VerifyCodeWithKey(key, "wrong", purpose))
			assert.False(t, VerifyCodeWithKey(key, "", purpose))
			assert.False(t, VerifyCodeWithKey("other@example.invalid", "old-code", purpose))
			assert.False(t, VerifyCodeWithKey(key, "old-code", "other-purpose"))
			require.NoError(t, RegisterVerificationCodeWithKey(key, "new-code", purpose))
			assert.False(t, VerifyCodeWithKey(key, "old-code", purpose))
			assert.True(t, VerifyCodeWithKey(key, "new-code", purpose))
			ttl, err := RDB.TTL(ctx, "verification:"+purpose+":"+key).Result()
			require.NoError(t, err)
			assert.InDelta(t, float64(time.Duration(VerificationValidMinutes)*time.Minute), float64(ttl), float64(time.Second))
			DeleteKey(key, purpose)
			assert.False(t, VerifyCodeWithKey(key, "new-code", purpose))
			require.NoError(t, RegisterVerificationCodeWithKey(key, "expiring", purpose))
			require.NoError(t, RDB.Expire(ctx, "verification:"+purpose+":"+key, -time.Second).Err())
			assert.False(t, VerifyCodeWithKey(key, "expiring", purpose))
		})
	}

	RedisEnabled = false
	require.NoError(t, RegisterVerificationCodeWithKey(key, "local-only", EmailVerificationPurpose))
	assert.True(t, VerifyCodeWithKey(key, "local-only", EmailVerificationPurpose))
	RedisEnabled = true
	require.NoError(t, RDB.Close())
	assert.Error(t, RegisterVerificationCodeWithKey(key, "not-stored", EmailVerificationPurpose))
	assert.False(t, VerifyCodeWithKey(key, "local-only", EmailVerificationPurpose), "Redis failures must not fall back to node-local codes")
	RDB = nil
	assert.Error(t, RegisterVerificationCodeWithKey(key, "not-stored", EmailVerificationPurpose))
	assert.False(t, VerifyCodeWithKey(key, "local-only", EmailVerificationPurpose))
	RedisEnabled = false
	DeleteKey(key, EmailVerificationPurpose)
	assert.False(t, VerifyCodeWithKey(key, "local-only", EmailVerificationPurpose))
}

func TestVerificationApplicationProcess(t *testing.T) {
	operation := os.Getenv("VERIFICATION_TEST_OPERATION")
	if operation == "" {
		t.Skip("subprocess fixture")
	}
	RedisEnabled = true
	RDB = redis.NewClient(&redis.Options{Addr: os.Getenv("TEST_VERIFICATION_REDIS_ADDR")})
	t.Cleanup(func() { _ = RDB.Close() })
	key, purpose := os.Getenv("VERIFICATION_TEST_KEY"), os.Getenv("VERIFICATION_TEST_PURPOSE")
	if operation == "register" {
		require.NoError(t, RegisterVerificationCodeWithKey(key, "old-code", purpose))
		return
	}
	require.Equal(t, "verify", operation)
	require.True(t, VerifyCodeWithKey(key, "old-code", purpose))
}
