package observability

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/magelift/magelift/internal/external/newrelic"
	providerobservability "github.com/magelift/magelift/internal/external/observability"
	"github.com/magelift/magelift/sdk"
)

// ECSNewRelicSignalProbe verifies markers emitted by the target ECS task
// through the collector sidecar and queried with a separate NerdGraph key.
type ECSNewRelicSignalProbe struct {
	query              newrelic.MarkerQueryer
	queryCredentialRef string
}

var _ ECSCollectorSignalProbe = (*ECSNewRelicSignalProbe)(nil)

func NewECSNewRelicSignalProbe(query newrelic.MarkerQueryer, queryCredentialRef string) (*ECSNewRelicSignalProbe, error) {
	if query == nil {
		return nil, errors.New("ECS New Relic collector signal query verifier is required")
	}
	queryCredentialRef = strings.TrimSpace(queryCredentialRef)
	if err := sdk.ValidateCredentialReference(queryCredentialRef); err != nil {
		return nil, fmt.Errorf("ECS New Relic collector query credential reference: %w", err)
	}
	return &ECSNewRelicSignalProbe{query: query, queryCredentialRef: queryCredentialRef}, nil
}

func (probe *ECSNewRelicSignalProbe) VerifyCollectorSignals(ctx context.Context, plan sdk.CollectorDeploymentPlan, resource providerobservability.CollectorResource) (bool, error) {
	if ctx == nil {
		return false, errors.New("ECS New Relic collector signal probe context is required")
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if probe == nil || probe.query == nil {
		return false, errors.New("ECS New Relic collector signal probe is required")
	}
	if !resource.Owned || resource.OwnershipMarker != plan.OwnershipMarker {
		return false, errors.New("ECS New Relic collector signal probe requires exact collector ownership")
	}
	if _, _, ok := parseECSCollectorIdentity(resource.Identity); !ok {
		return false, errors.New("ECS New Relic collector signal probe requires a valid ECS collector identity")
	}
	for _, requestedSignal := range plan.Signals {
		result, err := probe.query.QueryMarker(ctx, newrelic.MarkerQueryRequest{
			Signal:          newrelic.Signal(requestedSignal),
			CredentialRef:   probe.queryCredentialRef,
			OwnershipMarker: plan.OwnershipMarker,
		})
		if err != nil {
			return false, fmt.Errorf("query New Relic ECS collector signal %q: %w", requestedSignal, err)
		}
		if result.Count == 0 || !result.LabelsVerified {
			return false, nil
		}
	}
	return true, nil
}
