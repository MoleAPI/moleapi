package oauth

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

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

func TestGitHubUserInfoContinuesWithoutVerifiedPrimaryEmail(t *testing.T) {
	previousTransport := http.DefaultTransport
	http.DefaultTransport = roundTripFunc(func(request *http.Request) (*http.Response, error) {
		body := `[]`
		if request.URL.Path == "/user" {
			body = `{"id":12345,"login":"existing-user"}`
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(body)),
			Request:    request,
		}, nil
	})
	t.Cleanup(func() { http.DefaultTransport = previousTransport })

	user, err := (&GitHubProvider{}).GetUserInfo(context.Background(), &OAuthToken{AccessToken: "token"})

	require.NoError(t, err)
	assert.Equal(t, "12345", user.ProviderUserID)
	assert.Equal(t, "existing-user", user.Username)
	assert.Empty(t, user.Email)
	assert.False(t, user.EmailVerified)
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
