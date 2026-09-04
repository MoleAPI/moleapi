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
