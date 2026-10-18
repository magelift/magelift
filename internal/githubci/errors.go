package githubci

import (
	"errors"
	"fmt"
)

// ErrRequestFailed indicates the HTTP transport did not complete. Underlying
// network errors are not wrapped to avoid leaking sensitive content.
var ErrRequestFailed = errors.New("github api request failed")

// ErrVariableConflict means a repository variable already exists with a
// different value. The conflicting value is never included in the error.
type ErrVariableConflict struct {
	Name string
}

func (e *ErrVariableConflict) Error() string {
	if e == nil || e.Name == "" {
		return "repository variable already exists with a different value"
	}
	return fmt.Sprintf("repository variable %q already exists with a different value", e.Name)
}
