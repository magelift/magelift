package stack

import "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

// Program returns the inline Pulumi program used by Automation API and local
// mock tests. Provider construction stays inside the program so previews and
// updates use the same resource graph.
func Program(spec Spec) pulumi.RunFunc {
	return func(ctx *pulumi.Context) error {
		name := spec.Identity.Project + "-" + spec.Identity.Environment
		providers, err := NewProviders(ctx, name, spec.Identity.Region)
		if err != nil {
			return err
		}
		_, err = New(ctx, name, spec, providers)
		return err
	}
}
