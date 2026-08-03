package eksops

import (
	awsprovider "github.com/pulumi/pulumi-aws/sdk/v7/go/aws"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

func Program(spec Spec) pulumi.RunFunc {
	return func(ctx *pulumi.Context) error {
		name := spec.Identity.Project + "-" + spec.Identity.Environment
		provider, err := awsprovider.NewProvider(ctx, name+"-aws", &awsprovider.ProviderArgs{
			Region: pulumi.String(spec.Identity.Region),
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
