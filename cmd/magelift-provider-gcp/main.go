package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	gcpstack "github.com/magelift/magelift/internal/cloud/gcp/stack"
	"github.com/magelift/magelift/internal/config"
	"github.com/magelift/magelift/internal/platform"
	"github.com/magelift/magelift/internal/providerhost"
	sdk "github.com/magelift/magelift/sdk/v1"
)

// Magento cells stay on the in-process GCP module. The CLI is not wired to Dial.
func main() {
	providerhost.Serve(gcpAPI{module: gcpstack.Module{}})
}

type gcpAPI struct {
	module gcpstack.Module
}

func (gcpAPI) Ping(context.Context) (string, error) {
	return providerhost.SDKAPIVersion, nil
}

func (g gcpAPI) Describe(context.Context) (providerhost.Identity, error) {
	return providerhost.IdentityFromStack(
		g.module.Descriptor(),
		string(g.module.CertificationTier()),
		g.module.OutputKeys(),
	), nil
}

func (g gcpAPI) Plan(_ context.Context, request sdk.ModulePlanRequest) (sdk.ModulePlan, error) {
	cfg, err := configFromMap(request.Configuration)
	if err != nil {
		return sdk.ModulePlan{}, err
	}
	planned, err := g.module.Plan(cfg, request.Environment, platform.PlanOptions{AllowExpiredPreview: request.AllowExpired})
	if err != nil {
		return sdk.ModulePlan{}, err
	}
	gcpPlanned, ok := gcpstack.AsGCPPlanned(planned)
	if !ok {
		return sdk.ModulePlan{}, fmt.Errorf("GCP plugin received unexpected planned type %T", planned)
	}
	tier := sdk.ExtensionTierExperimental
	if planned.CertificationTier() == platform.TierCertified {
		tier = sdk.ExtensionTierCertified
	}
	return sdk.ModulePlan{
		StackName:        planned.StackName(),
		Provider:         planned.Provider(),
		Runtime:          planned.Runtime(),
		Project:          planned.Project(),
		Environment:      planned.Environment(),
		Region:           planned.Region(),
		EnvironmentClass: planned.EnvironmentClass(),
		Protected:        planned.Protected(),
		ImageDigest:      planned.ImageDigest(),
		Target:           planned.TargetDescriptor(),
		Tier:             tier,
		Opaque:           gcpPlanned.Spec,
	}, nil
}

func (g gcpAPI) Program(_ context.Context, plan sdk.ModulePlan) (providerhost.ProgramResult, error) {
	payload, err := json.Marshal(plan.Opaque)
	if err != nil {
		return providerhost.ProgramResult{}, fmt.Errorf("encode GCP plan opaque: %w", err)
	}
	var spec gcpstack.Spec
	if err := json.Unmarshal(payload, &spec); err != nil {
		return providerhost.ProgramResult{}, fmt.Errorf("decode GCP plan opaque: %w", err)
	}
	program, err := g.module.Program(gcpstack.Planned{Spec: spec})
	if err != nil {
		return providerhost.ProgramResult{}, err
	}
	if program == nil {
		return providerhost.ProgramResult{}, errors.New("GCP plugin returned a nil program")
	}
	return providerhost.ProgramResult{Kind: providerhost.ProgramKindPulumiRunFunc}, nil
}

func (g gcpAPI) Execute(ctx context.Context, request providerhost.ExecuteRequest) (providerhost.ExecuteResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	return execute(ctx, g.module, request)
}

func configFromMap(values map[string]any) (config.Config, error) {
	if len(values) == 0 {
		return config.Config{}, errors.New("plan configuration is required")
	}
	payload, err := json.Marshal(values)
	if err != nil {
		return config.Config{}, fmt.Errorf("encode plan configuration: %w", err)
	}
	var cfg config.Config
	if err := json.Unmarshal(payload, &cfg); err != nil {
		return config.Config{}, fmt.Errorf("decode plan configuration: %w", err)
	}
	return cfg, nil
}
