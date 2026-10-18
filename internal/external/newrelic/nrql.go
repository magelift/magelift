package newrelic

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	provider "github.com/magelift/magelift/internal/provider"
	"github.com/magelift/magelift/sdk"
)

// MarkerQueryRequest identifies one OTLP probe without carrying a provider
// secret or a provider-specific response type into the shared lifecycle.
type MarkerQueryRequest struct {
	Signal          Signal
	CredentialRef   string
	OwnershipMarker string
}

// MarkerQueryResult is the smallest proof needed by the shared observability
// lifecycle: an exact ownership-marker match was visible through NRQL.
type MarkerQueryResult struct {
	Count          int64
	LabelsVerified bool
}

// MarkerQueryer is the provider-owned seam used to turn OTLP acceptance into
// delayed, queryable delivery evidence. The core knows only this semantic
// contract, not NerdGraph or NRQL.
type MarkerQueryer interface {
	QueryMarker(context.Context, MarkerQueryRequest) (MarkerQueryResult, error)
}

// NRQLClient verifies OTLP probe visibility through New Relic NerdGraph. It
// does not create or delete New Relic resources; data-plane retention remains
// owned by the New Relic account policy.
type NRQLClient struct {
	httpClient    HTTPDoer
	resolver      provider.CredentialResolver
	credentialRef string
	endpoint      string
	accountID     int64
	retry         RetryPolicy
}

var _ MarkerQueryer = (*NRQLClient)(nil)

// NewNRQLClient creates a production query verifier with a bounded polling
// budget. The first request is immediate; subsequent empty results are
// retried for at most roughly 2 minutes before returning an unobserved
// result. This accommodates the longer visibility delay of newly ingested
// trace spans while successful log and metric checks still return immediately.
func NewNRQLClient(httpClient HTTPDoer, resolver provider.CredentialResolver, endpoint string, accountID int64) (*NRQLClient, error) {
	return NewNRQLClientWithQueryCredential(httpClient, resolver, endpoint, accountID, "")
}

// NewNRQLClientWithQueryCredential configures a separate user-key reference
// for NerdGraph. New Relic license keys are data-plane credentials and are not
// sufficient for NerdGraph queries; an empty reference keeps the injectable
// test/backward-compatible path that uses the request credential.
func NewNRQLClientWithQueryCredential(httpClient HTTPDoer, resolver provider.CredentialResolver, endpoint string, accountID int64, credentialRef string) (*NRQLClient, error) {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	return NewNRQLClientWithQueryCredentialAndRetry(httpClient, resolver, endpoint, accountID, credentialRef, RetryPolicy{
		MaxAttempts:  10,
		InitialDelay: 5 * time.Second,
		MaxDelay:     15 * time.Second,
	})
}

// NewNRQLClientWithRetry is the injectable constructor for deterministic
// tests and callers with an explicit certification polling budget.
func NewNRQLClientWithRetry(httpClient HTTPDoer, resolver provider.CredentialResolver, endpoint string, accountID int64, retry RetryPolicy) (*NRQLClient, error) {
	return NewNRQLClientWithQueryCredentialAndRetry(httpClient, resolver, endpoint, accountID, "", retry)
}

// NewNRQLClientWithQueryCredentialAndRetry is the fully injectable
// constructor for tests and certification callers with separate ingest and
// NerdGraph credentials.
func NewNRQLClientWithQueryCredentialAndRetry(httpClient HTTPDoer, resolver provider.CredentialResolver, endpoint string, accountID int64, credentialRef string, retry RetryPolicy) (*NRQLClient, error) {
	if httpClient == nil {
		return nil, errors.New("New Relic NRQL HTTP client is required")
	}
	if resolver == nil {
		return nil, errors.New("New Relic NRQL credential resolver is required")
	}
	if accountID <= 0 || accountID > (1<<31)-1 {
		return nil, errors.New("New Relic account ID must fit the NerdGraph account ID range")
	}
	if credentialRef != "" {
		if err := sdk.ValidateCredentialReference(credentialRef); err != nil {
			return nil, fmt.Errorf("New Relic NRQL credential reference: %w", err)
		}
	}
	endpoint, err := normalizeNerdGraphEndpoint(endpoint)
	if err != nil {
		return nil, err
	}
	if err := retry.validate(); err != nil {
		return nil, err
	}
	return &NRQLClient{httpClient: httpClient, resolver: resolver, credentialRef: credentialRef, endpoint: endpoint, accountID: accountID, retry: retry}, nil
}

func (client *NRQLClient) QueryMarker(ctx context.Context, request MarkerQueryRequest) (MarkerQueryResult, error) {
	if ctx == nil {
		return MarkerQueryResult{}, errors.New("New Relic NRQL verification context is required")
	}
	if client == nil || client.httpClient == nil || client.resolver == nil {
		return MarkerQueryResult{}, errors.New("New Relic NRQL client is required")
	}
	if err := ctx.Err(); err != nil {
		return MarkerQueryResult{}, err
	}
	if err := validateSignal(request.Signal); err != nil {
		return MarkerQueryResult{}, err
	}
	if err := validateNRQLMarker(request.OwnershipMarker); err != nil {
		return MarkerQueryResult{}, err
	}
	query, err := buildMarkerQuery(client.accountID, request.Signal, request.OwnershipMarker)
	if err != nil {
		return MarkerQueryResult{}, err
	}

	var result MarkerQueryResult
	credentialRef := request.CredentialRef
	if client.credentialRef != "" {
		credentialRef = client.credentialRef
	}
	var queryErr error
	err = provider.UseCredential(ctx, client.resolver, credentialRef, func(apiKey []byte) error {
		for attempt := 0; attempt < client.retry.MaxAttempts; attempt++ {
			if err := ctx.Err(); err != nil {
				return err
			}
			var response nrqlResponse
			if err := executeNerdGraph(ctx, client.httpClient, client.endpoint, apiKey, query, nil, &response); err != nil {
				queryErr = err
				return err
			}
			count, err := response.count()
			if err != nil {
				return err
			}
			result = MarkerQueryResult{Count: count, LabelsVerified: count > 0}
			if count > 0 || attempt+1 >= client.retry.MaxAttempts {
				return nil
			}
			if err := wait(ctx, client.retry.delay(attempt)); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		if queryErr != nil {
			return MarkerQueryResult{}, fmt.Errorf("New Relic NRQL request failed: %w", queryErr)
		}
		return MarkerQueryResult{}, err
	}
	return result, nil
}

type nrqlResponse struct {
	Actor struct {
		Account struct {
			NRQL struct {
				Results []struct {
					Count int64 `json:"count"`
				} `json:"results"`
			} `json:"nrql"`
		} `json:"account"`
	} `json:"actor"`
}

func (response nrqlResponse) count() (int64, error) {
	results := response.Actor.Account.NRQL.Results
	if len(results) != 1 {
		return 0, errors.New("New Relic NRQL response did not contain exactly one count result")
	}
	if results[0].Count < 0 {
		return 0, errors.New("New Relic NRQL response returned a negative count")
	}
	return results[0].Count, nil
}

func buildMarkerQuery(accountID int64, signal Signal, marker string) (string, error) {
	eventType, err := nrqlEventType(signal)
	if err != nil {
		return "", err
	}
	escapedMarker, err := quoteNRQLString(marker)
	if err != nil {
		return "", err
	}
	nrql := "SELECT count(*) FROM " + eventType + " WHERE `magelift.ownership_marker` = " + escapedMarker + " SINCE 1 hour ago"
	return `query { actor { account(id: ` + strconv.FormatInt(accountID, 10) + `) { nrql(query: ` + strconv.Quote(nrql) + `) { results } } } }`, nil
}

func nrqlEventType(signal Signal) (string, error) {
	switch signal {
	case SignalLogs:
		return "Log", nil
	case SignalMetrics:
		return "Metric", nil
	case SignalTraces:
		return "Span", nil
	default:
		return "", fmt.Errorf("unsupported New Relic NRQL signal %q", signal)
	}
}

func validateNRQLMarker(marker string) error {
	if err := validateOwnershipMarker(marker); err != nil {
		return err
	}
	for _, character := range marker {
		if character < 0x20 || character == 0x7f {
			return errors.New("New Relic NRQL ownership marker must not contain control characters")
		}
	}
	return nil
}

func quoteNRQLString(value string) (string, error) {
	if err := validateNRQLMarker(value); err != nil {
		return "", err
	}
	var quoted strings.Builder
	quoted.Grow(len(value) + 2)
	quoted.WriteByte('\'')
	for _, character := range value {
		if character == '\\' || character == '\'' {
			quoted.WriteByte('\\')
		}
		quoted.WriteRune(character)
	}
	quoted.WriteByte('\'')
	return quoted.String(), nil
}
