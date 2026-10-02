package channelprobe

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProbeCalibratesAndUsesHysteresis(t *testing.T) {
	state := State{}
	challenge := requireChallenge(t, LevelAdvanced)

	change := state.Apply("model-a", Evaluate(ModeIntelligence, challenge, "ANSWER:"+challenge.Answer, challenge.Answer), 1, 10)
	assert.Equal(t, LevelAdvanced, change.State.StableLevel)
	assert.Equal(t, StatusHealthy, change.State.Status)

	wrong := Evaluate(ModeIntelligence, challenge, "ANSWER:wrong", challenge.Answer)
	assert.False(t, state.Apply("model-a", wrong, 2, 10).Degraded)
	assert.False(t, state.Apply("model-a", wrong, 3, 10).Degraded)
	change = state.Apply("model-a", wrong, 4, 10)
	assert.True(t, change.Degraded)
	assert.Equal(t, StatusDegraded, change.State.Status)
	assert.Equal(t, "model-a", state.BlockedModel)

	pass := Evaluate(ModeIntelligence, challenge, "ANSWER:"+challenge.Answer, challenge.Answer)
	assert.False(t, state.Apply("model-a", pass, 5, 10).Recovered)
	change = state.Apply("model-a", pass, 6, 10)
	assert.True(t, change.Recovered)
	assert.Equal(t, StatusHealthy, change.State.Status)
	assert.Empty(t, state.BlockedModel)
	assert.Len(t, change.State.Recent, 5)
}

func TestProbeCalibrationFallsBackByLevel(t *testing.T) {
	state := State{}
	advanced := requireChallenge(t, LevelAdvanced)
	standard := requireChallenge(t, LevelStandard)
	basic := requireChallenge(t, LevelBasic)

	state.Apply("model-a", Evaluate(ModeIntelligence, advanced, "wrong", advanced.Answer), 1, 10)
	assert.Equal(t, LevelStandard, state.LevelFor("model-a"))
	state.Apply("model-a", Evaluate(ModeIntelligence, standard, "wrong", standard.Answer), 2, 10)
	assert.Equal(t, LevelBasic, state.LevelFor("model-a"))
	change := state.Apply("model-a", Evaluate(ModeIntelligence, basic, "ANSWER:"+basic.Answer, basic.Answer), 3, 10)
	assert.Equal(t, LevelBasic, change.State.StableLevel)
	assert.Equal(t, StatusHealthy, change.State.Status)
}

func TestProbeSelectsModelsRoundRobinAndPinsDegradedModel(t *testing.T) {
	state := State{}
	models := []string{" model-a ", "model-b", "model-a"}
	assert.Equal(t, "model-a", state.SelectModel(models))
	assert.Equal(t, "model-b", state.SelectModel(models))
	state.BlockedModel = "model-b"
	assert.Equal(t, "model-b", state.SelectModel(models))
}

func TestProbeStateRoundTripPreservesOtherChannelMetadata(t *testing.T) {
	state := State{Models: map[string]ModelState{
		"model-a": {StableLevel: LevelStandard, Status: StatusHealthy},
	}}
	raw, err := StateIntoOtherInfo(`{"status_reason":"upstream timeout"}`, state)
	require.NoError(t, err)

	var other map[string]any
	require.NoError(t, common.UnmarshalJsonStr(raw, &other))
	assert.Equal(t, "upstream timeout", other["status_reason"])
	assert.Equal(t, LevelStandard, StateFromOtherInfo(raw).Models["model-a"].StableLevel)
}

func TestProbeResetsBaselineWhenModeChanges(t *testing.T) {
	state := State{Models: map[string]ModelState{
		"model-a": {StableLevel: LevelAdvanced, Status: StatusDegraded, ConsecutiveFailure: 3},
	}}

	change := state.Apply("model-a", Evaluation{Mode: ModeCustom, Level: ModeCustom, Outcome: OutcomePass}, 1, 10)
	assert.Equal(t, ModeCustom, change.State.StableLevel)
	assert.Equal(t, StatusHealthy, change.State.Status)
	assert.False(t, change.Recovered)

	advanced := requireChallenge(t, LevelAdvanced)
	change = state.Apply("model-a", Evaluate(ModeIntelligence, advanced, "ANSWER:"+advanced.Answer, advanced.Answer), 2, 10)
	assert.Equal(t, LevelAdvanced, change.State.StableLevel)
	assert.Equal(t, StatusHealthy, change.State.Status)
}

func TestAutoPauseRecoveryUsesTwoPassHysteresis(t *testing.T) {
	state := State{}
	failure := Failure{OccurredAt: 10, StatusCode: 404, ErrorCode: "model_not_found", Requests: 3}

	paused := state.Pause("model-a", "confirmed model failure", 10, 40, failure)
	assert.True(t, paused.AutoPaused)
	assert.Equal(t, StatusDegraded, paused.Status)
	assert.Equal(t, int64(40), paused.NextProbeAt)

	change := state.RecordAutoProbe("model-a", true, "pass", 40, 70, nil)
	assert.False(t, change.Recovered)
	assert.True(t, change.State.AutoPaused)
	assert.Equal(t, int64(70), change.State.NextProbeAt)

	change = state.RecordAutoProbe("model-a", true, "pass", 70, 100, nil)
	assert.True(t, change.Recovered)
	assert.False(t, change.State.AutoPaused)
	assert.Empty(t, change.State.PauseReason)
	assert.Zero(t, change.State.PausedAt)
	assert.Equal(t, failure, *change.State.LastFailure)

	healthy := state.RecordAutoProbe("model-b", true, "candidate_pass", 80, 0, nil)
	assert.Equal(t, StatusHealthy, healthy.State.Status)
}

func requireChallenge(t *testing.T, level string) Challenge {
	t.Helper()
	challenges := GenerateChallenges(1, 42, level)
	require.Len(t, challenges, 1)
	return challenges[0]
}
