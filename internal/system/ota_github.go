package system

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/actonos/actonos/internal/security"
)

const (
	DefaultOTAReleasesAPI  = "https://api.github.com/repos/actonos/actonos/releases/latest"
	DefaultOTADownloadBase = "https://github.com/actonos/actonos/releases/latest/download"
	githubAPIVersion       = "2022-11-28"
	githubAccept           = "application/vnd.github+json"
	otaCheckTimeout        = 15 * time.Second
	otaDownloadTimeout     = 300 * time.Second
	otaCheckCacheTTL       = 15 * time.Minute
)

type githubRelease struct {
	TagName    string        `json:"tag_name"`
	Name       string        `json:"name"`
	Draft      bool          `json:"draft"`
	Prerelease bool          `json:"prerelease"`
	Published  string        `json:"published_at"`
	HTMLURL    string        `json:"html_url"`
	Assets     []githubAsset `json:"assets"`
}

type githubAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Digest             string `json:"digest"`
}

func (o *OTAEngine) githubHeaders() http.Header {
	h := make(http.Header)
	h.Set("Accept", githubAccept)
	h.Set("X-GitHub-Api-Version", githubAPIVersion)
	ua := "ActonOS-Daemon"
	if o.version != "" {
		ua = "ActonOS-Daemon/" + o.version
	}
	h.Set("User-Agent", ua)
	if o.token != "" {
		h.Set("Authorization", "Bearer "+o.token)
	}
	return h
}

func (o *OTAEngine) doGitHub(ctx context.Context, client *http.Client, method, rawURL string) (*http.Response, error) {
	if !o.allowLoopback {
		if err := validateOTAURL(rawURL); err != nil {
			return nil, err
		}
	}
	req, err := http.NewRequestWithContext(ctx, method, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header = o.githubHeaders()
	return client.Do(req)
}

func validateOTAURL(raw string) error {
	parsed, err := url.Parse(raw)
	if err != nil {
		return err
	}
	if parsed.Scheme != "https" && parsed.Scheme != "http" {
		return fmt.Errorf("ota url must be http(s)")
	}
	host := strings.ToLower(parsed.Hostname())
	if otaHostAllowed(host) {
		return nil
	}
	// Operator override (ACTONOS_OTA_RELEASES_API) still goes through SSRF checks.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return security.ValidateOutboundURL(ctx, raw)
}

func otaHostAllowed(host string) bool {
	switch host {
	case "github.com", "api.github.com",
		"objects.githubusercontent.com",
		"release-assets.githubusercontent.com",
		"github-releases.githubusercontent.com":
		return true
	}
	return strings.HasSuffix(host, ".githubusercontent.com")
}

func (o *OTAEngine) fetchLatestRelease(ctx context.Context) (*githubRelease, int, http.Header, error) {
	resp, err := o.doGitHub(ctx, o.checkClient, http.MethodGet, o.apiURL)
	if err != nil {
		return nil, 0, nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, resp.StatusCode, resp.Header, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, resp.StatusCode, resp.Header, fmt.Errorf("github releases: HTTP %d", resp.StatusCode)
	}
	var rel githubRelease
	if err := json.Unmarshal(body, &rel); err != nil {
		return nil, resp.StatusCode, resp.Header, fmt.Errorf("decoding github release: %w", err)
	}
	return &rel, resp.StatusCode, resp.Header, nil
}

func retryAfterSeconds(h http.Header) int {
	if h == nil {
		return 0
	}
	raw := strings.TrimSpace(h.Get("Retry-After"))
	if raw == "" {
		return 0
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return 0
	}
	return n
}

func (o *OTAEngine) assetChecksums(ctx context.Context, rel *githubRelease) map[string]string {
	sums := make(map[string]string)
	for _, a := range rel.Assets {
		if hex := NormalizeSHA256(a.Digest); hex != "" {
			sums[a.Name] = hex
		}
	}
	var sumsURL string
	for _, a := range rel.Assets {
		if a.Name == "SHA256SUMS" || a.Name == "SHA256SUMS.txt" {
			sumsURL = a.BrowserDownloadURL
			break
		}
	}
	if sumsURL == "" {
		return sums
	}
	resp, err := o.doGitHub(ctx, o.checkClient, http.MethodGet, sumsURL)
	if err != nil {
		return sums
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return sums
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return sums
	}
	for name, hex := range ParseSHA256SUMS(string(body)) {
		if _, ok := sums[name]; !ok {
			sums[name] = hex
		}
	}
	return sums
}
