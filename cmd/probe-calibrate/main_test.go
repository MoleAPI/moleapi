package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateChallengesIsDeterministicAndComplete(t *testing.T) {
	first := generateChallenges(30, 42, levelAll)
	second := generateChallenges(30, 42, levelAll)
	require.Len(t, first, 30)
	assert.Equal(t, first, second)

	seenIDs := make(map[string]struct{}, len(first))
	seenKinds := make(map[string]struct{})
	seenLevels := make(map[string]struct{})
	for _, item := range first {
		assert.NotEmpty(t, item.ID)
		assert.NotEmpty(t, item.Prompt)
		assert.NotEmpty(t, item.Answer)
		_, duplicate := seenIDs[item.ID]
		assert.False(t, duplicate, "duplicate challenge ID %s", item.ID)
		seenIDs[item.ID] = struct{}{}
		seenKinds[item.Kind] = struct{}{}
		seenLevels[item.Level] = struct{}{}
	}
	assert.Len(t, seenKinds, 6)
	assert.Len(t, seenLevels, 3)

	advanced := generateChallenges(9, 42, levelAdvanced)
	require.Len(t, advanced, 9)
	for _, item := range advanced {
		assert.Equal(t, levelAdvanced, item.Level)
	}
}

func TestExtractAnswerSeparatesCorrectnessFromFormat(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
		found   bool
		exact   bool
	}{
		{name: "exact", content: "ANSWER:42", want: "42", found: true, exact: true},
		{name: "case insensitive", content: "answer:Kiro", want: "Kiro", found: true, exact: true},
		{name: "explanation then answer", content: "Working omitted.\nANSWER:1, 2, E", want: "1, 2, E", found: true, exact: false},
		{name: "bare answer", content: "42", want: "42", found: true, exact: false},
		{name: "empty", content: "  ", found: false, exact: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			answer, found, exact := extractAnswer(test.content)
			assert.Equal(t, test.want, answer)
			assert.Equal(t, test.found, found)
			assert.Equal(t, test.exact, exact)
		})
	}
	assert.Equal(t, normalizeAnswer(" 1, 2, e "), normalizeAnswer("1,2,E"))
}

func TestCallModelUsesOpenAICompatibleRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "Bearer test-key", r.Header.Get("Authorization"))
		var request map[string]any
		require.NoError(t, common.DecodeJson(r.Body, &request))
		assert.Equal(t, "test-model", request["model"])
		assert.Equal(t, float64(32), request["max_tokens"])
		assert.Equal(t, "none", request["reasoning_effort"])
		assert.Equal(t, false, request["enable_thinking"])

		response, err := common.Marshal(map[string]any{
			"choices": []map[string]any{{"message": map[string]string{"content": "ANSWER:42"}, "finish_reason": "stop"}},
			"usage":   map[string]int{"prompt_tokens": 20, "completion_tokens": 3, "total_tokens": 23},
		})
		require.NoError(t, err)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(response)
	}))
	defer server.Close()

	content, usage, err := callModel(server.Client(), server.URL, "test-key", "test-model", "question", 32, "none", true)
	require.NoError(t, err)
	assert.Equal(t, "ANSWER:42", content)
	assert.Equal(t, 20, usage.PromptTokens)
	assert.Equal(t, 3, usage.CompletionTokens)
	assert.Equal(t, "stop", usage.FinishReason)
}
