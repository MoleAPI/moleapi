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
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupTopUpInvoiceTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	gin.SetMode(gin.TestMode)
	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	common.RedisEnabled = false

	dsn := "file:" + url.QueryEscape(t.Name()) + "?mode=memory&cache=shared"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)

	model.DB = db
	model.LOG_DB = db
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.TopUp{}))

	t.Cleanup(func() {
		sqlDB, err := db.DB()
		if err == nil {
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
		PaymentMethod:   "alipay",
		PaymentProvider: model.PaymentProviderEpay,
		CreateTime:      1779196211,
		CompleteTime:    1779196311,
		Status:          status,
	}
	require.NoError(t, db.Create(topUp).Error)
	return topUp
}

func performTopUpInvoiceRequest(topUpID int, requesterID int, requesterRole int, query string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	target := "/api/user/topup/" + strconv.Itoa(topUpID) + "/invoice" + query
	ctx.Request = httptest.NewRequest(http.MethodGet, target, nil)
	ctx.Params = gin.Params{{Key: "id", Value: strconv.Itoa(topUpID)}}
	ctx.Set("id", requesterID)
	ctx.Set("role", requesterRole)

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
	require.False(t, payload.Success)
	require.Equal(t, message, payload.Message)
}

func TestGetTopUpInvoiceAllowsOwnerForSuccessOrder(t *testing.T) {
	db := setupTopUpInvoiceTestDB(t)
	user := insertTopUpInvoiceUser(t, db, "invoice_owner", common.RoleCommonUser)
	topUp := insertTopUpInvoiceOrder(t, db, user.Id, common.TopUpStatusSuccess)

	recorder := performTopUpInvoiceRequest(topUp.Id, user.Id, common.RoleCommonUser, "")

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Contains(t, recorder.Header().Get("Content-Type"), "text/html")
	require.Contains(t, recorder.Header().Get("Content-Disposition"), "inline")
	body := recorder.Body.String()
	require.Contains(t, body, "Top-up Invoice")
	require.Contains(t, body, topUp.TradeNo)
	require.Contains(t, body, topUp.GatewayTradeNo)
	require.Contains(t, body, user.Email)
}

func TestGetTopUpInvoiceSupportsDownloadDisposition(t *testing.T) {
	db := setupTopUpInvoiceTestDB(t)
	user := insertTopUpInvoiceUser(t, db, "invoice_download", common.RoleCommonUser)
	topUp := insertTopUpInvoiceOrder(t, db, user.Id, common.TopUpStatusSuccess)

	recorder := performTopUpInvoiceRequest(topUp.Id, user.Id, common.RoleCommonUser, "?download=1")

	require.Equal(t, http.StatusOK, recorder.Code)
	disposition := recorder.Header().Get("Content-Disposition")
	require.Contains(t, disposition, "attachment")
	require.Contains(t, disposition, "invoice-"+topUp.TradeNo+".html")
}

func TestGetTopUpInvoiceRejectsOtherUsersOrder(t *testing.T) {
	db := setupTopUpInvoiceTestDB(t)
	owner := insertTopUpInvoiceUser(t, db, "invoice_owner_private", common.RoleCommonUser)
	other := insertTopUpInvoiceUser(t, db, "invoice_other", common.RoleCommonUser)
	topUp := insertTopUpInvoiceOrder(t, db, owner.Id, common.TopUpStatusSuccess)

	recorder := performTopUpInvoiceRequest(topUp.Id, other.Id, common.RoleCommonUser, "")

	require.Equal(t, http.StatusOK, recorder.Code)
	requireTopUpInvoiceAPIError(t, recorder, "无权查看该订单")
}

func TestGetTopUpInvoiceAllowsAdminForOtherUsersOrder(t *testing.T) {
	db := setupTopUpInvoiceTestDB(t)
	owner := insertTopUpInvoiceUser(t, db, "invoice_owner_admin", common.RoleCommonUser)
	admin := insertTopUpInvoiceUser(t, db, "invoice_admin", common.RoleAdminUser)
	topUp := insertTopUpInvoiceOrder(t, db, owner.Id, common.TopUpStatusSuccess)

	recorder := performTopUpInvoiceRequest(topUp.Id, admin.Id, common.RoleAdminUser, "")

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Contains(t, recorder.Header().Get("Content-Type"), "text/html")
	require.Contains(t, recorder.Body.String(), topUp.TradeNo)
}

func TestGetTopUpInvoiceRejectsPendingOrder(t *testing.T) {
	db := setupTopUpInvoiceTestDB(t)
	user := insertTopUpInvoiceUser(t, db, "invoice_pending", common.RoleCommonUser)
	topUp := insertTopUpInvoiceOrder(t, db, user.Id, common.TopUpStatusPending)

	recorder := performTopUpInvoiceRequest(topUp.Id, user.Id, common.RoleCommonUser, "")

	require.Equal(t, http.StatusOK, recorder.Code)
	requireTopUpInvoiceAPIError(t, recorder, "仅成功订单支持查看 Invoice")
	require.False(t, strings.Contains(recorder.Body.String(), "Top-up Invoice"))
}
