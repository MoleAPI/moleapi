package controller

import (
	"bytes"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
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

func supportAttachmentHeader(t *testing.T, filename string, content []byte) *multipart.FileHeader {
	t.Helper()
	var source bytes.Buffer
	writer := multipart.NewWriter(&source)
	part, err := writer.CreateFormFile("files", filename)
	require.NoError(t, err)
	_, err = part.Write(content)
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	request := httptest.NewRequest(http.MethodPost, "/", &source)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	require.NoError(t, request.ParseMultipartForm(1<<20))
	return request.MultipartForm.File["files"][0]
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

func TestSupportNotificationReadsReuseActivityAndSkipArchives(t *testing.T) {
	resetZohoDeskTokenCache()
	supportActivityCache.Purge()
	t.Cleanup(supportActivityCache.Purge)
	var listCalls, archiveCalls, conversationCalls atomic.Int32
	revision, direction, failConversation := "2026-10-01T01:00:00Z", "in", false
	threadCount := "1"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/oauth/v2/token":
			_, _ = w.Write([]byte(`{"access_token":"access","expires_in":3600}`))
		case "/api/v1/tickets":
			listCalls.Add(1)
			data, err := common.Marshal(gin.H{"data": []gin.H{
				{"id": "42", "departmentId": "7", "commentCount": "1", "threadCount": threadCount, "modifiedTime": revision},
				{"id": "43", "departmentId": "8", "commentCount": "1"},
			}})
			require.NoError(t, err)
			_, _ = w.Write(data)
		case "/api/v1/tickets/archivedTickets":
			archiveCalls.Add(1)
			_, _ = w.Write([]byte(`{"data":[{"id":"45","departmentId":"7","commentCount":"1"}]}`))
		case "/api/v1/tickets/42/conversations":
			conversationCalls.Add(1)
			if failConversation {
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}
			data, err := common.Marshal(gin.H{"data": []gin.H{{"type": "thread", "visibility": "public", "direction": direction, "createdTime": revision}}})
			require.NoError(t, err)
			_, _ = w.Write(data)
		case "/api/v1/tickets/45/conversations":
			conversationCalls.Add(1)
			_, _ = w.Write([]byte(`{"data":[]}`))
		case "/api/v1/tickets/42":
			assert.Equal(t, http.MethodPatch, r.Method)
			_, _ = w.Write([]byte(`{}`))
		default:
			t.Errorf("unexpected request: %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	common.OptionMapRWMutex.Lock()
	previousOptions := common.OptionMap
	common.OptionMap = map[string]string{
		"ZohoDeskEnabled": "true", "ZohoDeskClientId": "client", "ZohoDeskClientSecret": "secret",
		"ZohoDeskRefreshToken": "refresh", "ZohoDeskOrgId": "123", "ZohoDeskDepartmentId": "7",
		"ZohoDeskApiDomain": server.URL, "ZohoDeskAccountsDomain": server.URL,
	}
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		common.OptionMapRWMutex.Lock()
		common.OptionMap = previousOptions
		common.OptionMapRWMutex.Unlock()
		resetZohoDeskTokenCache()
	})
	request := func(view string) []zohoDeskTicket {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodGet, "/api/support/tickets?view="+view, nil)
		c.Set("role", common.RoleAdminUser)
		ListSupportTickets(c)
		var result struct {
			Success bool
			Data    struct{ Tickets []zohoDeskTicket }
		}
		require.NoError(t, common.Unmarshal(w.Body.Bytes(), &result))
		require.True(t, result.Success, w.Body.String())
		return result.Data.Tickets
	}

	for range 2 {
		tickets := request("notifications")
		require.Len(t, tickets, 1)
		assert.Equal(t, "customer", tickets[0].Activity)
	}
	assert.EqualValues(t, 2, listCalls.Load())
	assert.Zero(t, archiveCalls.Load())
	assert.EqualValues(t, 1, conversationCalls.Load(), "unchanged activity should be reused")

	revision, direction = "2026-10-01T02:00:00Z", "out"
	assert.Equal(t, "agent", request("notifications")[0].Activity)
	assert.EqualValues(t, 2, conversationCalls.Load(), "a changed ticket must refresh its activity")

	revision, failConversation = "2026-10-01T03:00:00Z", true
	assert.Equal(t, "unknown", request("notifications")[0].Activity)
	failConversation = false
	assert.Equal(t, "agent", request("notifications")[0].Activity)
	assert.EqualValues(t, 4, conversationCalls.Load(), "failed reads must not be cached")

	tickets := request("")
	require.Len(t, tickets, 2, "archives remain available in the support workspace")
	assert.True(t, tickets[1].Archived)
	assert.Equal(t, "archived", tickets[1].Activity)
	assert.EqualValues(t, 1, archiveCalls.Load())
	assert.EqualValues(t, 4, conversationCalls.Load(), "archived conversations are unnecessary")

	direction = "in"
	require.NoError(t, zohoDeskRequest(getZohoDeskConfig(), http.MethodPatch, "/tickets/42", gin.H{"status": "Open"}, nil))
	assert.Equal(t, "customer", request("notifications")[0].Activity)
	assert.EqualValues(t, 5, conversationCalls.Load(), "writes refresh activity even before list metadata changes")

	direction = "out"
	common.OptionMapRWMutex.Lock()
	common.OptionMap["ZohoDeskOrgId"] = "456"
	common.OptionMapRWMutex.Unlock()
	assert.Equal(t, "agent", request("notifications")[0].Activity)
	assert.EqualValues(t, 6, conversationCalls.Load(), "organizations must not share cached activity")
	threadCount, direction = "2", "in"
	assert.Equal(t, "customer", request("notifications")[0].Activity)
	assert.EqualValues(t, 7, conversationCalls.Load(), "a new email must refresh activity even if its timestamp is unchanged")
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

func TestSupportAttachmentContentValidation(t *testing.T) {
	for _, testCase := range []struct {
		name, filename string
		content        []byte
		valid          bool
	}{
		{name: "png", filename: "screen.png", content: []byte("\x89PNG\r\n\x1a\n"), valid: true},
		{name: "log", filename: "request.log", content: []byte("request failed\n"), valid: true},
		{name: "html disguised as image", filename: "screen.png", content: []byte("<script>alert(1)</script>"), valid: false},
		{name: "html disguised as text", filename: "request.txt", content: []byte("<html><script>alert(1)</script></html>"), valid: false},
		{name: "unsupported executable", filename: "report.exe", content: []byte("MZ"), valid: false},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			assert.Equal(t, testCase.valid, validSupportAttachment(supportAttachmentHeader(t, testCase.filename, testCase.content)))
		})
	}
}

func TestDownloadSupportAttachmentForcesSafeResponse(t *testing.T) {
	resetZohoDeskTokenCache()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/oauth/v2/token":
			_, _ = w.Write([]byte(`{"access_token":"access","expires_in":3600}`))
		case "/api/v1/tickets/42":
			_, _ = w.Write([]byte(`{"id":"42","departmentId":"7"}`))
		case "/api/v1/tickets/42/attachments/1/content":
			w.Header().Set("Content-Type", "text/html")
			w.Header().Set("Content-Disposition", `inline; filename="../../payload.html"`)
			_, _ = w.Write([]byte(`<script>window.parent.pwned=true</script>`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	common.OptionMapRWMutex.Lock()
	previousOptions := common.OptionMap
	common.OptionMap = map[string]string{
		"ZohoDeskEnabled": "true", "ZohoDeskClientId": "client", "ZohoDeskClientSecret": "secret",
		"ZohoDeskRefreshToken": "refresh", "ZohoDeskOrgId": "123", "ZohoDeskDepartmentId": "7",
		"ZohoDeskApiDomain": server.URL, "ZohoDeskAccountsDomain": server.URL,
	}
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		common.OptionMapRWMutex.Lock()
		common.OptionMap = previousOptions
		common.OptionMapRWMutex.Unlock()
		resetZohoDeskTokenCache()
	})

	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	context.Params = gin.Params{{Key: "id", Value: "42"}, {Key: "attachment_id", Value: "1"}}
	context.Set("role", common.RoleAdminUser)
	DownloadSupportAttachment(context)

	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Equal(t, "application/octet-stream", recorder.Header().Get("Content-Type"))
	assert.Equal(t, "attachment; filename=payload.html", recorder.Header().Get("Content-Disposition"))
	assert.Equal(t, "nosniff", recorder.Header().Get("X-Content-Type-Options"))
	assert.Equal(t, "sandbox", recorder.Header().Get("Content-Security-Policy"))
	assert.Equal(t, "private, no-store", recorder.Header().Get("Cache-Control"))
	assert.Equal(t, `<script>window.parent.pwned=true</script>`, recorder.Body.String())
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

	outbound := zohoDeskConversation{Type: "thread", ContentType: "text/plain", Content: "<div>Fixed and credited 1 USD</div>"}
	normalizeSupportConversationContent(&outbound)
	assert.Equal(t, "text/html", outbound.ContentType)
	plain := zohoDeskConversation{Type: "thread", ContentType: "text/plain", Content: "2 < 3"}
	normalizeSupportConversationContent(&plain)
	assert.Equal(t, "text/plain", plain.ContentType)
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
	assert.False(t, supportConversationMatchesTicketDescription(zohoDeskConversation{Type: "thread", Visibility: "public", Summary: "Email summary"}, zohoDeskTicket{}), "missing content must not be removed as a duplicate")
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
		if r.URL.Query().Has("include") {
			w.WriteHeader(http.StatusUnprocessableEntity)
			_, _ = w.Write([]byte(`{"message":"include=plainText is not supported"}`))
			return
		}
		switch r.URL.Path {
		case "/oauth/v2/token":
			_, _ = w.Write([]byte(`{"access_token":"access","expires_in":3600}`))
		case "/api/v1/tickets/42/threads":
			_, _ = w.Write([]byte(`{"data":[{"id":"thread-1","direction":"in","channel":"EMAIL","createdTime":"2026-09-19T06:26:31Z","content":"","summary":"Inbound email body","contentType":"text/html","isDescriptionThread":true}]}`))
		case "/api/v1/tickets/42/threads/thread-1":
			_, _ = w.Write([]byte(`{"id":"thread-1","content":"Full inbound email body","contentType":"text/html","attachments":[{"id":"image-1","isPublic":true}]}`))
		case "/api/v1/tickets/43/threads":
			_, _ = w.Write([]byte(`{"data":[]}`))
		case "/api/v1/tickets/43/latestThread":
			_, _ = w.Write([]byte(`{"id":"thread-2","direction":"in","channel":"EMAIL","createdTime":"2026-09-19T06:26:31Z","content":"Latest inbound email body","contentType":"text/plain"}`))
		case "/api/v1/tickets/45/threads":
			_, _ = w.Write([]byte(`{"data":[{"id":"thread-3","direction":"in","channel":"EMAIL","createdTime":"2026-09-19T06:26:31Z","plainText":"Plain-text inbound email body","contentType":"text/plain"}]}`))
		case "/api/v1/tickets/46/threads":
			_, _ = w.Write([]byte(`{"data":[{"id":"long","direction":"in","channel":"EMAIL"}]}`))
		case "/api/v1/tickets/46/threads/long":
			_, _ = w.Write([]byte(`{"content":"preview","isContentTruncated":true,"contentType":"text/html"}`))
		case "/api/v1/tickets/46/threads/long/fullContent":
			w.Header().Del("Content-Type")
			_, _ = w.Write([]byte(`<div>Complete long email body</div>`))
		case "/api/v1/tickets/44/comments":
			_, _ = w.Write([]byte(`{"data":[{"id":"comment-1","plainText":"Comment inbound email body","isPublic":true}]}`))
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
	assert.Equal(t, "Full inbound email body", threads[0].Content)
	assert.Equal(t, "public", threads[0].Visibility)
	assert.True(t, threads[0].IsPublic)
	require.Len(t, threads[0].Attachments, 1)
	threads, err = loadSupportEmailThreads(zohoDeskConfig{
		ClientID: "client", ClientSecret: "secret", RefreshToken: "refresh",
		OrgID: "org", APIDomain: server.URL, AccountsDomain: server.URL,
	}, "43")
	require.NoError(t, err)
	require.Len(t, threads, 1)
	assert.Equal(t, "Latest inbound email body", threads[0].Content)
	threads, err = loadSupportEmailThreads(zohoDeskConfig{
		ClientID: "client", ClientSecret: "secret", RefreshToken: "refresh",
		OrgID: "org", APIDomain: server.URL, AccountsDomain: server.URL,
	}, "45")
	require.NoError(t, err)
	require.Len(t, threads, 1)
	assert.Equal(t, "Plain-text inbound email body", threads[0].Content)
	threads, err = loadSupportEmailThreads(zohoDeskConfig{
		ClientID: "client", ClientSecret: "secret", RefreshToken: "refresh",
		OrgID: "org", APIDomain: server.URL, AccountsDomain: server.URL,
	}, "46")
	require.NoError(t, err)
	require.Len(t, threads, 1)
	assert.Equal(t, "<div>Complete long email body</div>", threads[0].Content)
	comments, err := loadSupportEmailComments(zohoDeskConfig{
		ClientID: "client", ClientSecret: "secret", RefreshToken: "refresh",
		OrgID: "org", APIDomain: server.URL, AccountsDomain: server.URL,
	}, "44")
	require.NoError(t, err)
	require.Len(t, comments, 1)
	assert.Equal(t, "Comment inbound email body", comments[0].Content)
	assert.Equal(t, "in", comments[0].Direction)
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
			require.NoError(t, db.AutoMigrate(&model.User{}, &model.TopUp{}))
			require.NoError(t, db.Create(&model.User{Id: 11, Username: "alice", Email: "Alice@example.com", AffCode: "alice"}).Error)
			require.NoError(t, db.Create(&model.User{Id: 12, Username: "bob", Email: "bob@example.com", AffCode: "bob"}).Error)
			var version string
			query := "SELECT VERSION()"
			if kind == "sqlite" {
				query = "SELECT sqlite_version()"
			}
			require.NoError(t, db.Raw(query).Scan(&version).Error)
			t.Logf("database version: %s", version)
			orders := []model.TopUp{
				{Id: 1, UserId: 11, TradeNo: "paid-1", Amount: 500, Money: 12.34, PaymentMethod: "alipay", PaymentCurrency: "CNY", Status: "success"},
				{Id: 2, UserId: 11, TradeNo: "paid-2", Amount: 500, Money: 0.66, PaymentMethod: "wxpay", Status: "success"},
				{Id: 3, UserId: 12, TradeNo: "foreign", Money: 99, PaymentMethod: "alipay", Status: "success"},
				{Id: 4, UserId: 11, TradeNo: "pending", Money: 99, PaymentMethod: "alipay", Status: "pending"},
				{Id: 5, UserId: 11, TradeNo: "stripe", Money: 99, PaymentMethod: "stripe", Status: "success"},
				{Id: 6, UserId: 11, TradeNo: "usd", Money: 2.50, PaymentMethod: "alipay", PaymentCurrency: "USD", Status: "success"},
			}
			require.NoError(t, db.Create(&orders).Error)
			summary, err := supportInvoiceSummary(11, []int{1, 2, 6})
			require.NoError(t, err)
			assert.Contains(t, summary, "Invoice total: CNY 13.00")
			assert.Contains(t, summary, "Invoice total: USD 2.50")
			assert.Contains(t, summary, "paid-1 | CNY 12.34")
			assert.NotContains(t, summary, "500")
			for _, ids := range [][]int{nil, {1, 1}, {3}, {4}, {5}, {999}, make([]int, 51)} {
				_, err := supportInvoiceSummary(11, ids)
				require.Error(t, err, "invalid selection: %v", ids)
			}

			resetZohoDeskTokenCache()
			status, department := "Open", "7"
			patches := 0
			var conversationReads atomic.Int32
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
					data, err := common.Marshal(gin.H{"id": "42", "departmentId": department, "status": status, "statusType": status, "email": "alice@example.com", "modifiedTime": "2026-10-01T01:00:00Z", "commentCount": "3", "threadCount": "1"})
					require.NoError(t, err)
					_, _ = w.Write(data)
				case "/api/v1/tickets", "/api/v1/tickets/search":
					if r.Method == http.MethodPost {
						var input struct{ Description string }
						require.NoError(t, common.DecodeJson(r.Body, &input))
						assert.Contains(t, input.Description, "Verified billing records (actual paid amounts)")
						assert.Contains(t, input.Description, "Invoice total: CNY 13.00")
						_, _ = w.Write([]byte(`{"id":"new-invoice","ticketNumber":"113"}`))
						return
					}
					assert.Equal(t, "20", r.URL.Query().Get("limit"))
					assert.Equal(t, "7", r.URL.Query().Get("departmentId"))
					_, _ = w.Write([]byte(`{"data":[{"id":"42","departmentId":"7","email":"alice@example.com","commentCount":"1"},{"id":"43","departmentId":"7","email":"bob@example.com"},{"id":"44","departmentId":"8","email":"alice@example.com"}]}`))
				case "/api/v1/tickets/archivedTickets":
					_, _ = w.Write([]byte(`{"data":[]}`))
				case "/api/v1/contacts/search":
					_, _ = w.Write([]byte(`{"data":[{"id":"contact-1","email":"alice@example.com"}]}`))
				case "/api/v1/tickets/42/conversations":
					conversationReads.Add(1)
					_, _ = w.Write([]byte(`{"data":[{"id":"1","type":"comment","isPublic":true,"content":"alice (UID 11): hello","commentedTime":"2026-09-18T10:00:00Z"},{"id":"2","type":"comment","isPublic":false,"content":"private note"},{"id":"3","type":"thread","visibility":"public","isDraft":true,"content":"draft"},{"id":"email","type":"thread","direction":"in","summary":"email preview"}]}`))
				case "/api/v1/tickets/42/threads":
					assert.Empty(t, r.URL.Query().Get("include"))
					_, _ = w.Write([]byte(`{"data":[{"id":"email","direction":"in","channel":"EMAIL","summary":"email preview"}]}`))
				case "/api/v1/tickets/42/threads/email":
					_, _ = w.Write([]byte(`{"content":"Actual complete email","contentType":"text/plain","attachments":[{"id":"1","isPublic":true}]}`))
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
			common.OptionMap = map[string]string{"ZohoDeskEnabled": "true", "ZohoDeskClientId": "client", "ZohoDeskClientSecret": "secret", "ZohoDeskRefreshToken": "refresh", "ZohoDeskOrgId": "123", "ZohoDeskDepartmentId": "7", "ZohoDeskApiDomain": server.URL, "ZohoDeskAccountsDomain": server.URL, "SupportTicketNotificationEmail": "notify@example.com"}
			common.OptionMapRWMutex.Unlock()
			t.Cleanup(func() {
				common.OptionMapRWMutex.Lock()
				common.OptionMap = previousOptions
				common.OptionMapRWMutex.Unlock()
				resetZohoDeskTokenCache()
			})
			request := func(handler gin.HandlerFunc, role, id int, body string, view ...string) *httptest.ResponseRecorder {
				w := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(w)
				c.Request = httptest.NewRequest(http.MethodGet, "/", strings.NewReader(body))
				if len(view) > 0 {
					c.Request.URL.RawQuery = "view=" + view[0]
				}
				c.Params = gin.Params{{Key: "id", Value: "42"}, {Key: "attachment_id", Value: "2"}}
				c.Set("id", id)
				c.Set("role", role)
				handler(c)
				return w
			}
			previousEmailSender := supportTicketEmailSender
			var notificationSubject, notificationReceiver, notificationBody string
			supportTicketEmailSender = func(subject, receiver, content string) error {
				notificationSubject, notificationReceiver, notificationBody = subject, receiver, content
				return nil
			}
			t.Cleanup(func() { supportTicketEmailSender = previousEmailSender })
			for _, tc := range []struct {
				role, id int
				allowed  bool
			}{
				{common.RoleCommonUser, 11, true},
				{common.RoleCommonUser, 12, false},
				{common.RoleAdminUser, 12, true},
			} {
				var response struct {
					Success bool
					Data    map[string]any
				}
				require.NoError(t, common.Unmarshal(request(GetSupportTicket, tc.role, tc.id, "", "updates").Body.Bytes(), &response))
				assert.Equal(t, tc.allowed, response.Success)
				if tc.allowed {
					assert.Equal(t, map[string]any{"modifiedTime": "2026-10-01T01:00:00Z", "commentCount": "3", "threadCount": "1"}, response.Data)
				}
			}
			department = "8"
			assert.Contains(t, request(GetSupportTicket, common.RoleAdminUser, 12, "", "updates").Body.String(), "Ticket not found")
			department = "7"
			assert.Zero(t, conversationReads.Load(), "update checks must not download conversations")
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
			invoice := request(CreateSupportTicket, common.RoleCommonUser, 11, `{"subject":"Combined invoice","content":"Please invoice these orders.","type":"Invoice Request","billing_record_ids":[1,2]}`)
			assert.Contains(t, invoice.Body.String(), `"success":true`)
			assert.Contains(t, invoice.Body.String(), `new-invoice`)
			assert.Contains(t, notificationSubject, "#113: Combined invoice")
			assert.Equal(t, "notify@example.com", notificationReceiver)
			assert.Contains(t, notificationBody, "Alice@example.com")
			assert.Contains(t, notificationBody, "Please invoice these orders.")
			assert.Contains(t, request(CreateSupportTicket, common.RoleCommonUser, 11, `{"subject":"Combined invoice","content":"Please invoice these orders.","type":"Invoice Request"}`).Body.String(), `"success":false`)
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
			require.Len(t, detail.Data.Conversations, 2)
			assert.Equal(t, "1", detail.Data.Conversations[0].ID)
			assert.Equal(t, "Actual complete email", detail.Data.Conversations[1].Content)
			assert.Len(t, detail.Data.Conversations[1].Attachments, 1)
			require.Len(t, detail.Data.Attachments, 1)
			assert.Equal(t, "1", detail.Data.Attachments[0].ID)
			assert.Contains(t, request(DownloadSupportAttachment, common.RoleCommonUser, 11, "").Body.String(), "Attachment not found")
			require.NoError(t, common.Unmarshal(request(GetSupportTicket, common.RoleAdminUser, 12, "").Body.Bytes(), &detail))
			require.True(t, detail.Success)
			require.NotNil(t, detail.Data.Ticket.User)
			assert.Equal(t, "alice", detail.Data.Ticket.User.Username)
			assert.Len(t, detail.Data.Conversations, 4)
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
