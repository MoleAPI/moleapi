package service

import (
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestShouldDisableChannelUsesStatusOrKeyword(t *testing.T) {
	originalEnabled := common.AutomaticDisableChannelEnabled
	originalKeywords := operation_setting.AutomaticDisableKeywords
	originalRanges := operation_setting.AutomaticDisableStatusCodeRanges
	t.Cleanup(func() {
		common.AutomaticDisableChannelEnabled = originalEnabled
		operation_setting.AutomaticDisableKeywords = originalKeywords
		operation_setting.AutomaticDisableStatusCodeRanges = originalRanges
	})

	common.AutomaticDisableChannelEnabled = true
	operation_setting.AutomaticDisableKeywords = []string{"quota exhausted"}
	operation_setting.AutomaticDisableStatusCodeRanges = []operation_setting.StatusCodeRange{{Start: 503, End: 503}}

	assert.True(t, ShouldDisableChannel(types.NewOpenAIError(errors.New("upstream unavailable"), types.ErrorCodeInvalidRequest, 503)))
	assert.True(t, ShouldDisableChannel(types.NewOpenAIError(errors.New("QUOTA EXHAUSTED"), types.ErrorCodeInvalidRequest, 400)))
	assert.False(t, ShouldDisableChannel(types.NewOpenAIError(errors.New("bad request"), types.ErrorCodeInvalidRequest, 400)))
}

func TestShouldDisableChannelRecognizesPlanQuotaExhaustion(t *testing.T) {
	originalEnabled := common.AutomaticDisableChannelEnabled
	originalKeywords := operation_setting.AutomaticDisableKeywords
	originalRanges := operation_setting.AutomaticDisableStatusCodeRanges
	t.Cleanup(func() {
		common.AutomaticDisableChannelEnabled = originalEnabled
		operation_setting.AutomaticDisableKeywords = originalKeywords
		operation_setting.AutomaticDisableStatusCodeRanges = originalRanges
	})

	common.AutomaticDisableChannelEnabled = true
	operation_setting.AutomaticDisableKeywords = nil
	operation_setting.AutomaticDisableStatusCodeRanges = nil
	for _, message := range []string{
		"Call count exceeded quota",
		"AFP quota exceeded",
		"usage limit exceeded",
		"insufficient quota",
		"no credits remaining",
		"coding plan subscription expired",
		"You've reached your weekly usage limit for your plan",
		"调用次数超出配额",
	} {
		assert.True(t, ShouldDisableChannel(types.NewOpenAIError(errors.New(message), types.ErrorCodeInvalidRequest, 429)), message)
	}
	assert.False(t, ShouldDisableChannel(types.NewOpenAIError(errors.New("rate limit exceeded"), types.ErrorCodeInvalidRequest, 429)))

	operation_setting.AutomaticDisableStatusCodeRanges = []operation_setting.StatusCodeRange{{Start: 429, End: 429}}
	assert.False(t, ShouldDisableChannel(types.NewOpenAIError(errors.New("rate limit exceeded"), types.ErrorCodeInvalidRequest, 429)))
	operation_setting.AutomaticDisableKeywords = []string{"rate limit exceeded"}
	assert.False(t, ShouldDisableChannel(types.NewOpenAIError(errors.New("rate limit exceeded"), types.ErrorCodeInvalidRequest, 429)))
	assert.True(t, ShouldDisableChannel(types.NewOpenAIError(errors.New("try later"), types.ErrorCode("insufficient_quota"), 429)))
}

func TestQuotaResetAtFromErrorReadsQuotaMetadata(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	err := types.NewOpenAIError(errors.New("quota exhausted"), types.ErrorCodeInvalidRequest, http.StatusTooManyRequests)
	err.Metadata = []byte(`{"limit":{"resetsAt":1700000123}}`)

	resetAt := quotaResetAtFromError(err, now)
	require.NotNil(t, resetAt)
	assert.Equal(t, time.Unix(1_700_000_123, 0), *resetAt)
}

func TestCodingPlanResponseRetryAt(t *testing.T) {
	want := time.Unix(1_700_000_123, 0)
	channel := &model.Channel{Status: common.ChannelStatusAutoDisabled}
	channel.SetOtherInfo(map[string]any{"status_reason": CodingPlanResponseReasonPrefix + "quota exhausted; reset_at=1700000123"})

	retryAt := CodingPlanResponseRetryAt(channel)
	require.NotNil(t, retryAt)
	assert.Equal(t, want, *retryAt)
}

func TestShouldConfirmModelFailureKeepsChannelFailuresImmediate(t *testing.T) {
	assert.True(t, ShouldConfirmModelFailure(types.NewOpenAIError(errors.New("missing"), types.ErrorCodeModelNotFound, 404)))
	assert.True(t, ShouldConfirmModelFailure(types.NewOpenAIError(errors.New("bad response"), types.ErrorCodeBadResponse, 503)))
	assert.False(t, ShouldConfirmModelFailure(types.NewOpenAIError(errors.New("network"), types.ErrorCodeDoRequestFailed, 500)))
	assert.False(t, ShouldConfirmModelFailure(types.NewOpenAIError(errors.New("auth"), types.ErrorCodeChannelInvalidKey, 401)))
}
