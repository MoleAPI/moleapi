package oauth

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPrimaryVerifiedGitHubEmail(t *testing.T) {
	emails := []gitHubEmail{
		{Email: "secondary@example.com", Verified: true},
		{Email: "primary-unverified@example.com", Primary: true},
		{Email: "primary@example.com", Primary: true, Verified: true},
	}

	assert.Equal(t, "primary@example.com", primaryVerifiedGitHubEmail(emails))
}

func TestPrimaryVerifiedGitHubEmailRejectsUnverifiedPrimary(t *testing.T) {
	emails := []gitHubEmail{
		{Email: "primary@example.com", Primary: true},
		{Email: "secondary@example.com", Verified: true},
	}

	assert.Empty(t, primaryVerifiedGitHubEmail(emails))
}

func TestOAuthEmailVerifiedHandlesGoogleAuthorityBoundary(t *testing.T) {
	assert.True(t, oauthEmailVerified("user@gmail.com", nil, true, nil))
	assert.True(t, oauthEmailVerified("user@example.com", nil, true, "example.com"))
	assert.False(t, oauthEmailVerified("user@example.com", nil, true, nil))
	assert.True(t, oauthEmailVerified("user@example.com", true, nil, nil))
}

func TestOAuthEmailVerifiedForEndpointAppliesGoogleAuthorityBoundary(t *testing.T) {
	endpoint := "https://openidconnect.googleapis.com/v1/userinfo"

	assert.True(t, oauthEmailVerifiedForEndpoint("user@gmail.com", true, nil, nil, endpoint))
	assert.True(t, oauthEmailVerifiedForEndpoint("user@example.com", true, nil, "example.com", endpoint))
	assert.False(t, oauthEmailVerifiedForEndpoint("user@example.com", true, nil, nil, endpoint))
}
