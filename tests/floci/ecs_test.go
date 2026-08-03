//go:build floci

package floci_test

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	"github.com/aws/aws-sdk-go-v2/service/ecs/types"

	"github.com/acourtiol/magelift/internal/cloud/aws/operations"
)

func TestECSRuntimeHealthAgainstFloci(t *testing.T) {
	if os.Getenv("MAGELIFT_FLOCI") != "1" {
		t.Skip("set MAGELIFT_FLOCI=1 to run the Floci integration test")
	}
	endpoint := os.Getenv("MAGELIFT_FLOCI_ENDPOINT")
	if endpoint == "" {
		endpoint = "http://localhost:4566"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	configuration, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithRegion("us-east-1"),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider("test", "test", "")),
	)
	if err != nil {
		t.Fatal(err)
	}
	client := ecs.NewFromConfig(configuration, func(options *ecs.Options) { options.BaseEndpoint = awssdk.String(endpoint) })
	clusterName := "magelift-floci-ecs"
	serviceName := "magelift-floci-runtime"
	cluster, err := client.CreateCluster(ctx, &ecs.CreateClusterInput{ClusterName: awssdk.String(clusterName)})
	if err != nil {
		t.Fatal(err)
	}
	defer client.DeleteCluster(context.Background(), &ecs.DeleteClusterInput{Cluster: cluster.Cluster.ClusterArn})
	taskDefinition, err := client.RegisterTaskDefinition(ctx, &ecs.RegisterTaskDefinitionInput{
		Family: awssdk.String("magelift-floci-runtime"), Cpu: awssdk.String("256"), Memory: awssdk.String("512"),
		NetworkMode: types.NetworkModeAwsvpc, RequiresCompatibilities: []types.Compatibility{types.CompatibilityFargate},
		ContainerDefinitions: []types.ContainerDefinition{{Name: awssdk.String("web"), Image: awssdk.String("registry.example.invalid/shop@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"), Essential: awssdk.Bool(true)}},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer client.DeregisterTaskDefinition(context.Background(), &ecs.DeregisterTaskDefinitionInput{TaskDefinition: taskDefinition.TaskDefinition.TaskDefinitionArn})
	if _, err := client.CreateService(ctx, &ecs.CreateServiceInput{
		Cluster: cluster.Cluster.ClusterArn, ServiceName: awssdk.String(serviceName), TaskDefinition: taskDefinition.TaskDefinition.TaskDefinitionArn,
		DesiredCount: awssdk.Int32(0), LaunchType: types.LaunchTypeFargate,
		NetworkConfiguration: &types.NetworkConfiguration{AwsvpcConfiguration: &types.AwsVpcConfiguration{AssignPublicIp: types.AssignPublicIpDisabled, Subnets: []string{"subnet-floci-a"}, SecurityGroups: []string{"sg-floci"}}},
	}); err != nil {
		t.Fatal(err)
	}
	defer client.DeleteService(context.Background(), &ecs.DeleteServiceInput{Cluster: cluster.Cluster.ClusterArn, Service: awssdk.String(serviceName), Force: awssdk.Bool(true)})
	t.Setenv("AWS_ACCESS_KEY_ID", "test")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "test")
	t.Setenv("MAGELIFT_AWS_ENDPOINT_URL", endpoint)
	store, err := operations.NewRuntime(ctx, "us-east-1")
	if err != nil {
		t.Fatal(err)
	}
	health, err := store.Check(ctx, clusterName, serviceName)
	if err != nil {
		t.Fatal(err)
	}
	if health.Cluster != clusterName || health.Service != serviceName || health.DesiredCount != 0 || health.RunningCount != 0 {
		t.Fatalf("runtime health = %#v", health)
	}
}

func TestECSCandidateRegistrationAgainstFloci(t *testing.T) {
	if os.Getenv("MAGELIFT_FLOCI") != "1" {
		t.Skip("set MAGELIFT_FLOCI=1 to run the Floci integration test")
	}
	endpoint := os.Getenv("MAGELIFT_FLOCI_ENDPOINT")
	if endpoint == "" {
		endpoint = "http://localhost:4566"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	configuration, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithRegion("us-east-1"),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider("test", "test", "")),
	)
	if err != nil {
		t.Fatal(err)
	}
	client := ecs.NewFromConfig(configuration, func(options *ecs.Options) { options.BaseEndpoint = awssdk.String(endpoint) })
	oldImage := "registry.example.invalid/shop@sha256:" + strings.Repeat("b", 64)
	newImage := "registry.example.invalid/shop@sha256:" + strings.Repeat("a", 64)
	definition, err := client.RegisterTaskDefinition(ctx, &ecs.RegisterTaskDefinitionInput{
		Family: awssdk.String("magelift-floci-candidate"), Cpu: awssdk.String("256"), Memory: awssdk.String("512"),
		NetworkMode: types.NetworkModeAwsvpc, RequiresCompatibilities: []types.Compatibility{types.CompatibilityFargate},
		ContainerDefinitions: []types.ContainerDefinition{
			{Name: awssdk.String("php-fpm"), Image: awssdk.String(oldImage), Essential: awssdk.Bool(true)},
			{Name: awssdk.String("web"), Image: awssdk.String(oldImage), Essential: awssdk.Bool(true)},
			{Name: awssdk.String("varnish"), Image: awssdk.String("public.ecr.aws/varnish:7"), Essential: awssdk.Bool(true)},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if definition == nil || definition.TaskDefinition == nil || definition.TaskDefinition.TaskDefinitionArn == nil {
		t.Fatal("Floci returned no task definition ARN")
	}
	defer client.DeregisterTaskDefinition(context.Background(), &ecs.DeregisterTaskDefinitionInput{TaskDefinition: definition.TaskDefinition.TaskDefinitionArn})
	store, err := operations.NewDeploymentFromClient(client)
	if err != nil {
		t.Fatal(err)
	}
	candidate, err := store.RegisterCandidate(ctx, operations.CandidateRequest{
		Cluster: "magelift-floci-ecs", TaskDefinitionARN: awssdk.ToString(definition.TaskDefinition.TaskDefinitionArn),
		ImageDigest: newImage, PrivateSubnetIDs: []string{"subnet-floci-a"}, SecurityGroupID: "sg-floci",
	})
	if err != nil {
		t.Fatal(err)
	}
	defer client.DeregisterTaskDefinition(context.Background(), &ecs.DeregisterTaskDefinitionInput{TaskDefinition: awssdk.String(candidate.TaskDefinitionARN)})
	registered, err := client.DescribeTaskDefinition(ctx, &ecs.DescribeTaskDefinitionInput{TaskDefinition: awssdk.String(candidate.TaskDefinitionARN)})
	if err != nil {
		t.Fatal(err)
	}
	if registered == nil || registered.TaskDefinition == nil || len(registered.TaskDefinition.ContainerDefinitions) != 3 {
		t.Fatalf("candidate definition = %#v", registered)
	}
	if got := awssdk.ToString(registered.TaskDefinition.ContainerDefinitions[0].Image); got != newImage {
		t.Fatalf("php-fpm image = %q, want %q", got, newImage)
	}
	if got := awssdk.ToString(registered.TaskDefinition.ContainerDefinitions[1].Image); got != newImage {
		t.Fatalf("web image = %q, want %q", got, newImage)
	}
	if got := awssdk.ToString(registered.TaskDefinition.ContainerDefinitions[2].Image); got != "public.ecr.aws/varnish:7" {
		t.Fatalf("varnish image = %q, sidecar was unexpectedly changed", got)
	}
}
