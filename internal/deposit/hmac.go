package deposit

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
)

// VerifyWebhookSignature checks whether the signature matches
// the exact request body using our shared secret.
func VerifyWebhookSignature(
	body []byte,
	signature string,
	secret string,
) bool {
	if secret == "" || signature == "" {
		return false
	}

	providedSignature, err := hex.DecodeString(signature)
	if err != nil {
		return false
	}

	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(body)
	expectedSignature := mac.Sum(nil)

	return hmac.Equal(providedSignature, expectedSignature)
}
