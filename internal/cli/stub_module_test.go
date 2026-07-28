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
		tier:        platform.TierCertified,
	}, nil
}

// stubExperimentalModule is an OVH/MKS stub so CLI tests can assert experimental
// tier messaging without importing cloud packages.
type stubExperimentalModule struct{}

func (stubExperimentalModule) Descriptor() sdk.TargetDescriptor {
	return sdk.TargetDescriptor{
		ID:       "ovh.mks",
		Provider: "ovh",
		Runtime:  "mks",
	}
}

func (stubExperimentalModule) CertificationTier() platform.CertificationTier {
	return platform.TierExperimental
}

func (stubExperimentalModule) OutputKeys() []string { return platform.RequiredOutputKeys() }

func (stubExperimentalModule) Program(platform.PlannedStack) (pulumi.RunFunc, error) {
	return nil, nil
}

func (stubExperimentalModule) Plan(cfg config.Config, environment string, _ platform.PlanOptions) (platform.PlannedStack, error) {
	if strings.TrimSpace(environment) == "" {
		return nil, fmt.Errorf("environment is required")
	}
	digest := ""
	if cfg.Target.OVH != nil {
		digest = cfg.Target.OVH.ImageDigest
	}
	return stubPlanned{
		stackName:   cfg.Project.Name + "-" + environment,
		provider:    "ovh",
		runtime:     "mks",
		project:     cfg.Project.Name,
		environment: environment,
		region:      cfg.Defaults.Region,
		envClass:    cfg.Class,
		protected:   cfg.Protection,
		digest:      digest,
		tier:        platform.TierExperimental,
	}, nil
}

// stubExperimentalAWSEKSModule is aws/eks-autopilot so tier-keyed warnings cannot
// pass by allowlisting provider names (same provider as certified ecs-fargate).
type stubExperimentalAWSEKSModule struct{}

func (stubExperimentalAWSEKSModule) Descriptor() sdk.TargetDescriptor {
	return sdk.TargetDescriptor{
		ID:       "aws.eks-autopilot",
		Provider: "aws",
		Runtime:  "eks-autopilot",
	}
}

func (stubExperimentalAWSEKSModule) CertificationTier() platform.CertificationTier {
	return platform.TierExperimental
}

func (stubExperimentalAWSEKSModule) OutputKeys() []string { return platform.RequiredOutputKeys() }

func (stubExperimentalAWSEKSModule) Program(platform.PlannedStack) (pulumi.RunFunc, error) {
	return nil, nil
}

func (stubExperimentalAWSEKSModule) Plan(cfg config.Config, environment string, _ platform.PlanOptions) (platform.PlannedStack, error) {
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
		runtime:     "eks-autopilot",
		project:     cfg.Project.Name,
		environment: environment,
		region:      cfg.Defaults.Region,
		envClass:    cfg.Class,
		protected:   cfg.Protection,
		digest:      digest,
		tier:        platform.TierExperimental,
	}, nil
}

type stubPlanned struct {
	stackName, provider, runtime, project, environment, region, envClass, digest string
	protected                                                                    bool
	tier                                                                         platform.CertificationTier
}

func (p stubPlanned) StackName() string        { return p.stackName }
func (p stubPlanned) Provider() sdk.ProviderID { return sdk.ProviderID(p.provider) }
func (p stubPlanned) Runtime() sdk.RuntimeID   { return sdk.RuntimeID(p.runtime) }
func (p stubPlanned) Project() string          { return p.project }
func (p stubPlanned) Environment() string      { return p.environment }
func (p stubPlanned) Region() string           { return p.region }
func (p stubPlanned) CertificationTier() platform.CertificationTier {
	if p.tier == "" {
		return platform.TierCertified
	}
	return p.tier
}
func (p stubPlanned) EnvironmentClass() string { return p.envClass }
func (p stubPlanned) Protected() bool          { return p.protected }
func (p stubPlanned) ImageDigest() string      { return p.digest }
func (p stubPlanned) WithImageDigest(digest string) (platform.PlannedStack, error) {
	p.digest = digest
	return p, nil
}
func (p stubPlanned) TargetDescriptor() sdk.TargetDescriptor {
	switch {
	case p.provider == "ovh" && p.runtime == "mks":
		return stubExperimentalModule{}.Descriptor()
	case p.provider == "aws" && p.runtime == "eks-autopilot":
		return stubExperimentalAWSEKSModule{}.Descriptor()
	default:
		return stubAWSModule{}.Descriptor()
	}
}

func registerTestModules(modules *platform.ModuleRegistry) {
	if err := modules.RegisterModule(stubAWSModule{}); err != nil {
		panic(err)
	}
	if err := modules.RegisterModule(stubExperimentalModule{}); err != nil {
		panic(err)
	}
	if err := modules.RegisterModule(stubExperimentalAWSEKSModule{}); err != nil {
		panic(err)
	}
}
