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

func TestShouldDisableChannelExcludesContentPolicyRefusals(t *testing.T) {
	previousEnabled := common.AutomaticDisableChannelEnabled
	previousKeywords := operation_setting.AutomaticDisableKeywords
	previousRanges := operation_setting.AutomaticDisableStatusCodeRanges
	t.Cleanup(func() {
		common.AutomaticDisableChannelEnabled = previousEnabled
		operation_setting.AutomaticDisableKeywords = previousKeywords
		operation_setting.AutomaticDisableStatusCodeRanges = previousRanges
	})
	common.AutomaticDisableChannelEnabled = true
	operation_setting.AutomaticDisableKeywords = []string{"safety", "blocked"}
	operation_setting.AutomaticDisableStatusCodeRanges = []operation_setting.StatusCodeRange{{Start: 400, End: 599}}

	for _, test := range []struct {
		name        string
		err         *types.NewAPIError
		wantDisable bool
	}{
		{name: "reported refusal", err: types.NewOpenAIError(errors.New("The request was blocked by the provider's safety policy. Revise the content and try again."), types.ErrorCodeBadResponseStatusCode, 422)},
		{name: "forbidden refusal", err: types.WithClaudeError(types.ClaudeError{Message: "Request blocked by safety policy", Type: "invalid_request_error"}, 403)},
		{name: "provider filter type", err: types.WithOpenAIError(types.OpenAIError{Message: "Request rejected", Type: "content_filter"}, 400)},
		{name: "provider policy code", err: types.WithOpenAIError(types.OpenAIError{Message: "Request rejected", Code: "content_policy_violation"}, 422)},
		{name: "provider numeric policy code", err: types.WithOpenAIError(types.OpenAIError{Message: "Request rejected", Code: 1301}, 400)},
		{name: "legal content restriction", err: types.NewOpenAIError(errors.New("Request rejected"), types.ErrorCodeBadResponseStatusCode, 451)},
		{name: "ordinary validation keeps configured rule", err: types.NewOpenAIError(errors.New("Invalid parameter"), types.ErrorCodeInvalidRequest, 422), wantDisable: true},
		{name: "credentials still disable", err: types.NewOpenAIError(errors.New("invalid_api_key"), types.ErrorCodeBadResponseStatusCode, 401), wantDisable: true},
		{name: "account permissions still disable", err: types.NewOpenAIError(errors.New("Access denied"), types.ErrorCodeBadResponseStatusCode, 403), wantDisable: true},
		{name: "balance fault takes precedence", err: types.NewOpenAIError(errors.New("Insufficient balance; request blocked by safety policy"), types.ErrorCodeBadResponseStatusCode, 402), wantDisable: true},
		{name: "server fault takes precedence", err: types.NewOpenAIError(errors.New("Safety service unavailable"), types.ErrorCodeBadResponseStatusCode, 503), wantDisable: true},
		{name: "explicit channel fault takes precedence", err: types.NewErrorWithStatusCode(errors.New("Safety service credential invalid"), types.ErrorCodeChannelInvalidKey, 403), wantDisable: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			originalStatus := test.err.StatusCode
			assert.Equal(t, test.wantDisable, ShouldDisableChannel(test.err), "before public classification")
			SetPublicUpstreamError(nil, test.err)
			assert.Equal(t, test.wantDisable, ShouldDisableChannel(test.err), "after public classification")
			assert.Equal(t, originalStatus, test.err.StatusCode)
			if !test.wantDisable {
				assert.Equal(t, http.StatusUnprocessableEntity, test.err.PublicStatusCode())
				assert.Equal(t, "content_policy_violation", test.err.ToOpenAIError().Code)
			}
		})
	}

	classified := types.NewOpenAIError(errors.New("Request rejected"), types.ErrorCodeBadResponseStatusCode, 403)
	classified.SetPublicError(422, types.OpenAIError{Code: "content_policy_violation", Type: "invalid_request_error"})
	assert.False(t, ShouldDisableChannel(classified))
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
