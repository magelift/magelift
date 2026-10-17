package githubci

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	apiOrigin           = "https://api.github.com"
	githubAPIVersion    = "2026-03-10"
	defaultRequestLimit = 30 * time.Second
	maxResponseBytes    = 1 << 20
	userAgent           = "magelift-githubci"
)

// HTTPDoer performs GitHub REST requests. Tests inject a custom implementation;
// production callers may pass nil to use a bounded default client.
type HTTPDoer interface {
	Do(*http.Request) (*http.Response, error)
}

// Client calls the GitHub REST API for repository preparation tasks.
type Client struct {
	token string
	doer  HTTPDoer
}

// New returns a client that always targets https://api.github.com. The token is
// held for Authorization headers only and is not exposed through JSON or String.
func New(token string, doer HTTPDoer) (*Client, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, errors.New("github token is required")
	}
	if doer == nil {
		doer = &http.Client{
			Timeout: defaultRequestLimit,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		}
	}
	return &Client{token: token, doer: doer}, nil
}

func (c *Client) newRequest(ctx context.Context, method, path string, body []byte) (*http.Request, error) {
	if c == nil || c.doer == nil {
		return nil, errors.New("github client is not configured")
	}
	path = strings.TrimSpace(path)
	if path == "" || !strings.HasPrefix(path, "/") {
		return nil, errors.New("github api path is invalid")
	}
	endpoint := apiOrigin + path
	var reader io.Reader
	if len(body) > 0 {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, reader)
	if err != nil {
		return nil, fmt.Errorf("create github api request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("X-GitHub-Api-Version", githubAPIVersion)
	req.Header.Set("User-Agent", userAgent)
	if len(body) > 0 {
		req.Header.Set("Content-Type", "application/json")
	}
	return req, nil
}

func (c *Client) do(ctx context.Context, method, path string, body []byte) (int, []byte, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultRequestLimit)
	defer cancel()
	req, err := c.newRequest(ctx, method, path, body)
	if err != nil {
		return 0, nil, err
	}
	resp, err := c.doer.Do(req)
	if err != nil {
		return 0, nil, ErrRequestFailed
	}
	defer resp.Body.Close()
	limited := io.LimitReader(resp.Body, maxResponseBytes+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return 0, nil, ErrRequestFailed
	}
	if len(data) > maxResponseBytes {
		return 0, nil, errors.New("github api response exceeds size limit")
	}
	return resp.StatusCode, data, nil
}

func repoPath(owner, repo string) (string, error) {
	if err := validateRepoSegment(owner, "owner"); err != nil {
		return "", err
	}
	if err := validateRepoSegment(repo, "repository"); err != nil {
		return "", err
	}
	return "/repos/" + url.PathEscape(owner) + "/" + url.PathEscape(repo), nil
}

func decodeJSONObject(data []byte, dest any) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	if err := dec.Decode(dest); err != nil {
		return errors.New("github api response is malformed")
	}
	var trailing any
	if err := dec.Decode(&trailing); !errors.Is(err, io.EOF) {
		return errors.New("github api response has trailing data")
	}
	return nil
}
