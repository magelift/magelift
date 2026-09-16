package target

import (
	"context"
	"errors"
	"fmt"
	"reflect"

	"github.com/magelift/magelift/internal/topology"
	"github.com/magelift/magelift/sdk"
)

const (
	ProviderID         sdk.ProviderID = "gcp"
	RuntimeID          sdk.RuntimeID  = "gke-autopilot"
	RuntimeAutopilotID sdk.RuntimeID  = RuntimeID
	RuntimeStandardID  sdk.RuntimeID  = "gke-standard"
	TargetID           sdk.TargetID   = "gcp.gke-autopilot"
	TargetAutopilotID  sdk.TargetID   = TargetID
	TargetStandardID   sdk.TargetID   = "gcp.gke-standard"
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
