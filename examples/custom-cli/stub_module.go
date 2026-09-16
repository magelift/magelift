package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/magelift/magelift/sdk"
)

var errCommunityStub = errors.New("community stub is not deployable")

// stubModule is a community registration slot for examples/custom-cli.
// It proves NewWithExtensions wiring; Plan refuses so it is not a fake deployable.
type stubModule struct{}

func (stubModule) Descriptor() sdk.ExtensionDescriptor {
	return sdk.ExtensionDescriptor{
		APIVersion: sdk.ExtensionAPIVersion,
		ID:         "example.community-stub",
		Version:    "1.0.0",
		Source:     "example.com/magelift/community-stub",
		Build:      "example",
		Tier:       sdk.ExtensionTierExperimental,
		Targets: []sdk.TargetDescriptor{{
			ID:       "example.community-stub",
			Provider: "example",
			Runtime:  "community-stub",
		}},
		OutputKeys: sdk.CoreOutputKeys(),
	}
}

func (stubModule) Plan(context.Context, sdk.ModulePlanRequest) (sdk.ModulePlan, error) {
	return sdk.ModulePlan{}, fmt.Errorf("%w: example.community-stub is a registration demo", errCommunityStub)
}

func (stubModule) Program(sdk.ModulePlan) (any, error) {
	return nil, fmt.Errorf("example.community-stub has no Pulumi program")
}
