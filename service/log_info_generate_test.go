package service

import (
	"errors"
	"testing"

	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAppendStreamStatusKeepsErrorDetailsAdminOnly(t *testing.T) {
	status := relaycommon.NewStreamStatus()
	status.SetEndReason(relaycommon.StreamEndReasonScannerErr, errors.New("private end error"))
	status.RecordError("private upstream error")
	info := &relaycommon.RelayInfo{IsStream: true, StreamStatus: status}
	other := model.NewLogOther()

	appendStreamStatus(info, other)
	stored := other.Snapshot()

	streamStatus, ok := stored["stream_status"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "error", streamStatus["status"])
	assert.Equal(t, 1, streamStatus["error_count"])
	assert.NotContains(t, streamStatus, "end_error")
	assert.NotContains(t, streamStatus, "errors")
	adminInfo, ok := stored["admin_info"].(map[string]interface{})
	require.True(t, ok)
	streamError, ok := adminInfo["stream_error"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "private end error", streamError["end_error"])
	assert.Equal(t, []string{"private upstream error"}, streamError["errors"])
}
