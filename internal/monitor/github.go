package monitor

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// GitHub is a minimal REST client for the few calls the monitor makes against the
// powder-hound repo: read a file, open a PR that changes it, and open or find issues.
type GitHub struct {
	http    *http.Client
	baseURL string
	token   string
	repo    string // owner/name
	// Base is the branch PRs target (master for production, dev for the dev environment).
	Base string
}

func NewGitHub() (*GitHub, error) {
	token := os.Getenv("GITHUB_TOKEN")
	repo := os.Getenv("GITHUB_REPO")
	if token == "" || repo == "" {
		return nil, errors.New("GITHUB_TOKEN and GITHUB_REPO must be set")
	}
	base := os.Getenv("GITHUB_BASE_BRANCH")
	if base == "" {
		base = "master"
	}
	return &GitHub{
		http:    &http.Client{Timeout: 30 * time.Second},
		baseURL: "https://api.github.com",
		token:   token,
		repo:    repo,
		Base:    base,
	}, nil
}

type Issue struct {
	Number  int    `json:"number"`
	Title   string `json:"title"`
	Body    string `json:"body"`
	HTMLURL string `json:"html_url"`
	// The issues endpoint also returns PRs; those carry this field.
	PullRequest *struct{} `json:"pull_request"`
}

type PullRequest struct {
	Number  int    `json:"number"`
	Title   string `json:"title"`
	Body    string `json:"body"`
	HTMLURL string `json:"html_url"`
}

func (g *GitHub) do(ctx context.Context, method, path string, body, out any) error {
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, g.baseURL+path, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+g.token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := g.http.Do(req)
	if err != nil {
		return fmt.Errorf("github %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode >= 300 {
		return &githubError{Status: resp.StatusCode, Method: method, Path: path, Body: truncateBody(string(data))}
	}
	if out != nil && len(data) > 0 {
		return json.Unmarshal(data, out)
	}
	return nil
}

type githubError struct {
	Status       int
	Method, Path string
	Body         string
}

func (e *githubError) Error() string {
	return fmt.Sprintf("github %s %s: %d %s", e.Method, e.Path, e.Status, e.Body)
}

func truncateBody(s string) string {
	if len(s) > 500 {
		return s[:500] + "…"
	}
	return s
}

// GetFile returns a file's contents and blob SHA on the given branch.
func (g *GitHub) GetFile(ctx context.Context, path, branch string) ([]byte, string, error) {
	var file struct {
		Content  string `json:"content"`
		Encoding string `json:"encoding"`
		SHA      string `json:"sha"`
	}
	err := g.do(ctx, http.MethodGet, fmt.Sprintf("/repos/%s/contents/%s?ref=%s", g.repo, path, url.QueryEscape(branch)), nil, &file)
	if err != nil {
		return nil, "", err
	}
	content, err := base64.StdEncoding.DecodeString(strings.ReplaceAll(file.Content, "\n", ""))
	if err != nil {
		return nil, "", fmt.Errorf("decode %s: %w", path, err)
	}
	return content, file.SHA, nil
}

// OpenFilePR creates branch from Base, commits content to path on it, and opens a PR.
func (g *GitHub) OpenFilePR(ctx context.Context, branch, path string, content []byte, commitMsg, title, body string, labels []string) (*PullRequest, error) {
	var ref struct {
		Object struct {
			SHA string `json:"sha"`
		} `json:"object"`
	}
	if err := g.do(ctx, http.MethodGet, fmt.Sprintf("/repos/%s/git/ref/heads/%s", g.repo, g.Base), nil, &ref); err != nil {
		return nil, err
	}
	if err := g.do(ctx, http.MethodPost, fmt.Sprintf("/repos/%s/git/refs", g.repo), map[string]string{
		"ref": "refs/heads/" + branch,
		"sha": ref.Object.SHA,
	}, nil); err != nil {
		return nil, err
	}

	_, fileSHA, err := g.GetFile(ctx, path, branch)
	if err != nil {
		return nil, err
	}
	if err := g.do(ctx, http.MethodPut, fmt.Sprintf("/repos/%s/contents/%s", g.repo, path), map[string]string{
		"message": commitMsg,
		"content": base64.StdEncoding.EncodeToString(content),
		"sha":     fileSHA,
		"branch":  branch,
	}, nil); err != nil {
		return nil, err
	}

	var pr PullRequest
	if err := g.do(ctx, http.MethodPost, fmt.Sprintf("/repos/%s/pulls", g.repo), map[string]string{
		"title": title,
		"head":  branch,
		"base":  g.Base,
		"body":  body,
	}, &pr); err != nil {
		return nil, err
	}
	if len(labels) > 0 {
		if err := g.do(ctx, http.MethodPost, fmt.Sprintf("/repos/%s/issues/%d/labels", g.repo, pr.Number), map[string][]string{"labels": labels}, nil); err != nil {
			return &pr, fmt.Errorf("label PR #%d: %w", pr.Number, err)
		}
	}
	return &pr, nil
}

// OpenPRsWithMarker returns open PRs whose body contains marker.
func (g *GitHub) OpenPRsWithMarker(ctx context.Context, marker string) ([]PullRequest, error) {
	var prs []PullRequest
	if err := g.do(ctx, http.MethodGet, fmt.Sprintf("/repos/%s/pulls?state=open&per_page=100", g.repo), nil, &prs); err != nil {
		return nil, err
	}
	var matches []PullRequest
	for _, pr := range prs {
		if strings.Contains(pr.Body, marker) {
			matches = append(matches, pr)
		}
	}
	return matches, nil
}

// OpenIssuesWithMarker returns open issues (not PRs) carrying label whose body contains marker.
func (g *GitHub) OpenIssuesWithMarker(ctx context.Context, label, marker string) ([]Issue, error) {
	var issues []Issue
	path := fmt.Sprintf("/repos/%s/issues?state=open&per_page=100&labels=%s", g.repo, url.QueryEscape(label))
	if err := g.do(ctx, http.MethodGet, path, nil, &issues); err != nil {
		return nil, err
	}
	var matches []Issue
	for _, issue := range issues {
		if issue.PullRequest == nil && strings.Contains(issue.Body, marker) {
			matches = append(matches, issue)
		}
	}
	return matches, nil
}

func (g *GitHub) CreateIssue(ctx context.Context, title, body string, labels []string) (*Issue, error) {
	var issue Issue
	err := g.do(ctx, http.MethodPost, fmt.Sprintf("/repos/%s/issues", g.repo), map[string]any{
		"title":  title,
		"body":   body,
		"labels": labels,
	}, &issue)
	if err != nil {
		return nil, err
	}
	return &issue, nil
}
