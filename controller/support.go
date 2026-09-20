package controller

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"math"
	"mime"
	"mime/multipart"
	"net/http"
	"net/url"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"golang.org/x/net/html"
)

type zohoDeskConfig struct {
	Enabled, ClientID, ClientSecret, RefreshToken, OrgID, DepartmentID string
	APIDomain, AccountsDomain, FromEmail                               string
}

type zohoDeskTicket struct {
	ID           string `json:"id"`
	TicketNumber string `json:"ticketNumber"`
	Subject      string `json:"subject"`
	Description  string `json:"description"`
	Status       string `json:"status"`
	Category     string `json:"category"`
	Priority     string `json:"priority"`
	Email        string `json:"email"`
	CreatedTime  string `json:"createdTime"`
	ModifiedTime string `json:"modifiedTime"`
	DepartmentID string `json:"departmentId"`
	StatusType   string `json:"statusType"`
	Archived     bool   `json:"isArchived"`
	CommentCount string `json:"commentCount"`
	LastThread   *struct {
		Direction string `json:"direction"`
		IsDraft   bool   `json:"isDraft"`
		IsForward bool   `json:"isForward"`
	} `json:"lastThread"`
	User     *supportTicketUser `json:"user,omitempty"`
	Activity string             `json:"activity"`
}

type supportTicketUser struct {
	ID       int    `json:"id"`
	Username string `json:"username"`
}

type zohoDeskConversation struct {
	ID            string `json:"id"`
	Type          string `json:"type"`
	Direction     string `json:"direction"`
	Summary       string `json:"summary"`
	Content       string `json:"content"`
	PlainText     string `json:"plainText"`
	CreatedTime   string `json:"createdTime"`
	FromEmail     string `json:"fromEmailAddress"`
	CommentedTime string `json:"commentedTime"`
	Visibility    string `json:"visibility"`
	IsPublic      bool   `json:"isPublic"`
	IsDraft       bool   `json:"isDraft"`
	IsForward     bool   `json:"isForward"`
	ContentType   string `json:"contentType"`
	Author        struct {
		Type string `json:"type"`
		Name string `json:"name"`
	} `json:"author"`
	Commenter struct {
		Type string `json:"type"`
		Name string `json:"name"`
	} `json:"commenter"`
	Attachments         []zohoDeskAttachment `json:"attachments,omitempty"`
	IsDescriptionThread bool                 `json:"isDescriptionThread,omitempty"`
}

type zohoDeskThread struct {
	ID                  string `json:"id"`
	Direction           string `json:"direction"`
	Summary             string `json:"summary"`
	Content             string `json:"content"`
	PlainText           string `json:"plainText"`
	CreatedTime         string `json:"createdTime"`
	Visibility          string `json:"visibility"`
	ContentType         string `json:"contentType"`
	Channel             string `json:"channel"`
	FromEmail           string `json:"fromEmailAddress"`
	IsDraft             bool   `json:"isDraft"`
	IsContentTruncated  bool   `json:"isContentTruncated"`
	IsForward           bool   `json:"isForward"`
	IsDescriptionThread bool   `json:"isDescriptionThread"`
	FullContentURL      string `json:"fullContentURL"`
	Author              struct {
		Type string `json:"type"`
		Name string `json:"name"`
	} `json:"author"`
	Attachments []zohoDeskAttachment `json:"attachments,omitempty"`
}

func (message zohoDeskConversation) public() bool {
	if message.IsDraft || message.IsForward {
		return false
	}
	if message.Type == "comment" {
		return message.IsPublic
	}
	return message.Visibility == "public"
}

var supportPortalComment = regexp.MustCompile(`(?s)^([^\r\n<]+) \(UID ([0-9]+)\):\s*(.*)$`)

func supportConversationText(value string) string {
	tokens := html.NewTokenizer(strings.NewReader(value))
	var text strings.Builder
	for token := tokens.Next(); token != html.ErrorToken; token = tokens.Next() {
		if token == html.TextToken {
			text.WriteString(string(tokens.Text()))
		}
	}
	return cleanSupportEmailText(text.String())
}

func normalizeSupportPortalComment(message *zohoDeskConversation) {
	if message.Type != "comment" || !message.IsPublic {
		return
	}
	match := supportPortalComment.FindStringSubmatch(supportConversationText(message.Content))
	if len(match) != 4 {
		return
	}
	message.Content = strings.TrimSpace(match[3])
	message.ContentType = "text/plain"
	message.Direction = "in"
	message.Author.Type = "END_USER"
	message.Author.Name = strings.TrimSpace(match[1])
	message.Commenter.Type = "END_USER"
	message.Commenter.Name = strings.TrimSpace(match[1])
}

func supportConversationMatchesTicketDescription(message zohoDeskConversation, ticket zohoDeskTicket) bool {
	if !message.public() || strings.TrimSpace(message.Content) == "" || strings.TrimSpace(ticket.Description) == "" || len(message.Attachments) > 0 || supportConversationText(message.Content) != supportConversationText(ticket.Description) {
		return false
	}
	messageTime := message.CreatedTime
	if messageTime == "" {
		messageTime = message.CommentedTime
	}
	ticketTime, ticketErr := time.Parse(time.RFC3339Nano, ticket.CreatedTime)
	conversationTime, conversationErr := time.Parse(time.RFC3339Nano, messageTime)
	if ticketErr == nil && conversationErr == nil {
		return ticketTime.Sub(conversationTime) <= 2*time.Minute && conversationTime.Sub(ticketTime) <= 2*time.Minute
	}
	return true
}

func cleanSupportEmailText(value string) string {
	lines := strings.Split(strings.ReplaceAll(value, "\r\n", "\n"), "\n")
	cleaned := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		line = strings.TrimLeft(line, "#")
		line = strings.TrimSpace(strings.ReplaceAll(line, "**", ""))
		if strings.Trim(line, "| -:") == "" && strings.Contains(line, "|") {
			continue
		}
		cleaned = append(cleaned, line)
	}
	return strings.TrimSpace(strings.Join(cleaned, "\n"))
}

func supportConversationActivity(messages []zohoDeskConversation) string {
	var latest time.Time
	activity := "unknown"
	for _, message := range messages {
		normalizeSupportPortalComment(&message)
		if !message.public() {
			continue
		}
		created := message.CreatedTime
		if created == "" {
			created = message.CommentedTime
		}
		createdAt, err := time.Parse(time.RFC3339Nano, created)
		if err != nil || !createdAt.After(latest) {
			continue
		}
		latest = createdAt
		activity = "unknown"
		portalReply := false
		if message.Type == "comment" {
			tokens := html.NewTokenizer(strings.NewReader(message.Content))
			for token := tokens.Next(); token != html.ErrorToken; token = tokens.Next() {
				if token == html.TextToken {
					text := strings.TrimSpace(string(tokens.Text()))
					if text != "" {
						portalReply = supportPortalComment.MatchString(text)
						break
					}
				}
			}
		}
		if message.Direction == "in" || message.Author.Type == "END_USER" || message.Commenter.Type == "END_USER" || portalReply {
			activity = "customer"
		} else if message.Direction == "out" || message.Author.Type == "AGENT" || message.Commenter.Type == "AGENT" {
			activity = "agent"
		}
	}
	return activity
}

func loadSupportEmailThreads(cfg zohoDeskConfig, ticketID string) ([]zohoDeskConversation, error) {
	var result struct {
		Data []zohoDeskThread `json:"data"`
	}
	if err := zohoDeskRequest(cfg, http.MethodGet, "/tickets/"+ticketID+"/threads?limit=100&from=0", nil, &result); err != nil {
		return nil, err
	}
	if len(result.Data) == 0 {
		var latest zohoDeskThread
		if err := zohoDeskRequest(cfg, http.MethodGet, "/tickets/"+ticketID+"/latestThread?needIncomingThread=true", nil, &latest); err == nil && latest.ID != "" {
			result.Data = []zohoDeskThread{latest}
		}
	}
	conversations := make([]zohoDeskConversation, 0, len(result.Data))
	for _, thread := range result.Data {
		if thread.Content == "" && thread.PlainText == "" && thread.ID != "" {
			// Listing endpoints contain summaries, not message bodies. Decode into
			// the existing thread so optional metadata from the list is retained.
			if err := zohoDeskRequest(cfg, http.MethodGet, "/tickets/"+ticketID+"/threads/"+thread.ID, nil, &thread); err != nil {
				return nil, err
			}
		}
		content := thread.Content
		if content == "" {
			content = thread.PlainText
		}
		if thread.IsContentTruncated {
			// fullContent returns raw HTML, not a JSON object.
			if err := zohoDeskRequest(cfg, http.MethodGet, "/tickets/"+ticketID+"/threads/"+thread.ID+"/fullContent", nil, &content); err != nil {
				return nil, err
			}
		}
		if content == "" {
			content = thread.Summary
		}
		visibility := thread.Visibility
		if thread.Direction == "in" && (thread.Channel == "EMAIL" || visibility == "private" || visibility == "") {
			visibility = "public"
		}
		conversations = append(conversations, zohoDeskConversation{
			ID: thread.ID, Type: "thread", Direction: thread.Direction,
			Summary: thread.Summary, Content: content, PlainText: thread.PlainText, CreatedTime: thread.CreatedTime,
			Visibility: visibility, IsPublic: visibility == "public", IsForward: thread.IsForward,
			FromEmail: thread.FromEmail, IsDraft: thread.IsDraft,
			ContentType: thread.ContentType, Author: thread.Author, Attachments: thread.Attachments,
			IsDescriptionThread: thread.IsDescriptionThread,
		})
	}
	return conversations, nil
}

func loadSupportEmailComments(cfg zohoDeskConfig, ticketID string) ([]zohoDeskConversation, error) {
	var result struct {
		Data []zohoDeskConversation `json:"data"`
	}
	if err := zohoDeskRequest(cfg, http.MethodGet, "/tickets/"+ticketID+"/comments?limit=100&from=0", nil, &result); err != nil {
		return nil, err
	}
	for i := range result.Data {
		if result.Data[i].Content == "" {
			result.Data[i].Content = result.Data[i].PlainText
		}
		result.Data[i].Type = "comment"
		if result.Data[i].Direction == "" {
			result.Data[i].Direction = "in"
		}
		result.Data[i].IsPublic = result.Data[i].IsPublic || result.Data[i].Visibility == "public"
	}
	return result.Data, nil
}

type zohoDeskAttachment struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Size        string `json:"size"`
	Href        string `json:"href"`
	IsPublic    *bool  `json:"isPublic,omitempty"`
	CreatedTime string `json:"createdTime,omitempty"`
	CreatorID   string `json:"creatorId,omitempty"`
	ContentType string `json:"contentType,omitempty"`
}

const (
	supportAttachmentLimit       = 3
	supportAttachmentMaxFileSize = 5 << 20
	supportAttachmentMaxBodySize = 16 << 20
)

var supportTicketTypes = map[string]struct{}{
	"API Integration":      {},
	"Authentication Issue": {},
	"Billing & Credits":    {},
	"Model Availability":   {},
	"Model Rate Limit":     {},
	"Partnership Inquiry":  {},
	"Feature Request":      {},
	"Invoice Request":      {},
	"Other":                {},
}

var supportAttachmentContentTypes = map[string]string{
	".gif": "image/gif", ".jpeg": "image/jpeg", ".jpg": "image/jpeg",
	".log": "text/plain", ".pdf": "application/pdf", ".png": "image/png",
	".txt": "text/plain", ".webp": "image/webp",
}

func validSupportAttachment(header *multipart.FileHeader) bool {
	expectedType, allowed := supportAttachmentContentTypes[strings.ToLower(filepath.Ext(header.Filename))]
	if header.Size <= 0 || header.Size > supportAttachmentMaxFileSize || !allowed {
		return false
	}
	file, err := header.Open()
	if err != nil {
		return false
	}
	defer file.Close()
	prefix := make([]byte, 512)
	n, err := io.ReadFull(file, prefix)
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
		return false
	}
	detectedType := http.DetectContentType(prefix[:n])
	if expectedType == "text/plain" {
		return strings.HasPrefix(detectedType, expectedType)
	}
	return detectedType == expectedType
}

func safeSupportDownloadContentType(prefix []byte) string {
	detectedType := http.DetectContentType(prefix)
	switch detectedType {
	case "image/gif", "image/jpeg", "image/png", "image/webp":
		return detectedType
	default:
		return "application/octet-stream"
	}
}

func safeSupportContentDisposition(value string) string {
	_, params, err := mime.ParseMediaType(value)
	if err != nil {
		return "attachment"
	}
	filename := strings.TrimSpace(params["filename"])
	if separator := strings.LastIndexAny(filename, `/\`); separator >= 0 {
		filename = filename[separator+1:]
	}
	if filename == "" {
		return "attachment"
	}
	disposition := mime.FormatMediaType("attachment", map[string]string{"filename": filename})
	if disposition == "" {
		return "attachment"
	}
	return disposition
}

var zohoDeskTokenCache struct {
	sync.Mutex
	token     string
	key       string
	expiresAt time.Time
}

func getZohoDeskConfig() zohoDeskConfig {
	common.OptionMapRWMutex.RLock()
	defer common.OptionMapRWMutex.RUnlock()
	return zohoDeskConfig{
		Enabled: common.OptionMap["ZohoDeskEnabled"], ClientID: common.OptionMap["ZohoDeskClientId"],
		ClientSecret: common.OptionMap["ZohoDeskClientSecret"], RefreshToken: common.OptionMap["ZohoDeskRefreshToken"],
		OrgID: common.OptionMap["ZohoDeskOrgId"], DepartmentID: common.OptionMap["ZohoDeskDepartmentId"],
		APIDomain: common.OptionMap["ZohoDeskApiDomain"], AccountsDomain: common.OptionMap["ZohoDeskAccountsDomain"],
		FromEmail: strings.TrimSpace(common.OptionMap["ZohoDeskFromEmail"]),
	}
}

func (cfg zohoDeskConfig) ready() bool {
	return cfg.Enabled == "true" && cfg.ClientID != "" && cfg.ClientSecret != "" && cfg.RefreshToken != "" && cfg.OrgID != "" && cfg.DepartmentID != "" && cfg.APIDomain != "" && cfg.AccountsDomain != ""
}

func zohoDeskAccessToken(cfg zohoDeskConfig) (string, error) {
	zohoDeskTokenCache.Lock()
	defer zohoDeskTokenCache.Unlock()
	cacheKey := cfg.AccountsDomain + "\x00" + cfg.ClientID + "\x00" + cfg.RefreshToken
	if zohoDeskTokenCache.key == cacheKey && zohoDeskTokenCache.token != "" && time.Now().Before(zohoDeskTokenCache.expiresAt) {
		return zohoDeskTokenCache.token, nil
	}
	form := url.Values{"refresh_token": {cfg.RefreshToken}, "client_id": {cfg.ClientID}, "client_secret": {cfg.ClientSecret}, "grant_type": {"refresh_token"}}
	req, err := http.NewRequest(http.MethodPost, strings.TrimRight(cfg.AccountsDomain, "/")+"/oauth/v2/token", strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res, err := (&http.Client{Timeout: 15 * time.Second}).Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	var payload struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
		Error       string `json:"error"`
	}
	if err = common.DecodeJson(res.Body, &payload); err != nil {
		return "", err
	}
	if res.StatusCode >= 300 || payload.AccessToken == "" {
		return "", fmt.Errorf("Zoho OAuth failed: %s", payload.Error)
	}
	zohoDeskTokenCache.token = payload.AccessToken
	zohoDeskTokenCache.key = cacheKey
	zohoDeskTokenCache.expiresAt = time.Now().Add(time.Duration(max(payload.ExpiresIn-60, 60)) * time.Second)
	return payload.AccessToken, nil
}

func zohoDeskRequest(cfg zohoDeskConfig, method, path string, body any, result any) error {
	token, err := zohoDeskAccessToken(cfg)
	if err != nil {
		return err
	}
	var reader io.Reader
	if body != nil {
		data, marshalErr := common.Marshal(body)
		if marshalErr != nil {
			return marshalErr
		}
		reader = bytes.NewReader(data)
	}
	req, err := http.NewRequest(method, strings.TrimRight(cfg.APIDomain, "/")+"/api/v1"+path, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Zoho-oauthtoken "+token)
	req.Header.Set("orgId", cfg.OrgID)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := (&http.Client{Timeout: 20 * time.Second}).Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode >= 300 {
		message, _ := io.ReadAll(io.LimitReader(res.Body, 2048))
		return fmt.Errorf("Zoho Desk returned %d: %s", res.StatusCode, strings.TrimSpace(string(message)))
	}
	if result == nil || res.StatusCode == http.StatusNoContent {
		return nil
	}
	if content, ok := result.(*string); ok {
		data, readErr := io.ReadAll(io.LimitReader(res.Body, 10<<20+1))
		if readErr != nil {
			return readErr
		}
		if len(data) > 10<<20 {
			return errors.New("Support message exceeds the 10 MB limit.")
		}
		*content = string(data)
		return nil
	}
	return common.DecodeJson(res.Body, result)
}

func zohoDeskUploadAttachment(cfg zohoDeskConfig, ticketID string, header *multipart.FileHeader) (zohoDeskAttachment, error) {
	file, err := header.Open()
	if err != nil {
		return zohoDeskAttachment{}, err
	}
	defer file.Close()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", filepath.Base(header.Filename))
	if err != nil {
		return zohoDeskAttachment{}, err
	}
	if _, err = io.Copy(part, file); err != nil {
		return zohoDeskAttachment{}, err
	}
	if err = writer.Close(); err != nil {
		return zohoDeskAttachment{}, err
	}

	token, err := zohoDeskAccessToken(cfg)
	if err != nil {
		return zohoDeskAttachment{}, err
	}
	req, err := http.NewRequest(http.MethodPost, strings.TrimRight(cfg.APIDomain, "/")+"/api/v1/tickets/"+ticketID+"/attachments?isPublic=true", &body)
	if err != nil {
		return zohoDeskAttachment{}, err
	}
	req.Header.Set("Authorization", "Zoho-oauthtoken "+token)
	req.Header.Set("orgId", cfg.OrgID)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	res, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		return zohoDeskAttachment{}, err
	}
	defer res.Body.Close()
	if res.StatusCode >= 300 {
		message, _ := io.ReadAll(io.LimitReader(res.Body, 2048))
		return zohoDeskAttachment{}, fmt.Errorf("Zoho Desk returned %d: %s", res.StatusCode, strings.TrimSpace(string(message)))
	}
	var attachment zohoDeskAttachment
	if err = common.DecodeJson(res.Body, &attachment); err != nil {
		return zohoDeskAttachment{}, err
	}
	return attachment, nil
}

func zohoDeskDownloadAttachment(cfg zohoDeskConfig, ticketID, attachmentID string) (*http.Response, error) {
	token, err := zohoDeskAccessToken(cfg)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest(http.MethodGet, strings.TrimRight(cfg.APIDomain, "/")+"/api/v1/tickets/"+ticketID+"/attachments/"+attachmentID+"/content", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Zoho-oauthtoken "+token)
	req.Header.Set("orgId", cfg.OrgID)
	res, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		return nil, err
	}
	if res.StatusCode >= 300 {
		defer res.Body.Close()
		message, _ := io.ReadAll(io.LimitReader(res.Body, 2048))
		return nil, fmt.Errorf("Zoho Desk returned %d: %s", res.StatusCode, strings.TrimSpace(string(message)))
	}
	return res, nil
}

func supportUser(c *gin.Context) (*model.User, bool) {
	user, err := model.GetUserById(c.GetInt("id"), false)
	if err != nil {
		common.ApiError(c, err)
		return nil, false
	}
	if strings.TrimSpace(user.Email) == "" {
		common.ApiErrorMsg(c, "Please bind an email address before creating a ticket.")
		return nil, false
	}
	return user, true
}

func GetSupportConfig(c *gin.Context) {
	cfg := getZohoDeskConfig()
	common.OptionMapRWMutex.RLock()
	links := gin.H{"discord": common.OptionMap["SupportDiscordUrl"], "telegram": common.OptionMap["SupportTelegramUrl"], "qq": common.OptionMap["SupportQQUrl"], "wechat": common.OptionMap["SupportWeChatUrl"]}
	common.OptionMapRWMutex.RUnlock()
	common.ApiSuccess(c, gin.H{"enabled": cfg.ready(), "community_links": links})
}

func ListSupportTickets(c *gin.Context) {
	cfg := getZohoDeskConfig()
	if !cfg.ready() {
		common.ApiErrorMsg(c, "Support tickets are not configured yet.")
		return
	}
	from, err := strconv.Atoi(c.DefaultQuery("from", "0"))
	if err != nil || from < 0 || from > 100000 {
		common.ApiErrorMsg(c, "Invalid page.")
		return
	}
	query := "?limit=20&from=" + strconv.Itoa(from) + "&sortBy=-modifiedTime&departmentId=" + url.QueryEscape(cfg.DepartmentID)
	path := "/tickets" + query
	archivedView := c.GetInt("role") >= common.RoleAdminUser && c.Query("view") == "archived"
	if archivedView {
		path = "/tickets/archivedTickets?limit=100&from=" + strconv.Itoa(from) + "&departmentId=" + url.QueryEscape(cfg.DepartmentID)
	}
	email := ""
	if c.GetInt("role") < common.RoleAdminUser {
		user, ok := supportUser(c)
		if !ok {
			return
		}
		email = user.Email
		path = "/tickets/search" + query + "&email=" + url.QueryEscape(user.Email)
	}
	var result struct {
		Data []zohoDeskTicket `json:"data"`
	}
	if err := zohoDeskRequest(cfg, http.MethodGet, path, nil, &result); err != nil {
		common.ApiError(c, err)
		return
	}
	tickets := make([]zohoDeskTicket, 0, len(result.Data))
	for _, ticket := range result.Data {
		if ticket.DepartmentID != cfg.DepartmentID || (email != "" && !strings.EqualFold(ticket.Email, email)) {
			continue
		}
		if c.GetInt("role") >= common.RoleAdminUser {
			user, err := model.GetUniqueUserByEmail(ticket.Email)
			if err == nil {
				ticket.User = &supportTicketUser{ID: user.Id, Username: user.Username}
			} else if !errors.Is(err, model.ErrEmailNotFound) && !errors.Is(err, model.ErrEmailAmbiguous) {
				common.ApiError(c, err)
				return
			}
		}
		ticket.Activity = "new"
		if ticket.LastThread != nil {
			ticket.Activity = "unknown"
			if !ticket.LastThread.IsDraft && !ticket.LastThread.IsForward {
				if ticket.LastThread.Direction == "in" {
					ticket.Activity = "customer"
				}
				if ticket.LastThread.Direction == "out" {
					ticket.Activity = "agent"
				}
			}
		}
		tickets = append(tickets, ticket)
	}
	if archivedView {
		for i := range tickets {
			tickets[i].Archived = true
			tickets[i].Activity = "archived"
			user, err := model.GetUniqueUserByEmail(tickets[i].Email)
			if err == nil {
				tickets[i].User = &supportTicketUser{ID: user.Id, Username: user.Username}
			} else if !errors.Is(err, model.ErrEmailNotFound) && !errors.Is(err, model.ErrEmailAmbiguous) {
				common.ApiError(c, err)
				return
			}
		}
	} else if c.GetInt("role") >= common.RoleAdminUser && from == 0 {
		var archived struct {
			Data []zohoDeskTicket `json:"data"`
		}
		archivePath := "/tickets/archivedTickets?limit=100&from=0&departmentId=" + url.QueryEscape(cfg.DepartmentID)
		if err := zohoDeskRequest(cfg, http.MethodGet, archivePath, nil, &archived); err == nil {
			for _, ticket := range archived.Data {
				if ticket.DepartmentID != cfg.DepartmentID {
					continue
				}
				ticket.Archived = true
				ticket.Activity = "archived"
				user, userErr := model.GetUniqueUserByEmail(ticket.Email)
				if userErr == nil {
					ticket.User = &supportTicketUser{ID: user.Id, Username: user.Username}
				} else if !errors.Is(userErr, model.ErrEmailNotFound) && !errors.Is(userErr, model.ErrEmailAmbiguous) {
					common.ApiError(c, userErr)
					return
				}
				tickets = append(tickets, ticket)
			}
		}
	}
	// Portal replies are public comments, not email threads. Bound concurrent
	// lookups to avoid exhausting Zoho's per-organization request allowance.
	var pending sync.WaitGroup
	slots := make(chan struct{}, 4)
	for i := range tickets {
		if tickets[i].CommentCount == "" || tickets[i].CommentCount == "0" {
			continue
		}
		slots <- struct{}{}
		pending.Add(1)
		go func(ticket *zohoDeskTicket) {
			defer pending.Done()
			defer func() { <-slots }()
			var conversations struct {
				Data []zohoDeskConversation `json:"data"`
			}
			ticket.Activity = "unknown"
			if err := zohoDeskRequest(cfg, http.MethodGet, "/tickets/"+ticket.ID+"/conversations?limit=20", nil, &conversations); err == nil {
				for i := range conversations.Data {
					normalizeSupportPortalComment(&conversations.Data[i])
				}
				ticket.Activity = supportConversationActivity(conversations.Data)
			}
		}(&tickets[i])
	}
	pending.Wait()
	common.ApiSuccess(c, gin.H{"tickets": tickets, "next_from": from + len(result.Data), "has_more": len(result.Data) == 20 && !archivedView})
}

func supportInvoiceSummary(userID int, ids []int) (string, error) {
	if len(ids) == 0 || len(ids) > 50 {
		return "", errors.New("Select between 1 and 50 paid orders for invoicing.")
	}
	seen := make(map[int]bool, len(ids))
	for _, id := range ids {
		if id <= 0 || seen[id] {
			return "", errors.New("Invalid or duplicate billing record.")
		}
		seen[id] = true
	}
	orders, err := model.GetUserTopUpsByIDs(userID, ids)
	if err != nil {
		return "", err
	}
	if len(orders) != len(ids) {
		return "", errors.New("One or more billing records are unavailable.")
	}
	lines := []string{"Verified billing records (actual paid amounts)"}
	totals := make(map[string]decimal.Decimal)
	for _, order := range orders {
		if order.Status != "success" || (order.PaymentMethod != "alipay" && order.PaymentMethod != "wxpay" && order.PaymentMethod != model.PaymentMethodLanTu) || order.Money <= 0 || math.IsNaN(order.Money) || math.IsInf(order.Money, 0) {
			return "", errors.New("Only completed WeChat Pay or Alipay orders can be invoiced.")
		}
		currency := strings.ToUpper(strings.TrimSpace(order.PaymentCurrency))
		if currency == "" {
			// These legacy payment methods settled in CNY before currency snapshots existed.
			currency = "CNY"
		}
		paid := decimal.NewFromFloat(order.Money).Round(2)
		lines = append(lines, fmt.Sprintf("#%d | %s | %s %s | %s", order.Id, order.TradeNo, currency, paid.StringFixed(2), order.PaymentMethod))
		totals[currency] = totals[currency].Add(paid)
	}
	currencies := make([]string, 0, len(totals))
	for currency := range totals {
		currencies = append(currencies, currency)
	}
	sort.Strings(currencies)
	for _, currency := range currencies {
		lines = append(lines, "Invoice total: "+currency+" "+totals[currency].StringFixed(2))
	}
	return strings.Join(lines, "\n"), nil
}

func CreateSupportTicket(c *gin.Context) {
	cfg := getZohoDeskConfig()
	if !cfg.ready() {
		common.ApiErrorMsg(c, "Support tickets are not configured yet.")
		return
	}
	user, ok := supportUser(c)
	if !ok {
		return
	}
	var input struct {
		Subject          string `json:"subject"`
		Content          string `json:"content"`
		Type             string `json:"type"`
		BillingRecordIDs []int  `json:"billing_record_ids"`
	}
	if err := common.DecodeJson(c.Request.Body, &input); err != nil {
		common.ApiErrorMsg(c, "Invalid request.")
		return
	}
	input.Subject, input.Content, input.Type = cleanSupportEmailText(input.Subject), cleanSupportEmailText(input.Content), strings.TrimSpace(input.Type)
	if utf8.RuneCountInString(input.Subject) < 3 || utf8.RuneCountInString(input.Subject) > 200 || utf8.RuneCountInString(input.Content) < 10 || utf8.RuneCountInString(input.Content) > 10000 {
		common.ApiErrorMsg(c, "Please check the subject and description length.")
		return
	}
	if _, ok := supportTicketTypes[input.Type]; !ok {
		common.ApiErrorMsg(c, "Invalid ticket type.")
		return
	}
	if input.Type == "Invoice Request" {
		summary, err := supportInvoiceSummary(user.Id, input.BillingRecordIDs)
		if err != nil {
			common.ApiError(c, err)
			return
		}
		input.Content += "\n\n" + summary
	}
	var contacts struct {
		Data []struct{ ID, Email string } `json:"data"`
	}
	if err := zohoDeskRequest(cfg, http.MethodGet, "/contacts/search?limit=1&email="+url.QueryEscape(user.Email), nil, &contacts); err != nil {
		common.ApiError(c, err)
		return
	}
	contactID := ""
	if len(contacts.Data) > 0 && strings.EqualFold(contacts.Data[0].Email, user.Email) {
		contactID = contacts.Data[0].ID
	}
	if contactID == "" {
		var contact struct {
			ID string `json:"id"`
		}
		name := strings.TrimSpace(user.DisplayName)
		if name == "" {
			name = user.Username
		}
		if err := zohoDeskRequest(cfg, http.MethodPost, "/contacts", gin.H{"lastName": name, "email": user.Email}, &contact); err != nil {
			common.ApiError(c, err)
			return
		}
		contactID = contact.ID
	}
	var ticket zohoDeskTicket
	body := gin.H{"subject": input.Subject, "description": input.Content, "category": input.Type, "channel": "Web", "priority": "Medium", "departmentId": cfg.DepartmentID, "contactId": contactID, "email": user.Email}
	if err := zohoDeskRequest(cfg, http.MethodPost, "/tickets", body, &ticket); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, ticket)
}

func UploadSupportAttachments(c *gin.Context) {
	cfg := getZohoDeskConfig()
	if !cfg.ready() {
		common.ApiErrorMsg(c, "Support tickets are not configured yet.")
		return
	}
	ticket, ok := loadSupportTicket(c, cfg)
	if !ok {
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, supportAttachmentMaxBodySize)
	if err := c.Request.ParseMultipartForm(1 << 20); err != nil {
		common.ApiErrorMsg(c, "Invalid or oversized attachment request.")
		return
	}
	files := c.Request.MultipartForm.File["files"]
	if len(files) == 0 || len(files) > supportAttachmentLimit {
		common.ApiErrorMsg(c, "Attach between 1 and 3 files.")
		return
	}
	for _, header := range files {
		if !validSupportAttachment(header) {
			common.ApiErrorMsg(c, "Only images, PDF, text, and log files up to 5 MB are supported.")
			return
		}
	}

	attachments := make([]zohoDeskAttachment, 0, len(files))
	for _, header := range files {
		attachment, err := zohoDeskUploadAttachment(cfg, ticket.ID, header)
		if err != nil {
			common.ApiError(c, err)
			return
		}
		attachments = append(attachments, attachment)
	}
	common.ApiSuccess(c, attachments)
}

func DownloadSupportAttachment(c *gin.Context) {
	cfg := getZohoDeskConfig()
	if !cfg.ready() {
		common.ApiErrorMsg(c, "Support tickets are not configured yet.")
		return
	}
	ticket, ok := loadSupportTicket(c, cfg)
	if !ok {
		return
	}
	attachmentID := c.Param("attachment_id")
	if _, err := strconv.ParseUint(attachmentID, 10, 64); err != nil {
		common.ApiErrorMsg(c, "Invalid attachment ID.")
		return
	}
	if c.GetInt("role") < common.RoleAdminUser {
		var attachments struct {
			Data []zohoDeskAttachment `json:"data"`
		}
		if err := zohoDeskRequest(cfg, http.MethodGet, "/tickets/"+ticket.ID+"/attachments", nil, &attachments); err != nil {
			common.ApiError(c, err)
			return
		}
		allowed := false
		for _, attachment := range attachments.Data {
			if attachment.ID == attachmentID && attachment.IsPublic != nil && *attachment.IsPublic {
				allowed = true
				break
			}
		}
		if !allowed {
			common.ApiErrorMsg(c, "Attachment not found.")
			return
		}
	}
	res, err := zohoDeskDownloadAttachment(cfg, ticket.ID, attachmentID)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	defer res.Body.Close()
	prefix := make([]byte, 512)
	n, readErr := io.ReadFull(res.Body, prefix)
	if readErr != nil && !errors.Is(readErr, io.EOF) && !errors.Is(readErr, io.ErrUnexpectedEOF) {
		common.ApiError(c, readErr)
		return
	}
	c.Header("Content-Type", safeSupportDownloadContentType(prefix[:n]))
	c.Header("Content-Disposition", safeSupportContentDisposition(res.Header.Get("Content-Disposition")))
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("Content-Security-Policy", "sandbox")
	c.Header("Cache-Control", "private, no-store")
	if value := res.Header.Get("Content-Length"); value != "" {
		c.Header("Content-Length", value)
	}
	c.Status(res.StatusCode)
	if _, err = c.Writer.Write(prefix[:n]); err != nil {
		common.SysError("failed to proxy support attachment: " + err.Error())
		return
	}
	if _, err = io.Copy(c.Writer, res.Body); err != nil {
		common.SysError("failed to proxy support attachment: " + err.Error())
	}
}

func loadSupportTicket(c *gin.Context, cfg zohoDeskConfig) (zohoDeskTicket, bool) {
	id := c.Param("id")
	if _, err := strconv.ParseUint(id, 10, 64); err != nil {
		common.ApiErrorMsg(c, "Invalid ticket ID.")
		return zohoDeskTicket{}, false
	}
	var ticket zohoDeskTicket
	if err := zohoDeskRequest(cfg, http.MethodGet, "/tickets/"+id, nil, &ticket); err != nil {
		common.ApiError(c, err)
		return ticket, false
	}
	if ticket.DepartmentID != cfg.DepartmentID {
		common.ApiErrorMsg(c, "Ticket not found.")
		return ticket, false
	}
	if c.GetInt("role") < common.RoleAdminUser {
		user, ok := supportUser(c)
		if !ok {
			return ticket, false
		}
		if !strings.EqualFold(ticket.Email, user.Email) {
			common.ApiError(c, errors.New("ticket not found"))
			return ticket, false
		}
	}
	return ticket, true
}

func GetSupportTicket(c *gin.Context) {
	cfg := getZohoDeskConfig()
	if !cfg.ready() {
		common.ApiErrorMsg(c, "Support tickets are not configured yet.")
		return
	}
	ticket, ok := loadSupportTicket(c, cfg)
	if !ok {
		return
	}
	if c.GetInt("role") >= common.RoleAdminUser {
		user, err := model.GetUniqueUserByEmail(ticket.Email)
		if err == nil {
			ticket.User = &supportTicketUser{ID: user.Id, Username: user.Username}
		} else if !errors.Is(err, model.ErrEmailNotFound) && !errors.Is(err, model.ErrEmailAmbiguous) {
			common.ApiError(c, err)
			return
		}
	}
	var conversations struct {
		Data []zohoDeskConversation `json:"data"`
	}
	var attachments struct {
		Data []zohoDeskAttachment `json:"data"`
	}
	if err := zohoDeskRequest(cfg, http.MethodGet, "/tickets/"+ticket.ID+"/attachments", nil, &attachments); err != nil {
		common.ApiError(c, err)
		return
	}
	if err := zohoDeskRequest(cfg, http.MethodGet, "/tickets/"+ticket.ID+"/conversations", nil, &conversations); err != nil {
		common.ApiError(c, err)
		return
	}
	for i := range conversations.Data {
		if conversations.Data[i].Type == "thread" && conversations.Data[i].Direction == "in" {
			conversations.Data[i].Visibility = "public"
			conversations.Data[i].IsPublic = true
		}
	}
	// A non-empty conversations list still contains only email summaries.
	needsThreads := len(conversations.Data) == 0
	for _, message := range conversations.Data {
		if message.Type == "thread" && strings.TrimSpace(message.Content) == "" {
			needsThreads = true
		}
	}
	if needsThreads {
		threads, err := loadSupportEmailThreads(cfg, ticket.ID)
		if err != nil {
			common.ApiError(c, err)
			return
		}
		for _, thread := range threads {
			replaced := false
			for i := range conversations.Data {
				if conversations.Data[i].Type == "thread" && conversations.Data[i].ID == thread.ID {
					conversations.Data[i] = thread
					replaced = true
					break
				}
			}
			if !replaced {
				conversations.Data = append(conversations.Data, thread)
			}
		}
	}
	if len(conversations.Data) == 0 {
		comments, err := loadSupportEmailComments(cfg, ticket.ID)
		if err != nil {
			common.ApiError(c, err)
			return
		}
		conversations.Data = comments
	}
	ticket.Activity = supportConversationActivity(conversations.Data)
	visible := make([]zohoDeskConversation, 0, len(conversations.Data))
	initialDescriptionShown := false
	for _, message := range conversations.Data {
		if c.GetInt("role") >= common.RoleAdminUser || message.public() {
			if !initialDescriptionShown && supportConversationMatchesTicketDescription(message, ticket) {
				initialDescriptionShown = true
				continue
			}
			normalizeSupportPortalComment(&message)
			if c.GetInt("role") < common.RoleAdminUser {
				publicAttachments := message.Attachments[:0]
				for _, attachment := range message.Attachments {
					if attachment.IsPublic != nil && *attachment.IsPublic {
						publicAttachments = append(publicAttachments, attachment)
					}
				}
				message.Attachments = publicAttachments
			}
			visible = append(visible, message)
		}
	}
	files := make([]zohoDeskAttachment, 0, len(attachments.Data))
	for _, attachment := range attachments.Data {
		if c.GetInt("role") >= common.RoleAdminUser || (attachment.IsPublic != nil && *attachment.IsPublic) {
			files = append(files, attachment)
		}
	}
	common.ApiSuccess(c, gin.H{"ticket": ticket, "conversations": visible, "attachments": files})
}

func UpdateSupportTicketStatus(c *gin.Context) {
	cfg := getZohoDeskConfig()
	if !cfg.ready() {
		common.ApiErrorMsg(c, "Support tickets are not configured yet.")
		return
	}
	ticket, ok := loadSupportTicket(c, cfg)
	if !ok {
		return
	}
	var input struct {
		Status string `json:"status"`
	}
	if err := common.DecodeJson(c.Request.Body, &input); err != nil {
		common.ApiErrorMsg(c, "Invalid request.")
		return
	}
	if input.Status == "Archived" {
		if c.GetInt("role") < common.RoleAdminUser {
			common.ApiErrorMsg(c, "Only administrators can archive tickets.")
			return
		}
		if err := zohoDeskRequest(cfg, http.MethodPost, "/tickets/"+ticket.ID+"/archive", nil, nil); err != nil {
			common.ApiError(c, err)
			return
		}
		common.ApiSuccess(c, nil)
		return
	}
	if input.Status == "Open" && ticket.Archived {
		if c.GetInt("role") < common.RoleAdminUser {
			common.ApiErrorMsg(c, "Only administrators can restore archived tickets.")
			return
		}
		if err := zohoDeskRequest(cfg, http.MethodPost, "/tickets/"+ticket.ID+"/unarchive", nil, nil); err != nil {
			common.ApiError(c, err)
			return
		}
		common.ApiSuccess(c, nil)
		return
	}
	if input.Status != "Open" && input.Status != "Closed" && !(input.Status == "On Hold" && c.GetInt("role") >= common.RoleAdminUser) {
		common.ApiErrorMsg(c, "Invalid ticket status.")
		return
	}
	if err := zohoDeskRequest(cfg, http.MethodPatch, "/tickets/"+ticket.ID, gin.H{"status": input.Status}, nil); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, nil)
}

func ReplySupportTicket(c *gin.Context) {
	cfg := getZohoDeskConfig()
	if !cfg.ready() {
		common.ApiErrorMsg(c, "Support tickets are not configured yet.")
		return
	}
	ticket, ok := loadSupportTicket(c, cfg)
	if !ok {
		return
	}
	if ticket.StatusType == "Closed" || ticket.Status == "Closed" {
		common.ApiErrorMsg(c, "Reopen the ticket before replying.")
		return
	}
	var input struct {
		Content string `json:"content"`
	}
	if err := common.DecodeJson(c.Request.Body, &input); err != nil {
		common.ApiErrorMsg(c, "Invalid request.")
		return
	}
	input.Content = strings.TrimSpace(input.Content)
	if utf8.RuneCountInString(input.Content) < 1 || utf8.RuneCountInString(input.Content) > 10000 {
		common.ApiErrorMsg(c, "Reply must be between 1 and 10000 characters.")
		return
	}
	if c.GetInt("role") >= common.RoleAdminUser {
		if cfg.FromEmail == "" {
			common.ApiErrorMsg(c, "Zoho Desk sender email is not configured.")
			return
		}
		body := gin.H{"channel": "EMAIL", "content": cleanSupportEmailText(input.Content), "contentType": "plainText", "fromEmailAddress": cfg.FromEmail, "to": ticket.Email}
		if err := zohoDeskRequest(cfg, http.MethodPost, "/tickets/"+ticket.ID+"/sendReply", body, nil); err != nil {
			common.ApiError(c, err)
			return
		}
	} else {
		user, ok := supportUser(c)
		if !ok {
			return
		}
		body := gin.H{"content": fmt.Sprintf("%s (UID %d):\n\n%s", user.Username, user.Id, input.Content), "contentType": "plainText", "isPublic": true}
		if err := zohoDeskRequest(cfg, http.MethodPost, "/tickets/"+ticket.ID+"/comments", body, nil); err != nil {
			common.ApiError(c, err)
			return
		}
	}
	common.ApiSuccess(c, nil)
}
