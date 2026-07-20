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
		newTradeNo func(string) (string, error)
		prefix     string
	}{
		{name: "topup epay", provider: PaymentProviderEpay, newTradeNo: NewTopUpTradeNo, prefix: "MO1TEP"},
		{name: "topup stripe", provider: PaymentProviderStripe, newTradeNo: NewTopUpTradeNo, prefix: "MO1TST"},
		{name: "topup creem", provider: PaymentProviderCreem, newTradeNo: NewTopUpTradeNo, prefix: "MO1TCR"},
		{name: "topup lantu", provider: PaymentProviderLanTu, newTradeNo: NewTopUpTradeNo, prefix: "MO1TLT"},
		{name: "topup waffo", provider: PaymentProviderWaffo, newTradeNo: NewTopUpTradeNo, prefix: "MO1TWF"},
		{name: "topup waffo pancake", provider: PaymentProviderWaffoPancake, newTradeNo: NewTopUpTradeNo, prefix: "MO1TWP"},
		{name: "subscription epay", provider: PaymentProviderEpay, newTradeNo: NewSubscriptionTradeNo, prefix: "MO1SEP"},
		{name: "subscription stripe", provider: PaymentProviderStripe, newTradeNo: NewSubscriptionTradeNo, prefix: "MO1SST"},
		{name: "subscription creem", provider: PaymentProviderCreem, newTradeNo: NewSubscriptionTradeNo, prefix: "MO1SCR"},
		{name: "subscription waffo pancake", provider: PaymentProviderWaffoPancake, newTradeNo: NewSubscriptionTradeNo, prefix: "MO1SWP"},
		{name: "subscription balance", provider: PaymentProviderBalance, newTradeNo: NewSubscriptionTradeNo, prefix: "MO1SBL"},
	}

	seen := map[string]struct{}{}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			tradeNo, err := test.newTradeNo(test.provider)
			require.NoError(t, err)
			assert.Len(t, tradeNo, 32)
			assert.Regexp(t, regexp.MustCompile(`^[A-Z0-9]{32}$`), tradeNo)
			assert.Equal(t, test.prefix, tradeNo[:6])
			_, duplicate := seen[tradeNo]
			assert.False(t, duplicate)
			seen[tradeNo] = struct{}{}
		})
	}
}

func TestNewPaymentOrderNoRejectsUnknownProvider(t *testing.T) {
	_, err := NewTopUpTradeNo("unknown")
	require.Error(t, err)
}

func TestIsWaffoPancakeSubscriptionTradeNo(t *testing.T) {
	assert.True(t, IsWaffoPancakeSubscriptionTradeNo("MO1SWP01234567890123456789012345"))
	assert.True(t, IsWaffoPancakeSubscriptionTradeNo("WAFFO_PANCAKE_SUB-legacy"))
	assert.False(t, IsWaffoPancakeSubscriptionTradeNo("MO1TWP01234567890123456789012345"))
}
