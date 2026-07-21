package system_setting

import "github.com/QuantumNous/new-api/setting/config"

type LegalSettings struct {
	UserAgreement string `json:"user_agreement"`
	PrivacyPolicy string `json:"privacy_policy"`
}

const (
	builtInUserAgreement = "builtin://user-agreement"
	builtInPrivacyPolicy = "builtin://privacy-policy"
)

var defaultLegalSettings = LegalSettings{
	// ponytail: reserved values keep the defaults translatable in the browser;
	// add structured legal-document modes only if more built-in variants are needed.
	UserAgreement: builtInUserAgreement,
	PrivacyPolicy: builtInPrivacyPolicy,
}

func init() {
	config.GlobalConfig.Register("legal", &defaultLegalSettings)
}

func GetLegalSettings() *LegalSettings {
	return &defaultLegalSettings
}
