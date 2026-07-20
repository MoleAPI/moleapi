package common

import (
	"crypto/md5"
	"crypto/subtle"
	"encoding/hex"
	"sort"
	"strings"
)

// GenerateLanTuSignature implements LanTu's required-field MD5 signature.
func GenerateLanTuSignature(params map[string]string, secretKey string) string {
	keys := make([]string, 0, len(params))
	for key, value := range params {
		if key != "sign" && value != "" {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)

	var payload strings.Builder
	for index, key := range keys {
		if index > 0 {
			payload.WriteByte('&')
		}
		payload.WriteString(key)
		payload.WriteByte('=')
		payload.WriteString(params[key])
	}
	payload.WriteString("&key=")
	payload.WriteString(secretKey)

	sum := md5.Sum([]byte(payload.String()))
	return strings.ToUpper(hex.EncodeToString(sum[:]))
}

func VerifyLanTuSignature(params map[string]string, signature string, secretKey string) bool {
	expected := GenerateLanTuSignature(params, secretKey)
	actual := strings.ToUpper(strings.TrimSpace(signature))
	return len(actual) == len(expected) && subtle.ConstantTimeCompare([]byte(actual), []byte(expected)) == 1
}
