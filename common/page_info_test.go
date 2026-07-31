package common

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestGetPageQueryClampsInvalidPageSize(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name string
		raw  string
		want int
	}{
		{name: "negative page size uses default", raw: "/?page_size=-1", want: ItemsPerPage},
		{name: "negative fallback uses default", raw: "/?page_size=0&ps=-5&size=-10", want: ItemsPerPage},
		{name: "positive fallback still works", raw: "/?ps=12", want: 12},
		{name: "large page size caps at 100", raw: "/?page_size=101", want: 100},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest("GET", tt.raw, nil)

			pageInfo := GetPageQuery(c)

			assert.Equal(t, tt.want, pageInfo.PageSize)
		})
	}
}
