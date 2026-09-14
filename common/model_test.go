package common

import (
	"testing"

	"github.com/QuantumNous/new-api/constant"
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
	for _, model := range []string{"gpt-image-2", "gpt-image-2.5-flare", "gpt-image-2.5-sunburst"} {
		assert.True(t, IsImageGenerationModel(model), model)
	}
}

func TestGetEndpointTypesByChannelTypeIncludesResponsesForOpenAITextModels(t *testing.T) {
	assert.Equal(t, []constant.EndpointType{
		constant.EndpointTypeOpenAI,
		constant.EndpointTypeOpenAIResponse,
	}, GetEndpointTypesByChannelType(constant.ChannelTypeOpenAI, "gpt-5.4"))
	assert.Equal(t, []constant.EndpointType{
		constant.EndpointTypeOpenAIResponse,
	}, GetEndpointTypesByChannelType(constant.ChannelTypeOpenAI, "o3-pro"))
	assert.Equal(t, []constant.EndpointType{
		constant.EndpointTypeImageGeneration,
		constant.EndpointTypeOpenAI,
	}, GetEndpointTypesByChannelType(constant.ChannelTypeOpenAI, "gpt-image-2"))
	assert.Equal(t, []constant.EndpointType{
		constant.EndpointTypeImageGeneration,
		constant.EndpointTypeOpenAI,
	}, GetEndpointTypesByChannelType(constant.ChannelTypeOpenAI, "gpt-image-2.5-flare"))
}

func TestWanEndpointsDistinguishImagesFromVideos(t *testing.T) {
	for _, name := range []string{
		"wan2.7-image-pro", "wan2.7-image", "wan2.6-image", "wan2.6-t2i",
		"wan2.5-t2i-preview", "wan2.2-t2i-flash", "wan2.2-t2i-plus",
		"wanx2.1-t2i-turbo", "wanx2.1-t2i-plus", "wanx2.0-t2i-turbo",
	} {
		t.Run(name, func(t *testing.T) {
			assert.Contains(t, GetEndpointTypesByChannelType(constant.ChannelTypeAli, name), constant.EndpointTypeImageGeneration)
		})
	}
	for _, name := range []string{
		"wanx2.1-t2v-plus", "wanx2.1-t2v-turbo", "wanx2.1-i2v-plus", "wanx2.1-i2v-turbo",
	} {
		t.Run(name, func(t *testing.T) {
			assert.NotContains(t, GetEndpointTypesByChannelType(constant.ChannelTypeAli, name), constant.EndpointTypeImageGeneration)
		})
	}
}
