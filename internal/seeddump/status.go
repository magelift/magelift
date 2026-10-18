package seeddump

import (
	"errors"
	"fmt"
	"strings"
)

// ErrInvalidTransition is returned when a journal Mark* call is illegal for the
// current status (automation contract D-03).
var ErrInvalidTransition = errors.New("seed dump journal invalid transition")

// ErrMissingJournal is returned when Mark* is called and no journal file exists.
var ErrMissingJournal = errors.New("seed dump journal is missing")

func validateFailedReason(reason string) (string, error) {
	trimmed := strings.TrimSpace(reason)
	if trimmed == "" {
		return "", errors.New("seed dump failure reason is required")
	}
	return trimmed, nil
}

func canMarkImporting(from string) error {
	switch from {
	case StatusRecorded, StatusImporting, StatusFailed:
		return nil
	case StatusImported:
		return fmt.Errorf("%w: cannot move from %s to %s without operator retry path", ErrInvalidTransition, from, StatusImporting)
	default:
		return fmt.Errorf("%w: unknown status %q", ErrInvalidTransition, from)
	}
}

func canMarkImported(from string) error {
	switch from {
	case StatusImporting:
		return nil
	default:
		return fmt.Errorf("%w: cannot move from %s to %s", ErrInvalidTransition, from, StatusImported)
	}
}

func canMarkFailed(from string) error {
	switch from {
	case StatusImporting:
		return nil
	default:
		return fmt.Errorf("%w: cannot move from %s to %s", ErrInvalidTransition, from, StatusFailed)
	}
}
