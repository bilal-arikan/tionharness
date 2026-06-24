package skills

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"
)

const githubHTTPTimeout = 30 * time.Second

// parseGitHubURL extracts owner/repo/ref/dir from a github.com tree or blob URL
// pointing at a skill directory (or its SKILL.md). Forms accepted:
//
//	github.com/<owner>/<repo>/tree/<ref>/<path...>   (dir)
//	github.com/<owner>/<repo>/blob/<ref>/<path...>/SKILL.md  (file → parent dir)
//	github.com/<owner>/<repo>                          (repo root, ref defaults to main)
func parseGitHubURL(raw string) (owner, repo, ref, dir string, err error) {
	u, perr := url.Parse(strings.TrimSpace(raw))
	if perr != nil {
		return "", "", "", "", fmt.Errorf("invalid URL: %w", perr)
	}
	if !strings.EqualFold(u.Host, "github.com") {
		return "", "", "", "", fmt.Errorf("only github.com URLs are supported (got %q)", u.Host)
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		return "", "", "", "", fmt.Errorf("URL must be github.com/<owner>/<repo>/tree/<ref>/<path>")
	}
	owner, repo, ref = parts[0], parts[1], "main"
	if len(parts) >= 4 && (parts[2] == "tree" || parts[2] == "blob") {
		kind := parts[2]
		ref = parts[3]
		dir = strings.Join(parts[4:], "/")
		if kind == "blob" && strings.EqualFold(path.Base(dir), "SKILL.md") {
			dir = path.Dir(dir)
			if dir == "." {
				dir = ""
			}
		}
	}
	return owner, repo, ref, dir, nil
}

// ghContent is one entry of a GitHub contents API directory listing.
type ghContent struct {
	Name        string `json:"name"`
	Type        string `json:"type"` // "file" | "dir"
	DownloadURL string `json:"download_url"`
}

// fetchGitHubSkill resolves a github.com skill-directory URL to its SKILL.md plus
// top-level bundled files via the public GitHub contents API (no auth; subject to
// GitHub's anonymous rate limit). Nested subdirectories are skipped.
func fetchGitHubSkill(rawURL string) (skillMD string, files map[string][]byte, err error) {
	owner, repo, ref, dir, perr := parseGitHubURL(rawURL)
	if perr != nil {
		return "", nil, perr
	}
	api := fmt.Sprintf("https://api.github.com/repos/%s/%s/contents/%s?ref=%s",
		owner, repo, dir, url.QueryEscape(ref))
	body, ferr := httpGetGitHub(api)
	if ferr != nil {
		return "", nil, ferr
	}
	var entries []ghContent
	if jerr := json.Unmarshal(body, &entries); jerr != nil {
		return "", nil, fmt.Errorf("GitHub did not return a directory listing — point the URL at the skill FOLDER, not a file (%w)", jerr)
	}
	files = map[string][]byte{}
	for _, e := range entries {
		if e.Type != "file" || e.DownloadURL == "" {
			continue
		}
		data, derr := httpGetGitHub(e.DownloadURL)
		if derr != nil {
			return "", nil, fmt.Errorf("download %s: %w", e.Name, derr)
		}
		if strings.EqualFold(e.Name, "SKILL.md") {
			skillMD = string(data)
		} else {
			files[e.Name] = data
		}
	}
	if skillMD == "" {
		return "", nil, fmt.Errorf("no SKILL.md found at %s", rawURL)
	}
	return skillMD, files, nil
}

// httpGetGitHub performs a bounded GET restricted to GitHub hosts (the contents
// API + raw download host), so an import cannot be redirected to fetch arbitrary
// internal URLs.
func httpGetGitHub(u string) ([]byte, error) {
	pu, perr := url.Parse(u)
	if perr != nil {
		return nil, perr
	}
	switch strings.ToLower(pu.Host) {
	case "api.github.com", "github.com", "raw.githubusercontent.com", "codeload.github.com":
	default:
		return nil, fmt.Errorf("refusing non-GitHub host %q", pu.Host)
	}
	req, _ := http.NewRequest(http.MethodGet, u, nil)
	req.Header.Set("User-Agent", "SwarmGo-skill-importer")
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := (&http.Client{Timeout: githubHTTPTimeout}).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20)) // 8 MB cap
	if resp.StatusCode != http.StatusOK {
		msg := strings.TrimSpace(string(data))
		if len(msg) > 200 {
			msg = msg[:200]
		}
		return nil, fmt.Errorf("GitHub HTTP %d: %s", resp.StatusCode, msg)
	}
	return data, nil
}
