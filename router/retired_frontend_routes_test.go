package router

import (
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestRetiredFrontendAPIRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	SetApiRouter(engine)

	routes := make(map[string]struct{}, len(engine.Routes()))
	for _, route := range engine.Routes() {
		routes[route.Method+" "+route.Path] = struct{}{}
	}
	_, hasAsyncCleanup := routes[http.MethodPost+" /api/system-task/log-cleanup"]
	_, hasDirectDelete := routes[http.MethodDelete+" /api/log/"]
	_, hasConsoleMigration := routes[http.MethodPost+" /api/option/migrate_console_setting"]
	_, hasSecurePancakeCatalog := routes[http.MethodPost+" /api/option/waffo-pancake/catalog"]
	_, hasLeakyPancakeCatalog := routes[http.MethodGet+" /api/option/waffo-pancake/catalog"]
	assert.True(t, hasAsyncCleanup)
	assert.False(t, hasDirectDelete)
	assert.False(t, hasConsoleMigration)
	assert.True(t, hasSecurePancakeCatalog)
	assert.False(t, hasLeakyPancakeCatalog)
}
