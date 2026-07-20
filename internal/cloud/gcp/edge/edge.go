// Package edge adds Magento public ingress hardening on GCP (Cloud Armor policy).
// Application URL remains the GKE LoadBalancer Service until a custom domain is supplied.
package edge

import (
	"errors"
	"fmt"
	"strings"

	"github.com/pulumi/pulumi-gcp/sdk/v9/go/gcp/compute"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

const TypeToken = "magelift:gcp:Edge"

type Args struct {
	Project string
	Enabled bool
	Labels  map[string]string
}

type Component struct {
	pulumi.ResourceState
	SecurityPolicyName pulumi.StringOutput
}

func New(ctx *pulumi.Context, name string, args Args, opts ...pulumi.ResourceOption) (*Component, error) {
	if strings.TrimSpace(name) == "" {
		return nil, errors.New("edge name is required")
	}
	component := &Component{}
	if err := ctx.RegisterComponentResourceV2(TypeToken, name, pulumi.Map{
		"project": pulumi.String(args.Project),
	}, component, opts...); err != nil {
		return nil, err
	}
	if !args.Enabled {
		component.SecurityPolicyName = pulumi.String("").ToStringOutput()
		_ = ctx.RegisterResourceOutputs(component, pulumi.Map{"securityPolicyName": component.SecurityPolicyName})
		return component, nil
	}
	if strings.TrimSpace(args.Project) == "" {
		return nil, errors.New("GCP project is required for Cloud Armor")
	}
	parent := pulumi.Parent(component)
	policy, err := compute.NewSecurityPolicy(ctx, name+"-armor", &compute.SecurityPolicyArgs{
		Project: pulumi.String(args.Project),
		Name:    pulumi.String(name + "-armor"),
		Type:    pulumi.String("CLOUD_ARMOR"),
		Rules: compute.SecurityPolicyRuleTypeArray{
			compute.SecurityPolicyRuleTypeArgs{
				Action:      pulumi.String("allow"),
				Priority:    pulumi.Int(2147483647),
				Description: pulumi.String("default allow"),
				Match: compute.SecurityPolicyRuleMatchArgs{
					VersionedExpr: pulumi.String("SRC_IPS_V1"),
					Config: &compute.SecurityPolicyRuleMatchConfigArgs{
						SrcIpRanges: pulumi.StringArray{pulumi.String("*")},
					},
				},
			},
		},
	}, parent)
	if err != nil {
		return nil, fmt.Errorf("create Cloud Armor policy: %w", err)
	}
	component.SecurityPolicyName = policy.Name
	if err := ctx.RegisterResourceOutputs(component, pulumi.Map{"securityPolicyName": component.SecurityPolicyName}); err != nil {
		return nil, err
	}
	return component, nil
}
