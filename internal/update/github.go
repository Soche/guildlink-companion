// Package update keeps the companion and the GuildLink addon up to date
// from their GitHub releases.
package update

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	CompanionRepo = "Soche/guildlink-companion"
	AddonRepo     = "Soche/guildlink-addon"
	// Release assets are at most a few tens of MB; refuse anything larger.
	maxDownload = 100 << 20
)

// Release is the part of GitHub's release JSON the updater uses.
type Release struct {
	Tag    string  `json:"tag_name"`
	Notes  string  `json:"body"`
	Assets []Asset `json:"assets"`
}

type Asset struct {
	Name string `json:"name"`
	URL  string `json:"browser_download_url"`
}

func (r *Release) Asset(name string) (Asset, bool) {
	for _, a := range r.Assets {
		if a.Name == name {
			return a, true
		}
	}
	return Asset{}, false
}

// GitHub fetches releases. APIBase is overridable for tests.
type GitHub struct {
	APIBase   string
	HTTP      *http.Client
	UserAgent string
}

func NewGitHub(version string) *GitHub {
	api := "https://api.github.com"
	// For testing updates against a local fake of the GitHub API.
	if v := os.Getenv("GUILDLINK_UPDATE_API"); v != "" {
		api = strings.TrimRight(v, "/")
	}
	return &GitHub{
		APIBase:   api,
		HTTP:      &http.Client{Timeout: 2 * time.Minute},
		UserAgent: "GuildLink-Companion/" + version,
	}
}

func (g *GitHub) get(ctx context.Context, url string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", g.UserAgent)
	resp, err := g.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("GET %s: HTTP %d", url, resp.StatusCode)
	}
	return resp, nil
}

// Latest returns the newest published (non-draft, non-prerelease) release.
func (g *GitHub) Latest(ctx context.Context, repo string) (*Release, error) {
	resp, err := g.get(ctx, fmt.Sprintf("%s/repos/%s/releases/latest", g.APIBase, repo))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var r Release
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&r); err != nil {
		return nil, fmt.Errorf("reading release of %s: %w", repo, err)
	}
	return &r, nil
}

// Download fetches an asset into memory.
func (g *GitHub) Download(ctx context.Context, a Asset) ([]byte, error) {
	resp, err := g.get(ctx, a.URL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxDownload+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxDownload {
		return nil, fmt.Errorf("%s is larger than %d bytes", a.Name, maxDownload)
	}
	return data, nil
}

// DownloadVerified downloads an asset and checks it against the release's
// SHA256SUMS file, so a truncated or tampered download is never installed.
func (g *GitHub) DownloadVerified(ctx context.Context, r *Release, name string) ([]byte, error) {
	asset, ok := r.Asset(name)
	if !ok {
		return nil, fmt.Errorf("release %s has no %s", r.Tag, name)
	}
	sumsAsset, ok := r.Asset("SHA256SUMS")
	if !ok {
		return nil, fmt.Errorf("release %s has no SHA256SUMS", r.Tag)
	}
	sums, err := g.Download(ctx, sumsAsset)
	if err != nil {
		return nil, err
	}
	want, ok := checksumFor(sums, name)
	if !ok {
		return nil, fmt.Errorf("SHA256SUMS of %s doesn't list %s", r.Tag, name)
	}
	data, err := g.Download(ctx, asset)
	if err != nil {
		return nil, err
	}
	got := sha256.Sum256(data)
	if hex.EncodeToString(got[:]) != want {
		return nil, fmt.Errorf("%s failed its checksum; not installing it", name)
	}
	return data, nil
}

// checksumFor reads "<hex>  <name>" lines as written by sha256sum.
func checksumFor(sums []byte, name string) (string, bool) {
	sc := bufio.NewScanner(strings.NewReader(string(sums)))
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) == 2 && strings.TrimPrefix(fields[1], "*") == name {
			return strings.ToLower(fields[0]), true
		}
	}
	return "", false
}

// Newer reports whether version a (e.g. "v0.5.1") is newer than b. Versions
// that don't parse (like "dev") are never newer and never get replaced.
func Newer(a, b string) bool {
	pa, okA := parseVersion(a)
	pb, okB := parseVersion(b)
	if !okA || !okB {
		return false
	}
	for i := range pa {
		if pa[i] != pb[i] {
			return pa[i] > pb[i]
		}
	}
	return false
}

func parseVersion(v string) ([3]int, bool) {
	var out [3]int
	parts := strings.Split(strings.TrimPrefix(strings.TrimSpace(v), "v"), ".")
	if len(parts) != 3 {
		return out, false
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return out, false
		}
		out[i] = n
	}
	return out, true
}

// writeFileAtomic writes data next to path and renames it into place.
func writeFileAtomic(path string, data []byte, mode os.FileMode) error {
	tmp := path + ".download"
	if err := os.WriteFile(tmp, data, mode); err != nil {
		return err
	}
	if err := os.Chmod(tmp, mode); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
