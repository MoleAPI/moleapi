package model

import (
	"crypto/rand"
	"encoding/base32"
	"fmt"
	"strings"
	"time"
)

const paymentOrderPrefix = "USR"

var paymentProviderOrderCodes = map[string]string{
	PaymentProviderEpay:         "EP",
	PaymentProviderStripe:       "ST",
	PaymentProviderCreem:        "CR",
	PaymentProviderLanTu:        "LT",
	PaymentProviderNowPayments:  "NP",
	PaymentProviderWaffo:        "WF",
	PaymentProviderWaffoPancake: "WP",
	PaymentProviderBalance:      "BL",
}

func NewTopUpTradeNo(paymentProvider string, userID int) (string, error) {
	return newPaymentOrderNo("T", paymentProvider, userID)
}

func NewSubscriptionTradeNo(paymentProvider string, userID int) (string, error) {
	return newPaymentOrderNo("S", paymentProvider, userID)
}

func newPaymentOrderNo(orderType string, paymentProvider string, userID int) (string, error) {
	providerCode, ok := paymentProviderOrderCodes[paymentProvider]
	if !ok {
		return "", fmt.Errorf("unsupported payment provider: %s", paymentProvider)
	}

	if userID < 0 {
		userID = 0
	}

	random := make([]byte, 2)
	if _, err := rand.Read(random); err != nil {
		return "", fmt.Errorf("generate payment order number: %w", err)
	}
	suffix := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(random)[:4]
	return fmt.Sprintf(
		"%s%s%s%08d%s%s",
		paymentOrderPrefix,
		orderType,
		providerCode,
		userID%100000000,
		time.Now().Format("20060102150405"),
		suffix,
	), nil
}

func IsWaffoPancakeSubscriptionTradeNo(tradeNo string) bool {
	return strings.HasPrefix(tradeNo, paymentOrderPrefix+"SWP") ||
		strings.HasPrefix(tradeNo, "MO1SWP") ||
		strings.HasPrefix(tradeNo, "WAFFO_PANCAKE_SUB-")
}
