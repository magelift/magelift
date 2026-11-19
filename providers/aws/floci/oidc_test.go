//go:build floci

package floci_test

import (
	"context"
	"errors"
	"testing"
	"time"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"

	"github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/aws/aws-sdk-go-v2/service/iam/types"
	"github.com/aws/smithy-go"

	"github.com/magelift/magelift/providers/aws/bootstrap"
)

func TestIAMOpenIDConnectProviderAgainstFloci(t *testing.T) {
	endpoint := flociEndpoint(t)
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	configuration := flociConfig(t, ctx, endpoint)
	client := iam.NewFromConfig(configuration)

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
