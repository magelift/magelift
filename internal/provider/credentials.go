package provider

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/magelift/magelift/sdk"
)

// CredentialResolver executes a callback while the resolved credential is
// held by the provider boundary. Implementations must not return or retain
// the secret, and callers must not copy it into plans, logs, checkpoints, or
// evidence.
type CredentialResolver interface {
	Use(context.Context, string, func([]byte) error) error
}

// CredentialSink stores a provider-generated replacement at an opaque
// reference. Secret values must be consumed within the provider boundary and
// must never be returned through SDK plans, results, checkpoints, or errors.
type CredentialSink interface {
	Store(context.Context, string, []byte) error
}

// CredentialResolverFunc adapts a function to CredentialResolver. The
// function still owns the secret lifetime and should clear temporary buffers
// after the callback returns where the underlying SDK permits it.
type CredentialResolverFunc func(context.Context, string, func([]byte) error) error

func (f CredentialResolverFunc) Use(ctx context.Context, reference string, consume func([]byte) error) error {
	if f == nil {
		return errors.New("credential resolver function is required")
	}
	return f(ctx, reference, consume)
}

// UseCredential validates an opaque reference and invokes the provider-owned
// resolver without allowing secret-shaped values into the returned error.
func UseCredential(ctx context.Context, resolver CredentialResolver, reference string, consume func([]byte) error) error {
	if ctx == nil {
		return errors.New("credential context is required")
	}
	if resolver == nil {
		return errors.New("credential resolver is required")
	}
	if consume == nil {
		return errors.New("credential consumer is required")
	}
	if err := sdk.ValidateCredentialReference(reference); err != nil {
		return fmt.Errorf("validate credential reference: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := resolver.Use(ctx, reference, func(value []byte) error {
		if value == nil {
			return errors.New("credential resolver returned no value")
		}
		if err := consume(value); err != nil {
			// The consumer is provider code and may have received SDK text. Do
			// not wrap it here; provider adapters must normalize their own API
			// errors before they cross this boundary.
			return errors.New("credential consumer failed")
		}
		return nil
	}); err != nil {
		if contextErr := ctx.Err(); contextErr != nil {
			return contextErr
		}
		return fmt.Errorf("resolve credential reference %q: credential provider failed", strings.TrimSpace(reference))
	}
	return nil
}
