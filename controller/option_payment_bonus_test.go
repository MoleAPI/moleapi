package controller

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestUpdateOptionRejectsUnsafeTopupBonus(t *testing.T) {
	response := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(response)
	context.Request = httptest.NewRequest(
		http.MethodPut,
		"/api/option/",
		strings.NewReader(`{"key":"payment_setting.amount_bonus","value":"{\"10\":1.01}"}`),
	)

	UpdateOption(context)

	assert.Equal(t, http.StatusOK, response.Code)
	assert.JSONEq(t, `{"success":false,"message":"充值赠额配置无效：档位不能为负数，赠额比例必须在 0 到 1 之间"}`, response.Body.String())
}

func TestUpdateOptionRejectsOverflowingFirstTopupReward(t *testing.T) {
	response := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(response)
	context.Request = httptest.NewRequest(
		http.MethodPut,
		"/api/option/",
		strings.NewReader(`{"key":"QuotaForInviterOnFirstTopup","value":2147483648}`),
	)

	UpdateOption(context)

	assert.Equal(t, http.StatusOK, response.Code)
	assert.JSONEq(t, `{"success":false,"message":"首充邀请奖励额度无效"}`, response.Body.String())
}
