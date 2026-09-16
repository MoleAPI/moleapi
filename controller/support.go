package controller

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
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
}

type zohoDeskConversation struct {
	ID          string `json:"id"`
	Type        string `json:"type"`
	Direction   string `json:"direction"`
	Summary     string `json:"summary"`
	Content     string `json:"content"`
	CreatedTime string `json:"createdTime"`
	FromEmail   string `json:"fromEmailAddress"`
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
		FromEmail: common.OptionMap["ZohoDeskFromEmail"],
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
	return common.DecodeJson(res.Body, result)
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
	path := "/tickets?limit=50&sortBy=-modifiedTime&departmentId=" + url.QueryEscape(cfg.DepartmentID)
	if c.GetInt("role") < common.RoleAdminUser {
		user, ok := supportUser(c)
		if !ok {
			return
		}
		path = "/tickets/search?limit=50&sortBy=-modifiedTime&departmentId=" + url.QueryEscape(cfg.DepartmentID) + "&email=" + url.QueryEscape(user.Email)
	}
	var result struct {
		Data []zohoDeskTicket `json:"data"`
	}
	if err := zohoDeskRequest(cfg, http.MethodGet, path, nil, &result); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, result.Data)
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
		Subject string `json:"subject"`
		Content string `json:"content"`
		Type    string `json:"type"`
	}
	if err := common.DecodeJson(c.Request.Body, &input); err != nil {
		common.ApiErrorMsg(c, "Invalid request.")
		return
	}
	input.Subject, input.Content, input.Type = strings.TrimSpace(input.Subject), strings.TrimSpace(input.Content), strings.TrimSpace(input.Type)
	if utf8.RuneCountInString(input.Subject) < 3 || utf8.RuneCountInString(input.Subject) > 200 || utf8.RuneCountInString(input.Content) < 10 || utf8.RuneCountInString(input.Content) > 10000 {
		common.ApiErrorMsg(c, "Please check the subject and description length.")
		return
	}
	if input.Type != "Support" && input.Type != "Billing & Invoice" {
		common.ApiErrorMsg(c, "Invalid ticket type.")
		return
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
	var conversations struct {
		Data []zohoDeskConversation `json:"data"`
	}
	if err := zohoDeskRequest(cfg, http.MethodGet, "/tickets/"+ticket.ID+"/conversations", nil, &conversations); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"ticket": ticket, "conversations": conversations.Data})
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
		body := gin.H{"channel": "EMAIL", "content": input.Content, "contentType": "plainText", "fromEmailAddress": cfg.FromEmail, "to": ticket.Email}
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
