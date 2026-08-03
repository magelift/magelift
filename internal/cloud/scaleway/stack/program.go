package stack

import (
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/pulumiverse/pulumi-scaleway/sdk/go/scaleway"
)

// Program returns the inline Pulumi program used by Automation API and mock tests.
func Program(spec Spec) pulumi.RunFunc {
	return func(ctx *pulumi.Context) error {
		name := spec.Identity.Project + "-" + spec.Identity.Environment
		provider, err := scaleway.NewProvider(ctx, name+"-scaleway", &scaleway.ProviderArgs{
			Region:    pulumi.String(spec.Identity.Region),
			ProjectId: pulumi.String(spec.Identity.ScalewayProject),
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
