package oauth

import (
	"net/url"
	"strings"

	"github.com/QuantumNous/new-api/common"
)

// OAuthToken represents the token received from OAuth provider
type OAuthToken struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	RefreshToken string `json:"refresh_token,omitempty"`
	ExpiresIn    int    `json:"expires_in,omitempty"`
	Scope        string `json:"scope,omitempty"`
	IDToken      string `json:"id_token,omitempty"`
	ClientID     string `json:"-"` // Expected OIDC audience from the server-owned flow.
}

// OAuthUser represents the user info from OAuth provider
type OAuthUser struct {
	// ProviderUserID is the unique identifier from the OAuth provider
	ProviderUserID string
	// Username is the username from the OAuth provider (e.g., GitHub login)
	Username string
	// DisplayName is the display name from the OAuth provider
	DisplayName string
	// Email is the email from the OAuth provider
	Email string
	// EmailVerified means the provider has verified ownership of Email.
	EmailVerified bool
	// Extra contains any additional provider-specific data
	Extra map[string]any
}

func oauthBool(value any) bool {
	switch v := value.(type) {
	case bool:
		return v
	case string:
		return strings.EqualFold(strings.TrimSpace(v), "true")
	default:
		return false
	}
}

func oauthEmailVerified(email string, emailVerified any, googleVerifiedEmail any, hostedDomain any) bool {
	if googleVerifiedEmail != nil {
		return oauthBool(googleVerifiedEmail) && oauthGoogleEmailAuthoritative(email, oauthString(hostedDomain))
	}
	return oauthBool(emailVerified)
}

func oauthEmailVerifiedForEndpoint(email string, emailVerified any, googleVerifiedEmail any, hostedDomain any, userInfoEndpoint string) bool {
	if oauthGoogleUserInfoEndpoint(userInfoEndpoint) {
		return (oauthBool(emailVerified) || oauthBool(googleVerifiedEmail)) && oauthGoogleEmailAuthoritative(email, oauthString(hostedDomain))
	}
	return oauthEmailVerified(email, emailVerified, googleVerifiedEmail, hostedDomain)
}

func oauthString(value any) string {
	if v, ok := value.(string); ok {
		return strings.TrimSpace(v)
	}
	return ""
}

func oauthGoogleUserInfoEndpoint(rawURL string) bool {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return false
	}
	host := strings.ToLower(parsed.Hostname())
	return host == "openidconnect.googleapis.com" || host == "www.googleapis.com"
}

func oauthGoogleEmailAuthoritative(email string, hostedDomain string) bool {
	if strings.TrimSpace(hostedDomain) != "" {
		return true
	}
	at := strings.LastIndex(email, "@")
	if at < 0 {
		return false
	}
	domain := strings.ToLower(strings.TrimSpace(email[at+1:]))
	return domain == "gmail.com" || domain == "googlemail.com"
}

// Texts of OAuthError that the web console translates.
const (
	msgInvalidCode   = "Invalid authorization code"
	msgTokenFailed   = "Failed to get token from {{provider}}, please check settings"
	msgUserInfoEmpty = "{{provider}} returned empty user info, please check settings"
	msgConnectFailed = "Unable to connect to {{provider}} server, please try again later"
	msgGetUserFailed = "Failed to get user information"
)

// OAuthError represents a translatable OAuth error
type OAuthError struct {
	// Message is the text the web console translates
	Message *common.Message
	// RawError is the underlying error for logging purposes
	RawError string
}

func (e *OAuthError) Error() string {
	if e.RawError != "" {
		return e.RawError
	}
	return e.Message.Error()
}

// NewOAuthError creates a new OAuth error with the given web console message
func NewOAuthError(message *common.Message) *OAuthError {
	return &OAuthError{Message: message}
}

// NewOAuthErrorWithRaw creates a new OAuth error with raw error message for logging
func NewOAuthErrorWithRaw(message *common.Message, rawError string) *OAuthError {
	return &OAuthError{Message: message, RawError: rawError}
}

// AccessDeniedError is a direct user-facing access denial message.
type AccessDeniedError struct {
	Message string
}

func (e *AccessDeniedError) Error() string {
	return e.Message
}
