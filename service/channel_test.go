package service

import (
	"errors"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/stretchr/testify/assert"
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
}

func TestShouldConfirmModelFailureKeepsChannelFailuresImmediate(t *testing.T) {
	assert.True(t, ShouldConfirmModelFailure(types.NewOpenAIError(errors.New("missing"), types.ErrorCodeModelNotFound, 404)))
	assert.True(t, ShouldConfirmModelFailure(types.NewOpenAIError(errors.New("bad response"), types.ErrorCodeBadResponse, 503)))
	assert.False(t, ShouldConfirmModelFailure(types.NewOpenAIError(errors.New("network"), types.ErrorCodeDoRequestFailed, 500)))
	assert.False(t, ShouldConfirmModelFailure(types.NewOpenAIError(errors.New("auth"), types.ErrorCodeChannelInvalidKey, 401)))
}
