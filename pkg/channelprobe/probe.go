package channelprobe

import (
	"strings"

	"github.com/QuantumNous/new-api/common"
)

const (
	ModeHi = "hi"

	StatusPending  = "pending"
	StatusHealthy  = "healthy"
	StatusDegraded = "degraded"

	OutcomePass = "pass"
)

type Evaluation struct {
	Mode    string `json:"mode"`
	Outcome string `json:"outcome"`
}

func (e Evaluation) Passed() bool { return e.Outcome == OutcomePass }

type ModelState struct {
	Status          string   `json:"status"`
	AutoPaused      bool     `json:"auto_paused,omitempty"`
	PauseReason     string   `json:"pause_reason,omitempty"`
	PausedAt        int64    `json:"paused_at,omitempty"`
	NextProbeAt     int64    `json:"next_probe_at,omitempty"`
	LastAutoProbeAt int64    `json:"last_auto_probe_at,omitempty"`
	LastProbeResult string   `json:"last_probe_result,omitempty"`
	LastRecoveredAt int64    `json:"last_recovered_at,omitempty"`
	LastFailure     *Failure `json:"last_failure,omitempty"`
}

type Failure struct {
	OccurredAt int64  `json:"occurred_at"`
	StatusCode int    `json:"status_code,omitempty"`
	ErrorType  string `json:"error_type,omitempty"`
	ErrorCode  string `json:"error_code,omitempty"`
	Requests   int    `json:"requests,omitempty"`
}

type State struct {
	Models map[string]ModelState `json:"models,omitempty"`
}

type StateChange struct {
	Recovered bool
	State     ModelState
}

func StateFromOtherInfo(raw string) State {
	state := State{}
	if strings.TrimSpace(raw) == "" {
		return state
	}
	other := make(map[string]any)
	if err := common.UnmarshalJsonStr(raw, &other); err != nil {
		return state
	}
	encoded, err := common.Marshal(other["channel_probe"])
	if err != nil {
		return state
	}
	_ = common.Unmarshal(encoded, &state)
	return state
}

func StateIntoOtherInfo(raw string, state State) (string, error) {
	other := make(map[string]any)
	if strings.TrimSpace(raw) != "" {
		if err := common.UnmarshalJsonStr(raw, &other); err != nil {
			return "", err
		}
	}
	other["channel_probe"] = state
	encoded, err := common.Marshal(other)
	return string(encoded), err
}

func (s *State) IsAutoPaused(model string) bool {
	return s.Models != nil && s.Models[model].AutoPaused
}

func (s *State) Pause(model string, reason string, now int64, nextProbeAt int64, failure Failure) ModelState {
	state := s.modelState(model)
	state.Status = StatusDegraded
	state.AutoPaused = true
	state.PauseReason = reason
	state.PausedAt = now
	state.NextProbeAt = nextProbeAt
	state.LastAutoProbeAt = now
	state.LastProbeResult = "failed"
	state.LastFailure = &failure
	s.setModelState(model, state)
	return state
}

func (s *State) RecordAutoProbe(model string, passed bool, result string, now int64, nextProbeAt int64, failure *Failure) StateChange {
	state := s.modelState(model)
	wasPaused := state.AutoPaused
	state.LastAutoProbeAt = now
	state.LastProbeResult = result
	if failure != nil {
		state.LastFailure = failure
	}
	if passed {
		state.Status = StatusHealthy
		if wasPaused {
			state.AutoPaused = false
			state.PauseReason = ""
			state.PausedAt = 0
			state.NextProbeAt = 0
			state.LastRecoveredAt = now
		}
	} else if wasPaused {
		state.Status = StatusDegraded
		state.NextProbeAt = nextProbeAt
	}
	s.setModelState(model, state)
	return StateChange{Recovered: wasPaused && !state.AutoPaused, State: state}
}

func (s *State) RecoverNow(model string, now int64) ModelState {
	state := s.modelState(model)
	state.Status = StatusHealthy
	state.AutoPaused = false
	state.PauseReason = ""
	state.PausedAt = 0
	state.NextProbeAt = 0
	state.LastAutoProbeAt = now
	state.LastProbeResult = "manual_pass"
	state.LastRecoveredAt = now
	s.setModelState(model, state)
	return state
}

func (s *State) modelState(model string) ModelState {
	if s.Models == nil {
		s.Models = make(map[string]ModelState)
	}
	state := s.Models[model]
	if state.Status == "" {
		state.Status = StatusPending
	}
	return state
}

func (s *State) setModelState(model string, state ModelState) {
	if s.Models == nil {
		s.Models = make(map[string]ModelState)
	}
	s.Models[model] = state
}
