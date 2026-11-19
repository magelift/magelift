package awsendpoint

import (
	"context"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// S3Client uses path-style addressing only for an explicitly selected emulator.
func S3Client(ctx context.Context, region, endpoint string) (*s3.Client, error) {
	validatedEndpoint, err := Parse(endpoint)
	if err != nil {
		return nil, err
	}
	config, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(region))
	if err != nil {
		return nil, err
	}
	return s3.NewFromConfig(config, func(options *s3.Options) {
		if validatedEndpoint != "" {
			options.BaseEndpoint = &validatedEndpoint
			options.UsePathStyle = true
		}
	}), nil
}
