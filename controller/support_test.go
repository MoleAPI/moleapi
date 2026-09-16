package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestZohoDeskRequestRefreshesTokenAndAuthenticatesAPIRequest(t *testing.T) {
	zohoDeskTokenCache.Lock()
	zohoDeskTokenCache.token = ""
	zohoDeskTokenCache.key = ""
	zohoDeskTokenCache.Unlock()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/oauth/v2/token":
			require.NoError(t, r.ParseForm())
			assert.Equal(t, "refresh", r.Form.Get("refresh_token"))
			assert.Equal(t, "client", r.Form.Get("client_id"))
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token":"access","expires_in":3600}`))
		case "/api/v1/tickets":
			assert.Equal(t, "Zoho-oauthtoken access", r.Header.Get("Authorization"))
			assert.Equal(t, "123", r.Header.Get("orgId"))
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":[]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	cfg := zohoDeskConfig{ClientID: "client", ClientSecret: "secret", RefreshToken: "refresh", OrgID: "123", APIDomain: server.URL, AccountsDomain: server.URL}
	var result struct {
		Data []zohoDeskTicket `json:"data"`
	}
	require.NoError(t, zohoDeskRequest(cfg, http.MethodGet, "/tickets", nil, &result))
	assert.Empty(t, result.Data)
}
