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
		component, err := New(ctx, name, spec, providers)
		if err != nil {
			return err
		}
		// Component RegisterResourceOutputs are not stack outputs. Export the
		// same map so automation.Outputs / magelift outputs / deploy can read them.
		for key, value := range component.Outputs() {
			ctx.Export(key, value)
		}
		return nil
	}
}
