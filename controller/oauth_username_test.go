package controller

import (
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGeneratedOAuthUsername(t *testing.T) {
	tests := []struct {
		name     string
		prefix   string
		nextID   int
		expected string
	}{
		{name: "built in prefix", prefix: "github_", nextID: 42, expected: "github_42"},
		{name: "short custom prefix", prefix: "a_", nextID: 1, expected: "ua_1"},
		{name: "invalid prefix falls back", prefix: "bad.", nextID: 1, expected: "uuu1"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			username := generatedOAuthUsername(test.prefix, test.nextID)
			require.NoError(t, model.ValidateUsername(username))
			assert.Equal(t, test.expected, username)
		})
	}

	longPrefixUsername := generatedOAuthUsername("verylongproviderprefix_", 123)
	require.NoError(t, model.ValidateUsername(longPrefixUsername))
	assert.True(t, strings.HasSuffix(longPrefixUsername, "123"))
	assert.LessOrEqual(t, len(longPrefixUsername), model.UserNameMaxLength)
}
