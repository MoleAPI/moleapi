package model

import (
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAdvancedCustomChannelRequiresModelListRouteOnlyWhenUpdateChecksEnabled(t *testing.T) {
	inferenceRoute := dto.AdvancedCustomRoute{
		IncomingPath: "/v1/chat/completions",
		UpstreamPath: "/v1/chat/completions",
		Converter:    "none",
	}

	tests := []struct {
		name          string
		checksEnabled bool
		routes        []dto.AdvancedCustomRoute
		wantErr       string
	}{
		{
			name:   "legacy channel without discovery route remains valid",
			routes: []dto.AdvancedCustomRoute{inferenceRoute},
		},
		{
			name:          "enabled checks require discovery route",
			checksEnabled: true,
			routes:        []dto.AdvancedCustomRoute{inferenceRoute},
			wantErr:       dto.AdvancedCustomModelListPath,
		},
		{
			name:          "enabled checks accept discovery route",
			checksEnabled: true,
			routes: []dto.AdvancedCustomRoute{
				inferenceRoute,
				{
					IncomingPath: dto.AdvancedCustomModelListPath,
					UpstreamPath: dto.AdvancedCustomModelListPath,
					Converter:    "none",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			channel := &Channel{Type: constant.ChannelTypeAdvancedCustom}
			channel.SetOtherSettings(dto.ChannelOtherSettings{
				UpstreamModelUpdateCheckEnabled: tt.checksEnabled,
				AdvancedCustom: &dto.AdvancedCustomConfig{
					Routes: tt.routes,
				},
			})

			err := channel.ValidateSettings()
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestCodingPlanChannelBuildsAdvancedCustomSettingsFromProvider(t *testing.T) {
	baseURL := dto.CodingPlanProviderGLMChina
	channel := &Channel{
		Type:    constant.ChannelTypeCodingPlan,
		BaseURL: &baseURL,
	}

	require.NoError(t, channel.ValidateSettings())

	settings := channel.GetOtherSettings()
	require.NotNil(t, settings.AdvancedCustom)
	assert.Equal(t, dto.CodingPlanProviderGLMChina, settings.CodingPlanProvider)
	assert.True(t, settings.AdvancedCustom.SupportsPathForModel("/v1/responses", "glm-5-turbo"))
	assert.True(t, settings.AdvancedCustom.SupportsPathForModel("/v1/messages", "glm-5-turbo"))
	assert.True(t, settings.AdvancedCustom.SupportsPathForModel("/v1/chat/completions", "glm-5-turbo"))
}

func TestCodingPlanChannelPreservesEditedAdvancedCustomSettings(t *testing.T) {
	baseURL := dto.CodingPlanProviderGLMChina
	channel := &Channel{
		Type:    constant.ChannelTypeCodingPlan,
		BaseURL: &baseURL,
	}
	channel.SetOtherSettings(dto.ChannelOtherSettings{
		CodingPlanProvider: dto.CodingPlanProviderGLMChina,
		AdvancedCustom: &dto.AdvancedCustomConfig{
			Routes: []dto.AdvancedCustomRoute{
				{
					IncomingPath: "/v1/chat/completions",
					UpstreamPath: "https://custom.example/v1/chat/completions",
					Converter:    "none",
				},
			},
		},
	})

	require.NoError(t, channel.ValidateSettings())

	settings := channel.GetOtherSettings()
	route, ok := settings.AdvancedCustom.MatchPath("/v1/chat/completions")
	require.True(t, ok)
	assert.Equal(t, "https://custom.example/v1/chat/completions", route.UpstreamPath)
}
