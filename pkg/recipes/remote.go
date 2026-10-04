package recipes

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// HTTPRemote fetches git sources from GitHub over HTTPS and archives from any
// https URL. It never runs a git binary: a pinned source needs one ref lookup
// and one tarball, which plain HTTP covers on every platform.
type HTTPRemote struct {
	Client *http.Client
	// APIBase and ArchiveBase default to GitHub's; tests point them at a fake.
	APIBase     string
	ArchiveBase string
	// Token, when set, authenticates GitHub requests (private repositories,
	// rate limits). It is sent to the GitHub bases only, never to an archive
	// source's host.
	Token     string
	UserAgent string
}

const maxArchiveBytes = 64 << 20

var commitRe = regexp.MustCompile(`^[0-9a-f]{40}$`)

func (h HTTPRemote) client() *http.Client {
	if h.Client != nil {
		return h.Client
	}
	return &http.Client{Timeout: 60 * time.Second}
}

func (h HTTPRemote) apiBase() string {
	if h.APIBase != "" {
		return strings.TrimSuffix(h.APIBase, "/")
	}
	return "https://api.github.com"
}

func (h HTTPRemote) archiveBase() string {
	if h.ArchiveBase != "" {
		return strings.TrimSuffix(h.ArchiveBase, "/")
	}
	return "https://codeload.github.com"
}

// ResolveGitRef asks GitHub which commit a ref names. A full commit hash is
// returned as is, without a request.
func (h HTTPRemote) ResolveGitRef(ctx context.Context, repo GitHubRepo, ref string) (string, error) {
	if commitRe.MatchString(ref) {
		return ref, nil
	}
	if ref == "" {
		ref = "HEAD"
	}
	u := fmt.Sprintf("%s/repos/%s/%s/commits/%s", h.apiBase(), url.PathEscape(repo.Owner), url.PathEscape(repo.Repo), url.PathEscape(ref))
	body, err := h.get(ctx, u, "application/vnd.github.sha", true, 4096)
	if err != nil {
		return "", fmt.Errorf("resolve %s@%s: %w", repo, ref, err)
	}
	sha := strings.TrimSpace(string(body))
	if !commitRe.MatchString(sha) {
		return "", fmt.Errorf("resolve %s@%s: unexpected response %q", repo, ref, shortHash(sha, 60))
	}
	return sha, nil
}

// GitArchive downloads the repository tarball at a commit.
func (h HTTPRemote) GitArchive(ctx context.Context, repo GitHubRepo, commit string) ([]byte, error) {
	if !commitRe.MatchString(commit) {
		return nil, fmt.Errorf("%q is not a commit hash", commit)
	}
	u := fmt.Sprintf("%s/%s/%s/tar.gz/%s", h.archiveBase(), url.PathEscape(repo.Owner), url.PathEscape(repo.Repo), commit)
	data, err := h.get(ctx, u, "", true, maxArchiveBytes)
	if err != nil {
		return nil, fmt.Errorf("download %s@%s: %w", repo, shortHash(commit, 12), err)
	}
	return data, nil
}

// Archive downloads an archive source.
func (h HTTPRemote) Archive(ctx context.Context, rawURL string) ([]byte, error) {
	u, err := url.Parse(rawURL)
	if err != nil || u.Scheme != "https" {
		return nil, fmt.Errorf("archive %q: an https URL is required", rawURL)
	}
	data, err := h.get(ctx, rawURL, "", false, maxArchiveBytes)
	if err != nil {
		return nil, fmt.Errorf("download %s: %w", rawURL, err)
	}
	return data, nil
}

func (h HTTPRemote) get(ctx context.Context, u, accept string, github bool, limit int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	if accept != "" {
		req.Header.Set("Accept", accept)
	}
	if h.UserAgent != "" {
		req.Header.Set("User-Agent", h.UserAgent)
	}
	if github && h.Token != "" {
		req.Header.Set("Authorization", "Bearer "+h.Token)
	}
	resp, err := h.client().Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		msg := resp.Status
		if resp.StatusCode == http.StatusNotFound && github {
			msg += " (a private repository needs GITHUB_TOKEN)"
		}
		return nil, fmt.Errorf("%s", msg)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("response is larger than %d bytes", limit)
	}
	return data, nil
}
