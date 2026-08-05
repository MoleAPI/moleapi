package model

import (
	"testing"

	"github.com/QuantumNous/new-api/setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateOptionValueRejectsInvalidMaxTokenAutoGroups(t *testing.T) {
	for _, value := range []string{"", "0", "-1", "1.5", "invalid"} {
		t.Run(value, func(t *testing.T) {
			assert.Error(t, validateOptionValue("MaxTokenAutoGroups", value))
		})
	}
	require.NoError(t, validateOptionValue("MaxTokenAutoGroups", "999999"))
}

func TestUpdateOptionRejectsInvalidAutoGroupsBeforePersisting(t *testing.T) {
	db := useFrontendOptionMigrationDB(t)
	original := setting.AutoGroups2JsonString()
	t.Cleanup(func() {
		require.NoError(t, setting.UpdateAutoGroupsByJsonString(original))
	})

	require.Error(t, UpdateOption("AutoGroups", `{"invalid":true}`))
	requireOptionMissing(t, db, "AutoGroups")
	assert.JSONEq(t, original, setting.AutoGroups2JsonString())
}
