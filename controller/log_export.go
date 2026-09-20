package controller

import (
	"bytes"
	"context"
	"encoding/csv"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

// ExportLogs streams a single database query, avoiding offset pagination drift.
// Only the explicit date range and self/all scope apply; table filters do not.
func ExportLogs(c *gin.Context) {
	var request struct {
		Start    int64 `json:"start_timestamp"`
		End      int64 `json:"end_timestamp"`
		AllUsers bool  `json:"all_users"`
	}
	if err := c.ShouldBindJSON(&request); err != nil || request.Start <= 0 || request.End < request.Start || request.End > time.Now().Unix() {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid date range"})
		return
	}
	if request.AllUsers && c.GetInt("role") < common.RoleAdminUser {
		c.JSON(http.StatusForbidden, gin.H{"success": false, "message": "Admin access required"})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Minute)
	defer cancel()
	if err := model.ReserveLogExport(ctx, c.GetInt("id"), time.Now()); err != nil {
		if errors.Is(err, model.ErrLogExportLimit) {
			c.JSON(http.StatusTooManyRequests, gin.H{"success": false, "message": err.Error()})
			return
		}
		c.JSON(http.StatusServiceUnavailable, gin.H{"success": false, "message": "Unable to start export"})
		return
	}
	// No Other metadata, prompts, credentials, or internal diagnostics are exported.
	query := model.LOG_DB.WithContext(ctx).Model(&model.Log{}).
		Select([]string{"user_id", "username", "created_at", "type", "content", "token_name", "model_name", "quota", "prompt_tokens", "completion_tokens", "use_time", "is_stream", "channel_id", "group", "request_id"}).
		Where("created_at >= ? AND created_at <= ?", request.Start, request.End).
		Order("created_at ASC, id ASC")
	if !request.AllUsers {
		query = query.Where("user_id = ?", c.GetInt("id"))
	}
	rows, err := query.Rows()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Unable to read logs"})
		return
	}
	defer rows.Close()
	c.Header("Content-Type", "application/x-ndjson; charset=utf-8")
	c.Header("Cache-Control", "no-store")
	c.Header("X-Accel-Buffering", "no")
	var count, size int
	emit := func(csvText string, done bool, message string) error {
		payload, err := common.Marshal(gin.H{"csv": csvText, "count": count, "bytes": size, "done": done, "error": message})
		if err != nil {
			return err
		}
		_, err = c.Writer.Write(append(payload, '\n'))
		c.Writer.Flush()
		return err
	}
	var batch bytes.Buffer
	batch.WriteString("\xef\xbb\xbf")
	writer := csv.NewWriter(&batch)
	header := []string{"time_utc", "type", "model", "token_name", "input_tokens", "output_tokens", "quota_units", "duration_seconds", "stream", "group", "request_id", "content"}
	if request.AllUsers {
		header = append(header, "user_id", "username", "channel_id")
	}
	_ = writer.Write(header)
	writer.Flush()
	for rows.Next() {
		var entry model.Log
		if err = model.LOG_DB.ScanRows(rows, &entry); err != nil {
			break
		}
		if !request.AllUsers && entry.Type == model.LogTypeSystem && strings.HasPrefix(entry.Content, "邀请好友充值返利 ") {
			entry.Content, _, _ = strings.Cut(entry.Content, "，订单号 ")
		}
		record := []string{time.Unix(entry.CreatedAt, 0).UTC().Format(time.RFC3339), strconv.Itoa(entry.Type), entry.ModelName, entry.TokenName, strconv.Itoa(entry.PromptTokens), strconv.Itoa(entry.CompletionTokens), strconv.Itoa(entry.Quota), strconv.Itoa(entry.UseTime), strconv.FormatBool(entry.IsStream), entry.Group, entry.RequestId, entry.Content}
		if request.AllUsers {
			record = append(record, strconv.Itoa(entry.UserId), entry.Username, strconv.Itoa(entry.ChannelId))
		}
		for i, value := range record {
			trimmed := strings.TrimLeft(value, " ")
			if len(trimmed) > 0 && strings.ContainsRune("=+-@\t\r\n", rune(trimmed[0])) {
				record[i] = "'" + value
			}
		}
		_ = writer.Write(record)
		writer.Flush()
		count++
		// ponytail: cap the browser download at 50 MiB; use background file storage
		// if larger exports are needed. Never label a truncated download complete.
		if size+batch.Len() > 50*1024*1024 {
			err = errors.New("Export exceeds 50 MiB; select a shorter range")
			break
		}
		if count%500 == 0 {
			size += batch.Len()
			if emit(batch.String(), false, "") != nil {
				return
			}
			batch.Reset()
		}
	}
	if err == nil {
		err = rows.Err()
	}
	if err != nil {
		_ = emit("", false, "Export interrupted; select a shorter range and retry")
		return
	}
	size += batch.Len()
	_ = emit(batch.String(), true, "")
}
