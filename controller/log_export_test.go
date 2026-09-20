package controller

import (
	"context"
	"encoding/csv"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLogExportDatabaseMatrix(t *testing.T) {
	for _, kind := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(kind, func(t *testing.T) {
			dsn := os.Getenv("TEST_" + strings.ToUpper(kind) + "_DSN")
			if kind != "sqlite" && dsn == "" {
				t.Skip("test database DSN is not configured")
			}
			db, _ := newAuditTestDatabase(t, kind, dsn)
			logDB, _ := newAuditTestDatabase(t, kind, dsn)
			previous, previousLogs := model.DB, model.LOG_DB
			model.DB, model.LOG_DB = db, logDB
			t.Cleanup(func() { model.DB, model.LOG_DB = previous, previousLogs })
			var version string
			versionSQL := "SELECT version()"
			if kind == "sqlite" {
				versionSQL = "SELECT sqlite_version()"
			}
			require.NoError(t, db.Raw(versionSQL).Scan(&version).Error)
			t.Log("database version:", version)
			// Representative existing wallet/log data predates the new allowance table.
			require.NoError(t, db.AutoMigrate(&model.TopUp{}))
			require.NoError(t, logDB.AutoMigrate(&model.Log{}))
			require.NoError(t, db.Create(&model.TopUp{UserId: 7, TradeNo: "old-paid-order", CreateTime: 100, Money: 12.34}).Error)
			for _, row := range []model.Log{
				{UserId: 7, CreatedAt: 99, Content: "before"},
				{UserId: 7, CreatedAt: 100, Content: "=SUM(1,2)", ModelName: "  =formula", Other: `{"admin_info":{"secret":"never-export"}}`},
				{UserId: 7, CreatedAt: 200, Content: "line 1\nline 2"},
				{UserId: 7, CreatedAt: 201, Content: "after"},
				{UserId: 8, CreatedAt: 150, Content: "foreign-user"},
			} {
				require.NoError(t, logDB.Create(&row).Error)
			}
			for i := 0; i < 2; i++ {
				require.NoError(t, db.AutoMigrate(&model.LogExportAllowance{}))
			}
			var order model.TopUp
			require.NoError(t, db.First(&order).Error)
			assert.Equal(t, 12.34, order.Money)
			orders, total, err := model.SearchUserTopUpsWithParams(7, model.TopUpSearchParams{StartTimestamp: 100, EndTimestamp: 100}, &common.PageInfo{Page: 1, PageSize: 10})
			require.NoError(t, err)
			assert.EqualValues(t, 1, total)
			require.Len(t, orders, 1)
			request := func(user, role int, body string) *httptest.ResponseRecorder {
				w := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(w)
				c.Set("id", user)
				c.Set("role", role)
				c.Request = httptest.NewRequest("POST", "/api/log/export", strings.NewReader(body))
				c.Request.Header.Set("Content-Type", "application/json")
				ExportLogs(c)
				return w
			}
			body := `{"start_timestamp":100,"end_timestamp":200}`
			response := request(7, common.RoleCommonUser, body)
			require.Equal(t, 200, response.Code, response.Body.String())
			var csvText strings.Builder
			var event struct {
				CSV          string `json:"csv"`
				Count, Bytes int
				Done         bool
				Error        string
			}
			for _, line := range strings.Split(strings.TrimSpace(response.Body.String()), "\n") {
				require.NoError(t, common.UnmarshalJsonStr(line, &event))
				csvText.WriteString(event.CSV)
			}
			require.True(t, event.Done)
			assert.Empty(t, event.Error)
			assert.Equal(t, 2, event.Count)
			assert.Equal(t, csvText.Len(), event.Bytes)
			records, err := csv.NewReader(strings.NewReader(strings.TrimPrefix(csvText.String(), "\xef\xbb\xbf"))).ReadAll()
			require.NoError(t, err)
			require.Len(t, records, 3)
			assert.Equal(t, "'=SUM(1,2)", records[1][11])
			assert.Equal(t, "'  =formula", records[1][2])
			assert.Equal(t, "line 1\nline 2", records[2][11])
			for _, excluded := range []string{"before", "after", "foreign-user", "never-export", "channel_id", "user_id"} {
				assert.NotContains(t, csvText.String(), excluded)
			}
			for _, invalid := range []string{`{}`, `{"start_timestamp":200,"end_timestamp":100}`, `{"start_timestamp":-1,"end_timestamp":200}`} {
				assert.Equal(t, 400, request(7, 1, invalid).Code)
			}
			assert.Equal(t, 403, request(7, 1, `{"start_timestamp":100,"end_timestamp":200,"all_users":true}`).Code)
			assert.Equal(t, 200, request(7, 1, body).Code)
			// Re-migration/reopening the application does not reset the persisted allowance.
			require.NoError(t, db.AutoMigrate(&model.LogExportAllowance{}))
			assert.Equal(t, 200, request(7, 1, body).Code)
			assert.Equal(t, http.StatusTooManyRequests, request(7, 1, body).Code)
			admin := request(9, common.RoleAdminUser, `{"start_timestamp":100,"end_timestamp":200,"all_users":true}`)
			assert.Equal(t, 200, admin.Code)
			assert.Contains(t, admin.Body.String(), "foreign-user")
			assert.Contains(t, admin.Body.String(), "channel_id")
			empty := request(8, 1, `{"start_timestamp":300,"end_timestamp":400}`)
			assert.Contains(t, empty.Body.String(), `"count":0`)
			assert.Contains(t, empty.Body.String(), `"done":true`)
			require.NoError(t, logDB.Create(&model.Log{UserId: 8, CreatedAt: 300, Type: model.LogTypeSystem, Content: "邀请好友充值返利 1元，订单号 private-order"}).Error)
			private := request(8, 1, `{"start_timestamp":300,"end_timestamp":400}`)
			assert.Contains(t, private.Body.String(), "邀请好友充值返利 1元")
			assert.NotContains(t, private.Body.String(), "private-order")
			// A simultaneous fourth request cannot pass, including on SQLite.
			var wg sync.WaitGroup
			outcomes := make(chan error, 4)
			now := time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)
			for i := 0; i < 4; i++ {
				wg.Add(1)
				go func() { defer wg.Done(); outcomes <- model.ReserveLogExport(context.Background(), 77, now) }()
			}
			wg.Wait()
			close(outcomes)
			successes := 0
			limited := 0
			for err := range outcomes {
				if err == nil {
					successes++
				} else if err == model.ErrLogExportLimit {
					limited++
				} else {
					t.Fatal(err)
				}
			}
			assert.Equal(t, 3, successes)
			assert.Equal(t, 1, limited)
			require.NoError(t, model.ReserveLogExport(context.Background(), 77, now.Add(24*time.Hour)))
			var allowance model.LogExportAllowance
			require.NoError(t, db.First(&allowance, "user_id = ?", 77).Error)
			assert.Equal(t, 1, allowance.Attempts)
			t.Log(fmt.Sprintf("range, privacy, CSV, migration, concurrency and UTC reset verified on %s", kind))
		})
	}
}
