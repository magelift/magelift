// Package usererr formats operator-facing CLI errors with a cause, next step,
// and optional docs anchor. Keep messages short and concrete — agencies reading
// stderr should know what to do without digging through stack traces.
package usererr

import (
	"errors"
	"fmt"
	"strings"
)

// Error is a structured CLI failure for humans.
type Error struct {
	Cause string
	Next  string
	Doc   string
	err   error
}

func (e *Error) Error() string {
	var b strings.Builder
	b.WriteString(e.Cause)
	if e.Next != "" {
		b.WriteString("\nNext: ")
		b.WriteString(e.Next)
	}
	if e.Doc != "" {
		b.WriteString("\nDocs: ")
		b.WriteString(e.Doc)
	}
	return b.String()
}

func (e *Error) Unwrap() error { return e.err }

// New builds an Error. cause is required.
func New(cause, next, doc string) *Error {
	return &Error{Cause: cause, Next: next, Doc: doc}
}

// Wrap adds user guidance around an existing error.
func Wrap(err error, cause, next, doc string) *Error {
	if err == nil {
		return New(cause, next, doc)
	}
	return &Error{Cause: cause, Next: next, Doc: doc, err: err}
}

// Format is a convenience for fmt.Errorf-style causes with guidance.
func Format(next, doc, format string, args ...any) *Error {
	return New(fmt.Sprintf(format, args...), next, doc)
}

// As extracts *Error from an error chain.
func As(err error) (*Error, bool) {
	var target *Error
	ok := errors.As(err, &target)
	return target, ok
}
