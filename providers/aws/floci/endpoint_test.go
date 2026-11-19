//go:build floci

package floci_test

import (
	"context"
	"os"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/magelift/magelift/internal/awsendpoint"
)

func flociEndpoint(t *testing.T) string {
	t.Helper()
	if os.Getenv("MAGELIFT_FLOCI_AWS") != "1" {
		t.Skip("run make floci-aws-test to start the AWS emulator")
	}
	endpoint, err := awsendpoint.FromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if endpoint == "" {
		return "http://127.0.0.1:4566"
	}
	return endpoint
}

func flociConfig(t *testing.T, ctx context.Context, endpoint string) aws.Config {
	t.Helper()
	cfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion("us-east-1"),
		awsconfig.WithBaseEndpoint(endpoint),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider("test", "test", "")))
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}
