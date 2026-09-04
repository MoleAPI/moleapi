package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/channelprobe"
)

type challenge = channelprobe.Challenge

const (
	levelAll      = "all"
	levelBasic    = channelprobe.LevelBasic
	levelStandard = channelprobe.LevelStandard
	levelAdvanced = channelprobe.LevelAdvanced
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
	return channelprobe.GenerateChallenges(count, seed, level)
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
	return channelprobe.ExtractAnswer(content)
}

func normalizeAnswer(value string) string {
	return channelprobe.NormalizeAnswer(value)
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
