package common

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLanTuSignature(t *testing.T) {
	params := map[string]string{
		"code":         "0",
		"timestamp":    "1700000000",
		"mch_id":       "merchant-1",
		"order_no":     "WX123",
		"out_trade_no": "MO1TLTABC",
		"pay_no":       "PAY123",
		"total_fee":    "1.25",
		"sign":         "ignored",
		"optional":     "",
	}

	assert.Equal(t, "98869F18BF36FF955BD4C0BAC3D4E29B", GenerateLanTuSignature(params, "secret-1"))
	assert.True(t, VerifyLanTuSignature(params, "98869f18bf36ff955bd4c0bac3d4e29b", "secret-1"))
	assert.False(t, VerifyLanTuSignature(params, "98869F18BF36FF955BD4C0BAC3D4E29A", "secret-1"))
}
