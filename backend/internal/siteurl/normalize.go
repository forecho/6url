package siteurl

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/url"
	"strings"

	"golang.org/x/net/publicsuffix"
)

var ErrInvalid = errors.New("URL must be an absolute public HTTP or HTTPS URL")

type Normalized struct {
	URL               string
	RegistrableDomain string
	Hash              string
}

func Normalize(raw string) (Normalized, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Hostname() == "" || parsed.User != nil {
		return Normalized{}, ErrInvalid
	}
	parsed.Fragment = ""
	parsed.Host = strings.ToLower(parsed.Host)
	domain, err := publicsuffix.EffectiveTLDPlusOne(strings.ToLower(parsed.Hostname()))
	if err != nil {
		return Normalized{}, ErrInvalid
	}
	normalizedURL := parsed.String()
	digest := sha256.Sum256([]byte(normalizedURL))
	return Normalized{
		URL:               normalizedURL,
		RegistrableDomain: domain,
		Hash:              hex.EncodeToString(digest[:]),
	}, nil
}
