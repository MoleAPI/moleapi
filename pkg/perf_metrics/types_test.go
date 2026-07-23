package perfmetrics

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestModelSummaryJSONIncludesRequestCount(t *testing.T) {
	data, err := common.Marshal(ModelSummary{
		ModelName:    "gpt-4o",
		RequestCount: 42,
	})

	require.NoError(t, err)
	assert.Contains(t, string(data), `"request_count":42`)
}
