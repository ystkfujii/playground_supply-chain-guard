package github

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/ystkfujii/playground_supply-chain-guard/sbomreport-github-sync/internal/render"
)

type Client struct {
	baseURL    string
	token      string
	apiVersion string
	httpClient *http.Client
}

type CommitRequest struct {
	Owner         string
	Repo          string
	Branch        string
	Message       string
	Files         []render.File
	DeleteMissing bool
	PathPrefix    string
}

type CommitResult struct {
	Skipped   bool
	CommitSHA string
	CommitURL string
}

type refResponse struct {
	Ref    string `json:"ref"`
	Object struct {
		SHA string `json:"sha"`
	} `json:"object"`
}

type commitResponse struct {
	SHA     string `json:"sha"`
	HTMLURL string `json:"html_url"`
	Tree    struct {
		SHA string `json:"sha"`
	} `json:"tree"`
}

type blobResponse struct {
	SHA string `json:"sha"`
}

type treeEntry struct {
	Path string  `json:"path"`
	Mode string  `json:"mode,omitempty"`
	Type string  `json:"type,omitempty"`
	SHA  *string `json:"sha"`
}

type treeResponse struct {
	SHA  string `json:"sha"`
	Tree []struct {
		Path string `json:"path"`
		Type string `json:"type"`
		SHA  string `json:"sha"`
	} `json:"tree"`
	Truncated bool `json:"truncated"`
}

func NewClient(baseURL, token, apiVersion string) *Client {
	baseURL = strings.TrimRight(baseURL, "/")
	if baseURL == "" {
		baseURL = "https://api.github.com"
	}
	return &Client{
		baseURL:    baseURL,
		token:      token,
		apiVersion: apiVersion,
		httpClient: &http.Client{Timeout: 60 * time.Second},
	}
}

func (c *Client) CommitFiles(ctx context.Context, req CommitRequest) (*CommitResult, error) {
	if len(req.Files) == 0 && !req.DeleteMissing {
		return &CommitResult{Skipped: true}, nil
	}
	branch := req.Branch
	if branch == "" {
		branch = "main"
	}

	ref, err := c.getRef(ctx, req.Owner, req.Repo, branch)
	if err != nil {
		return nil, err
	}
	baseCommitSHA := ref.Object.SHA
	baseCommit, err := c.getCommit(ctx, req.Owner, req.Repo, baseCommitSHA)
	if err != nil {
		return nil, err
	}
	baseTreeSHA := baseCommit.Tree.SHA

	entries := make([]treeEntry, 0, len(req.Files))
	generatedPaths := map[string]struct{}{}
	for _, file := range req.Files {
		p := cleanRepoPath(file.Path)
		if p == "" {
			return nil, fmt.Errorf("empty repository path generated")
		}
		sha, err := c.createBlob(ctx, req.Owner, req.Repo, file.Content)
		if err != nil {
			return nil, fmt.Errorf("create blob for %s: %w", p, err)
		}
		generatedPaths[p] = struct{}{}
		entries = append(entries, treeEntry{
			Path: p,
			Mode: "100644",
			Type: "blob",
			SHA:  &sha,
		})
	}

	if req.DeleteMissing {
		prefix := cleanRepoPath(req.PathPrefix)
		if prefix == "" {
			return nil, fmt.Errorf("delete missing requires non-empty path prefix")
		}
		existingTree, err := c.getTreeRecursive(ctx, req.Owner, req.Repo, baseTreeSHA)
		if err != nil {
			return nil, err
		}
		if existingTree.Truncated {
			return nil, fmt.Errorf("GitHub returned a truncated tree; refusing to delete missing files under %q", prefix)
		}
		prefixWithSlash := prefix + "/"
		for _, item := range existingTree.Tree {
			if item.Type != "blob" || !strings.HasSuffix(item.Path, ".json") {
				continue
			}
			if item.Path == prefix || strings.HasPrefix(item.Path, prefixWithSlash) {
				if _, ok := generatedPaths[item.Path]; !ok {
					entries = append(entries, treeEntry{
						Path: item.Path,
						Mode: "100644",
						Type: "blob",
						SHA:  nil,
					})
				}
			}
		}
	}

	newTree, err := c.createTree(ctx, req.Owner, req.Repo, baseTreeSHA, entries)
	if err != nil {
		return nil, err
	}
	if newTree.SHA == baseTreeSHA {
		return &CommitResult{Skipped: true}, nil
	}

	newCommit, err := c.createCommit(ctx, req.Owner, req.Repo, req.Message, newTree.SHA, []string{baseCommitSHA})
	if err != nil {
		return nil, err
	}
	if err := c.updateRef(ctx, req.Owner, req.Repo, branch, newCommit.SHA); err != nil {
		return nil, err
	}

	return &CommitResult{CommitSHA: newCommit.SHA, CommitURL: newCommit.HTMLURL}, nil
}

func (c *Client) getRef(ctx context.Context, owner, repo, branch string) (*refResponse, error) {
	var out refResponse
	ref := "heads/" + branch
	apiPath := fmt.Sprintf("/repos/%s/%s/git/ref/%s", esc(owner), esc(repo), escRef(ref))
	if err := c.do(ctx, http.MethodGet, apiPath, nil, &out); err != nil {
		return nil, fmt.Errorf("get GitHub ref %s: %w", ref, err)
	}
	return &out, nil
}

func (c *Client) getCommit(ctx context.Context, owner, repo, sha string) (*commitResponse, error) {
	var out commitResponse
	apiPath := fmt.Sprintf("/repos/%s/%s/git/commits/%s", esc(owner), esc(repo), esc(sha))
	if err := c.do(ctx, http.MethodGet, apiPath, nil, &out); err != nil {
		return nil, fmt.Errorf("get GitHub commit %s: %w", sha, err)
	}
	return &out, nil
}

func (c *Client) createBlob(ctx context.Context, owner, repo string, content []byte) (string, error) {
	var out blobResponse
	in := map[string]string{
		"content":  base64.StdEncoding.EncodeToString(content),
		"encoding": "base64",
	}
	apiPath := fmt.Sprintf("/repos/%s/%s/git/blobs", esc(owner), esc(repo))
	if err := c.do(ctx, http.MethodPost, apiPath, in, &out); err != nil {
		return "", err
	}
	return out.SHA, nil
}

func (c *Client) getTreeRecursive(ctx context.Context, owner, repo, treeSHA string) (*treeResponse, error) {
	var out treeResponse
	apiPath := fmt.Sprintf("/repos/%s/%s/git/trees/%s?recursive=1", esc(owner), esc(repo), esc(treeSHA))
	if err := c.do(ctx, http.MethodGet, apiPath, nil, &out); err != nil {
		return nil, fmt.Errorf("get recursive tree %s: %w", treeSHA, err)
	}
	return &out, nil
}

func (c *Client) createTree(ctx context.Context, owner, repo, baseTreeSHA string, entries []treeEntry) (*treeResponse, error) {
	var out treeResponse
	in := map[string]any{
		"base_tree": baseTreeSHA,
		"tree":      entries,
	}
	apiPath := fmt.Sprintf("/repos/%s/%s/git/trees", esc(owner), esc(repo))
	if err := c.do(ctx, http.MethodPost, apiPath, in, &out); err != nil {
		return nil, fmt.Errorf("create GitHub tree: %w", err)
	}
	return &out, nil
}

func (c *Client) createCommit(ctx context.Context, owner, repo, message, treeSHA string, parents []string) (*commitResponse, error) {
	var out commitResponse
	in := map[string]any{
		"message": message,
		"tree":    treeSHA,
		"parents": parents,
	}
	apiPath := fmt.Sprintf("/repos/%s/%s/git/commits", esc(owner), esc(repo))
	if err := c.do(ctx, http.MethodPost, apiPath, in, &out); err != nil {
		return nil, fmt.Errorf("create GitHub commit: %w", err)
	}
	return &out, nil
}

func (c *Client) updateRef(ctx context.Context, owner, repo, branch, commitSHA string) error {
	in := map[string]any{
		"sha":   commitSHA,
		"force": false,
	}
	apiPath := fmt.Sprintf("/repos/%s/%s/git/refs/%s", esc(owner), esc(repo), escRef("heads/"+branch))
	if err := c.do(ctx, http.MethodPatch, apiPath, in, nil); err != nil {
		return fmt.Errorf("update GitHub ref heads/%s: %w", branch, err)
	}
	return nil
}

func (c *Client) do(ctx context.Context, method, apiPath string, in any, out any) error {
	var body io.Reader
	if in != nil {
		payload, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(payload)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+apiPath, body)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	if c.apiVersion != "" {
		req.Header.Set("X-GitHub-Api-Version", c.apiVersion)
	}
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 8192))
		return fmt.Errorf("%s %s: %s: %s", method, apiPath, resp.Status, strings.TrimSpace(string(b)))
	}
	if out == nil {
		io.Copy(io.Discard, resp.Body)
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decode GitHub response: %w", err)
	}
	return nil
}

func cleanRepoPath(p string) string {
	p = strings.TrimSpace(p)
	p = strings.Trim(p, "/")
	if p == "" || p == "." {
		return ""
	}
	return path.Clean(p)
}

func esc(s string) string {
	return url.PathEscape(s)
}

func escRef(s string) string {
	return strings.ReplaceAll(url.PathEscape(s), "%2F", "/")
}
