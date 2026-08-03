package main

import (
	"fmt"

	"github.com/magelift/magelift/internal/config"
	"github.com/magelift/magelift/internal/platform"
	sdk "github.com/magelift/magelift/sdk/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// stubModule is a community registration slot for examples/custom-cli.
// It proves RegisterModule wiring; Plan refuses so it is not a fake deployable.
type stubModule struct{}

func (stubModule) Descriptor() sdk.TargetDescriptor {
	return sdk.TargetDescriptor{
		ID:       "example.community-stub",
		Provider: "example",
		Runtime:  "community-stub",
	}
}

func (stubModule) CertificationTier() platform.CertificationTier {
	// Registry accepts certified|experimental only; community targets use
	// experimental tier with a community-labeled ID (see README).
	return platform.TierExperimental
}

func (stubModule) Plan(config.Config, string, platform.PlanOptions) (platform.PlannedStack, error) {
	return nil, fmt.Errorf("%w: example.community-stub is a registration demo, not a deployable provider", platform.ErrNotSupported)
}

func (stubModule) Program(platform.PlannedStack) (pulumi.RunFunc, error) {
	return nil, fmt.Errorf("%w: example.community-stub has no Pulumi program", platform.ErrNotSupported)
}

func (stubModule) OutputKeys() []string {
	return platform.RequiredOutputKeys()
}
