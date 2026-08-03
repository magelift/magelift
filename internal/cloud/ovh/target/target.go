package target

import (
	"context"
	"errors"
	"fmt"
	"reflect"

	"github.com/magelift/magelift/internal/infra"
	"github.com/magelift/magelift/internal/topology"
	sdk "github.com/magelift/magelift/sdk/v1"
)

const (
	ProviderID sdk.ProviderID = "ovh"
	RuntimeID  sdk.RuntimeID  = "mks"
	TargetID   sdk.TargetID   = "ovh.mks"
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
		return fmt.Errorf("validate OVH desired topology: %w", err)
	}
	expected, err := topology.ForPreset(request.Topology.Preset)
	if err != nil {
		return fmt.Errorf("resolve OVH preset: %w", err)
	}
	if !reflect.DeepEqual(request.Topology, expected) {
		return fmt.Errorf("topology for OVH MKS does not match the %q preset constraints", request.Topology.Preset)
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
