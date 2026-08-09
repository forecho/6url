package siteurl

import "testing"

func TestNormalize(t *testing.T) {
	got, err := Normalize("HTTPS://CN.WSJ.COM/path?q=1#section")
	if err != nil {
		t.Fatal(err)
	}
	if got.URL != "https://cn.wsj.com/path?q=1" {
		t.Fatalf("URL = %q", got.URL)
	}
	if got.RegistrableDomain != "wsj.com" {
		t.Fatalf("registrable domain = %q", got.RegistrableDomain)
	}
	if len(got.Hash) != 64 {
		t.Fatalf("hash length = %d", len(got.Hash))
	}
}

func TestNormalizeRejectsCredentialsAndPrivateNames(t *testing.T) {
	for _, raw := range []string{"https://user:pass@example.com", "http://localhost/path"} {
		if _, err := Normalize(raw); err == nil {
			t.Fatalf("Normalize(%q) unexpectedly succeeded", raw)
		}
	}
}
