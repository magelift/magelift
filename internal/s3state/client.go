package s3state

import (
	"context"

	"github.com/magelift/magelift/internal/awsendpoint"
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
	client, err := awsendpoint.S3Client(ctx, region, endpoint)
	if err != nil {
		return nil, err
	}
	return NewManager(client, bucket, project, environment, encryption)
}
