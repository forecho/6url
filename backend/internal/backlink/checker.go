package backlink

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"

	"golang.org/x/net/html"
)

const maxResponseBytes = 2 << 20

var nonPublicPrefixes = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("192.0.0.0/24"),
	netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("240.0.0.0/4"),
	netip.MustParsePrefix("2001:db8::/32"),
}

type Result struct {
	Status        string
	RequestedURL  string
	FinalURL      string
	HTTPStatus    *int
	MatchedHref   string
	MatchedAnchor string
	MatchedRel    string
	RedirectChain []string
	ErrorCode     string
	CheckedAt     time.Time
}

func (r Result) RedirectChainJSON() string {
	encoded, err := json.Marshal(r.RedirectChain)
	if err != nil {
		return "[]"
	}
	return string(encoded)
}

type Checker interface {
	Check(context.Context, string, string) Result
}

type Client struct {
	Resolver *net.Resolver
	Timeout  time.Duration
}

func (c Client) Check(ctx context.Context, sourceURL, targetURL string) Result {
	now := time.Now().UTC()
	result := Result{Status: "network_error", RequestedURL: sourceURL, RedirectChain: []string{}, CheckedAt: now}
	source, err := url.Parse(sourceURL)
	if err != nil || !validHTTPURL(source) {
		result.Status, result.ErrorCode = "unsafe", "invalid_source_url"
		return result
	}
	target, err := url.Parse(targetURL)
	if err != nil || target.Scheme != "https" || target.Hostname() == "" {
		result.Status, result.ErrorCode = "unsafe", "invalid_target_url"
		return result
	}
	resolver := c.Resolver
	if resolver == nil {
		resolver = net.DefaultResolver
	}
	if err := validateRemoteHost(ctx, resolver, source.Hostname()); err != nil {
		result.Status, result.ErrorCode = "unsafe", "private_or_unresolved_host"
		return result
	}
	dialer := &net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}
	transport := &http.Transport{
		Proxy:                 nil,
		ForceAttemptHTTP2:     true,
		TLSHandshakeTimeout:   5 * time.Second,
		ResponseHeaderTimeout: 8 * time.Second,
		TLSClientConfig:       &tls.Config{MinVersion: tls.VersionTLS12},
		DialContext: func(dialCtx context.Context, network, address string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(address)
			if err != nil {
				return nil, err
			}
			addresses, err := resolver.LookupIPAddr(dialCtx, host)
			if err != nil || len(addresses) == 0 {
				return nil, errors.New("host resolution failed")
			}
			for _, candidate := range addresses {
				if !isPublicIP(candidate.IP) {
					return nil, errors.New("host resolves to a non-public address")
				}
			}
			return dialer.DialContext(dialCtx, network, net.JoinHostPort(addresses[0].IP.String(), port))
		},
	}
	timeout := c.Timeout
	if timeout <= 0 {
		timeout = 12 * time.Second
	}
	redirects := []string{source.String()}
	httpClient := &http.Client{
		Transport: transport,
		Timeout:   timeout,
		CheckRedirect: func(request *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return errors.New("too many redirects")
			}
			if !validHTTPURL(request.URL) || validateRemoteHost(request.Context(), resolver, request.URL.Hostname()) != nil {
				return errors.New("unsafe redirect")
			}
			redirects = append(redirects, request.URL.String())
			return nil
		},
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, source.String(), nil)
	if err != nil {
		result.ErrorCode = "request_build_failed"
		return result
	}
	request.Header.Set("User-Agent", "6URL-LinkChecker/1.0 (+https://6url.net)")
	request.Header.Set("Accept", "text/html,application/xhtml+xml")
	response, err := httpClient.Do(request)
	result.RedirectChain = redirects
	if err != nil {
		result.ErrorCode = "request_failed"
		return result
	}
	defer response.Body.Close()
	result.FinalURL = response.Request.URL.String()
	result.HTTPStatus = &response.StatusCode
	if response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden || response.StatusCode == http.StatusTooManyRequests {
		result.Status, result.ErrorCode = "blocked", "http_blocked"
		return result
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		result.ErrorCode = fmt.Sprintf("http_%d", response.StatusCode)
		return result
	}
	limited := io.LimitReader(response.Body, maxResponseBytes+1)
	document, err := html.Parse(limited)
	if err != nil {
		result.ErrorCode = "invalid_html"
		return result
	}
	href, anchor, rel, found := findTargetLink(document, response.Request.URL, strings.ToLower(target.Hostname()))
	if !found {
		result.Status, result.ErrorCode = "missing", "target_link_missing"
		return result
	}
	result.Status = "passed"
	result.MatchedHref = href
	result.MatchedAnchor = anchor
	result.MatchedRel = rel
	return result
}

func validateRemoteHost(ctx context.Context, resolver *net.Resolver, host string) error {
	if parsed := net.ParseIP(host); parsed != nil {
		if !isPublicIP(parsed) {
			return errors.New("non-public IP address")
		}
		return nil
	}
	addresses, err := resolver.LookupIPAddr(ctx, host)
	if err != nil || len(addresses) == 0 {
		return errors.New("host resolution failed")
	}
	for _, address := range addresses {
		if !isPublicIP(address.IP) {
			return errors.New("host resolves to a non-public address")
		}
	}
	return nil
}

func isPublicIP(ip net.IP) bool {
	if ip == nil || !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() {
		return false
	}
	address, ok := netip.AddrFromSlice(ip)
	if !ok {
		return false
	}
	address = address.Unmap()
	for _, prefix := range nonPublicPrefixes {
		if prefix.Contains(address) {
			return false
		}
	}
	return true
}

func validHTTPURL(value *url.URL) bool {
	return value != nil && (value.Scheme == "http" || value.Scheme == "https") && value.Hostname() != "" && value.User == nil
}

func findTargetLink(node *html.Node, base *url.URL, targetHost string) (href, anchor, rel string, found bool) {
	if node.Type == html.ElementNode && node.Data == "a" {
		var rawHref string
		for _, attribute := range node.Attr {
			switch strings.ToLower(attribute.Key) {
			case "href":
				rawHref = strings.TrimSpace(attribute.Val)
			case "rel":
				rel = strings.TrimSpace(attribute.Val)
			}
		}
		if rawHref != "" {
			candidate, err := base.Parse(rawHref)
			if err == nil && candidate.Scheme == "https" && strings.EqualFold(candidate.Hostname(), targetHost) {
				return candidate.String(), strings.TrimSpace(textContent(node)), rel, true
			}
		}
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if href, anchor, rel, found := findTargetLink(child, base, targetHost); found {
			return href, anchor, rel, true
		}
	}
	return "", "", "", false
}

func textContent(node *html.Node) string {
	if node.Type == html.TextNode {
		return node.Data
	}
	var builder strings.Builder
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		builder.WriteString(textContent(child))
	}
	return builder.String()
}
