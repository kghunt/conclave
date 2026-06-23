package handlers

import (
	"net"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

type linkPreview struct {
	URL         string `json:"url"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Image       string `json:"image"`
	SiteName    string `json:"site_name"`
}

var (
	unfurlCache   = map[string]*linkPreview{}
	unfurlExpiry  = map[string]time.Time{}
	unfurlMu      sync.Mutex
	ogTitle       = regexp.MustCompile(`(?i)<meta[^>]+property=["']og:title["'][^>]+content=["']([^"']+)["']`)
	ogDesc        = regexp.MustCompile(`(?i)<meta[^>]+property=["']og:description["'][^>]+content=["']([^"']+)["']`)
	ogImage       = regexp.MustCompile(`(?i)<meta[^>]+property=["']og:image["'][^>]+content=["']([^"']+)["']`)
	ogSite        = regexp.MustCompile(`(?i)<meta[^>]+property=["']og:site_name["'][^>]+content=["']([^"']+)["']`)
	htmlTitle     = regexp.MustCompile(`(?i)<title[^>]*>([^<]+)</title>`)
	// Also match content-first ordering: content="..." property="og:..."
	ogTitleAlt    = regexp.MustCompile(`(?i)<meta[^>]+content=["']([^"']+)["'][^>]+property=["']og:title["']`)
	ogDescAlt     = regexp.MustCompile(`(?i)<meta[^>]+content=["']([^"']+)["'][^>]+property=["']og:description["']`)
	ogImageAlt    = regexp.MustCompile(`(?i)<meta[^>]+content=["']([^"']+)["'][^>]+property=["']og:image["']`)
	ogSiteAlt     = regexp.MustCompile(`(?i)<meta[^>]+content=["']([^"']+)["'][^>]+property=["']og:site_name["']`)
)

func matchFirst(re1, re2 *regexp.Regexp, body string) string {
	if m := re1.FindStringSubmatch(body); len(m) > 1 {
		return m[1]
	}
	if m := re2.FindStringSubmatch(body); len(m) > 1 {
		return m[1]
	}
	return ""
}

func Unfurl(w http.ResponseWriter, r *http.Request) {
	rawURL := strings.TrimSpace(r.URL.Query().Get("url"))
	if rawURL == "" {
		writeErr(w, http.StatusBadRequest, "url required")
		return
	}
	if !strings.HasPrefix(rawURL, "http://") && !strings.HasPrefix(rawURL, "https://") {
		writeErr(w, http.StatusBadRequest, "only http/https URLs allowed")
		return
	}

	unfurlMu.Lock()
	if p, ok := unfurlCache[rawURL]; ok && time.Now().Before(unfurlExpiry[rawURL]) {
		unfurlMu.Unlock()
		writeJSON(w, http.StatusOK, p)
		return
	}
	unfurlMu.Unlock()

	// Guard against SSRF: resolve hostname and reject private IPs.
	parsed, err := http.NewRequest("GET", rawURL, nil)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid url")
		return
	}
	host := parsed.URL.Hostname()
	addrs, err := net.LookupHost(host)
	if err != nil || len(addrs) == 0 {
		writeErr(w, http.StatusBadRequest, "could not resolve host")
		return
	}
	for _, addr := range addrs {
		ip := net.ParseIP(addr)
		if ip == nil || ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() {
			writeErr(w, http.StatusForbidden, "private addresses not allowed")
			return
		}
	}

	client := &http.Client{
		Timeout: 4 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 3 {
				return http.ErrUseLastResponse
			}
			return nil
		},
	}
	req, _ := http.NewRequestWithContext(r.Context(), "GET", rawURL, nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; Conclave-Unfurl/1.0)")
	req.Header.Set("Accept", "text/html")

	resp, err := client.Do(req)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "fetch failed")
		return
	}
	defer resp.Body.Close()

	ct := resp.Header.Get("Content-Type")
	if !strings.Contains(ct, "text/html") {
		writeErr(w, http.StatusUnprocessableEntity, "not an HTML page")
		return
	}

	// Read at most 64KB — enough for the <head>.
	buf := make([]byte, 64*1024)
	n, _ := resp.Body.Read(buf)
	body := string(buf[:n])

	preview := &linkPreview{
		URL:         rawURL,
		Title:       matchFirst(ogTitle, ogTitleAlt, body),
		Description: matchFirst(ogDesc, ogDescAlt, body),
		Image:       matchFirst(ogImage, ogImageAlt, body),
		SiteName:    matchFirst(ogSite, ogSiteAlt, body),
	}
	if preview.Title == "" {
		if m := htmlTitle.FindStringSubmatch(body); len(m) > 1 {
			preview.Title = strings.TrimSpace(m[1])
		}
	}
	if !utf8.ValidString(preview.Title) {
		preview.Title = ""
	}

	unfurlMu.Lock()
	unfurlCache[rawURL] = preview
	unfurlExpiry[rawURL] = time.Now().Add(time.Hour)
	unfurlMu.Unlock()

	writeJSON(w, http.StatusOK, preview)
}
