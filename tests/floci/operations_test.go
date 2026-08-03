//go:build floci

package floci_test

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	awscfg "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	cloudwatchlogstypes "github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs/types"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"

	"github.com/magelift/magelift/internal/cloud/aws/operations"
)

func TestSecretsManagerAgainstFloci(t *testing.T) {
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
	client := secretsmanager.NewFromConfig(configuration, func(options *secretsmanager.Options) {
		options.BaseEndpoint = awssdk.String(endpoint)
	})
	name := "magelift/floci/operations-secret"
	created, err := client.CreateSecret(ctx, &secretsmanager.CreateSecretInput{
		Name:         awssdk.String(name),
		SecretString: awssdk.String(`{"password":"initial"}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_, _ = client.DeleteSecret(context.Background(), &secretsmanager.DeleteSecretInput{
			SecretId:                   created.ARN,
			ForceDeleteWithoutRecovery: awssdk.Bool(true),
		})
	}()
	if created == nil || created.ARN == nil {
		t.Fatal("Floci returned no secret ARN")
	}
	if _, err := client.PutSecretValue(ctx, &secretsmanager.PutSecretValueInput{
		SecretId:     created.ARN,
		SecretString: awssdk.String(`{"password":"updated"}`),
	}); err != nil {
		t.Fatal(err)
	}
	value, err := client.GetSecretValue(ctx, &secretsmanager.GetSecretValueInput{SecretId: created.ARN})
	if err != nil {
		t.Fatal(err)
	}
	if awssdk.ToString(value.SecretString) != `{"password":"updated"}` {
		t.Fatalf("secret value = %q", awssdk.ToString(value.SecretString))
	}
	listed, err := client.ListSecrets(ctx, &secretsmanager.ListSecretsInput{})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, secret := range listed.SecretList {
		if awssdk.ToString(secret.Name) == name {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("secret %q was not listed", name)
	}
}

func TestCloudWatchLogsAgainstFloci(t *testing.T) {
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
	client := cloudwatchlogs.NewFromConfig(configuration, func(options *cloudwatchlogs.Options) {
		options.BaseEndpoint = awssdk.String(endpoint)
	})
	group := "/magelift/floci/operations"
	stream := "web"
	if _, err := client.CreateLogGroup(ctx, &cloudwatchlogs.CreateLogGroupInput{LogGroupName: awssdk.String(group)}); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_, _ = client.DeleteLogGroup(context.Background(), &cloudwatchlogs.DeleteLogGroupInput{LogGroupName: awssdk.String(group)})
	}()
	if _, err := client.CreateLogStream(ctx, &cloudwatchlogs.CreateLogStreamInput{LogGroupName: awssdk.String(group), LogStreamName: awssdk.String(stream)}); err != nil {
		t.Fatal(err)
	}
	message := "magelift floci log contract"
	if _, err := client.PutLogEvents(ctx, &cloudwatchlogs.PutLogEventsInput{
		LogGroupName:  awssdk.String(group),
		LogStreamName: awssdk.String(stream),
		LogEvents: []cloudwatchlogstypes.InputLogEvent{{
			Message:   awssdk.String(message),
			Timestamp: awssdk.Int64(time.Now().UnixMilli()),
		}},
	}); err != nil {
		t.Fatal(err)
	}
	store, err := operations.NewFromClient(client)
	if err != nil {
		t.Fatal(err)
	}
	events, err := store.Tail(ctx, group, time.Now().Add(-time.Minute), nil, "", operations.DefaultLogLimit)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || !strings.Contains(events[0].Message, message) {
		t.Fatalf("log events = %#v", events)
	}
}
