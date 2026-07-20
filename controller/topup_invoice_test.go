package controller

import (
	"net/http"
	"net/http/httptest"
	"net/url"
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
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	common.RedisEnabled = false

	dsn := "file:" + url.QueryEscape(t.Name()) + "?mode=memory&cache=shared"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	model.LOG_DB = db
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.TopUp{}))

	t.Cleanup(func() {
		model.DB = originalDB
		model.LOG_DB = originalLogDB
		common.SetDatabaseTypes(originalMainDatabaseType, originalLogDatabaseType)
		common.RedisEnabled = originalRedisEnabled
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

func performTopUpInvoiceRequest(topUpID int, requesterID int) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	target := "/api/user/topup/" + strconv.Itoa(topUpID) + "/invoice"
	ctx.Request = httptest.NewRequest(http.MethodGet, target, nil)
	ctx.Params = gin.Params{{Key: "id", Value: strconv.Itoa(topUpID)}}
	ctx.Set("id", requesterID)
	GetTopUpInvoice(ctx)
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

func TestGetTopUpInvoiceDownloadsCompletedOrderForOwner(t *testing.T) {
	db := setupTopUpInvoiceTestDB(t)
	user := insertTopUpInvoiceUser(t, db, "invoice_owner", common.RoleCommonUser)
	topUp := insertTopUpInvoiceOrder(t, db, user.Id, common.TopUpStatusSuccess)

	recorder := performTopUpInvoiceRequest(topUp.Id, user.Id)

	require.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Header().Get("Content-Type"), "text/html")
	assert.Contains(t, recorder.Header().Get("Content-Disposition"), "attachment")
	assert.Equal(t, "private, no-store", recorder.Header().Get("Cache-Control"))
	body := recorder.Body.String()
	assert.Contains(t, body, "Top-up Invoice")
	assert.Contains(t, body, topUp.TradeNo)
	assert.Contains(t, body, topUp.GatewayTradeNo)
	assert.Contains(t, body, "10000000")
	assert.Contains(t, body, "USD 6.90")
	assert.Contains(t, body, user.Email)
}

func TestGetTopUpInvoiceDoesNotAllowAdminToDownloadAnotherUsersOrder(t *testing.T) {
	db := setupTopUpInvoiceTestDB(t)
	owner := insertTopUpInvoiceUser(t, db, "invoice_owner_private", common.RoleCommonUser)
	admin := insertTopUpInvoiceUser(t, db, "invoice_admin", common.RoleAdminUser)
	topUp := insertTopUpInvoiceOrder(t, db, owner.Id, common.TopUpStatusSuccess)

	recorder := performTopUpInvoiceRequest(topUp.Id, admin.Id)

	requireTopUpInvoiceAPIError(t, recorder, "充值订单不存在")
	assert.NotContains(t, recorder.Body.String(), topUp.TradeNo)
}

func TestGetTopUpInvoiceRejectsIncompleteOrder(t *testing.T) {
	db := setupTopUpInvoiceTestDB(t)
	user := insertTopUpInvoiceUser(t, db, "invoice_pending", common.RoleCommonUser)
	topUp := insertTopUpInvoiceOrder(t, db, user.Id, common.TopUpStatusPending)

	recorder := performTopUpInvoiceRequest(topUp.Id, user.Id)

	requireTopUpInvoiceAPIError(t, recorder, "仅成功订单支持下载凭证")
	assert.NotContains(t, recorder.Body.String(), "Top-up Invoice")
}

func TestRenderTopUpInvoiceEscapesStoredCustomerContent(t *testing.T) {
	topUp := &model.TopUp{Id: 1, UserId: 2, Status: common.TopUpStatusSuccess, TradeNo: "safe-order"}
	user := &model.User{DisplayName: `<script>alert("x")</script>`, Email: "safe@example.com"}

	htmlBytes, err := renderTopUpInvoice(topUp, user)

	require.NoError(t, err)
	html := string(htmlBytes)
	assert.NotContains(t, html, "<script>")
	assert.True(t, strings.Contains(html, "&lt;script&gt;") || strings.Contains(html, "&lt;script"))
}
