package common

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetSystemRedirectedModelName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
		redirect bool
	}{
		{name: "mole gpt alias", input: "mole-gpt5.6-sol", expected: "gpt-5.6-sol", redirect: true},
		{name: "bare gpt alias", input: "gpt5.6-sol", expected: "gpt-5.6-sol", redirect: true},
		{name: "canonical gpt", input: "gpt-5.6-sol", expected: "gpt-5.6-sol", redirect: false},
		{name: "mole non gpt", input: "mole-claude-opus-4-6", expected: "claude-opus-4-6", redirect: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual, redirected := GetSystemRedirectedModelName(tt.input)
			assert.Equal(t, tt.expected, actual)
			assert.Equal(t, tt.redirect, redirected)
		})
	}
}

func TestIsImageGenerationModelIncludesGPTImage2(t *testing.T) {
	assert.True(t, IsImageGenerationModel("gpt-image-2"))
}
