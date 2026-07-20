package cli

import (
	"fmt"
	"strings"

	"github.com/acourtiol/magelift/internal/config"
	"github.com/acourtiol/magelift/internal/platform"
	sdk "github.com/acourtiol/magelift/sdk/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// stubAWSModule lets CLI unit tests Plan without linking Pulumi AWS SDKs.
type stubAWSModule struct{}

func (stubAWSModule) Descriptor() sdk.TargetDescriptor {
	return sdk.TargetDescriptor{
		ID:       "aws.ecs-fargate",
		Provider: "aws",
		Runtime:  "ecs-fargate",
	}
}

func (stubAWSModule) CertificationTier() platform.CertificationTier {
	return platform.TierCertified
}

func (stubAWSModule) OutputKeys() []string { return platform.RequiredOutputKeys() }

func (stubAWSModule) Program(platform.PlannedStack) (pulumi.RunFunc, error) {
	return nil, nil
}

func (stubAWSModule) Plan(cfg config.Config, environment string, _ platform.PlanOptions) (platform.PlannedStack, error) {
	if strings.TrimSpace(environment) == "" {
		return nil, fmt.Errorf("environment is required")
	}
	digest := ""
	if cfg.Target.AWS != nil {
		digest = cfg.Target.AWS.ImageDigest
	}
	return stubPlanned{
		stackName:   cfg.Project.Name + "-" + environment,
		provider:    "aws",
		runtime:     "ecs-fargate",
		project:     cfg.Project.Name,
		environment: environment,
		region:      cfg.Defaults.Region,
		envClass:    cfg.Class,
		protected:   cfg.Protection,
		digest:      digest,
	}, nil
}

type stubPlanned struct {
	stackName, provider, runtime, project, environment, region, envClass, digest string
	protected                                                                    bool
}

func (p stubPlanned) StackName() string        { return p.stackName }
func (p stubPlanned) Provider() sdk.ProviderID { return sdk.ProviderID(p.provider) }
func (p stubPlanned) Runtime() sdk.RuntimeID   { return sdk.RuntimeID(p.runtime) }
func (p stubPlanned) Project() string          { return p.project }
func (p stubPlanned) Environment() string      { return p.environment }
func (p stubPlanned) Region() string           { return p.region }
func (p stubPlanned) CertificationTier() platform.CertificationTier {
	return platform.TierCertified
}
func (p stubPlanned) EnvironmentClass() string { return p.envClass }
func (p stubPlanned) Protected() bool          { return p.protected }
func (p stubPlanned) ImageDigest() string      { return p.digest }
func (p stubPlanned) WithImageDigest(digest string) (platform.PlannedStack, error) {
	p.digest = digest
	return p, nil
}
func (p stubPlanned) TargetDescriptor() sdk.TargetDescriptor {
	return stubAWSModule{}.Descriptor()
}

func registerTestModules(modules *platform.ModuleRegistry) {
	if err := modules.RegisterModule(stubAWSModule{}); err != nil {
		panic(err)
	}
}
