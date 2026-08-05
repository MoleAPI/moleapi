package controller

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAuditContentENHasFallbacksForStructuredManagementActions(t *testing.T) {
	tests := []struct {
		action string
		params map[string]interface{}
		want   string
	}{
		{
			action: "option.model_pricing.import",
			params: map[string]interface{}{"updated_options": 4},
			want:   "Imported 4 model pricing settings",
		},
		{
			action: "channel.status_update",
			params: map[string]interface{}{"id": 7, "status": 1},
			want:   "Updated channel 7 status to 1",
		},
		{
			action: "channel.status_update_batch",
			params: map[string]interface{}{"count": 3, "total": 5, "status": 2},
			want:   "Updated 3 of 5 channels to 2",
		},
		{
			action: "channel.upstream_detect_all",
			params: map[string]interface{}{"task_id": "task-1"},
			want:   "Started upstream model update detection task task-1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.action, func(t *testing.T) {
			assert.Equal(t, tt.want, auditContentEN(tt.action, tt.params))
		})
	}
}
