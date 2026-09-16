// Command custom-extension-contract verifies that a community module can be
// built against the public MageLift SDK without importing first-party cloud
// adapters.
package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/magelift/magelift/sdk"
)

var _ sdk.Module = exampleModule{}

type exampleModule struct{}

func (exampleModule) Descriptor() sdk.ExtensionDescriptor {
	return sdk.ExtensionDescriptor{
		APIVersion: sdk.ExtensionAPIVersion,
		ID:         "example.community-contract",
		Version:    "1.0.0",
		Source:     "example.com/magelift/community-contract",
		Build:      "example",
		Tier:       sdk.ExtensionTierExperimental,
		Targets: []sdk.TargetDescriptor{{
			ID:       "example.community-contract",
			Provider: "example",
			Runtime:  "community-contract",
		}},
		OutputKeys: sdk.CoreOutputKeys(),
	}
}

func (exampleModule) Plan(context.Context, sdk.ModulePlanRequest) (sdk.ModulePlan, error) {
	return sdk.ModulePlan{}, errors.New("example module is a contract check")
}

func (exampleModule) Program(sdk.ModulePlan) (any, error) {
	return nil, errors.New("example module has no Pulumi program")
}

func main() {
	module := exampleModule{}
	if err := sdk.ValidateExtensionDescriptor(module.Descriptor()); err != nil {
		panic(err)
	}
	fmt.Println(module.Descriptor().ID)
}
