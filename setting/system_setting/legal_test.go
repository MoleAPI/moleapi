package system_setting

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLegalSettingsUseTranslatedDefaults(t *testing.T) {
	settings := GetLegalSettings()

	require.NotNil(t, settings)
	assert.Equal(t, "builtin://user-agreement", settings.UserAgreement)
	assert.Equal(t, "builtin://privacy-policy", settings.PrivacyPolicy)
}
