package automation

import (
	"errors"
	"strings"

	"github.com/pulumi/pulumi/sdk/v3/go/auto"
)

// ConcurrentUpdateError marks a backend failure caused by another Pulumi
// update holding the stack lock. The CLI uses it to return a distinct exit
// code. Unwrap preserves the Pulumi error.
type ConcurrentUpdateError struct {
	Cause error
}

func (e *ConcurrentUpdateError) Error() string {
	return "another Pulumi update is currently in progress on this stack; wait for it to finish, then retry"
}

func (e *ConcurrentUpdateError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

// Lock text fragments copied from auto.IsConcurrentUpdateError
// (pulumi/sdk v3.262.0, go/auto/errors.go). autoError has no exported
// constructor, so mocks cannot build a real one; the fallback keeps the
// classification testable and also catches lock text that crossed the
// subprocess boundary as a plain string. Re-check these fragments when
// bumping the Pulumi SDK.
const (
	concurrentUpdateConflictText = "[409] Conflict: Another update is currently in progress."
	concurrentUpdateLockText     = "the stack is currently locked by"
)

// IsConcurrentUpdate reports whether err is a Pulumi stack lock or 409
// conflict: first Pulumi's own predicate (which unwraps typed errors),
// then our own typed error, then the documented stderr fragments.
func IsConcurrentUpdate(err error) bool {
	if err == nil {
		return false
	}
	if auto.IsConcurrentUpdateError(err) {
		return true
	}
	var concurrentErr *ConcurrentUpdateError
	if errors.As(err, &concurrentErr) {
		return true
	}
	text := err.Error()
	return strings.Contains(text, concurrentUpdateConflictText) ||
		strings.Contains(text, concurrentUpdateLockText)
}
