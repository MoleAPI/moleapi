package controller

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupTopUpInvoiceTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	gin.SetMode(gin.TestMode)
	originalDB := model.DB
	originalLogDB := model.LOG_DB
	originalMainDatabaseType := common.MainDatabaseType()
	originalLogDatabaseType := common.LogDatabaseType()
	originalRedisEnabled := common.RedisEnabled
	originalInvoiceLogo := TopUpInvoiceLogo
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	common.RedisEnabled = false
	logo, err := os.ReadFile(filepath.Join("..", "web", "public", "logo.png"))
	require.NoError(t, err)
	TopUpInvoiceLogo = logo

	dsn := "file:" + url.QueryEscape(t.Name()) + "?mode=memory&cache=shared"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	model.LOG_DB = db
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.TopUp{}, &model.Log{}, &model.AuditLog{}))

	t.Cleanup(func() {
		model.DB = originalDB
		model.LOG_DB = originalLogDB
		common.SetDatabaseTypes(originalMainDatabaseType, originalLogDatabaseType)
		common.RedisEnabled = originalRedisEnabled
		TopUpInvoiceLogo = originalInvoiceLogo
		sqlDB, dbErr := db.DB()
		if dbErr == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

func insertTopUpInvoiceUser(t *testing.T, db *gorm.DB, username string, role int) *model.User {
	t.Helper()

	user := &model.User{
		Username:    username,
		Password:    "password123",
		DisplayName: "Invoice User",
		Email:       username + "@example.com",
		Role:        role,
		Status:      common.UserStatusEnabled,
		Group:       "default",
		AffCode:     "aff_" + username,
	}
	require.NoError(t, db.Create(user).Error)
	return user
}

func insertTopUpInvoiceOrder(t *testing.T, db *gorm.DB, userID int, status string) *model.TopUp {
	t.Helper()

	topUp := &model.TopUp{
		UserId:          userID,
		Amount:          20,
		Money:           6.90,
		TradeNo:         "invoice_order_" + strconv.Itoa(userID) + "_" + status,
		GatewayTradeNo:  "gateway_order_" + strconv.Itoa(userID),
		CreditedQuota:   10_000_000,
		PaymentCurrency: "usd",
		PaymentMethod:   "alipay",
		PaymentProvider: model.PaymentProviderEpay,
		CreateTime:      1779196211,
		CompleteTime:    1779196311,
		Status:          status,
	}
	require.NoError(t, db.Create(topUp).Error)
	return topUp
}

func performTopUpInvoiceRequest(topUpID int, requester *model.User, download bool) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	target := "/api/user/topup/" + strconv.Itoa(topUpID) + "/invoice"
	if download {
		target += "?download=1"
	}
	ctx.Request = httptest.NewRequest(http.MethodGet, target, nil)
	ctx.Params = gin.Params{{Key: "id", Value: strconv.Itoa(topUpID)}}
	ctx.Set("id", requester.Id)
	ctx.Set("username", requester.Username)
	ctx.Set("role", requester.Role)
	GetTopUpInvoice(ctx)
	return recorder
}

func performTopUpInvoiceUpdate(t *testing.T, topUpID int, requester *model.User, details topUpInvoiceDetails) *httptest.ResponseRecorder {
	t.Helper()

	body, err := common.Marshal(details)
	require.NoError(t, err)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	target := "/api/user/topup/" + strconv.Itoa(topUpID) + "/invoice"
	ctx.Request = httptest.NewRequest(http.MethodPut, target, strings.NewReader(string(body)))
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Params = gin.Params{{Key: "id", Value: strconv.Itoa(topUpID)}}
	ctx.Set("id", requester.Id)
	ctx.Set("username", requester.Username)
	ctx.Set("role", requester.Role)
	UpdateTopUpInvoice(ctx)
	return recorder
}

func requireTopUpInvoiceAPIError(t *testing.T, recorder *httptest.ResponseRecorder, message string) {
	t.Helper()

	var payload struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &payload))
	assert.False(t, payload.Success)
	assert.Equal(t, message, payload.Message)
}

func TestGetTopUpInvoiceShowsCompletedOrderInlineForOwner(t *testing.T) {
	db := setupTopUpInvoiceTestDB(t)
	user := insertTopUpInvoiceUser(t, db, "invoice_owner", common.RoleCommonUser)
	topUp := insertTopUpInvoiceOrder(t, db, user.Id, common.TopUpStatusSuccess)

	recorder := performTopUpInvoiceRequest(topUp.Id, user, false)

	require.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Header().Get("Content-Type"), "text/html")
	assert.Contains(t, recorder.Header().Get("Content-Disposition"), "inline")
	assert.Equal(t, "private, no-store", recorder.Header().Get("Cache-Control"))
	assert.Contains(t, recorder.Header().Get("Content-Security-Policy"), "allow-downloads")
	body := recorder.Body.String()
	assert.Contains(t, body, "<h1>Top-up receipt</h1>")
	assert.Contains(t, body, "Receipt No.")
	assert.Contains(t, body, "Edit information")
	assert.Contains(t, body, "Download PDF")
	assert.Contains(t, body, `class="brand-icon"`)
	assert.Contains(t, body, "data:image/png;base64,")
	assert.Contains(t, body, user.DisplayName)
	assert.Contains(t, body, topUp.TradeNo)
	assert.Contains(t, body, topUp.GatewayTradeNo)
	assert.Contains(t, body, "Top-up Amount")
	assert.Contains(t, body, "10000000")
	assert.Contains(t, body, "USD 6.90")
	assert.Contains(t, body, user.Email)
}

func TestGetTopUpInvoiceReturnsDataForAuthenticatedPage(t *testing.T) {
	db := setupTopUpInvoiceTestDB(t)
	user := insertTopUpInvoiceUser(t, db, "invoice_page", common.RoleCommonUser)
	topUp := insertTopUpInvoiceOrder(t, db, user.Id, common.TopUpStatusSuccess)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/user/topup/"+strconv.Itoa(topUp.Id)+"/invoice?format=json", nil)
	ctx.Params = gin.Params{{Key: "id", Value: strconv.Itoa(topUp.Id)}}
	ctx.Set("id", user.Id)
	ctx.Set("username", user.Username)
	ctx.Set("role", user.Role)
	GetTopUpInvoice(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	var response struct {
		Success bool             `json:"success"`
		Data    topUpInvoiceView `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.True(t, response.Success)
	assert.True(t, response.Data.CanEdit)
	assert.Equal(t, formatTopUpReceiptNumber(topUp), response.Data.InvoiceNo)
	assert.Regexp(t, `^INV-202605-[A-Z2-7]{8}$`, response.Data.InvoiceNo)
	assert.Equal(t, user.DisplayName, response.Data.CustomerName)
	assert.Equal(t, topUp.TradeNo, response.Data.TradeNo)
}

func TestGetTopUpInvoiceDownloadsCompletedOrderWhenRequested(t *testing.T) {
	db := setupTopUpInvoiceTestDB(t)
	user := insertTopUpInvoiceUser(t, db, "invoice_download", common.RoleCommonUser)
	topUp := insertTopUpInvoiceOrder(t, db, user.Id, common.TopUpStatusSuccess)

	recorder := performTopUpInvoiceRequest(topUp.Id, user, true)

	require.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Header().Get("Content-Disposition"), "attachment")
	assert.Contains(t, recorder.Header().Get("Content-Disposition"), ".pdf")
	assert.Contains(t, recorder.Header().Get("Content-Type"), "application/pdf")
	assert.True(t, strings.HasPrefix(recorder.Body.String(), "%PDF-"))
	assert.Contains(t, recorder.Body.String(), "/Subtype /Image")
}

func TestTopUpInvoiceAllowsAuditedAdminViewAndUpdateOfAnotherUsersOrder(t *testing.T) {
	db := setupTopUpInvoiceTestDB(t)
	owner := insertTopUpInvoiceUser(t, db, "invoice_owner_private", common.RoleCommonUser)
	admin := insertTopUpInvoiceUser(t, db, "invoice_admin", common.RoleAdminUser)
	topUp := insertTopUpInvoiceOrder(t, db, owner.Id, common.TopUpStatusSuccess)

	recorder := performTopUpInvoiceRequest(topUp.Id, admin, false)

	require.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), topUp.TradeNo)
	assert.Contains(t, recorder.Body.String(), "Edit information")
	var auditLog model.AuditLog
	require.NoError(t, db.Where("user_id = ? AND action = ?", admin.Id, "topup.invoice_view").First(&auditLog).Error)
	assert.Contains(t, auditLog.Content, topUp.TradeNo)
	require.NotNil(t, auditLog.Other.Op)
	assert.Equal(t, "topup.invoice_view", auditLog.Other.Op.Action)

	updateRecorder := performTopUpInvoiceUpdate(t, topUp.Id, admin, topUpInvoiceDetails{Company: "Admin Updated"})
	require.Equal(t, http.StatusOK, updateRecorder.Code)
	var stored model.TopUp
	require.NoError(t, db.First(&stored, topUp.Id).Error)
	assert.Contains(t, stored.InvoiceDetails, "Admin Updated")
	var updateAuditLog model.AuditLog
	require.NoError(t, db.Where("user_id = ? AND action = ?", admin.Id, "topup.invoice_update").First(&updateAuditLog).Error)
	require.NotNil(t, updateAuditLog.Other.Op)
	assert.Equal(t, "topup.invoice_update", updateAuditLog.Other.Op.Action)
}

func TestGetTopUpInvoiceDoesNotAllowAnotherUserToViewOrder(t *testing.T) {
	db := setupTopUpInvoiceTestDB(t)
	owner := insertTopUpInvoiceUser(t, db, "invoice_other_owner", common.RoleCommonUser)
	requester := insertTopUpInvoiceUser(t, db, "invoice_other_requester", common.RoleCommonUser)
	topUp := insertTopUpInvoiceOrder(t, db, owner.Id, common.TopUpStatusSuccess)

	recorder := performTopUpInvoiceRequest(topUp.Id, requester, false)

	requireTopUpInvoiceAPIError(t, recorder, "充值订单不存在")
	assert.NotContains(t, recorder.Body.String(), topUp.TradeNo)
}

func TestGetTopUpInvoiceRejectsIncompleteOrder(t *testing.T) {
	db := setupTopUpInvoiceTestDB(t)
	user := insertTopUpInvoiceUser(t, db, "invoice_pending", common.RoleCommonUser)
	topUp := insertTopUpInvoiceOrder(t, db, user.Id, common.TopUpStatusPending)

	recorder := performTopUpInvoiceRequest(topUp.Id, user, false)

	requireTopUpInvoiceAPIError(t, recorder, "仅成功订单支持下载凭证")
	assert.NotContains(t, recorder.Body.String(), "<h1>Top-up receipt</h1>")
}

func TestGetTopUpInvoiceRejectsWaffoPancakeOrder(t *testing.T) {
	db := setupTopUpInvoiceTestDB(t)
	user := insertTopUpInvoiceUser(t, db, "invoice_waffo_pancake", common.RoleCommonUser)
	topUp := insertTopUpInvoiceOrder(t, db, user.Id, common.TopUpStatusSuccess)
	topUp.PaymentMethod = model.PaymentProviderWaffoPancake
	topUp.PaymentProvider = model.PaymentProviderWaffoPancake
	require.NoError(t, db.Save(topUp).Error)

	recorder := performTopUpInvoiceRequest(topUp.Id, user, false)

	requireTopUpInvoiceAPIError(t, recorder, "Waffo Pancake 订单仅支持官方发票")
	updateRecorder := performTopUpInvoiceUpdate(t, topUp.Id, user, topUpInvoiceDetails{Name: "Customer"})
	requireTopUpInvoiceAPIError(t, updateRecorder, "Waffo Pancake orders use the official provider invoice")
}

func TestUpdateTopUpInvoicePersistsEditableCustomerInformation(t *testing.T) {
	db := setupTopUpInvoiceTestDB(t)
	user := insertTopUpInvoiceUser(t, db, "invoice_edit", common.RoleCommonUser)
	topUp := insertTopUpInvoiceOrder(t, db, user.Id, common.TopUpStatusSuccess)
	details := topUpInvoiceDetails{
		Name:       "  Billing Contact  ",
		Email:      "billing@example.com",
		Company:    "Example & Partners",
		TaxID:      "VAT-123",
		Address:    "1 Main Street",
		City:       "Taipei",
		State:      "Taiwan",
		PostalCode: "100",
		Country:    "Taiwan",
	}

	recorder := performTopUpInvoiceUpdate(t, topUp.Id, user, details)

	require.Equal(t, http.StatusOK, recorder.Code)
	var response struct {
		Success bool `json:"success"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.True(t, response.Success)
	var stored model.TopUp
	require.NoError(t, db.First(&stored, topUp.Id).Error)
	var storedDetails topUpInvoiceDetails
	require.NoError(t, common.UnmarshalJsonStr(stored.InvoiceDetails, &storedDetails))
	assert.Equal(t, "Billing Contact", storedDetails.Name)
	assert.Equal(t, details.Company, storedDetails.Company)

	invoiceRecorder := performTopUpInvoiceRequest(topUp.Id, user, false)
	require.Equal(t, http.StatusOK, invoiceRecorder.Code)
	html := invoiceRecorder.Body.String()
	assert.Contains(t, html, "Billing Contact")
	assert.Contains(t, html, "Example &amp; Partners")
	assert.Contains(t, html, "1 Main Street, Taipei, Taiwan, 100, Taiwan")
}

func TestUpdateTopUpInvoiceRejectsForeignAndInvalidChanges(t *testing.T) {
	db := setupTopUpInvoiceTestDB(t)
	owner := insertTopUpInvoiceUser(t, db, "invoice_update_owner", common.RoleCommonUser)
	other := insertTopUpInvoiceUser(t, db, "invoice_update_other", common.RoleCommonUser)
	topUp := insertTopUpInvoiceOrder(t, db, owner.Id, common.TopUpStatusSuccess)

	foreignRecorder := performTopUpInvoiceUpdate(t, topUp.Id, other, topUpInvoiceDetails{Name: "Other"})
	requireTopUpInvoiceAPIError(t, foreignRecorder, "Top-up order not found")
	invalidRecorder := performTopUpInvoiceUpdate(t, topUp.Id, owner, topUpInvoiceDetails{Email: "not-an-email"})
	requireTopUpInvoiceAPIError(t, invalidRecorder, "Email is invalid")
	controlRecorder := performTopUpInvoiceUpdate(t, topUp.Id, owner, topUpInvoiceDetails{Company: "Example\nLtd"})
	requireTopUpInvoiceAPIError(t, controlRecorder, "Company name contains unsupported characters")
	longRecorder := performTopUpInvoiceUpdate(t, topUp.Id, owner, topUpInvoiceDetails{Name: strings.Repeat("x", 121)})
	requireTopUpInvoiceAPIError(t, longRecorder, "Name is too long")
	longAddressRecorder := performTopUpInvoiceUpdate(t, topUp.Id, owner, topUpInvoiceDetails{
		Address: strings.Repeat("a", 240),
		City:    strings.Repeat("b", 60),
	})
	requireTopUpInvoiceAPIError(t, longAddressRecorder, "Billing address is too long")
}

func TestRenderTopUpInvoiceEscapesStoredCustomerContent(t *testing.T) {
	topUp := &model.TopUp{Id: 1, UserId: 2, Status: common.TopUpStatusSuccess, TradeNo: "safe-order"}
	user := &model.User{DisplayName: `<script>alert("x")</script>`, Email: "safe@example.com"}

	htmlBytes, err := renderTopUpInvoice(topUp, user)

	require.NoError(t, err)
	html := string(htmlBytes)
	assert.NotContains(t, html, `<script>alert("x")</script>`)
	assert.True(t, strings.Contains(html, "&lt;script&gt;") || strings.Contains(html, "&lt;script"))
}

func TestRenderTopUpInvoicePDFPreservesChineseCustomerName(t *testing.T) {
	topUp := &model.TopUp{Id: 1, UserId: 2, Status: common.TopUpStatusSuccess, TradeNo: "chinese-name"}
	user := &model.User{DisplayName: "张三", Email: "zhangsan@example.com"}

	pdfBytes, err := renderTopUpInvoicePDF(topUp, user)

	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(string(pdfBytes), "%PDF-"))
	assert.Equal(t, "张三", pdfSafeText(user.DisplayName))
}
