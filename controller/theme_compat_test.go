package controller

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUpdateOptionRejectsRetiredFrontendTheme(t *testing.T) {
	response := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(response)
	context.Request = httptest.NewRequest(
		http.MethodPut,
		"/api/option/",
		strings.NewReader(`{"key":"theme.frontend","value":"classic"}`),
	)

	UpdateOption(context)

	assert.Equal(t, http.StatusOK, response.Code)
	assert.JSONEq(t, `{"success":false,"message":"Classic 前端已移除，主题只能设置为 default"}`, response.Body.String())
}

func TestGetStatusAdvertisesDefaultDashboard(t *testing.T) {
	previousMap := common.OptionMap
	previousVersion := common.Version
	previousUpstreamVersion := common.UpstreamVersion
	previousCommit := common.Commit
	common.OptionMap = map[string]string{}
	common.Version = "v0.10.0-dev1"
	common.UpstreamVersion = "v1.0.0-rc.21"
	common.Commit = "abc1234"
	t.Cleanup(func() {
		common.OptionMap = previousMap
		common.Version = previousVersion
		common.UpstreamVersion = previousUpstreamVersion
		common.Commit = previousCommit
	})
	response := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(response)
	context.Request = httptest.NewRequest(http.MethodGet, "/api/status", nil)

	GetStatus(context)

	var payload struct {
		Success bool           `json:"success"`
		Data    map[string]any `json:"data"`
	}
	require.NoError(t, common.Unmarshal(response.Body.Bytes(), &payload))
	assert.True(t, payload.Success)
	assert.Equal(t, "default", payload.Data["theme"])
	assert.Equal(t, "v1.0.0-rc.21", payload.Data["version"])
	assert.Equal(t, "v1.0.0-rc.21", payload.Data["upstream_version"])
	assert.Equal(t, "v0.10.0-dev1", payload.Data["project_version"])
	assert.Equal(t, "abc1234", payload.Data["commit"])
	assert.Equal(t, "abc1234", payload.Data["project_commit"])
}
