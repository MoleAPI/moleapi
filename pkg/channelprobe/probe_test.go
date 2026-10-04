package channelprobe

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProbeStateRoundTripPreservesOtherChannelMetadata(t *testing.T) {
	state := State{Models: map[string]ModelState{
		"model-a": {Status: StatusDegraded, AutoPaused: true},
	}}
	raw, err := StateIntoOtherInfo(`{"status_reason":"upstream timeout"}`, state)
	require.NoError(t, err)

	var other map[string]any
	require.NoError(t, common.UnmarshalJsonStr(raw, &other))
	assert.Equal(t, "upstream timeout", other["status_reason"])
	assert.True(t, StateFromOtherInfo(raw).Models["model-a"].AutoPaused)
}

func TestAutoPauseRecoveryUsesOneSuccessfulProbe(t *testing.T) {
	state := State{}
	failure := Failure{OccurredAt: 10, StatusCode: 404, ErrorCode: "model_not_found", Requests: 3}

	paused := state.Pause("model-a", "confirmed model failure", 10, 40, failure)
	assert.True(t, paused.AutoPaused)
	assert.Equal(t, StatusDegraded, paused.Status)

	change := state.RecordAutoProbe("model-a", true, "pass", 40, 70, nil)
	assert.True(t, change.Recovered)
	assert.False(t, change.State.AutoPaused)
	assert.Empty(t, change.State.PauseReason)
	assert.Equal(t, failure, *change.State.LastFailure)
}
