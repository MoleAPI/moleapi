package model

import (
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewPaymentOrderNo(t *testing.T) {
	tests := []struct {
		name       string
		provider   string
		newTradeNo func(string, int) (string, error)
		prefix     string
	}{
		{name: "topup epay", provider: PaymentProviderEpay, newTradeNo: NewTopUpTradeNo, prefix: "USRTEP00000123"},
		{name: "topup stripe", provider: PaymentProviderStripe, newTradeNo: NewTopUpTradeNo, prefix: "USRTST00000123"},
		{name: "topup creem", provider: PaymentProviderCreem, newTradeNo: NewTopUpTradeNo, prefix: "USRTCR00000123"},
		{name: "topup lantu", provider: PaymentProviderLanTu, newTradeNo: NewTopUpTradeNo, prefix: "USRTLT00000123"},
		{name: "topup waffo", provider: PaymentProviderWaffo, newTradeNo: NewTopUpTradeNo, prefix: "USRTWF00000123"},
		{name: "topup waffo pancake", provider: PaymentProviderWaffoPancake, newTradeNo: NewTopUpTradeNo, prefix: "USRTWP00000123"},
		{name: "subscription epay", provider: PaymentProviderEpay, newTradeNo: NewSubscriptionTradeNo, prefix: "USRSEP00000123"},
		{name: "subscription stripe", provider: PaymentProviderStripe, newTradeNo: NewSubscriptionTradeNo, prefix: "USRSST00000123"},
		{name: "subscription creem", provider: PaymentProviderCreem, newTradeNo: NewSubscriptionTradeNo, prefix: "USRSCR00000123"},
		{name: "subscription waffo pancake", provider: PaymentProviderWaffoPancake, newTradeNo: NewSubscriptionTradeNo, prefix: "USRSWP00000123"},
		{name: "subscription balance", provider: PaymentProviderBalance, newTradeNo: NewSubscriptionTradeNo, prefix: "USRSBL00000123"},
	}

	seen := map[string]struct{}{}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			tradeNo, err := test.newTradeNo(test.provider, 123)
			require.NoError(t, err)
			assert.Len(t, tradeNo, 32)
			assert.Regexp(t, regexp.MustCompile(`^USR[TS][A-Z]{2}[0-9]{22}[A-Z2-7]{4}$`), tradeNo)
			assert.Equal(t, test.prefix, tradeNo[:14])
			_, duplicate := seen[tradeNo]
			assert.False(t, duplicate)
			seen[tradeNo] = struct{}{}
		})
	}
}

func TestNewPaymentOrderNoRejectsUnknownProvider(t *testing.T) {
	_, err := NewTopUpTradeNo("unknown", 1)
	require.Error(t, err)
}

func TestIsWaffoPancakeSubscriptionTradeNo(t *testing.T) {
	assert.True(t, IsWaffoPancakeSubscriptionTradeNo("USRSWP0000012320260722143101ABCD"))
	assert.True(t, IsWaffoPancakeSubscriptionTradeNo("MO1SWP01234567890123456789012345"))
	assert.True(t, IsWaffoPancakeSubscriptionTradeNo("WAFFO_PANCAKE_SUB-legacy"))
	assert.False(t, IsWaffoPancakeSubscriptionTradeNo("USRTWP0000012320260722143101ABCD"))
}
