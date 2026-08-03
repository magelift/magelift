package stack

import (
	"github.com/ovh/pulumi-ovh/sdk/v2/go/ovh"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// Program returns the inline Pulumi program used by Automation API and mock tests.
func Program(spec Spec) pulumi.RunFunc {
	return func(ctx *pulumi.Context) error {
		name := spec.Identity.Project + "-" + spec.Identity.Environment
		provider, err := ovh.NewProvider(ctx, name+"-ovh", &ovh.ProviderArgs{
			Endpoint: pulumi.String("ovh-eu"),
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
