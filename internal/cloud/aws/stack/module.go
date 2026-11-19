package stack

import (
	"fmt"

	awsbootstrap "github.com/magelift/magelift/internal/cloud/aws/bootstrap"
	"github.com/magelift/magelift/internal/config"
	"github.com/magelift/magelift/internal/platform"
	"github.com/magelift/magelift/sdk"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// Module wraps the AWS stack planner as a platform.StackModule.
type Module struct {
	platform.LifecycleFactories
}

// Planned is the AWS PlannedStack adapter.
type Planned struct {
	Spec      Spec
	stateOnly bool
}

func (p Planned) StackName() string {
	return platform.FormatStackName(p.Spec.Identity.Project, p.Spec.Identity.Environment, p.Provider(), p.Runtime())
}
func (p Planned) Provider() sdk.ProviderID {
	return sdk.ProviderID("aws")
}
func (p Planned) Runtime() sdk.RuntimeID { return sdk.RuntimeID("ecs-fargate") }
func (p Planned) Project() string        { return p.Spec.Identity.Project }
func (p Planned) Environment() string    { return p.Spec.Identity.Environment }
func (p Planned) Region() string         { return p.Spec.Identity.Region }
func (p Planned) CertificationTier() platform.CertificationTier {
	if awsPreviewFargateMagentoCertified(p.Spec) {
		return platform.TierBuiltIn
	}
	return platform.TierExperimental
}
func (p Planned) EnvironmentClass() string { return p.Spec.Identity.EnvironmentClass }
func (p Planned) Protected() bool          { return p.Spec.Lifecycle.Protection }
func (p Planned) ImageDigest() string      { return p.Spec.Artifact.ImageDigest }
func (p Planned) TargetDescriptor() sdk.TargetDescriptor {
	return sdk.TargetDescriptor{ID: "aws.ecs-fargate", Provider: "aws", Runtime: "ecs-fargate"}
}

func (p Planned) WithImageDigest(digest string) (platform.PlannedStack, error) {
	if p.stateOnly {
		return nil, fmt.Errorf("a state-only AWS plan cannot deploy an image")
	}
	next := p
	next.Spec.Artifact.ImageDigest = digest
	if err := next.Spec.Validate(); err != nil {
		return nil, err
	}
	return next, nil
}

// AWSSpec exposes the concrete Spec for AWS-only deploy/lock factories.
func (p Planned) AWSSpec() Spec { return p.Spec }

func (p Planned) StateBackendURL() string {
	url, err := awsbootstrap.StateBackendURL(p.Spec.Identity.Project, p.Spec.Identity.BootstrapEnvironment(), p.Spec.Identity.AccountID, p.Spec.Identity.Region)
	if err != nil {
		return ""
	}
	return url
}

func (Module) Descriptor() sdk.TargetDescriptor {
	return sdk.TargetDescriptor{ID: "aws.ecs-fargate", Provider: "aws", Runtime: "ecs-fargate"}
}

func (Module) CertificationTier() platform.CertificationTier { return platform.TierBuiltIn }

// PlanAdmission resolves current AWS account and catalog availability before
// the Pulumi program can begin any paid resource mutation.
func (Module) PlanAdmission() platform.PlanAdmission { return RegionAdmission{} }

func (Module) Plan(cfg config.Config, environment string, opts platform.PlanOptions) (platform.PlannedStack, error) {
	if opts.AccountOnly || opts.AllowExpiredPreview {
		if cfg.Target.AWS == nil {
			return nil, fmt.Errorf("target.aws is required to access AWS state")
		}
		preset := cfg.Preset
		if preset == "" {
			preset = cfg.Defaults.Preset
		}
		id := Identity{Project: cfg.Project.Name, Environment: environment, StateEnvironment: cfg.StateEnvironment, AccountID: cfg.Account, Region: cfg.Defaults.Region, EnvironmentClass: cfg.Class, Preset: sdk.PresetID(preset)}
		if err := id.validate(); err != nil {
			return nil, err
		}
		return Planned{stateOnly: true, Spec: Spec{Identity: id, Lifecycle: Lifecycle{Protection: cfg.Protection}, Dependencies: Dependencies{KMSKeyARN: cfg.Target.AWS.KMSKeyARN}, Artifact: Artifact{ImageDigest: cfg.Target.AWS.ImageDigest}}}, nil
	}
	spec, err := PlanFromConfigWithOptions(cfg, environment, PlanOptions{AllowExpiredPreview: opts.AllowExpiredPreview})
	if err != nil {
		return nil, err
	}
	return Planned{Spec: spec}, nil
}

func (Module) Program(planned platform.PlannedStack) (pulumi.RunFunc, error) {
	awsPlanned, ok := planned.(Planned)
	if !ok {
		return nil, fmt.Errorf("AWS stack module received unexpected planned type %T", planned)
	}
	if awsPlanned.stateOnly {
		// Pulumi destroy and preview-destroy operate from stored state and
		// run without executing the program. Refuse an accidental update.
		return func(*pulumi.Context) error { return fmt.Errorf("AWS state-only plans cannot run a resource program") }, nil
	}
	return Program(awsPlanned.Spec), nil
}

func (Module) OutputKeys() []string {
	keys := append([]string(nil), platform.RequiredOutputKeys()...)
	keys = append(keys,
		"edgeDistributionId", "mediaURL", "mediaBucket", "searchEndpoint", "queueMode",
		"clusterArn", "taskDefinitionArn", "deployTaskDefinitionArn",
		"cronServiceName", "cronTaskDefinitionArn", "queueServiceName", "queueTaskDefinitionArn",
		"taskRoleArn", "deploymentRoleArn", "securityGroupId",
	)
	return keys
}

// AsAWSPlanned extracts the AWS Planned value when present.
func AsAWSPlanned(planned platform.PlannedStack) (Planned, bool) {
	value, ok := planned.(Planned)
	return value, ok
}
