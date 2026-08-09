package admintoken

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"strings"
)

const tokenMarker = "6url_"

var allowedScopes = map[string]struct{}{
	"catalog:read": {}, "content:write": {}, "content:publish": {}, "assets:write": {},
	"submissions:review": {}, "ad_applications:review": {}, "placements:write": {}, "ads:write": {},
	"payments:read": {}, "payments:write": {}, "tokens:write": {},
}

func Generate() (raw, prefix string, err error) {
	prefixBytes := make([]byte, 6)
	secretBytes := make([]byte, 32)
	if _, err = rand.Read(prefixBytes); err != nil {
		return "", "", err
	}
	if _, err = rand.Read(secretBytes); err != nil {
		return "", "", err
	}
	prefix = hex.EncodeToString(prefixBytes)
	raw = tokenMarker + prefix + "_" + hex.EncodeToString(secretBytes)
	return raw, prefix, nil
}

func Prefix(raw string) (string, error) {
	if !strings.HasPrefix(raw, tokenMarker) {
		return "", errors.New("invalid token marker")
	}
	remainder := strings.TrimPrefix(raw, tokenMarker)
	separator := strings.IndexByte(remainder, '_')
	if separator != 12 {
		return "", errors.New("invalid token prefix")
	}
	if len(remainder) != 12+1+64 {
		return "", errors.New("invalid token length")
	}
	return remainder[:separator], nil
}

func Hash(raw, pepper string) string {
	mac := hmac.New(sha256.New, []byte(pepper))
	_, _ = mac.Write([]byte(raw))
	return hex.EncodeToString(mac.Sum(nil))
}

func Equal(expectedHex, actualHex string) bool {
	expected, err := hex.DecodeString(expectedHex)
	if err != nil {
		return false
	}
	actual, err := hex.DecodeString(actualHex)
	if err != nil || len(expected) != len(actual) {
		return false
	}
	return subtle.ConstantTimeCompare(expected, actual) == 1
}

func ValidateScopes(scopes []string) error {
	if len(scopes) == 0 {
		return errors.New("at least one scope is required")
	}
	for _, scope := range scopes {
		if _, ok := allowedScopes[scope]; !ok {
			return errors.New("unsupported scope: " + scope)
		}
	}
	return nil
}

func HasScope(scopes []string, required string) bool {
	for _, scope := range scopes {
		if scope == required {
			return true
		}
	}
	return false
}
