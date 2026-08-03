package target

import (
	"context"
	"errors"
	"fmt"
	"reflect"

	"github.com/acourtiol/magelift/internal/infra"
	"github.com/acourtiol/magelift/internal/topology"
	sdk "github.com/acourtiol/magelift/sdk/v1"
)

const (
	ProviderID sdk.ProviderID = "gcp"
	RuntimeID  sdk.RuntimeID  = "gke-autopilot"
	TargetID   sdk.TargetID   = "gcp.gke-autopilot"
)

type Target struct{}

func (Target) Descriptor() sdk.TargetDescriptor {
	return sdk.TargetDescriptor{ID: TargetID, Provider: ProviderID, Runtime: RuntimeID}
}

func (Target) Validate(ctx context.Context, request sdk.TargetRequest) error {
	if cause := context.Cause(ctx); cause != nil {
		return cause
	}
	if err := sdk.ValidateDesiredTopology(request.Topology); err != nil {
		return fmt.Errorf("validate GCP desired topology: %w", err)
	}
	expected, err := topology.ForPreset(request.Topology.Preset)
	if err != nil {
		return fmt.Errorf("resolve GCP preset: %w", err)
	}
	if !reflect.DeepEqual(request.Topology, expected) {
		return fmt.Errorf("topology for GCP GKE Autopilot does not match the %q preset constraints", request.Topology.Preset)
	}
	if request.EnvironmentClass == "production" && request.Topology.Preset == sdk.PresetPreview {
		return errors.New("production environments cannot use the preview preset")
	}
	return nil
}

func Register(registry *infra.Registry) error {
	if registry == nil {
		return errors.New("infrastructure registry is required")
	}
	return registry.RegisterTarget(Target{})
}
