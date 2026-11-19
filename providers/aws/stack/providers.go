package stack

import (
	"errors"
	"fmt"

	awsprovider "github.com/pulumi/pulumi-aws/sdk/v7/go/aws"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// Providers keeps regional resources separate from CloudFront and WAF resources.
// The global provider must always target us-east-1.
type Providers struct {
	Regional *awsprovider.Provider
	Global   *awsprovider.Provider
}

func (p Providers) Validate() error {
	if p.Regional == nil || p.Global == nil {
		return errors.New("regional and global AWS providers are required")
	}
	return nil
}

func NewProviders(ctx *pulumi.Context, name, region string, options ...pulumi.ResourceOption) (Providers, error) {
	if name == "" || region == "" {
		return Providers{}, errors.New("provider name and regional AWS region are required")
	}
	regional, err := awsprovider.NewProvider(ctx, name+"-regional", &awsprovider.ProviderArgs{Region: pulumi.String(region)}, options...)
	if err != nil {
		return Providers{}, fmt.Errorf("create regional AWS provider: %w", err)
	}
	global, err := awsprovider.NewProvider(ctx, name+"-global", &awsprovider.ProviderArgs{Region: pulumi.String("us-east-1")}, options...)
	if err != nil {
		return Providers{}, fmt.Errorf("create global AWS provider: %w", err)
	}
	return Providers{Regional: regional, Global: global}, nil
}
