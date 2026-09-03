package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
)

type challenge struct {
	ID     string
	Kind   string
	Level  string
	Prompt string
	Answer string
}

const (
	levelAll      = "all"
	levelBasic    = "basic"
	levelStandard = "standard"
	levelAdvanced = "advanced"
)

type chatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}

type categoryResult struct {
	Total           int
	Correct         int
	ExactFormat     int
	RequestErrors   int
	NoAnswer        int
	BudgetExhausted int
}

func main() {
	count := flag.Int("count", 30, "number of generated challenges")
	seed := flag.Int64("seed", 20260902, "challenge seed shared by every model")
	level := flag.String("level", levelAll, "challenge level: basic, standard, advanced, or all")
	dryRun := flag.Bool("dry-run", false, "print challenges without calling a model")
	maxOutput := flag.Int("max-output", 64, "maximum output tokens per request")
	timeout := flag.Int("timeout", 60, "request timeout in seconds")
	reasoningEffort := flag.String("reasoning-effort", "", "optional reasoning effort such as none or low")
	disableThinking := flag.Bool("disable-thinking", false, "send enable_thinking=false for compatible models")
	flag.Parse()

	if *count < 1 || *count > 200 {
		fmt.Fprintln(os.Stderr, "count must be between 1 and 200")
		os.Exit(2)
	}
	if *maxOutput < 1 || *maxOutput > 512 {
		fmt.Fprintln(os.Stderr, "max-output must be between 1 and 512")
		os.Exit(2)
	}
	if *timeout < 1 || *timeout > 600 {
		fmt.Fprintln(os.Stderr, "timeout must be between 1 and 600 seconds")
		os.Exit(2)
	}
	*level = strings.ToLower(strings.TrimSpace(*level))
	switch *level {
	case levelAll, levelBasic, levelStandard, levelAdvanced:
	default:
		fmt.Fprintln(os.Stderr, "level must be basic, standard, advanced, or all")
		os.Exit(2)
	}

	challenges := generateChallenges(*count, *seed, *level)
	endpoint := strings.TrimSpace(os.Getenv("PROBE_URL"))
	apiKey := strings.TrimSpace(os.Getenv("PROBE_API_KEY"))
	models := splitModels(os.Getenv("PROBE_MODELS"))
	if *dryRun || endpoint == "" || apiKey == "" || len(models) == 0 {
		if !*dryRun {
			fmt.Fprintln(os.Stderr, "live run skipped: set PROBE_URL, PROBE_API_KEY, and PROBE_MODELS")
		}
		printChallenges(challenges)
		return
	}

	client := &http.Client{Timeout: time.Duration(*timeout) * time.Second}
	for _, model := range models {
		runModel(client, endpoint, apiKey, model, challenges, *maxOutput, *reasoningEffort, *disableThinking)
	}
}

func generateChallenges(count int, seed int64, level string) []challenge {
	rng := rand.New(rand.NewSource(seed))
	generators := []struct {
		level    string
		generate func(*rand.Rand, int) challenge
	}{
		{level: levelStandard, generate: arithmeticChallenge},
		{level: levelAdvanced, generate: orderingChallenge},
		{level: levelAdvanced, generate: sequenceChallenge},
		{level: levelBasic, generate: tableChallenge},
		{level: levelAdvanced, generate: navigationChallenge},
		{level: levelStandard, generate: conditionalChallenge},
	}
	if level != levelAll {
		filtered := generators[:0]
		for _, generator := range generators {
			if generator.level == level {
				filtered = append(filtered, generator)
			}
		}
		generators = filtered
	}
	challenges := make([]challenge, 0, count)
	for i := range count {
		generator := generators[i%len(generators)]
		item := generator.generate(rng, i+1)
		item.Level = generator.level
		challenges = append(challenges, item)
	}
	return challenges
}

func arithmeticChallenge(rng *rand.Rand, index int) challenge {
	a, b := rng.Intn(70)+20, rng.Intn(8)+3
	c, d, e := rng.Intn(80)+10, rng.Intn(7)+2, rng.Intn(40)+5
	answer := ((a*b)-c)*d + e
	return challenge{
		ID:     fmt.Sprintf("arithmetic-%02d", index),
		Kind:   "arithmetic",
		Prompt: fmt.Sprintf("Compute exactly: ((%d × %d) − %d) × %d + %d. Do not explain. Reply with ANSWER: followed by the integer.", a, b, c, d, e),
		Answer: strconv.Itoa(answer),
	}
}

func orderingChallenge(rng *rand.Rand, index int) challenge {
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
	return challenge{
		ID:     fmt.Sprintf("ordering-%02d", index),
		Kind:   "ordering",
		Prompt: fmt.Sprintf("Six people stand in one line. %s. Who is in position %d counting from the front? Do not explain. Reply with ANSWER: followed by the name.", strings.Join(clues, "; "), targetPosition+1),
		Answer: names[targetPosition],
	}
}

func sequenceChallenge(rng *rand.Rand, index int) challenge {
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
	return challenge{
		ID:     fmt.Sprintf("sequence-%02d", index),
		Kind:   "sequence",
		Prompt: fmt.Sprintf("Start with the sequence %s. Rotate it left by %d positions, reverse the whole sequence, then swap positions %d and %d (positions start at 1). Do not explain. Reply with ANSWER: followed by the final sequence without spaces.", start, rotation, swapA+1, swapB+1),
		Answer: strings.Join(items, ""),
	}
}

func tableChallenge(rng *rand.Rand, index int) challenge {
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
	return challenge{
		ID:     fmt.Sprintf("table-%02d", index),
		Kind:   "table",
		Prompt: fmt.Sprintf("Rows are written as name=color/value: %s. Sum the values of only the %s rows whose value is %s. If none match, use 0. Do not explain. Reply with ANSWER: followed by the integer.", strings.Join(rows, ", "), targetType, parityName),
		Answer: strconv.Itoa(sum),
	}
}

func navigationChallenge(rng *rand.Rand, index int) challenge {
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
	return challenge{
		ID:     fmt.Sprintf("navigation-%02d", index),
		Kind:   "navigation",
		Prompt: fmt.Sprintf("Begin at (0,0) facing %s. North increases y and east increases x. Perform in order: %s. Do not explain. Reply as ANSWER:x,y,direction using the actual coordinates and N/E/S/W, without angle brackets.", startDirection, strings.Join(steps, "; ")),
		Answer: fmt.Sprintf("%d,%d,%s", x, y, directions[direction]),
	}
}

func conditionalChallenge(rng *rand.Rand, index int) challenge {
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
	return challenge{
		ID:     fmt.Sprintf("conditional-%02d", index),
		Kind:   "conditional",
		Prompt: fmt.Sprintf("Transform [%s]: replace each even number n with n/2, and each odd n with 3n+1. Keep only results greater than %d and sort ascending. Do not explain. Reply with ANSWER: followed by the comma-separated values, or reply ANSWER:NONE.", strings.Join(shown, ","), threshold),
		Answer: answer,
	}
}

func splitModels(value string) []string {
	parts := strings.Split(value, ",")
	models := make([]string, 0, len(parts))
	for _, part := range parts {
		if model := strings.TrimSpace(part); model != "" {
			models = append(models, model)
		}
	}
	return models
}

func printChallenges(challenges []challenge) {
	for _, item := range challenges {
		fmt.Printf("[%s] level=%s kind=%s expected=%s\n%s\n\n", item.ID, item.Level, item.Kind, item.Answer, item.Prompt)
	}
}

func runModel(client *http.Client, endpoint string, apiKey string, model string, challenges []challenge, maxOutput int, reasoningEffort string, disableThinking bool) {
	results := make(map[string]*categoryResult)
	levelResults := make(map[string]*categoryResult)
	totalPromptTokens, totalCompletionTokens := 0, 0
	fmt.Printf("\nMODEL %s\n", model)
	for _, item := range challenges {
		result := results[item.Kind]
		if result == nil {
			result = &categoryResult{}
			results[item.Kind] = result
		}
		levelResult := levelResults[item.Level]
		if levelResult == nil {
			levelResult = &categoryResult{}
			levelResults[item.Level] = levelResult
		}
		targets := []*categoryResult{result, levelResult}
		for _, target := range targets {
			target.Total++
		}

		startedAt := time.Now()
		content, usage, err := callModel(client, endpoint, apiKey, model, item.Prompt, maxOutput, reasoningEffort, disableThinking)
		latency := time.Since(startedAt).Round(time.Millisecond)
		if err != nil {
			for _, target := range targets {
				target.RequestErrors++
			}
			fmt.Printf("%-16s request_error=%q latency=%s\n", item.ID, truncate(err.Error(), 200), latency)
			continue
		}
		extracted, found, exact := extractAnswer(content)
		correct := found && normalizeAnswer(extracted) == normalizeAnswer(item.Answer)
		if correct {
			for _, target := range targets {
				target.Correct++
			}
		}
		if exact {
			for _, target := range targets {
				target.ExactFormat++
			}
		}
		if !found {
			for _, target := range targets {
				target.NoAnswer++
			}
		}
		if usage.FinishReason == "length" {
			for _, target := range targets {
				target.BudgetExhausted++
			}
		}
		totalPromptTokens += usage.PromptTokens
		totalCompletionTokens += usage.CompletionTokens
		fmt.Printf("%-16s correct=%-5t exact=%-5t expected=%-12s got=%q tokens=%d+%d finish=%s latency=%s\n", item.ID, correct, exact, item.Answer, truncate(content, 80), usage.PromptTokens, usage.CompletionTokens, usage.FinishReason, latency)
	}
	for _, level := range []string{levelBasic, levelStandard, levelAdvanced} {
		if result := levelResults[level]; result != nil {
			fmt.Printf("LEVEL %-12s correct=%d/%d exact_format=%d/%d errors=%d no_answer=%d budget_exhausted=%d\n", level, result.Correct, result.Total, result.ExactFormat, result.Total, result.RequestErrors, result.NoAnswer, result.BudgetExhausted)
		}
	}

	kinds := make([]string, 0, len(results))
	for kind := range results {
		kinds = append(kinds, kind)
	}
	sort.Strings(kinds)
	total, correct, exact, requestErrors, noAnswer, budgetExhausted := 0, 0, 0, 0, 0, 0
	for _, kind := range kinds {
		result := results[kind]
		total += result.Total
		correct += result.Correct
		exact += result.ExactFormat
		requestErrors += result.RequestErrors
		noAnswer += result.NoAnswer
		budgetExhausted += result.BudgetExhausted
		fmt.Printf("CATEGORY %-12s correct=%d/%d exact_format=%d/%d errors=%d no_answer=%d budget_exhausted=%d\n", kind, result.Correct, result.Total, result.ExactFormat, result.Total, result.RequestErrors, result.NoAnswer, result.BudgetExhausted)
	}
	fmt.Printf("SUMMARY correct=%d/%d (%.1f%%) exact_format=%d/%d errors=%d no_answer=%d budget_exhausted=%d tokens=%d+%d\n", correct, total, percentage(correct, total), exact, total, requestErrors, noAnswer, budgetExhausted, totalPromptTokens, totalCompletionTokens)
}

func callModel(client *http.Client, endpoint string, apiKey string, model string, prompt string, maxOutput int, reasoningEffort string, disableThinking bool) (string, chatResponseUsage, error) {
	requestBody := map[string]any{
		"model":       model,
		"temperature": 0,
		"max_tokens":  maxOutput,
		"messages": []map[string]string{
			{"role": "system", "content": "Follow the requested answer format exactly."},
			{"role": "user", "content": prompt},
		},
	}
	if reasoningEffort = strings.TrimSpace(reasoningEffort); reasoningEffort != "" {
		requestBody["reasoning_effort"] = reasoningEffort
	}
	if disableThinking {
		requestBody["enable_thinking"] = false
	}
	body, err := common.Marshal(requestBody)
	if err != nil {
		return "", chatResponseUsage{}, err
	}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return "", chatResponseUsage{}, err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return "", chatResponseUsage{}, err
	}
	defer func() { _ = resp.Body.Close() }()
	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", chatResponseUsage{}, err
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return "", chatResponseUsage{}, fmt.Errorf("HTTP %d: %s", resp.StatusCode, truncate(string(responseBody), 300))
	}
	var parsed chatResponse
	if err := common.Unmarshal(responseBody, &parsed); err != nil {
		return "", chatResponseUsage{}, err
	}
	if len(parsed.Choices) == 0 {
		return "", chatResponseUsage{}, fmt.Errorf("response has no choices")
	}
	return parsed.Choices[0].Message.Content, chatResponseUsage{
		PromptTokens:     parsed.Usage.PromptTokens,
		CompletionTokens: parsed.Usage.CompletionTokens,
		FinishReason:     parsed.Choices[0].FinishReason,
	}, nil
}

type chatResponseUsage struct {
	PromptTokens     int
	CompletionTokens int
	FinishReason     string
}

func extractAnswer(content string) (answer string, found bool, exact bool) {
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

func normalizeAnswer(value string) string {
	value = strings.TrimSpace(strings.Trim(value, "`\"'"))
	value = strings.ReplaceAll(value, " ", "")
	return strings.ToUpper(value)
}

func truncate(value string, limit int) string {
	value = strings.Join(strings.Fields(value), " ")
	if len(value) <= limit {
		return value
	}
	return value[:limit] + "…"
}

func percentage(value int, total int) float64 {
	if total == 0 {
		return 0
	}
	return float64(value) * 100 / float64(total)
}
