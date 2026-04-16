package main

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"html"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"time"
)

type SearchResult struct {
	Engine      string `json:"engine"`
	Title       string `json:"title"`
	URL         string `json:"url"`
	DisplayURL  string `json:"display_url,omitempty"`
	Snippet     string `json:"snippet,omitempty"`
	Rank        int    `json:"rank"`
	Source      string `json:"source,omitempty"`
}

type SearchOutput struct {
	OK           bool           `json:"ok"`
	Command      string         `json:"command"`
	Query        string         `json:"query"`
	Engine       string         `json:"engine"`
	TriedEngines []string       `json:"tried_engines,omitempty"`
	Count        int            `json:"count"`
	Results      []SearchResult `json:"results"`
	DurationMs   int64          `json:"duration_ms"`
	Error        string         `json:"error,omitempty"`
}

type FetchOutput struct {
	OK               bool              `json:"ok"`
	Command          string            `json:"command"`
	URL              string            `json:"url"`
	FinalURL         string            `json:"final_url,omitempty"`
	Status           int               `json:"status"`
	ContentType      string            `json:"content_type,omitempty"`
	Charset          string            `json:"charset,omitempty"`
	Bytes            int64             `json:"bytes"`
	SavedTo          string            `json:"saved_to,omitempty"`
	SHA256           string            `json:"sha256,omitempty"`
	PreviewText      string            `json:"preview_text,omitempty"`
	PreviewBase64    string            `json:"preview_base64,omitempty"`
	Headers          map[string]string `json:"headers,omitempty"`
	DurationMs       int64             `json:"duration_ms"`
	Error            string            `json:"error,omitempty"`
}

type ListItem struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	Bytes   int64  `json:"bytes"`
	ModTime string `json:"mod_time"`
}

type ListOutput struct {
	OK      bool       `json:"ok"`
	Command string     `json:"command"`
	Dir     string     `json:"dir"`
	Count   int        `json:"count"`
	Files   []ListItem `json:"files"`
	Error   string     `json:"error,omitempty"`
}

var (
	reAnchor  = regexp.MustCompile(`(?is)<a[^>]+href=["']([^"']+)["'][^>]*>(.*?)</a>`)
	reTags    = regexp.MustCompile(`(?is)<[^>]+>`)
	reSpace   = regexp.MustCompile(`\s+`)
	reScript  = regexp.MustCompile(`(?is)<script.*?</script>|<style.*?</style>`)
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(2)
	}

	switch os.Args[1] {
	case "search":
		runSearch(os.Args[2:])
	case "fetch":
		runFetch(os.Args[2:], false)
	case "download":
		runFetch(os.Args[2:], true)
	case "list":
		runList(os.Args[2:])
	case "help", "--help", "-h":
		printUsage()
	default:
		printUsage()
		os.Exit(2)
	}
}

func printUsage() {
	fmt.Println(`websearch: standalone cross-platform search/fetch/download CLI by Harald Kubovy 16.04.2026

Commands:
  search <query> [--engine auto|google|bing|duckduckgo] [--max 5] [--timeout 20] [--json]
  fetch <url> [--timeout 30] [--save path] [--json] [--max-bytes 2000000]
  download <url> [--timeout 120] [--save path] [--json]
  list [--dir downloads] [--json]

Notes:
  - Default output is JSON, optimized for AI tool use.
  - search engine auto tries google, bing, duckduckgo until results are found.
  - fetch returns text preview for HTML/text and base64 preview for binary/image data.
  - download always saves to disk.

Examples:
  websearch search "OpenClaw latest release" --engine auto --max 5
  websearch fetch https://github.com/openclaw/openclaw
  websearch download https://example.com/file.zip --save downloads\file.zip
`)
}

func runSearch(args []string) {
	engine := "auto"
	maxResults := 5
	timeout := 20
	queryParts := make([]string, 0, len(args))

	for i := 0; i < len(args); i++ {
		a := args[i]
		switch a {
		case "--engine", "-engine":
			if i+1 < len(args) {
				engine = args[i+1]
				i++
			}
		case "--max", "-max":
			if i+1 < len(args) {
				fmt.Sscanf(args[i+1], "%d", &maxResults)
				i++
			}
		case "--timeout", "-timeout":
			if i+1 < len(args) {
				fmt.Sscanf(args[i+1], "%d", &timeout)
				i++
			}
		case "--json", "-json":
			// default already JSON
		default:
			queryParts = append(queryParts, a)
		}
	}

	query := strings.TrimSpace(strings.Join(queryParts, " "))
	if query == "" {
		emitJSON(SearchOutput{OK: false, Command: "search", Error: "missing query"}, 2)
		return
	}

	if maxResults < 1 {
		maxResults = 1
	}
	if maxResults > 20 {
		maxResults = 20
	}

	start := time.Now()
	client := newHTTPClient(time.Duration(timeout) * time.Second)
	engines := resolveEngines(engine)
	tried := make([]string, 0, len(engines))
	var results []SearchResult
	var lastErr error

	for _, eng := range engines {
		tried = append(tried, eng)
		res, err := searchWithEngine(client, eng, query, maxResults)
		if err != nil {
			lastErr = err
			continue
		}
		if len(res) > 0 {
			results = res
			break
		}
	}

	out := SearchOutput{
		OK:           len(results) > 0,
		Command:      "search",
		Query:        query,
		Engine:       engine,
		TriedEngines: tried,
		Count:        len(results),
		Results:      results,
		DurationMs:   time.Since(start).Milliseconds(),
	}
	if len(results) == 0 && lastErr != nil {
		out.Error = lastErr.Error()
	}
	if len(results) == 0 && out.Error == "" {
		out.Error = "no results"
	}
	code := 0
	if !out.OK {
		code = 1
	}
	emitJSON(out, code)
}

func runFetch(args []string, forceSave bool) {
	cmd := "fetch"
	if forceSave {
		cmd = "download"
	}

	timeout := 30
	savePath := ""
	maxBytes := int64(2_000_000)
	positional := make([]string, 0, len(args))

	for i := 0; i < len(args); i++ {
		a := args[i]
		switch a {
		case "--timeout", "-timeout":
			if i+1 < len(args) {
				fmt.Sscanf(args[i+1], "%d", &timeout)
				i++
			}
		case "--save", "-save":
			if i+1 < len(args) {
				savePath = args[i+1]
				i++
			}
		case "--max-bytes", "-max-bytes":
			if i+1 < len(args) {
				fmt.Sscanf(args[i+1], "%d", &maxBytes)
				i++
			}
		case "--json", "-json":
			// default already JSON
		default:
			positional = append(positional, a)
		}
	}

	if len(positional) < 1 {
		emitJSON(FetchOutput{OK: false, Command: cmd, Error: "missing url"}, 2)
		return
	}

	rawURL := positional[0]
	if forceSave && savePath == "" {
		savePath = defaultSavePath(rawURL)
	}
	if forceSave && maxBytes < 50_000_000 {
		maxBytes = 50_000_000
	}

	start := time.Now()
	client := newHTTPClient(time.Duration(timeout) * time.Second)
	resp, err := makeRequest(client, rawURL)
	if err != nil {
		emitJSON(FetchOutput{OK: false, Command: cmd, URL: rawURL, Error: err.Error(), DurationMs: time.Since(start).Milliseconds()}, 1)
		return
	}
	defer resp.Body.Close()

	body, truncated, readErr := readUpTo(resp.Body, maxBytes)
	if readErr != nil {
		emitJSON(FetchOutput{OK: false, Command: cmd, URL: rawURL, Error: readErr.Error(), DurationMs: time.Since(start).Milliseconds()}, 1)
		return
	}

	headers := pickHeaders(resp.Header)
	contentType := resp.Header.Get("Content-Type")
	mediatype, params, _ := mime.ParseMediaType(contentType)
	out := FetchOutput{
		OK:          resp.StatusCode >= 200 && resp.StatusCode < 400,
		Command:     cmd,
		URL:         rawURL,
		FinalURL:    resp.Request.URL.String(),
		Status:      resp.StatusCode,
		ContentType: mediatype,
		Charset:     params["charset"],
		Bytes:       int64(len(body)),
		Headers:     headers,
		DurationMs:  time.Since(start).Milliseconds(),
	}

	sum := sha256.Sum256(body)
	out.SHA256 = hex.EncodeToString(sum[:])

	if shouldTreatAsText(mediatype, body) {
		text := normalizeWhitespace(extractTextPreview(body, mediatype))
		out.PreviewText = truncate(text, 8000)
	} else {
		out.PreviewBase64 = truncate(base64.StdEncoding.EncodeToString(body), 4000)
	}
	if truncated {
		if out.PreviewText != "" {
			out.PreviewText += " [truncated]"
		}
		if out.PreviewBase64 != "" {
			out.PreviewBase64 += " [truncated]"
		}
	}

	if savePath != "" {
		fullPath, saveErr := saveResponseBody(resp, body, savePath)
		if saveErr != nil {
			out.OK = false
			out.Error = saveErr.Error()
			emitJSON(out, 1)
			return
		}
		out.SavedTo = fullPath
	}

	code := 0
	if !out.OK {
		code = 1
	}
	emitJSON(out, code)
}

func runList(args []string) {
	fs := flag.NewFlagSet("list", flag.ExitOnError)
	dir := fs.String("dir", "downloads", "directory to list")
	jsonOut := fs.Bool("json", true, "json output")
	_ = jsonOut
	fs.Parse(args)

	items, err := os.ReadDir(*dir)
	if err != nil {
		emitJSON(ListOutput{OK: false, Command: "list", Dir: *dir, Error: err.Error()}, 1)
		return
	}
	files := make([]ListItem, 0, len(items))
	for _, item := range items {
		info, err := item.Info()
		if err != nil || info.IsDir() {
			continue
		}
		files = append(files, ListItem{
			Name:    item.Name(),
			Path:    filepath.Join(*dir, item.Name()),
			Bytes:   info.Size(),
			ModTime: info.ModTime().Format(time.RFC3339),
		})
	}
	sort.Slice(files, func(i, j int) bool { return files[i].ModTime > files[j].ModTime })
	emitJSON(ListOutput{OK: true, Command: "list", Dir: *dir, Count: len(files), Files: files}, 0)
}

func resolveEngines(engine string) []string {
	switch strings.ToLower(strings.TrimSpace(engine)) {
	case "", "auto":
		return []string{"google", "bing", "duckduckgo"}
	case "ddg":
		return []string{"duckduckgo"}
	case "google", "bing", "duckduckgo":
		return []string{strings.ToLower(engine)}
	default:
		return []string{strings.ToLower(engine)}
	}
}

func searchWithEngine(client *http.Client, engine, query string, maxResults int) ([]SearchResult, error) {
	var endpoint string
	switch engine {
	case "google":
		endpoint = "https://www.google.com/search?q=" + url.QueryEscape(query) + "&num=" + fmt.Sprint(maxResults)
	case "bing":
		endpoint = "https://www.bing.com/search?q=" + url.QueryEscape(query) + "&count=" + fmt.Sprint(maxResults)
	case "duckduckgo":
		endpoint = "https://html.duckduckgo.com/html/?q=" + url.QueryEscape(query)
	default:
		return nil, errors.New("unsupported engine: " + engine)
	}

	resp, err := newRequest(client, endpoint)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 400 {
		return nil, fmt.Errorf("%s returned status %d", engine, resp.StatusCode)
	}
	body, _, err := readUpTo(resp.Body, 1_500_000)
	if err != nil {
		return nil, err
	}
	results := parseHTMLResults(engine, body, maxResults)
	return dedupeResults(results, maxResults), nil
}

func parseHTMLResults(engine string, body []byte, maxResults int) []SearchResult {
	s := reScript.ReplaceAllString(string(body), " ")
	matches := reAnchor.FindAllStringSubmatch(s, -1)
	results := make([]SearchResult, 0, maxResults)
	seen := map[string]bool{}
	rank := 1

	for _, m := range matches {
		if len(m) < 3 {
			continue
		}
		href := html.UnescapeString(strings.TrimSpace(m[1]))
		text := cleanText(m[2])
		if text == "" || href == "" {
			continue
		}
		if strings.HasPrefix(href, "/") {
			switch engine {
			case "google":
				href = "https://www.google.com" + href
			case "bing":
				href = "https://www.bing.com" + href
			case "duckduckgo":
				href = "https://html.duckduckgo.com" + href
			}
		}
		parsed, ok := normalizeResultURL(engine, href)
		if !ok {
			continue
		}
		if len(text) < 3 {
			continue
		}
		lower := strings.ToLower(text)
		if strings.Contains(lower, "sign in") || strings.Contains(lower, "privacy") || strings.Contains(lower, "terms") || strings.Contains(lower, "feedback") {
			continue
		}
		if seen[parsed] {
			continue
		}
		seen[parsed] = true
		results = append(results, SearchResult{
			Engine:     engine,
			Title:      text,
			URL:        parsed,
			DisplayURL: hostOnly(parsed),
			Snippet:    "",
			Rank:       rank,
			Source:     engine,
		})
		rank++
		if len(results) >= maxResults {
			break
		}
	}
	return results
}

func normalizeResultURL(engine, href string) (string, bool) {
	if strings.HasPrefix(href, "javascript:") || strings.HasPrefix(href, "#") {
		return "", false
	}
	u, err := url.Parse(href)
	if err != nil {
		return "", false
	}
	if engine == "google" {
		if u.Path == "/url" {
			q := u.Query().Get("q")
			if q == "" {
				q = u.Query().Get("url")
			}
			if q != "" {
				href = q
			}
		}
	}
	if engine == "duckduckgo" {
		uddg := u.Query().Get("uddg")
		if uddg != "" {
			href = uddg
		}
	}
	if !strings.HasPrefix(href, "http://") && !strings.HasPrefix(href, "https://") {
		return "", false
	}
	badHosts := []string{"google.com", "www.google.com", "bing.com", "www.bing.com", "duckduckgo.com", "html.duckduckgo.com"}
	parsed, err := url.Parse(href)
	if err != nil || parsed.Host == "" {
		return "", false
	}
	for _, bad := range badHosts {
		if strings.EqualFold(parsed.Host, bad) && parsed.Path != "/url" {
			return "", false
		}
	}
	return href, true
}

func dedupeResults(in []SearchResult, max int) []SearchResult {
	seen := map[string]bool{}
	out := make([]SearchResult, 0, len(in))
	for _, r := range in {
		if seen[r.URL] {
			continue
		}
		seen[r.URL] = true
		out = append(out, r)
		if len(out) >= max {
			break
		}
	}
	for i := range out {
		out[i].Rank = i + 1
	}
	return out
}

func newHTTPClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout: timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return errors.New("too many redirects")
			}
			return nil
		},
	}
}

func newRequest(client *http.Client, target string) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodGet, target, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", defaultUserAgent())
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	return client.Do(req)
}

func makeRequest(client *http.Client, target string) (*http.Response, error) {
	return newRequest(client, target)
}

func saveResponseBody(resp *http.Response, preview []byte, path string) (string, error) {
	full, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return "", err
	}
	return full, os.WriteFile(full, preview, 0o644)
}

func readUpTo(r io.Reader, maxBytes int64) ([]byte, bool, error) {
	if maxBytes <= 0 {
		maxBytes = 1_000_000
	}
	lr := &io.LimitedReader{R: r, N: maxBytes + 1}
	b, err := io.ReadAll(lr)
	if err != nil {
		return nil, false, err
	}
	truncated := int64(len(b)) > maxBytes
	if truncated {
		b = b[:maxBytes]
	}
	return b, truncated, nil
}

func shouldTreatAsText(mediaType string, body []byte) bool {
	if strings.HasPrefix(mediaType, "text/") || mediaType == "application/json" || mediaType == "application/xml" || mediaType == "application/javascript" {
		return true
	}
	if mediaType == "" {
		for _, c := range body {
			if c == 0 {
				return false
			}
		}
		return true
	}
	return false
}

func extractTextPreview(body []byte, mediaType string) string {
	s := string(body)
	if strings.Contains(mediaType, "html") {
		s = reScript.ReplaceAllString(s, " ")
		s = reTags.ReplaceAllString(s, " ")
		s = html.UnescapeString(s)
	}
	return s
}

func pickHeaders(h http.Header) map[string]string {
	keys := []string{"Content-Type", "Content-Length", "Last-Modified", "ETag", "Location"}
	out := map[string]string{}
	for _, k := range keys {
		if v := h.Get(k); v != "" {
			out[k] = v
		}
	}
	return out
}

func cleanText(s string) string {
	s = reTags.ReplaceAllString(s, " ")
	s = html.UnescapeString(s)
	return normalizeWhitespace(s)
}

func normalizeWhitespace(s string) string {
	return strings.TrimSpace(reSpace.ReplaceAllString(s, " "))
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	if n < 4 {
		return s[:n]
	}
	return s[:n-3] + "..."
}

func hostOnly(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return u.Host
}

func defaultSavePath(raw string) string {
	u, err := url.Parse(raw)
	name := "download.bin"
	if err == nil {
		base := filepath.Base(u.Path)
		if base != "." && base != "/" && base != "" {
			name = base
		}
	}
	if runtime.GOOS == "windows" {
		return filepath.Join("downloads", name)
	}
	return filepath.Join("downloads", name)
}

func defaultUserAgent() string {
	return "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/138.0.0.0 Safari/537.36"
}

func emitJSON(v any, code int) {
	enc := json.NewEncoder(os.Stdout)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
	if code != 0 {
		os.Exit(code)
	}
}
