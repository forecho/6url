package backlink

import (
	"net"
	"net/url"
	"strings"
	"testing"

	"golang.org/x/net/html"
)

func TestFindTargetLinkAcceptsNofollow(t *testing.T) {
	document, err := html.Parse(strings.NewReader(`<html><body><a href="https://6url.net/" rel="nofollow noopener">6URL Directory</a></body></html>`))
	if err != nil {
		t.Fatal(err)
	}
	base, _ := url.Parse("https://example.com/links")
	href, anchor, rel, found := findTargetLink(document, base, "6url.net")
	if !found || href != "https://6url.net/" || anchor != "6URL Directory" || rel != "nofollow noopener" {
		t.Fatalf("unexpected match: href=%q anchor=%q rel=%q found=%t", href, anchor, rel, found)
	}
}

func TestPrivateAddressesAreRejected(t *testing.T) {
	for _, raw := range []string{"127.0.0.1", "10.0.0.1", "100.64.0.1", "169.254.169.254", "192.0.2.1", "198.18.0.1", "203.0.113.1", "::1", "fc00::1", "2001:db8::1"} {
		if isPublicIP(net.ParseIP(raw)) {
			t.Fatalf("private address accepted: %s", raw)
		}
	}
	if !isPublicIP(net.ParseIP("1.1.1.1")) {
		t.Fatal("public address was rejected")
	}
}
