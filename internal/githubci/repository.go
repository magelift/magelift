package githubci

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
)

// Repository is immutable metadata returned by ResolveRepository.
type Repository struct {
	ID            int64
	OwnerID       int64
	FullName      string
	OwnerLogin    string
	Name          string
	DefaultBranch string
}

type repoAPIResponse struct {
	ID            int64  `json:"id"`
	Name          string `json:"name"`
	FullName      string `json:"full_name"`
	DefaultBranch string `json:"default_branch"`
	Owner         struct {
		ID    int64  `json:"id"`
		Login string `json:"login"`
	} `json:"owner"`
}

// ResolveRepository loads canonical repository metadata for the requested owner
// and name. Renamed or transferred repositories are rejected when the response
// does not match the requested segments under EqualFold.
func (c *Client) ResolveRepository(ctx context.Context, owner, repo string) (Repository, error) {
	path, err := repoPath(owner, repo)
	if err != nil {
		return Repository{}, err
	}
	status, body, err := c.do(ctx, http.MethodGet, path, nil)
	if err != nil {
		return Repository{}, err
	}
	if status == http.StatusForbidden {
		return Repository{}, errors.New("github api denied repository access")
	}
	if status == http.StatusNotFound {
		return Repository{}, errors.New("github repository was not found")
	}
	if status != http.StatusOK {
		return Repository{}, fmt.Errorf("github api returned unexpected status %d", status)
	}
	var payload repoAPIResponse
	if err := decodeJSONObject(body, &payload); err != nil {
		return Repository{}, err
	}
	if payload.ID <= 0 || payload.Owner.ID <= 0 {
		return Repository{}, errors.New("github repository id is invalid")
	}
	if strings.TrimSpace(payload.FullName) == "" ||
		strings.TrimSpace(payload.Name) == "" ||
		strings.TrimSpace(payload.Owner.Login) == "" ||
		strings.TrimSpace(payload.DefaultBranch) == "" {
		return Repository{}, errors.New("github repository metadata is incomplete")
	}
	if !strings.EqualFold(payload.Owner.Login, owner) || !strings.EqualFold(payload.Name, repo) {
		return Repository{}, errors.New("github repository owner or name does not match request")
	}
	expectedFull := payload.Owner.Login + "/" + payload.Name
	if !strings.EqualFold(payload.FullName, expectedFull) {
		return Repository{}, errors.New("github repository full_name is inconsistent")
	}
	return Repository{
		ID:            payload.ID,
		OwnerID:       payload.Owner.ID,
		FullName:      payload.FullName,
		OwnerLogin:    payload.Owner.Login,
		Name:          payload.Name,
		DefaultBranch: payload.DefaultBranch,
	}, nil
}
