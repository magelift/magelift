package githubci

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
)

// EnsureVariableResult reports whether a variable was created or left unchanged.
// Variable values are never returned.
type EnsureVariableResult struct {
	Name    string
	Created bool
}

type variableResponse struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type createVariableRequest struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// EnsureVariable creates the allowlisted repository variable when absent, or
// leaves it unchanged when it already holds the same value. Existing variables
// with a different value produce ErrVariableConflict without revealing the
// stored value.
func (c *Client) EnsureVariable(ctx context.Context, owner, repo, name, value string) (EnsureVariableResult, error) {
	if err := validateVariableName(name); err != nil {
		return EnsureVariableResult{}, err
	}
	if err := validateVariableValue(value); err != nil {
		return EnsureVariableResult{}, err
	}
	base, err := repoPath(owner, repo)
	if err != nil {
		return EnsureVariableResult{}, err
	}
	existing, found, err := c.getVariable(ctx, base, name)
	if err != nil {
		return EnsureVariableResult{}, err
	}
	if found {
		if existing == value {
			return EnsureVariableResult{Name: name, Created: false}, nil
		}
		return EnsureVariableResult{}, &ErrVariableConflict{Name: name}
	}
	created, err := c.createVariable(ctx, base, name, value)
	if err != nil {
		return EnsureVariableResult{}, err
	}
	if created {
		return EnsureVariableResult{Name: name, Created: true}, nil
	}
	// POST raced or conflicted; reread once.
	existing, found, err = c.getVariable(ctx, base, name)
	if err != nil {
		return EnsureVariableResult{}, err
	}
	if !found {
		return EnsureVariableResult{}, errors.New("github repository variable is missing after create")
	}
	if existing == value {
		return EnsureVariableResult{Name: name, Created: false}, nil
	}
	return EnsureVariableResult{}, &ErrVariableConflict{Name: name}
}

func (c *Client) getVariable(ctx context.Context, repoBase, name string) (string, bool, error) {
	path := repoBase + "/actions/variables/" + url.PathEscape(name)
	status, body, err := c.do(ctx, http.MethodGet, path, nil)
	if err != nil {
		return "", false, err
	}
	switch status {
	case http.StatusNotFound:
		return "", false, nil
	case http.StatusForbidden:
		return "", false, errors.New("github api denied variable access")
	case http.StatusOK:
		var payload variableResponse
		if err := decodeJSONObject(body, &payload); err != nil {
			return "", false, err
		}
		if payload.Name != name {
			return "", false, errors.New("github variable name in response does not match request")
		}
		return payload.Value, true, nil
	default:
		return "", false, fmt.Errorf("github api returned unexpected status %d", status)
	}
}

func (c *Client) createVariable(ctx context.Context, repoBase, name, value string) (bool, error) {
	payload, err := json.Marshal(createVariableRequest{Name: name, Value: value})
	if err != nil {
		return false, errors.New("encode variable create request failed")
	}
	path := repoBase + "/actions/variables"
	status, _, err := c.do(ctx, http.MethodPost, path, payload)
	if err != nil {
		return false, err
	}
	switch status {
	case http.StatusCreated:
		return true, nil
	case http.StatusConflict, http.StatusUnprocessableEntity:
		return false, nil
	case http.StatusForbidden:
		return false, errors.New("github api denied variable access")
	default:
		return false, fmt.Errorf("github api returned unexpected status %d", status)
	}
}
