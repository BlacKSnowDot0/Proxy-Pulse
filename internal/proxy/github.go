package proxy

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"sync/atomic"
)

type RequestCounter struct {
	total atomic.Int64
}

func (c *RequestCounter) Inc() {
	c.total.Add(1)
}

func (c *RequestCounter) Load() int64 {
	return c.total.Load()
}

type GitHubClient struct {
	cfg        Config
	httpClient *http.Client
	counter    *RequestCounter
}

func NewGitHubClient(cfg Config, counter *RequestCounter) *GitHubClient {
	return &GitHubClient{
		cfg:        cfg,
		httpClient: &http.Client{Timeout: cfg.ValidationTimeout},
		counter:    counter,
	}
}

func (c *GitHubClient) Discover(ctx context.Context) (DiscoveryResult, error) {
	result := DiscoveryResult{
		SourceCounts: map[string]int{
			"repository": 0,
			"gist":       0,
		},
	}

	repos := make(map[string]repoSearchItem)
	gists := make(map[string]struct{})

	for _, query := range c.cfg.Queries {
		repoItems, err := c.searchRepositories(ctx, query)
		if err != nil {
			result.ErrorCount++
		} else {
			for _, item := range repoItems {
				repos[item.FullName] = item
			}
		}

		gistIDs, err := c.searchGists(ctx, query)
		if err != nil {
			result.ErrorCount++
		} else {
			for _, gistID := range gistIDs {
				gists[gistID] = struct{}{}
			}
		}
	}

	repoKeys := sortedRepoKeys(repos)
	for _, key := range repoKeys {
		files, err := c.discoverRepositoryFiles(ctx, repos[key])
		if err != nil {
			result.ErrorCount++
			continue
		}
		if len(files) == 0 {
			continue
		}
		result.SourceCounts["repository"]++
		result.Files = append(result.Files, files...)
	}

	gistKeys := sortedStringKeys(gists)
	for _, gistID := range gistKeys {
		files, err := c.discoverGistFiles(ctx, gistID)
		if err != nil {
			result.ErrorCount++
			continue
		}
		if len(files) == 0 {
			continue
		}
		result.SourceCounts["gist"]++
		result.Files = append(result.Files, files...)
	}

	return result, nil
}

func (c *GitHubClient) FetchText(ctx context.Context, file SourceFile) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, file.DownloadURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", c.cfg.UserAgent)

	c.counter.Inc()
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("unexpected status %d for %s", resp.StatusCode, file.DownloadURL)
	}

	limited := io.LimitReader(resp.Body, c.cfg.MaxFileBytes+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return "", err
	}
	if int64(len(data)) > c.cfg.MaxFileBytes {
		return "", fmt.Errorf("file too large: %s", file.Path)
	}
	return string(data), nil
}

func (c *GitHubClient) searchRepositories(ctx context.Context, query string) ([]repoSearchItem, error) {
	endpoint := fmt.Sprintf(
		"%s/search/repositories?q=%s&sort=updated&order=desc&per_page=%d",
		strings.TrimRight(c.cfg.GitHubAPIBase, "/"),
		url.QueryEscape(query),
		c.cfg.MaxReposPerQuery,
	)

	var payload repoSearchResponse
	if err := c.getJSON(ctx, endpoint, &payload); err != nil {
		return nil, err
	}
	return payload.Items, nil
}

func (c *GitHubClient) searchGists(ctx context.Context, query string) ([]string, error) {
	endpoint := fmt.Sprintf("%s/search?q=%s", strings.TrimRight(c.cfg.GistWebBase, "/"), url.QueryEscape(query))
	body, err := c.getText(ctx, endpoint)
	if err != nil {
		return nil, err
	}
	return extractGistIDs(body, c.cfg.MaxGistsPerQuery), nil
}

func extractGistIDs(body string, limit int) []string {
	re := regexp.MustCompile(`href="/[^"/]+/([0-9a-f]{20,32})"`)
	matches := re.FindAllStringSubmatch(body, -1)
	if len(matches) == 0 {
		return nil
	}

	seen := make(map[string]struct{}, len(matches))
	out := make([]string, 0, len(matches))
	for _, match := range matches {
		gistID := match[1]
		if _, ok := seen[gistID]; ok {
			continue
		}
		seen[gistID] = struct{}{}
		out = append(out, gistID)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out
}

func (c *GitHubClient) discoverRepositoryFiles(ctx context.Context, repo repoSearchItem) ([]SourceFile, error) {
	endpoint := fmt.Sprintf(
		"%s/repos/%s/git/trees/%s?recursive=1",
		strings.TrimRight(c.cfg.GitHubAPIBase, "/"),
		repo.FullName,
		url.PathEscape(repo.DefaultBranch),
	)

	var payload repoTreeResponse
	if err := c.getJSON(ctx, endpoint, &payload); err != nil {
		return nil, err
	}

	paths := make([]repoTreeItem, 0, len(payload.Tree))
	for _, item := range payload.Tree {
		if item.Type != "blob" {
			continue
		}
		if item.Size <= 0 || item.Size > c.cfg.MaxFileBytes {
			continue
		}
		if !shouldInspectPath(item.Path) {
			continue
		}
		paths = append(paths, item)
	}

	sort.Slice(paths, func(i, j int) bool {
		return paths[i].Path < paths[j].Path
	})
	if len(paths) > c.cfg.MaxFilesPerSource {
		paths = paths[:c.cfg.MaxFilesPerSource]
	}

	files := make([]SourceFile, 0, len(paths))
	for _, item := range paths {
		files = append(files, SourceFile{
			SourceType:  "repository",
			SourceID:    repo.FullName,
			SourceURL:   "https://github.com/" + repo.FullName,
			Path:        item.Path,
			DownloadURL: buildRawRepoURL(c.cfg.GitHubRawBase, repo.FullName, repo.DefaultBranch, item.Path),
		})
	}
	return files, nil
}

func (c *GitHubClient) discoverGistFiles(ctx context.Context, gistID string) ([]SourceFile, error) {
	endpoint := fmt.Sprintf("%s/gists/%s", strings.TrimRight(c.cfg.GitHubAPIBase, "/"), gistID)
	var payload gistResponse
	if err := c.getJSON(ctx, endpoint, &payload); err != nil {
		return nil, err
	}

	keys := make([]string, 0, len(payload.Files))
	for key := range payload.Files {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	files := make([]SourceFile, 0, len(keys))
	for _, key := range keys {
		file := payload.Files[key]
		if file.Size <= 0 || file.Size > c.cfg.MaxFileBytes {
			continue
		}
		if !shouldInspectPath(file.Filename) {
			continue
		}
		files = append(files, SourceFile{
			SourceType:  "gist",
			SourceID:    gistID,
			SourceURL:   payload.HTMLURL,
			Path:        file.Filename,
			DownloadURL: file.RawURL,
		})
		if len(files) >= c.cfg.MaxFilesPerSource {
			break
		}
	}
	return files, nil
}

func (c *GitHubClient) getJSON(ctx context.Context, endpoint string, target any) error {
	body, err := c.getBytes(ctx, endpoint)
	if err != nil {
		return err
	}
	return json.Unmarshal(body, target)
}

func (c *GitHubClient) getText(ctx context.Context, endpoint string) (string, error) {
	body, err := c.getBytes(ctx, endpoint)
	if err != nil {
		return "", err
	}
	return string(body), nil
}

func (c *GitHubClient) getBytes(ctx context.Context, endpoint string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", c.cfg.UserAgent)
	if c.cfg.GitHubToken != "" && strings.HasPrefix(endpoint, strings.TrimRight(c.cfg.GitHubAPIBase, "/")) {
		req.Header.Set("Authorization", "Bearer "+c.cfg.GitHubToken)
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	c.counter.Inc()
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("request %s failed with %d: %s", endpoint, resp.StatusCode, strings.TrimSpace(string(body)))
	}

	return io.ReadAll(resp.Body)
}

func buildRawRepoURL(rawBase string, fullName string, branch string, path string) string {
	rawBase = strings.TrimRight(rawBase, "/")
	path = strings.ReplaceAll(path, " ", "%20")
	return fmt.Sprintf("%s/%s/%s/%s", rawBase, fullName, branch, path)
}

func sortedRepoKeys(items map[string]repoSearchItem) []string {
	keys := make([]string, 0, len(items))
	for key := range items {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func sortedStringKeys(items map[string]struct{}) []string {
	keys := make([]string, 0, len(items))
	for key := range items {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

type repoSearchResponse struct {
	Items []repoSearchItem `json:"items"`
}

type repoSearchItem struct {
	FullName      string `json:"full_name"`
	DefaultBranch string `json:"default_branch"`
}

type repoTreeResponse struct {
	Tree []repoTreeItem `json:"tree"`
}

type repoTreeItem struct {
	Path string `json:"path"`
	Type string `json:"type"`
	Size int64  `json:"size"`
}

type gistResponse struct {
	HTMLURL string                  `json:"html_url"`
	Files   map[string]gistFileInfo `json:"files"`
}

type gistFileInfo struct {
	Filename string `json:"filename"`
	RawURL   string `json:"raw_url"`
	Size     int64  `json:"size"`
}
