package controller

import (
	"bytes"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
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
			assert.Equal(t, "true", r.URL.Query().Get("isPublic"))
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

func TestSupportConversationActivity(t *testing.T) {
	for _, tc := range []struct {
		name, messages, want string
	}{
		{"newest public reply", `[{"type":"thread","visibility":"public","direction":"out","createdTime":"2026-09-18T10:00:00Z"},{"type":"comment","isPublic":true,"commentedTime":"2026-09-18T10:00:00.500Z","content":"<div>alice (UID 1):<br>Still broken</div>","commenter":{"type":"AGENT"}}]`, "customer"},
		{"private and draft messages ignored", `[{"type":"comment","isPublic":false,"commentedTime":"2026-09-18T12:00:00Z","commenter":{"type":"AGENT"}},{"type":"thread","visibility":"public","isDraft":true,"direction":"out","createdTime":"2026-09-18T11:00:00Z"},{"type":"thread","visibility":"public","direction":"in","createdTime":"2026-09-18T10:00:00Z"}]`, "customer"},
		{"support answered", `[{"type":"thread","visibility":"public","direction":"out","createdTime":"2026-09-18T10:00:00Z"}]`, "agent"},
		{"unknown visibility", `[{"direction":"out","createdTime":"2026-09-18T10:00:00Z"}]`, "unknown"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var messages []zohoDeskConversation
			require.NoError(t, common.UnmarshalJsonStr(tc.messages, &messages))
			assert.Equal(t, tc.want, supportConversationActivity(messages))
		})
	}
}

func TestSupportPortalCommentNormalization(t *testing.T) {
	message := zohoDeskConversation{
		Type:     "comment",
		IsPublic: true,
		Content:  "<div>alice (UID 11):<br>Still broken</div>",
	}
	normalizeSupportPortalComment(&message)
	assert.Equal(t, "Still broken", message.Content)
	assert.Equal(t, "text/plain", message.ContentType)
	assert.Equal(t, "in", message.Direction)
	assert.Equal(t, "END_USER", message.Author.Type)
	assert.Equal(t, "alice", message.Author.Name)
}

func TestSupportConversationMatchesTicketDescription(t *testing.T) {
	ticket := zohoDeskTicket{
		Description: "Please help",
		CreatedTime: "2026-09-18T10:00:00Z",
	}
	message := zohoDeskConversation{
		Type:        "thread",
		Visibility:  "public",
		Content:     "<p>Please help</p>",
		CreatedTime: "2026-09-18T10:00:01Z",
	}
	assert.True(t, supportConversationMatchesTicketDescription(message, ticket))
}

func TestSupportEmailConfigPreservesConfiguredAddress(t *testing.T) {
	common.OptionMapRWMutex.Lock()
	previous := common.OptionMap
	common.OptionMap = map[string]string{"ZohoDeskFromEmail": "support@moleapi.zohodesk.jp"}
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		common.OptionMapRWMutex.Lock()
		common.OptionMap = previous
		common.OptionMapRWMutex.Unlock()
	})
	assert.Equal(t, "support@moleapi.zohodesk.jp", getZohoDeskConfig().FromEmail)
	assert.Equal(t, "A clean reply\nwith emphasis", cleanSupportEmailText("## **A clean reply**\nwith emphasis"))
}

func TestSupportEmailThreadsRestoreInboundBody(t *testing.T) {
	resetZohoDeskTokenCache()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/oauth/v2/token":
			_, _ = w.Write([]byte(`{"access_token":"access","expires_in":3600}`))
		case "/api/v1/tickets/42/threads":
			_, _ = w.Write([]byte(`{"data":[{"id":"thread-1","direction":"in","channel":"EMAIL","createdTime":"2026-09-19T06:26:31Z","content":"","summary":"Inbound email body","contentType":"text/html","isDescriptionThread":true}]}`))
		case "/api/v1/tickets/43/threads":
			_, _ = w.Write([]byte(`{"data":[]}`))
		case "/api/v1/tickets/43/latestThread":
			_, _ = w.Write([]byte(`{"id":"thread-2","direction":"in","channel":"EMAIL","createdTime":"2026-09-19T06:26:31Z","content":"Latest inbound email body","contentType":"text/plain"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	threads, err := loadSupportEmailThreads(zohoDeskConfig{
		ClientID: "client", ClientSecret: "secret", RefreshToken: "refresh",
		OrgID: "org", APIDomain: server.URL, AccountsDomain: server.URL,
	}, "42")
	require.NoError(t, err)
	require.Len(t, threads, 1)
	assert.Equal(t, "Inbound email body", threads[0].Content)
	assert.Equal(t, "public", threads[0].Visibility)
	assert.True(t, threads[0].IsPublic)
	threads, err = loadSupportEmailThreads(zohoDeskConfig{
		ClientID: "client", ClientSecret: "secret", RefreshToken: "refresh",
		OrgID: "org", APIDomain: server.URL, AccountsDomain: server.URL,
	}, "43")
	require.NoError(t, err)
	require.Len(t, threads, 1)
	assert.Equal(t, "Latest inbound email body", threads[0].Content)
}

func TestSupportTicketWorkflow(t *testing.T) {
	for _, kind := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(kind, func(t *testing.T) {
			dsn := os.Getenv("TEST_" + strings.ToUpper(kind) + "_DSN")
			if kind != "sqlite" && dsn == "" {
				t.Skip("test database DSN is not configured")
			}
			db, _ := newAuditTestDatabase(t, kind, dsn)
			previousDB, previousType := model.DB, common.MainDatabaseType()
			model.DB = db
			common.SetMainDatabaseType(common.DatabaseType(kind))
			t.Cleanup(func() { model.DB = previousDB; common.SetMainDatabaseType(previousType) })
			require.NoError(t, db.AutoMigrate(&model.User{}))
			require.NoError(t, db.Create(&model.User{Id: 11, Username: "alice", Email: "Alice@example.com", AffCode: "alice"}).Error)
			require.NoError(t, db.Create(&model.User{Id: 12, Username: "bob", Email: "bob@example.com", AffCode: "bob"}).Error)
			var version string
			query := "SELECT VERSION()"
			if kind == "sqlite" {
				query = "SELECT sqlite_version()"
			}
			require.NoError(t, db.Raw(query).Scan(&version).Error)
			t.Logf("database version: %s", version)

			resetZohoDeskTokenCache()
			status, department := "Open", "7"
			patches := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case "/oauth/v2/token":
					_, _ = w.Write([]byte(`{"access_token":"access","expires_in":3600}`))
				case "/api/v1/tickets/42":
					if r.Method == http.MethodPatch {
						var input struct {
							Status string `json:"status"`
						}
						require.NoError(t, common.DecodeJson(r.Body, &input))
						status = input.Status
						patches++
					}
					data, err := common.Marshal(gin.H{"id": "42", "departmentId": department, "status": status, "statusType": status, "email": "alice@example.com"})
					require.NoError(t, err)
					_, _ = w.Write(data)
				case "/api/v1/tickets", "/api/v1/tickets/search":
					assert.Equal(t, "20", r.URL.Query().Get("limit"))
					assert.Equal(t, "7", r.URL.Query().Get("departmentId"))
					_, _ = w.Write([]byte(`{"data":[{"id":"42","departmentId":"7","email":"alice@example.com","commentCount":"1"},{"id":"43","departmentId":"7","email":"bob@example.com"},{"id":"44","departmentId":"8","email":"alice@example.com"}]}`))
				case "/api/v1/tickets/archivedTickets":
					_, _ = w.Write([]byte(`{"data":[]}`))
				case "/api/v1/tickets/42/conversations":
					_, _ = w.Write([]byte(`{"data":[{"id":"1","type":"comment","isPublic":true,"content":"alice (UID 11): hello","commentedTime":"2026-09-18T10:00:00Z"},{"id":"2","type":"comment","isPublic":false,"content":"private note"},{"id":"3","type":"thread","visibility":"public","isDraft":true,"content":"draft"}]}`))
				case "/api/v1/tickets/42/attachments":
					_, _ = w.Write([]byte(`{"data":[{"id":"1","isPublic":true},{"id":"2","isPublic":false},{"id":"3"}]}`))
				default:
					t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
					http.NotFound(w, r)
				}
			}))
			t.Cleanup(server.Close)
			common.OptionMapRWMutex.Lock()
			previousOptions := common.OptionMap
			common.OptionMap = map[string]string{"ZohoDeskEnabled": "true", "ZohoDeskClientId": "client", "ZohoDeskClientSecret": "secret", "ZohoDeskRefreshToken": "refresh", "ZohoDeskOrgId": "123", "ZohoDeskDepartmentId": "7", "ZohoDeskApiDomain": server.URL, "ZohoDeskAccountsDomain": server.URL}
			common.OptionMapRWMutex.Unlock()
			t.Cleanup(func() {
				common.OptionMapRWMutex.Lock()
				common.OptionMap = previousOptions
				common.OptionMapRWMutex.Unlock()
				resetZohoDeskTokenCache()
			})
			request := func(handler gin.HandlerFunc, role, id int, body string) *httptest.ResponseRecorder {
				w := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(w)
				c.Request = httptest.NewRequest(http.MethodGet, "/", strings.NewReader(body))
				c.Params = gin.Params{{Key: "id", Value: "42"}, {Key: "attachment_id", Value: "2"}}
				c.Set("id", id)
				c.Set("role", role)
				handler(c)
				return w
			}
			for _, tc := range []struct {
				role, id int
				target   string
				allowed  bool
			}{
				{common.RoleCommonUser, 11, "Closed", true},
				{common.RoleCommonUser, 11, "Open", true},
				{common.RoleCommonUser, 11, "On Hold", false},
				{common.RoleCommonUser, 12, "Closed", false},
				{common.RoleAdminUser, 12, "On Hold", true},
				{common.RoleAdminUser, 12, "Deleted", false},
			} {
				before := patches
				response := request(UpdateSupportTicketStatus, tc.role, tc.id, `{"status":"`+tc.target+`"}`)
				var result struct{ Success bool }
				require.NoError(t, common.Unmarshal(response.Body.Bytes(), &result))
				assert.Equal(t, tc.allowed, result.Success, response.Body.String())
				assert.Equal(t, tc.allowed, patches == before+1)
			}
			status = "Closed"
			assert.Contains(t, request(ReplySupportTicket, common.RoleCommonUser, 11, `{"content":"hello"}`).Body.String(), "Reopen the ticket")
			department = "8"
			assert.Contains(t, request(UpdateSupportTicketStatus, common.RoleAdminUser, 12, `{"status":"Open"}`).Body.String(), "Ticket not found")
			department = "7"
			var detail struct {
				Success bool
				Data    struct {
					Ticket        zohoDeskTicket
					Conversations []zohoDeskConversation
					Attachments   []zohoDeskAttachment
				}
			}
			require.NoError(t, common.Unmarshal(request(GetSupportTicket, common.RoleCommonUser, 11, "").Body.Bytes(), &detail))
			require.True(t, detail.Success)
			require.Len(t, detail.Data.Conversations, 1)
			assert.Equal(t, "1", detail.Data.Conversations[0].ID)
			require.Len(t, detail.Data.Attachments, 1)
			assert.Equal(t, "1", detail.Data.Attachments[0].ID)
			assert.Contains(t, request(DownloadSupportAttachment, common.RoleCommonUser, 11, "").Body.String(), "Attachment not found")
			require.NoError(t, common.Unmarshal(request(GetSupportTicket, common.RoleAdminUser, 12, "").Body.Bytes(), &detail))
			require.True(t, detail.Success)
			require.NotNil(t, detail.Data.Ticket.User)
			assert.Equal(t, "alice", detail.Data.Ticket.User.Username)
			assert.Len(t, detail.Data.Conversations, 3)
			for _, role := range []int{common.RoleCommonUser, common.RoleAdminUser} {
				var list struct {
					Success bool
					Data    struct {
						Tickets  []zohoDeskTicket
						NextFrom int  `json:"next_from"`
						HasMore  bool `json:"has_more"`
					}
				}
				require.NoError(t, common.Unmarshal(request(ListSupportTickets, role, 11, "").Body.Bytes(), &list))
				require.True(t, list.Success)
				require.NotEmpty(t, list.Data.Tickets)
				assert.Equal(t, "customer", list.Data.Tickets[0].Activity)
				if role == common.RoleAdminUser {
					require.Len(t, list.Data.Tickets, 2)
					assert.Equal(t, 11, list.Data.Tickets[0].User.ID)
				} else {
					assert.Len(t, list.Data.Tickets, 1)
					assert.Nil(t, list.Data.Tickets[0].User)
				}
				assert.Equal(t, 3, list.Data.NextFrom)
				assert.False(t, list.Data.HasMore)
			}
			require.NoError(t, db.Create(&model.User{Id: 13, Username: "duplicate", Email: "ALICE@example.com", AffCode: "duplicate"}).Error)
			detail.Data.Ticket.User = nil
			require.NoError(t, common.Unmarshal(request(GetSupportTicket, common.RoleAdminUser, 12, "").Body.Bytes(), &detail))
			assert.True(t, detail.Success)
			assert.Nil(t, detail.Data.Ticket.User, "ambiguous emails must not create a misleading user shortcut")
		})
	}
}
