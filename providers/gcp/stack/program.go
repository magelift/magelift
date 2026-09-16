package stack

import (
	"github.com/pulumi/pulumi-gcp/sdk/v9/go/gcp"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// Program returns the inline Pulumi program used by Automation API and mock tests.
func Program(spec Spec) pulumi.RunFunc {
	return func(ctx *pulumi.Context) error {
		name := spec.Identity.Project + "-" + spec.Identity.Environment
		provider, err := gcp.NewProvider(ctx, name+"-gcp", &gcp.ProviderArgs{
			Project: pulumi.String(spec.Identity.GCPProject),
			Region:  pulumi.String(spec.Identity.Region),
		})
		if err != nil {
			return err
		}
		component, err := New(ctx, name, spec, provider)
		if err != nil {
			return err
		}
		for key, value := range component.Outputs() {
			ctx.Export(key, value)
		}
		return nil
	}
}
