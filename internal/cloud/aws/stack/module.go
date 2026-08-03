package stack

import (
	"fmt"

	"github.com/acourtiol/magelift/internal/config"
	"github.com/acourtiol/magelift/internal/platform"
	sdk "github.com/acourtiol/magelift/sdk/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// Module wraps the AWS stack planner as a platform.StackModule.
type Module struct{}

// Planned is the AWS PlannedStack adapter.
type Planned struct {
	Spec Spec
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
	return platform.TierCertified
}
func (p Planned) EnvironmentClass() string { return p.Spec.Identity.EnvironmentClass }
func (p Planned) Protected() bool          { return p.Spec.Lifecycle.Protection }
func (p Planned) ImageDigest() string      { return p.Spec.Artifact.ImageDigest }
func (p Planned) TargetDescriptor() sdk.TargetDescriptor {
	return sdk.TargetDescriptor{ID: "aws.ecs-fargate", Provider: "aws", Runtime: "ecs-fargate"}
}

func (p Planned) WithImageDigest(digest string) (platform.PlannedStack, error) {
	next := p
	next.Spec.Artifact.ImageDigest = digest
	if err := next.Spec.Validate(); err != nil {
		return nil, err
	}
	return next, nil
}

// AWSSpec exposes the concrete Spec for AWS-only deploy/lock factories.
func (p Planned) AWSSpec() Spec { return p.Spec }

func (Module) Descriptor() sdk.TargetDescriptor {
	return sdk.TargetDescriptor{ID: "aws.ecs-fargate", Provider: "aws", Runtime: "ecs-fargate"}
}

func (Module) CertificationTier() platform.CertificationTier { return platform.TierCertified }

func (Module) Plan(cfg config.Config, environment string, opts platform.PlanOptions) (platform.PlannedStack, error) {
	spec, err := PlanFromConfigWithOptions(cfg, environment, PlanOptions{AllowExpiredPreview: opts.AllowExpiredPreview})
	if err != nil {
		return nil, err
	}
	if err := spec.Validate(); err != nil {
		return nil, fmt.Errorf("deployment configuration is invalid: %w", err)
	}
	return Planned{Spec: spec}, nil
}

func (Module) Program(planned platform.PlannedStack) (pulumi.RunFunc, error) {
	awsPlanned, ok := planned.(Planned)
	if !ok {
		return nil, fmt.Errorf("AWS stack module received unexpected planned type %T", planned)
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
