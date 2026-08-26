// Package version compares the running binary against published releases.
package version

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	DefaultRepo   = "karutoil/grok-build-webui"
	cacheTTL      = time.Hour
	releasesURLTpl = "https://github.com/%s/releases"
)

// Status values reported to the UI.
const (
	StatusUpToDate  = "up_to_date"
	StatusOutOfDate = "out_of_date"
	StatusLocal     = "local_build"
	StatusUnknown   = "unknown"
)

// Channels reported to the UI.
const (
	ChannelRelease = "release"
	ChannelLocal   = "local"
	ChannelDev     = "dev"
)

// Semver is the numeric core of a tag like v0.12.3.
type Semver struct {
	Parts []int
}

// Parse extracts the numeric core from raw version strings of the shapes we
// build/stamp: "v0.4", "v0.4+g9abc123", "v0.4-g9abc123", "0.4.2", "dev".
func Parse(raw string) (Semver, bool) {
	s := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(raw), "v"))
	if i := strings.IndexAny(s, "+-"); i >= 0 { // strip build metadata (+g<sha>, -g<sha>, -dirty)
		s = s[:i]
	}
	if s == "" || s == "dev" {
		return Semver{}, false
	}
	var parts []int
	for _, seg := range strings.Split(s, ".") {
		n, err := strconv.Atoi(strings.TrimSpace(seg))
		if err != nil || n < 0 {
			return Semver{}, false
		}
		parts = append(parts, n)
	}
	return Semver{Parts: parts}, true
}

// IsLocalBuild reports whether the stamp marks a non-release build
// (source build, git describe output, plain dev).
func IsLocalBuild(raw string) bool {
	s := strings.TrimSpace(raw)
	if s == "" || s == "dev" {
		return true
	}
	return strings.Contains(s, "+g") || strings.Contains(s, "-g") || strings.Contains(s, "-dirty")
}

// Compare returns -1, 0, 1 as a<b, a==b, a>b. Missing segments count as 0.
func Compare(a, b Semver) int {
	n := len(a.Parts)
	if len(b.Parts) > n {
		n = len(b.Parts)
	}
	for i := 0; i < n; i++ {
		x, y := 0, 0
		if i < len(a.Parts) {
			x = a.Parts[i]
		}
		if i < len(b.Parts) {
			y = b.Parts[i]
		}
		switch {
		case x < y:
			return -1
		case x > y:
			return 1
		}
	}
	return 0
}

// ---- latest-release lookup (cached) ---------------------------------------

var (
	mu          sync.Mutex
	cacheTag    string
	cacheAt     time.Time
	lastSuccess time.Time
)

// LastChecked reports when the cache was last successfully populated.
func LastChecked() time.Time {
	mu.Lock()
	defer mu.Unlock()
	return lastSuccess
}

// Repo is the upstream repo override (GROK_WEBUI_UPDATE_REPO).
func Repo() string {
	if r := strings.TrimSpace(os.Getenv("GROK_WEBUI_UPDATE_REPO")); r != "" {
		return r
	}
	return DefaultRepo
}

// Refresh forces the next Latest() call to bypass the cache.
func Refresh() {
	mu.Lock()
	defer mu.Unlock()
	cacheAt = time.Time{}
}

// Latest returns the most recent release tag, or "" when unavailable.
// Results are cached for an hour to stay well inside GitHub rate limits.
func Latest() string {
	mu.Lock()
	fresh := time.Since(cacheAt) < cacheTTL && cacheTag != ""
	tag := cacheTag
	mu.Unlock()
	if fresh {
		return tag
	}
	out, err := fetchLatest()
	if err != nil || out == "" {
		mu.Lock()
		if !cacheAt.IsZero() && cacheTag != "" { // keep serving stale data briefly rather than unknown
			mu.Unlock()
			return cacheTag
		}
		cacheAt = time.Now().Add(-cacheTTL / 2) // retry sooner after total failure
		mu.Unlock()
		return ""
	}
	mu.Lock()
	cacheTag, cacheAt, lastSuccess = out, time.Now(), time.Now()
	mu.Unlock()
	return out
}

func fetchLatest() (string, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", Repo())
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	if tok := os.Getenv("GITHUB_TOKEN"); tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	client := &http.Client{Timeout: 6 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("github api status %d", resp.StatusCode)
	}
	var payload struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", err
	}
	return strings.TrimSpace(payload.TagName), nil
}

// ReleasesURL is the human-facing releases page.
func ReleasesURL() string { return fmt.Sprintf(releasesURLTpl, Repo()) }

// Status classifies current vs latest.
func Status(currentRaw, latest string) string {
	if IsLocalBuild(currentRaw) {
		return StatusLocal
	}
	if latest == "" {
		return StatusUnknown
	}
	cur, okA := Parse(currentRaw)
	lat, okB := Parse(latest)
	if !okA || !okB {
		return StatusUnknown
	}
	if Compare(cur, lat) < 0 {
		return StatusOutOfDate
	}
	return StatusUpToDate
}

// Channel classifies how this binary was produced.
func Channel(currentRaw string) string {
	if IsLocalBuild(currentRaw) {
		s := strings.TrimSpace(currentRaw)
		if s == "dev" || s == "" {
			return ChannelDev
		}
		return ChannelLocal
	}
	return ChannelRelease
}
