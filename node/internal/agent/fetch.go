package agent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// Fetching the web is the agent's most useful and most dangerous reach. The
// rules: https only, GET only, no cookies or credentials, public addresses
// only (checked at every redirect), one megabyte, fifteen seconds, and the
// page comes back as text wrapped as untrusted data. Every fetch is written
// into the run record so a person can see exactly what their agent read.

const (
	fetchTimeout = 15 * time.Second
	fetchMax     = 1 << 20
	fetchChars   = 24_000 // what the model sees; the rest is dropped, not summarised
)

// FetchRecord is one fetch as recorded in agent.run.
type FetchRecord struct {
	URL    string `json:"url"`
	Status int    `json:"status"`
	Bytes  int    `json:"bytes"`
	SHA256 string `json:"sha256,omitempty"`
	Error  string `json:"error,omitempty"`
}

// PublicHost refuses anything that resolves to a private, loopback,
// link-local or unspecified address. Resolution happens here, before the
// connection, so a hostname cannot point the agent at the node itself or at
// a cloud metadata service.
func PublicHost(raw string) (*url.URL, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme != "https" || u.Host == "" {
		return nil, fmt.Errorf("only https URLs can be fetched")
	}
	if u.User != nil {
		return nil, fmt.Errorf("URLs with credentials are refused")
	}
	host := u.Hostname()
	if ip := net.ParseIP(host); ip != nil {
		if !publicIP(ip) {
			return nil, fmt.Errorf("%s is not a public address", host)
		}
		return u, nil
	}
	ips, err := net.LookupIP(host)
	if err != nil || len(ips) == 0 {
		return nil, fmt.Errorf("cannot resolve %s", host)
	}
	for _, ip := range ips {
		if !publicIP(ip) {
			return nil, fmt.Errorf("%s resolves to a private address", host)
		}
	}
	return u, nil
}

func publicIP(ip net.IP) bool {
	return !(ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() ||
		ip.IsUnspecified() || ip.IsMulticast() || ip.IsLinkLocalMulticast())
}

// Fetch gets one page as text. It never sends cookies or auth.
func Fetch(ctx context.Context, raw string) (string, FetchRecord) {
	rec := FetchRecord{URL: raw}
	u, err := PublicHost(raw)
	if err != nil {
		rec.Error = err.Error()
		return "", rec
	}
	client := &http.Client{
		Timeout: fetchTimeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 3 {
				return fmt.Errorf("too many redirects")
			}
			if _, err := PublicHost(req.URL.String()); err != nil {
				return err
			}
			return nil
		},
	}
	req, err := http.NewRequestWithContext(ctx, "GET", u.String(), nil)
	if err != nil {
		rec.Error = err.Error()
		return "", rec
	}
	req.Header.Set("User-Agent", "Lamdis-agent/1 (+https://lamdis.ai)")
	req.Header.Set("Accept", "text/html, text/plain, application/json;q=0.9, */*;q=0.1")
	resp, err := client.Do(req)
	if err != nil {
		rec.Error = err.Error()
		return "", rec
	}
	defer resp.Body.Close()
	rec.Status = resp.StatusCode
	body, err := io.ReadAll(io.LimitReader(resp.Body, fetchMax))
	if err != nil {
		rec.Error = err.Error()
		return "", rec
	}
	rec.Bytes = len(body)
	text := string(body)
	if ct := resp.Header.Get("Content-Type"); strings.Contains(ct, "html") || looksHTML(text) {
		text = htmlToText(text)
	}
	text = strings.TrimSpace(text)
	sum := sha256.Sum256([]byte(text))
	rec.SHA256 = hex.EncodeToString(sum[:])
	if len(text) > fetchChars {
		text = text[:fetchChars] + "\n[truncated]"
	}
	if resp.StatusCode >= 400 {
		rec.Error = fmt.Sprintf("HTTP %d", resp.StatusCode)
	}
	return text, rec
}

func looksHTML(s string) bool {
	h := strings.ToLower(s[:min(len(s), 512)])
	return strings.Contains(h, "<html") || strings.Contains(h, "<!doctype html")
}

var (
	reDrop  = regexp.MustCompile(`(?is)<(script|style|noscript|svg|head)[^>]*>.*?</\s*(script|style|noscript|svg|head)\s*>`)
	reBlock = regexp.MustCompile(`(?i)</?(p|div|br|li|ul|ol|h[1-6]|tr|td|th|table|section|article|header|footer|blockquote|pre)[^>]*>`)
	reTag   = regexp.MustCompile(`(?s)<[^>]+>`)
	reWS    = regexp.MustCompile(`[ \t\r\f\v]+`)
	reNL    = regexp.MustCompile(`\n{3,}`)
)

// htmlToText is a deliberately small extractor: drop scripts and styles,
// turn block tags into newlines, strip the rest, decode the common entities.
func htmlToText(s string) string {
	s = reDrop.ReplaceAllString(s, " ")
	s = reBlock.ReplaceAllString(s, "\n")
	s = reTag.ReplaceAllString(s, " ")
	r := strings.NewReplacer("&nbsp;", " ", "&amp;", "&", "&lt;", "<", "&gt;", ">", "&quot;", `"`, "&#39;", "'", "&#x27;", "'")
	s = r.Replace(s)
	s = reWS.ReplaceAllString(s, " ")
	var lines []string
	for _, l := range strings.Split(s, "\n") {
		if t := strings.TrimSpace(l); t != "" {
			lines = append(lines, t)
		}
	}
	return reNL.ReplaceAllString(strings.Join(lines, "\n"), "\n\n")
}
