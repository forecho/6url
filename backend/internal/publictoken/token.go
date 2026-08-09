package publictoken

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strings"
)

type Audience string

const (
	Submission        Audience = "submission"
	AdApplication     Audience = "ad_application"
	SubmissionSession Audience = "submission_session"
	AdSession         Audience = "ad_session"
	CSRF              Audience = "csrf"
)

func Generate(audience Audience) (string, error) {
	if audience == "" {
		return "", errors.New("token audience is required")
	}
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return "", err
	}
	return "6url_" + string(audience) + "_" + base64.RawURLEncoding.EncodeToString(secret), nil
}

func Hash(raw, pepper string, audience Audience) string {
	mac := hmac.New(sha256.New, []byte(pepper))
	_, _ = mac.Write([]byte(string(audience)))
	_, _ = mac.Write([]byte{0})
	_, _ = mac.Write([]byte(raw))
	return hex.EncodeToString(mac.Sum(nil))
}

func ValidShape(raw string, audience Audience) bool {
	prefix := "6url_" + string(audience) + "_"
	if !strings.HasPrefix(raw, prefix) {
		return false
	}
	decoded, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(raw, prefix))
	return err == nil && len(decoded) == 32
}

func Equal(expectedHex, actualHex string) bool {
	expected, err := hex.DecodeString(expectedHex)
	if err != nil {
		return false
	}
	actual, err := hex.DecodeString(actualHex)
	return err == nil && len(expected) == len(actual) && subtle.ConstantTimeCompare(expected, actual) == 1
}
