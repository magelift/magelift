// Package auth provides fresh GCP credentials per operation. Implementations
// must never depend on saved tokens: every operation resolves ambient
// credentials (ADC, including CI workload identity) and refreshes expired
// tokens through the platform token source. Refresh failure surfaces a typed
// credential error; no cached-token fallback exists.
package auth

import "context"

// TokenRefresher resolves a fresh access token for GCP API calls.
type TokenRefresher interface {
	// Token returns a valid access token, refreshing when expired.
	Token(ctx context.Context) (string, error)
}
