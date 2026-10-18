//go:build floci

package floci_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	awscfg "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/aws/aws-sdk-go-v2/service/iam/types"
	"github.com/aws/smithy-go"

	"github.com/magelift/magelift/internal/cloud/aws/bootstrap"
)

func TestIAMOpenIDConnectProviderAgainstFloci(t *testing.T) {
	if os.Getenv("MAGELIFT_FLOCI") != "1" {
		t.Skip("set MAGELIFT_FLOCI=1 to run the Floci integration test")
	}
	endpoint := os.Getenv("MAGELIFT_FLOCI_ENDPOINT")
	if endpoint == "" {
		endpoint = "http://localhost:4566"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	configuration, err := awscfg.LoadDefaultConfig(ctx,
		awscfg.WithRegion("us-east-1"),
		awscfg.WithCredentialsProvider(credentials.NewStaticCredentialsProvider("test", "test", "")),
	)
	if err != nil {
		t.Fatal(err)
	}
	client := iam.NewFromConfig(configuration, func(options *iam.Options) {
		options.BaseEndpoint = awssdk.String(endpoint)
	})

	plan, err := bootstrap.BuildIdentityPlan(bootstrap.IdentitySpec{
		Project:     "shop",
		Environment: "staging",
		AccountID:   "000000000000",
		Region:      "us-east-1",
		GitHubOwner: "magelift",
		GitHubRepo:  "magelift",
		StateBucket: "magelift-shop-staging-state",
		KMSKeyARN:   "arn:aws:kms:us-east-1:000000000000:key/00000000-0000-0000-0000-000000000000",
	})
	if err != nil {
		t.Fatal(err)
	}

	_, err = client.GetOpenIDConnectProvider(ctx, &iam.GetOpenIDConnectProviderInput{
		OpenIDConnectProviderArn: awssdk.String(plan.OIDCProviderARN),
	})
	if err == nil {
		t.Fatal("expected NoSuchEntity for a missing OIDC provider")
	}
	var apiError smithy.APIError
	if !errors.As(err, &apiError) || apiError.ErrorCode() != "NoSuchEntity" {
		t.Fatalf("missing OIDC provider: %v", err)
	}

	tags := make([]types.Tag, 0, len(plan.Tags))
	for key, value := range plan.Tags {
		tags = append(tags, types.Tag{Key: awssdk.String(key), Value: awssdk.String(value)})
	}
	if _, err := client.CreateOpenIDConnectProvider(ctx, &iam.CreateOpenIDConnectProviderInput{
		Url:          awssdk.String("https://token.actions.githubusercontent.com"),
		ClientIDList: []string{"sts.amazonaws.com"},
		Tags:         tags,
	}); err != nil {
		t.Fatalf("CreateOpenIDConnectProvider: %v", err)
	}
	got, err := client.GetOpenIDConnectProvider(ctx, &iam.GetOpenIDConnectProviderInput{
		OpenIDConnectProviderArn: awssdk.String(plan.OIDCProviderARN),
	})
	if err != nil {
		t.Fatalf("GetOpenIDConnectProvider: %v", err)
	}
	foundClient := false
	for _, clientID := range got.ClientIDList {
		if clientID == "sts.amazonaws.com" {
			foundClient = true
			break
		}
	}
	if !foundClient {
		t.Fatalf("ClientIDList = %#v", got.ClientIDList)
	}
	if _, err := client.TagOpenIDConnectProvider(ctx, &iam.TagOpenIDConnectProviderInput{
		OpenIDConnectProviderArn: awssdk.String(plan.OIDCProviderARN),
		Tags:                     tags,
	}); err != nil {
		t.Fatalf("TagOpenIDConnectProvider: %v", err)
	}
}
