package model

import (
	"crypto/rand"
	"encoding/base32"
	"fmt"
	"strings"
)

const paymentOrderPrefix = "MO1"

var paymentProviderOrderCodes = map[string]string{
	PaymentProviderEpay:         "EP",
	PaymentProviderStripe:       "ST",
	PaymentProviderCreem:        "CR",
	PaymentProviderLanTu:        "LT",
	PaymentProviderWaffo:        "WF",
	PaymentProviderWaffoPancake: "WP",
	PaymentProviderBalance:      "BL",
}

func NewTopUpTradeNo(paymentProvider string) (string, error) {
	return newPaymentOrderNo("T", paymentProvider)
}

func NewSubscriptionTradeNo(paymentProvider string) (string, error) {
	return newPaymentOrderNo("S", paymentProvider)
}

func newPaymentOrderNo(orderType string, paymentProvider string) (string, error) {
	providerCode, ok := paymentProviderOrderCodes[paymentProvider]
	if !ok {
		return "", fmt.Errorf("unsupported payment provider: %s", paymentProvider)
	}

	random := make([]byte, 16)
	if _, err := rand.Read(random); err != nil {
		return "", fmt.Errorf("generate payment order number: %w", err)
	}
	suffix := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(random)
	return paymentOrderPrefix + orderType + providerCode + suffix, nil
}

func IsWaffoPancakeSubscriptionTradeNo(tradeNo string) bool {
	return strings.HasPrefix(tradeNo, paymentOrderPrefix+"SWP") ||
		strings.HasPrefix(tradeNo, "WAFFO_PANCAKE_SUB-")
}
