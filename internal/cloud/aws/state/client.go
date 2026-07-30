package state

import (
	"context"

	awsendpoint "github.com/acourtiol/magelift/internal/cloud/aws/endpoint"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

func NewAWS(ctx context.Context, region, bucket, project, environment string, encryption ObjectEncryption) (*Manager, error) {
	endpoint, err := awsendpoint.FromEnv()
	if err != nil {
		return nil, err
	}
	return NewAWSWithEndpoint(ctx, region, bucket, project, environment, encryption, endpoint)
}

// NewAWSWithEndpoint is used by local AWS-compatible emulators such as Floci.
// Production callers should leave endpoint empty so the SDK uses AWS defaults.
func NewAWSWithEndpoint(ctx context.Context, region, bucket, project, environment string, encryption ObjectEncryption, endpoint string) (*Manager, error) {
	validatedEndpoint, err := awsendpoint.Parse(endpoint)
	if err != nil {
		return nil, err
	}
	config, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(region))
	if err != nil {
		return nil, err
	}
	client := s3.NewFromConfig(config, func(options *s3.Options) {
		if validatedEndpoint != "" {
			options.BaseEndpoint = &validatedEndpoint
			options.UsePathStyle = true
		}
	})
	return NewManager(client, bucket, project, environment, encryption)
}
