// Package newrelic contains the New Relic-specific edge of the observability
// integration. The portable observability planner owns signals and evidence;
// this package only translates an OTLP protobuf payload into a New Relic
// OTLP/HTTP request.
package newrelic

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	provider "github.com/magelift/magelift/internal/provider"
	"google.golang.org/protobuf/proto"
)

// Signal identifies one OTLP signal. It is deliberately narrower than the
// portable signal strings so an exporter cannot silently send an unsupported
// payload to the wrong endpoint.
type Signal string

const (
	SignalTraces  Signal = "traces"
	SignalMetrics Signal = "metrics"
	SignalLogs    Signal = "logs"
)

// ExportRequest contains only provider-neutral OTLP data and references. The
// API key is resolved inside Export and never appears in this type.
type ExportRequest struct {
	Endpoint        string
	CredentialRef   string
	Signal          Signal
	Payload         proto.Message
	OwnershipMarker string
}

// ExportResult contains identities safe to place in checkpoints and evidence.
// New Relic validates authentication and payload size synchronously, but it
// validates payload contents asynchronously; Accepted is therefore not proof
// of eventual signal delivery or retention.
type ExportResult struct {
	Signal          Signal
	Endpoint        string
	StatusCode      int
	Accepted        bool
	OperationID     string
	OwnershipMarker string
}

// HTTPDoer is the smallest transport seam needed for deterministic tests and
// approved private-endpoint clients.
type HTTPDoer interface {
	Do(*http.Request) (*http.Response, error)
}

// RetryPolicy bounds provider API retries or eventual-consistency polling.
// OTLP retry decisions are based only on transport errors and status codes;
// response bodies are never copied into an error or result.
type RetryPolicy struct {
	MaxAttempts  int
	InitialDelay time.Duration
	MaxDelay     time.Duration
}

func (policy RetryPolicy) validate() error {
	if policy.MaxAttempts < 1 || policy.MaxAttempts > 10 {
		return errors.New("New Relic retry attempts must be between 1 and 10")
	}
	if policy.InitialDelay < 0 || policy.MaxDelay < policy.InitialDelay {
		return errors.New("New Relic retry delays are invalid")
	}
	return nil
}

// Client sends OTLP/HTTP protobuf requests to New Relic. It has no New Relic
// object lifecycle because OTLP ingestion is data-plane delivery, not a
// provider-owned resource that the core can safely model as a dashboard or
// alert object.
type Client struct {
	httpClient HTTPDoer
	resolver   provider.CredentialResolver
	retry      RetryPolicy
}

var _ interface {
	Export(context.Context, ExportRequest) (ExportResult, error)
} = (*Client)(nil)

// NewClient creates a production client with a bounded default HTTP retry
// policy. A nil HTTP client uses a transport with a finite request timeout.
func NewClient(httpClient HTTPDoer, resolver provider.CredentialResolver) (*Client, error) {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	if resolver == nil {
		return nil, errors.New("New Relic credential resolver is required")
	}
	return NewClientWithRetry(httpClient, resolver, RetryPolicy{MaxAttempts: 3, InitialDelay: 250 * time.Millisecond, MaxDelay: 2 * time.Second})
}

// NewClientWithRetry is the injectable constructor used by tests and by
// callers with an explicitly chosen bounded retry budget.
func NewClientWithRetry(httpClient HTTPDoer, resolver provider.CredentialResolver, retry RetryPolicy) (*Client, error) {
	if httpClient == nil {
		return nil, errors.New("New Relic HTTP client is required")
	}
	if resolver == nil {
		return nil, errors.New("New Relic credential resolver is required")
	}
	if err := retry.validate(); err != nil {
		return nil, err
	}
	return &Client{httpClient: httpClient, resolver: resolver, retry: retry}, nil
}

// Export sends one OTLP/HTTP binary protobuf payload. Credentials are held by
// the callback scope only, and all provider failures are normalized before
// they cross the provider boundary.
func (client *Client) Export(ctx context.Context, request ExportRequest) (ExportResult, error) {
	if ctx == nil {
		return ExportResult{}, errors.New("New Relic export context is required")
	}
	if client == nil || client.httpClient == nil || client.resolver == nil {
		return ExportResult{}, errors.New("New Relic client is required")
	}
	if err := validateSignal(request.Signal); err != nil {
		return ExportResult{}, err
	}
	if request.Payload == nil {
		return ExportResult{}, errors.New("New Relic OTLP payload is required")
	}
	if err := validateOwnershipMarker(request.OwnershipMarker); err != nil {
		return ExportResult{}, err
	}
	endpoint, err := endpointForSignal(request.Endpoint, request.Signal)
	if err != nil {
		return ExportResult{}, err
	}
	payload, err := proto.Marshal(request.Payload)
	if err != nil {
		return ExportResult{}, errors.New("marshal New Relic OTLP payload failed")
	}
	digest := sha256.Sum256(payload)
	operationID := "newrelic-otlp:" + string(request.Signal) + ":" + hex.EncodeToString(digest[:8])
	result := ExportResult{
		Signal:          request.Signal,
		Endpoint:        endpoint,
		OperationID:     operationID,
		OwnershipMarker: request.OwnershipMarker,
	}

	var sendErr error
	err = provider.UseCredential(ctx, client.resolver, request.CredentialRef, func(apiKey []byte) error {
		statusCode, err := client.send(ctx, endpoint, request.Signal, payload, apiKey)
		result.StatusCode = statusCode
		sendErr = err
		if err != nil {
			return err
		}
		result.Accepted = true
		return nil
	})
	if err != nil {
		if result.StatusCode != 0 {
			return ExportResult{}, fmt.Errorf("New Relic OTLP request failed with status %d", result.StatusCode)
		}
		if sendErr != nil {
			return ExportResult{}, fmt.Errorf("New Relic OTLP request failed: %w", sendErr)
		}
		return ExportResult{}, err
	}
	return result, nil
}

func (client *Client) send(ctx context.Context, endpoint string, signal Signal, payload, apiKey []byte) (int, error) {
	var lastStatus int
	var lastErr error
	for attempt := 0; attempt < client.retry.MaxAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return 0, err
		}
		request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
		if err != nil {
			return 0, errors.New("build New Relic OTLP request failed")
		}
		request.Header.Set("Content-Type", "application/x-protobuf")
		request.Header.Set("Accept", "application/x-protobuf")
		request.Header.Set("api-key", string(apiKey))
		response, err := client.httpClient.Do(request)
		if err != nil {
			if contextErr := ctx.Err(); contextErr != nil {
				return 0, contextErr
			}
			lastErr = errors.New("New Relic OTLP request failed")
			if attempt+1 < client.retry.MaxAttempts {
				if err := wait(ctx, client.retry.delay(attempt)); err != nil {
					return 0, err
				}
				continue
			}
			return 0, lastErr
		}
		if response == nil {
			lastErr = errors.New("New Relic OTLP request returned no response")
			if attempt+1 < client.retry.MaxAttempts {
				if err := wait(ctx, client.retry.delay(attempt)); err != nil {
					return 0, err
				}
				continue
			}
			return 0, lastErr
		}
		lastStatus = response.StatusCode
		if response.Body != nil {
			_, _ = io.Copy(io.Discard, response.Body)
			_ = response.Body.Close()
		}
		if response.StatusCode >= http.StatusOK && response.StatusCode < http.StatusMultipleChoices {
			return response.StatusCode, nil
		}
		lastErr = fmt.Errorf("New Relic OTLP request returned status %d", response.StatusCode)
		if !retryableStatus(response.StatusCode) || attempt+1 >= client.retry.MaxAttempts {
			return response.StatusCode, lastErr
		}
		if err := wait(ctx, client.retry.delay(attempt)); err != nil {
			return 0, err
		}
	}
	return lastStatus, lastErr
}

func (policy RetryPolicy) delay(attempt int) time.Duration {
	delay := policy.InitialDelay
	for index := 0; index < attempt; index++ {
		if delay >= policy.MaxDelay/2 && policy.MaxDelay > 0 {
			return policy.MaxDelay
		}
		delay *= 2
	}
	if delay > policy.MaxDelay {
		return policy.MaxDelay
	}
	return delay
}

func wait(ctx context.Context, delay time.Duration) error {
	if delay <= 0 {
		return nil
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func retryableStatus(status int) bool {
	return status == http.StatusRequestTimeout || status == http.StatusTooEarly || status == http.StatusTooManyRequests || status >= http.StatusInternalServerError
}

func validateSignal(signal Signal) error {
	switch signal {
	case SignalTraces, SignalMetrics, SignalLogs:
		return nil
	default:
		return fmt.Errorf("unsupported New Relic OTLP signal %q", signal)
	}
}

func validateOwnershipMarker(marker string) error {
	if strings.TrimSpace(marker) == "" || strings.ContainsAny(marker, "\r\n\x00") {
		return errors.New("New Relic ownership marker is required and must be single-line")
	}
	return nil
}

func endpointForSignal(raw string, signal Signal) (string, error) {
	if err := validateSignal(signal); err != nil {
		return "", err
	}
	base, err := normalizeEndpoint(raw)
	if err != nil {
		return "", err
	}
	parsed, _ := url.Parse(base)
	suffix := "/v1/" + string(signal)
	path := strings.TrimRight(parsed.Path, "/")
	switch {
	case path == "":
		path += suffix
	case path == suffix:
		// The caller supplied the signal-specific OTLP/HTTP endpoint.
	case strings.HasPrefix(path, "/v1/"):
		return "", fmt.Errorf("New Relic OTLP endpoint path %q does not match signal %q", parsed.Path, signal)
	default:
		return "", fmt.Errorf("New Relic OTLP endpoint path %q must be empty or %s", parsed.Path, suffix)
	}
	parsed.Path = path
	parsed.RawPath = ""
	return parsed.String(), nil
}

func normalizeEndpoint(raw string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", errors.New("New Relic OTLP endpoint must be an HTTPS URL without credentials, query, or fragment")
	}
	parsed.RawPath = ""
	return strings.TrimRight(parsed.String(), "/"), nil
}
