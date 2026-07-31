package controller

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestTransferAffQuotaHidesDatabaseErrors(t *testing.T) {
	db := setupManageUserTestDB(t)
	payment := operation_setting.GetPaymentSetting()
	originalCompliance, originalVersion := payment.ComplianceConfirmed, payment.ComplianceTermsVersion
	originalQuotaPerUnit := common.QuotaPerUnit
	payment.ComplianceConfirmed = true
	payment.ComplianceTermsVersion = operation_setting.CurrentComplianceTermsVersion
	common.QuotaPerUnit = 1
	t.Cleanup(func() {
		payment.ComplianceConfirmed, payment.ComplianceTermsVersion = originalCompliance, originalVersion
		common.QuotaPerUnit = originalQuotaPerUnit
	})

	user := model.User{
		Username: "affiliate-transfer-user", Password: "password", Role: common.RoleCommonUser,
		Status: common.UserStatusEnabled, Group: "default", AffQuota: 100,
	}
	require.NoError(t, db.Create(&user).Error)
	require.NoError(t, db.Callback().Update().Before("gorm:update").Register("test:fail_affiliate_transfer", func(tx *gorm.DB) {
		if tx.Statement != nil && tx.Statement.Table == "users" {
			tx.AddError(errors.New("sensitive transfer database detail"))
		}
	}))
	t.Cleanup(func() {
		db.Callback().Update().Remove("test:fail_affiliate_transfer")
	})

	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodPost, "/api/user/aff_transfer", strings.NewReader(`{"quota":10}`))
	context.Request.Header.Set("Content-Type", "application/json")
	context.Set("id", user.Id)

	TransferAffQuota(context)

	assert.Contains(t, recorder.Body.String(), `"success":false`)
	assert.Contains(t, recorder.Body.String(), "common.operation_failed")
	assert.NotContains(t, recorder.Body.String(), "sensitive transfer database detail")
}
