package providerhost

import (
	"context"
	"errors"
	"fmt"
	"slices"

	sdk "github.com/magelift/magelift/sdk/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// Identity is the subprocess view of a first-party stack module. Plan and
// Program JSON RPCs exist; Magento cells stay in-process until Dial is wired
// to a real execute RPC.
type Identity struct {
	APIVersion string   `json:"apiVersion"`
	Provider   string   `json:"provider"`
	Runtime    string   `json:"runtime"`
	TargetID   string   `json:"targetId"`
	Tier       string   `json:"tier"`
	OutputKeys []string `json:"outputKeys"`
}

func IdentityFromStack(desc sdk.TargetDescriptor, tier string, outputKeys []string) Identity {
	return Identity{
		APIVersion: SDKAPIVersion,
		Provider:   string(desc.Provider),
		Runtime:    string(desc.Runtime),
		TargetID:   string(desc.ID),
		Tier:       tier,
		OutputKeys: slices.Clone(outputKeys),
	}
}

type API interface {
	Ping(ctx context.Context) (string, error)
	Describe(ctx context.Context) (Identity, error)
	Plan(ctx context.Context, request sdk.ModulePlanRequest) (sdk.ModulePlan, error)
	Program(ctx context.Context, plan sdk.ModulePlan) (ProgramResult, error)
	Execute(ctx context.Context, request ExecuteRequest) (ExecuteResult, error)
}

type ProgramResult struct {
	Kind string `json:"kind"`
}

const ProgramKindPulumiRunFunc = "pulumi.RunFunc"

// ErrProgramNotExecutable reports that a subprocess program result describes a
// program but cannot carry its in-memory Pulumi callback across gRPC.
var ErrProgramNotExecutable = errors.New("subprocess program result is not executable")

// PulumiRunFunc refuses to materialize a kind-only gRPC result as a Pulumi
// callback. A real execute RPC is required before subprocess programs run.
func (r ProgramResult) PulumiRunFunc() (pulumi.RunFunc, error) {
	if r.Kind == "" {
		return nil, fmt.Errorf("%w: program kind is empty", ErrProgramNotExecutable)
	}
	return nil, fmt.Errorf("%w: %q requires an execute RPC", ErrProgramNotExecutable, r.Kind)
}
