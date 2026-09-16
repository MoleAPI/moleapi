package controller

import (
	"bytes"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func resetZohoDeskTokenCache() {
	zohoDeskTokenCache.Lock()
	defer zohoDeskTokenCache.Unlock()
	zohoDeskTokenCache.token = ""
	zohoDeskTokenCache.key = ""
}

func TestZohoDeskRequestRefreshesTokenAndAuthenticatesAPIRequest(t *testing.T) {
	resetZohoDeskTokenCache()

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

func TestZohoDeskUploadAttachment(t *testing.T) {
	resetZohoDeskTokenCache()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/oauth/v2/token":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token":"access","expires_in":3600}`))
		case "/api/v1/tickets/42/attachments":
			if r.Method == http.MethodGet {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"data":[{"id":"attachment-1","name":"screen.png","size":"3"}]}`))
				return
			}
			assert.Equal(t, "Zoho-oauthtoken access", r.Header.Get("Authorization"))
			assert.Equal(t, "123", r.Header.Get("orgId"))
			require.NoError(t, r.ParseMultipartForm(1<<20))
			file, header, err := r.FormFile("file")
			require.NoError(t, err)
			defer file.Close()
			assert.Equal(t, "screen.png", header.Filename)
			data := make([]byte, 3)
			_, err = file.Read(data)
			require.NoError(t, err)
			assert.Equal(t, []byte("png"), data)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"attachment-1","name":"screen.png","size":"3","href":"https://example.com/screen.png"}`))
		case "/api/v1/tickets/42/attachments/1/content":
			assert.Equal(t, "Zoho-oauthtoken access", r.Header.Get("Authorization"))
			assert.Equal(t, "123", r.Header.Get("orgId"))
			w.Header().Set("Content-Type", "image/png")
			w.Header().Set("Content-Disposition", `attachment; filename="screen.png"`)
			_, _ = w.Write([]byte("png"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	var source bytes.Buffer
	writer := multipart.NewWriter(&source)
	part, err := writer.CreateFormFile("files", "screen.png")
	require.NoError(t, err)
	_, err = part.Write([]byte("png"))
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	request := httptest.NewRequest(http.MethodPost, "/", &source)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	require.NoError(t, request.ParseMultipartForm(1<<20))
	header := request.MultipartForm.File["files"][0]

	cfg := zohoDeskConfig{ClientID: "client", ClientSecret: "secret", RefreshToken: "refresh", OrgID: "123", APIDomain: server.URL, AccountsDomain: server.URL}
	attachment, err := zohoDeskUploadAttachment(cfg, "42", header)
	require.NoError(t, err)
	assert.Equal(t, "attachment-1", attachment.ID)
	assert.Equal(t, "screen.png", attachment.Name)

	response, err := zohoDeskDownloadAttachment(cfg, "42", "1")
	require.NoError(t, err)
	defer response.Body.Close()
	data, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	assert.Equal(t, "image/png", response.Header.Get("Content-Type"))
	assert.Equal(t, []byte("png"), data)
}
