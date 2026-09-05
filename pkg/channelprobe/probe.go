package channelprobe

import (
	"fmt"
	"math/rand"
	"sort"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
)

const (
	ModeHi           = "hi"
	ModeIntelligence = "intelligence"
	ModeCustom       = "custom"

	LevelBasic    = "basic"
	LevelStandard = "standard"
	LevelAdvanced = "advanced"

	StatusPending  = "pending"
	StatusHealthy  = "healthy"
	StatusDegraded = "degraded"

	OutcomePass      = "pass"
	OutcomeWrong     = "wrong"
	OutcomeNoAnswer  = "no_answer"
	OutcomeCompleted = "completed"
)

type Challenge struct {
	ID     string `json:"id"`
	Kind   string `json:"kind"`
	Level  string `json:"level"`
	Prompt string `json:"-"`
	Answer string `json:"-"`
}

type Evaluation struct {
	Mode           string `json:"mode"`
	QuestionID     string `json:"question_id,omitempty"`
	QuestionKind   string `json:"question_kind,omitempty"`
	Level          string `json:"level,omitempty"`
	Outcome        string `json:"outcome"`
	ExpectedAnswer string `json:"expected_answer,omitempty"`
	ActualAnswer   string `json:"actual_answer,omitempty"`
	ExactFormat    bool   `json:"exact_format,omitempty"`
}

func (e Evaluation) Passed() bool {
	return e.Outcome == OutcomePass || e.Outcome == OutcomeCompleted
}

type Sample struct {
	Level     string `json:"level"`
	Outcome   string `json:"outcome"`
	TestedAt  int64  `json:"tested_at"`
	LatencyMs int64  `json:"latency_ms"`
}

type ModelState struct {
	StableLevel        string   `json:"stable_level,omitempty"`
	CalibrationLevel   string   `json:"calibration_level,omitempty"`
	Status             string   `json:"status"`
	ConsecutivePasses  int      `json:"consecutive_passes,omitempty"`
	ConsecutiveFailure int      `json:"consecutive_failures,omitempty"`
	LastTestAt         int64    `json:"last_test_at,omitempty"`
	Recent             []Sample `json:"recent,omitempty"`
}

type State struct {
	NextModelIndex int                   `json:"next_model_index,omitempty"`
	BlockedModel   string                `json:"blocked_model,omitempty"`
	Models         map[string]ModelState `json:"models,omitempty"`
}

type StateChange struct {
	Degraded  bool
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

func GenerateChallenges(count int, seed int64, level string) []Challenge {
	if count <= 0 {
		return nil
	}
	rng := rand.New(rand.NewSource(seed))
	generators := []struct {
		level    string
		generate func(*rand.Rand, int) Challenge
	}{
		{level: LevelBasic, generate: tableChallenge},
		{level: LevelStandard, generate: arithmeticChallenge},
		{level: LevelStandard, generate: conditionalChallenge},
		{level: LevelAdvanced, generate: orderingChallenge},
		{level: LevelAdvanced, generate: sequenceChallenge},
		{level: LevelAdvanced, generate: navigationChallenge},
	}
	if level != "" && level != "all" {
		filtered := generators[:0]
		for _, generator := range generators {
			if generator.level == level {
				filtered = append(filtered, generator)
			}
		}
		generators = filtered
	}
	if len(generators) == 0 {
		return nil
	}
	start := int(uint64(seed) % uint64(len(generators)))
	challenges := make([]Challenge, 0, count)
	for i := range count {
		generator := generators[(start+i)%len(generators)]
		item := generator.generate(rng, i+1)
		item.Level = generator.level
		challenges = append(challenges, item)
	}
	return challenges
}

func Evaluate(mode string, challenge Challenge, content string, expected string) Evaluation {
	answer, found, exact := ExtractAnswer(content)
	evaluation := Evaluation{
		Mode:           mode,
		QuestionID:     challenge.ID,
		QuestionKind:   challenge.Kind,
		Level:          challenge.Level,
		ExpectedAnswer: expected,
		ActualAnswer:   answer,
		ExactFormat:    exact,
	}
	if strings.TrimSpace(expected) == "" {
		evaluation.Outcome = OutcomeCompleted
		return evaluation
	}
	if !found {
		evaluation.Outcome = OutcomeNoAnswer
		return evaluation
	}
	if NormalizeAnswer(answer) == NormalizeAnswer(expected) {
		evaluation.Outcome = OutcomePass
		return evaluation
	}
	evaluation.Outcome = OutcomeWrong
	return evaluation
}

func ExtractAnswer(content string) (answer string, found bool, exact bool) {
	trimmed := strings.TrimSpace(content)
	for _, line := range strings.Split(trimmed, "\n") {
		line = strings.TrimSpace(strings.Trim(line, "`"))
		if len(line) >= len("ANSWER:") && strings.EqualFold(line[:len("ANSWER:")], "ANSWER:") {
			value := strings.TrimSpace(line[len("ANSWER:"):])
			return value, value != "", trimmed == line
		}
	}
	if trimmed == "" {
		return "", false, false
	}
	return trimmed, true, false
}

func NormalizeAnswer(value string) string {
	value = strings.TrimSpace(strings.Trim(value, "`\"'"))
	value = strings.ReplaceAll(value, " ", "")
	return strings.ToUpper(value)
}

func (s *State) SelectModel(models []string) string {
	models = normalizeModels(models)
	if len(models) == 0 {
		return ""
	}
	if s.BlockedModel != "" {
		for _, model := range models {
			if model == s.BlockedModel {
				return model
			}
		}
		s.BlockedModel = ""
	}
	if s.NextModelIndex < 0 || s.NextModelIndex >= len(models) {
		s.NextModelIndex = 0
	}
	model := models[s.NextModelIndex]
	s.NextModelIndex = (s.NextModelIndex + 1) % len(models)
	return model
}

func (s *State) LevelFor(model string) string {
	state := s.modelState(model)
	if state.StableLevel != "" {
		return state.StableLevel
	}
	if state.CalibrationLevel != "" {
		return state.CalibrationLevel
	}
	return LevelAdvanced
}

func (s *State) Apply(model string, evaluation Evaluation, testedAt int64, latencyMs int64) StateChange {
	// ponytail: fixed 3-down/2-up hysteresis avoids more settings; expose it only if production data shows one threshold cannot fit all routes.
	state := s.modelState(model)
	if state.StableLevel != "" && ((evaluation.Mode == ModeCustom && state.StableLevel != ModeCustom) ||
		(evaluation.Mode == ModeIntelligence && state.StableLevel == ModeCustom)) {
		if s.BlockedModel == model {
			s.BlockedModel = ""
		}
		state = ModelState{Status: StatusPending}
	}
	wasDegraded := state.Status == StatusDegraded
	state.LastTestAt = testedAt

	if evaluation.Outcome != OutcomePass && evaluation.Outcome != OutcomeWrong && evaluation.Outcome != OutcomeNoAnswer {
		s.setModelState(model, state)
		return StateChange{State: state}
	}

	state.Recent = append(state.Recent, Sample{
		Level:     evaluation.Level,
		Outcome:   evaluation.Outcome,
		TestedAt:  testedAt,
		LatencyMs: latencyMs,
	})
	if len(state.Recent) > 5 {
		state.Recent = state.Recent[len(state.Recent)-5:]
	}

	if state.StableLevel == "" {
		if evaluation.Mode == ModeCustom {
			state.StableLevel = ModeCustom
			if evaluation.Outcome == OutcomePass {
				state.Status = StatusHealthy
				state.ConsecutivePasses = 1
			} else {
				state.Status = StatusPending
				state.ConsecutiveFailure = 1
			}
			s.setModelState(model, state)
			return StateChange{State: state}
		}
		if evaluation.Outcome == OutcomePass {
			state.StableLevel = evaluation.Level
			state.CalibrationLevel = ""
			state.Status = StatusHealthy
			state.ConsecutivePasses = 1
			state.ConsecutiveFailure = 0
		} else {
			switch evaluation.Level {
			case LevelAdvanced:
				state.CalibrationLevel = LevelStandard
			case LevelStandard:
				state.CalibrationLevel = LevelBasic
			default:
				state.StableLevel = LevelBasic
				state.CalibrationLevel = ""
				state.Status = StatusPending
				state.ConsecutiveFailure = 1
			}
		}
		s.setModelState(model, state)
		return StateChange{State: state}
	}

	if evaluation.Outcome == OutcomePass {
		state.ConsecutivePasses++
		state.ConsecutiveFailure = 0
		if state.Status == StatusPending {
			state.Status = StatusHealthy
		}
		if wasDegraded && state.ConsecutivePasses >= 2 {
			state.Status = StatusHealthy
			s.BlockedModel = ""
		}
	} else {
		state.ConsecutivePasses = 0
		state.ConsecutiveFailure++
		if state.ConsecutiveFailure >= 3 {
			state.Status = StatusDegraded
			s.BlockedModel = model
		}
	}
	s.setModelState(model, state)
	return StateChange{
		Degraded:  !wasDegraded && state.Status == StatusDegraded,
		Recovered: wasDegraded && state.Status == StatusHealthy,
		State:     state,
	}
}

func (s *State) RecordRequestError(model string, testedAt int64) {
	state := s.modelState(model)
	state.LastTestAt = testedAt
	s.setModelState(model, state)
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

func normalizeModels(models []string) []string {
	seen := make(map[string]struct{}, len(models))
	normalized := make([]string, 0, len(models))
	for _, model := range models {
		model = strings.TrimSpace(model)
		if model == "" {
			continue
		}
		if _, ok := seen[model]; ok {
			continue
		}
		seen[model] = struct{}{}
		normalized = append(normalized, model)
	}
	return normalized
}

func arithmeticChallenge(rng *rand.Rand, index int) Challenge {
	a, b := rng.Intn(70)+20, rng.Intn(8)+3
	c, d, e := rng.Intn(80)+10, rng.Intn(7)+2, rng.Intn(40)+5
	answer := ((a*b)-c)*d + e
	return Challenge{
		ID:     fmt.Sprintf("arithmetic-%02d", index),
		Kind:   "arithmetic",
		Prompt: fmt.Sprintf("Compute exactly: ((%d × %d) − %d) × %d + %d. Do not explain. Reply with ANSWER: followed by the integer.", a, b, c, d, e),
		Answer: strconv.Itoa(answer),
	}
}

func orderingChallenge(rng *rand.Rand, index int) Challenge {
	names := []string{"Kiro", "Luma", "Navi", "Pavo", "Reni", "Sola"}
	rng.Shuffle(len(names), func(i, j int) { names[i], names[j] = names[j], names[i] })
	clues := make([]string, 0, len(names)-1)
	for i := 0; i < len(names)-1; i++ {
		if rng.Intn(2) == 0 {
			clues = append(clues, fmt.Sprintf("%s is immediately before %s", names[i], names[i+1]))
		} else {
			clues = append(clues, fmt.Sprintf("%s is immediately after %s", names[i+1], names[i]))
		}
	}
	rng.Shuffle(len(clues), func(i, j int) { clues[i], clues[j] = clues[j], clues[i] })
	targetPosition := rng.Intn(len(names))
	return Challenge{
		ID:     fmt.Sprintf("ordering-%02d", index),
		Kind:   "ordering",
		Prompt: fmt.Sprintf("Six people stand in one line. %s. Who is in position %d counting from the front? Do not explain. Reply with ANSWER: followed by the name.", strings.Join(clues, "; "), targetPosition+1),
		Answer: names[targetPosition],
	}
}

func sequenceChallenge(rng *rand.Rand, index int) Challenge {
	letters := []string{"A", "B", "C", "D", "E", "F", "G", "H"}
	rng.Shuffle(len(letters), func(i, j int) { letters[i], letters[j] = letters[j], letters[i] })
	items := append([]string(nil), letters[:6]...)
	start := strings.Join(items, "")
	rotation := rng.Intn(3) + 1
	items = append(append([]string(nil), items[rotation:]...), items[:rotation]...)
	for i, j := 0, len(items)-1; i < j; i, j = i+1, j-1 {
		items[i], items[j] = items[j], items[i]
	}
	swapA, swapB := rng.Intn(3), rng.Intn(3)+3
	items[swapA], items[swapB] = items[swapB], items[swapA]
	return Challenge{
		ID:     fmt.Sprintf("sequence-%02d", index),
		Kind:   "sequence",
		Prompt: fmt.Sprintf("Start with the sequence %s. Rotate it left by %d positions, reverse the whole sequence, then swap positions %d and %d (positions start at 1). Do not explain. Reply with ANSWER: followed by the final sequence without spaces.", start, rotation, swapA+1, swapB+1),
		Answer: strings.Join(items, ""),
	}
}

func tableChallenge(rng *rand.Rand, index int) Challenge {
	types := []string{"red", "blue"}
	rows := make([]string, 0, 6)
	sum := 0
	targetType := types[rng.Intn(len(types))]
	parity := rng.Intn(2)
	for i := range 6 {
		kind := types[rng.Intn(len(types))]
		value := rng.Intn(40) + 10
		rows = append(rows, fmt.Sprintf("R%d=%s/%d", i+1, kind, value))
		if kind == targetType && value%2 == parity {
			sum += value
		}
	}
	parityName := "even"
	if parity == 1 {
		parityName = "odd"
	}
	return Challenge{
		ID:     fmt.Sprintf("table-%02d", index),
		Kind:   "table",
		Prompt: fmt.Sprintf("Rows are written as name=color/value: %s. Sum the values of only the %s rows whose value is %s. If none match, use 0. Do not explain. Reply with ANSWER: followed by the integer.", strings.Join(rows, ", "), targetType, parityName),
		Answer: strconv.Itoa(sum),
	}
}

func navigationChallenge(rng *rand.Rand, index int) Challenge {
	directions := []string{"N", "E", "S", "W"}
	direction := rng.Intn(4)
	startDirection := directions[direction]
	x, y := 0, 0
	steps := make([]string, 0, 7)
	for i := range 7 {
		if i%2 == 1 {
			if rng.Intn(2) == 0 {
				direction = (direction + 3) % 4
				steps = append(steps, "turn left")
			} else {
				direction = (direction + 1) % 4
				steps = append(steps, "turn right")
			}
			continue
		}
		distance := rng.Intn(4) + 1
		switch direction {
		case 0:
			y += distance
		case 1:
			x += distance
		case 2:
			y -= distance
		case 3:
			x -= distance
		}
		steps = append(steps, fmt.Sprintf("move %d", distance))
	}
	return Challenge{
		ID:     fmt.Sprintf("navigation-%02d", index),
		Kind:   "navigation",
		Prompt: fmt.Sprintf("Begin at (0,0) facing %s. North increases y and east increases x. Perform in order: %s. Do not explain. Reply as ANSWER:x,y,direction using the actual coordinates and N/E/S/W, without angle brackets.", startDirection, strings.Join(steps, "; ")),
		Answer: fmt.Sprintf("%d,%d,%s", x, y, directions[direction]),
	}
}

func conditionalChallenge(rng *rand.Rand, index int) Challenge {
	numbers := make([]int, 7)
	shown := make([]string, len(numbers))
	threshold := rng.Intn(15) + 15
	kept := make([]int, 0, len(numbers))
	for i := range numbers {
		numbers[i] = rng.Intn(30) + 2
		shown[i] = strconv.Itoa(numbers[i])
		value := numbers[i] * 3
		if numbers[i]%2 == 0 {
			value = numbers[i] / 2
		} else {
			value++
		}
		if value > threshold {
			kept = append(kept, value)
		}
	}
	sort.Ints(kept)
	answerParts := make([]string, len(kept))
	for i, value := range kept {
		answerParts[i] = strconv.Itoa(value)
	}
	answer := "NONE"
	if len(answerParts) > 0 {
		answer = strings.Join(answerParts, ",")
	}
	return Challenge{
		ID:     fmt.Sprintf("conditional-%02d", index),
		Kind:   "conditional",
		Prompt: fmt.Sprintf("Transform [%s]: replace each even number n with n/2, and each odd n with 3n+1. Keep only results greater than %d and sort ascending. Do not explain. Reply with ANSWER: followed by the comma-separated values, or reply ANSWER:NONE.", strings.Join(shown, ","), threshold),
		Answer: answer,
	}
}
