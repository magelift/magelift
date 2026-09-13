package main

import (
	"context"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/rds"
	rdstypes "github.com/aws/aws-sdk-go-v2/service/rds/types"

	awsresilience "github.com/magelift/magelift/internal/cloud/aws/resilience"
	sdk "github.com/magelift/magelift/sdk/v1"
)

func TestRunValidatesArgumentsBeforeCreatingClients(t *testing.T) {
	err := run(context.Background(), nil, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "region is required") {
		t.Fatalf("missing region error = %v", err)
	}
}

func TestValidatePartAcceptsRDSIdentifiers(t *testing.T) {
	for _, value := range []string{"magelift-source-20260813", "db.t3.micro"} {
		pattern := safeRDSIdentifier
		if strings.Contains(value, ".") {
			pattern = safeRDSClass
		}
		if err := validatePart(value, "value", pattern); err != nil {
			t.Fatalf("validatePart(%q) = %v", value, err)
		}
	}
}

func TestValidatePartRejectsUnsafeRDSValues(t *testing.T) {
	for _, value := range []string{"", "source/name", "source\nname", "source\x00name"} {
		if err := validatePart(value, "value", safeRDSIdentifier); err == nil {
			t.Fatalf("validatePart(%q) accepted unsafe value", value)
		}
	}
}

func TestRunRejectsUnsupportedSourceKindBeforeCreatingClients(t *testing.T) {
	err := run(context.Background(), []string{"--region", "eu-west-3", "--source-kind", "database", "--instance", "source", "--restore-subnet-group", "subnet", "--restore-security-group-id", "sg-123", "--marker", "marker", "--fixture", "fixture"}, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "source-kind must be instance or cluster") {
		t.Fatalf("unsupported source kind error = %v", err)
	}
}

func TestCommandMySQLExecutorKeepsPasswordOutOfArguments(t *testing.T) {
	var command string
	var args []string
	var env []string
	executor := commandMySQLExecutor{
		image: "mysql:8.4",
		lookup: func(name string) (string, error) {
			if name == "mysql" {
				return "", errors.New("host mysql unavailable")
			}
			return "/usr/bin/docker", nil
		},
		run: func(_ context.Context, gotCommand string, gotArgs, gotEnv []string) (string, error) {
			command, args, env = gotCommand, append([]string(nil), gotArgs...), append([]string(nil), gotEnv...)
			return "1", nil
		},
	}
	output, err := executor.Query(context.Background(), "db.example.amazonaws.com", "magelift", "secret-value", recoveryDatabase, "SELECT 1")
	if err != nil {
		t.Fatal(err)
	}
	if output != "1" || command != "/usr/bin/docker" {
		t.Fatalf("query output or command = %q, %q", output, command)
	}
	if strings.Contains(strings.Join(args, " "), "secret-value") {
		t.Fatalf("password leaked into command arguments: %v", args)
	}
	if !reflect.DeepEqual(env, []string{"MYSQL_PWD=secret-value"}) {
		t.Fatalf("command environment = %v", env)
	}
	if !containsArg(args, "--ssl-mode=REQUIRED") {
		t.Fatalf("mysql arguments did not require TLS: %v", args)
	}
}

func TestSQLFixtureVerifierUsesRestoredResourceIdentity(t *testing.T) {
	queryer := &recordingMySQLQueryer{output: map[string]string{
		"SELECT COUNT(*)": "1\tmarker\tfixture\t" + recoveryPayload,
		"SELECT 1":        "1",
	}}
	var resolvedResource string
	verifier := sqlFixtureVerifier{
		queryer: queryer,
		resolveHost: func(_ context.Context, resource string) (string, error) {
			resolvedResource = resource
			return "db-restored.example.amazonaws.com", nil
		},
		user: "magelift", password: "secret-value", database: recoveryDatabase, payload: recoveryPayload,
	}
	verification, err := verifier.Verify(context.Background(), awsresilience.RecoveryVerificationRequest{
		DataClass: "database", Resource: "aws-rds://instance/restored", FixtureID: "fixture", OwnershipMarker: "marker",
	})
	if err != nil {
		t.Fatal(err)
	}
	if resolvedResource != "aws-rds://instance/restored" {
		t.Fatalf("resolved resource = %q", resolvedResource)
	}
	if !verification.ManifestVerified || !verification.CountsVerified || !verification.ApplicationReadsVerified || !verification.PermissionsVerified || !verification.SecretReferencesVerified || !verification.ServiceHealthVerified {
		t.Fatalf("verification = %#v", verification)
	}
	if len(queryer.queries) != 3 {
		t.Fatalf("queries = %v, want fixture, permissions, and health queries", queryer.queries)
	}
}

func TestResolveRDSHost(t *testing.T) {
	client := fakeRDSEndpointAPI{output: &rds.DescribeDBInstancesOutput{DBInstances: []rdstypes.DBInstance{{Endpoint: &rdstypes.Endpoint{Address: aws.String("db.example.amazonaws.com")}}}}}
	host, err := resolveRDSHost(context.Background(), client, "aws-rds://instance/restored")
	if err != nil {
		t.Fatal(err)
	}
	if host != "db.example.amazonaws.com" {
		t.Fatalf("host = %q", host)
	}
}

func TestResolveAuroraHost(t *testing.T) {
	client := fakeRDSEndpointAPI{clusterOutput: &rds.DescribeDBClustersOutput{DBClusters: []rdstypes.DBCluster{{Endpoint: aws.String("cluster.example.amazonaws.com")}}}}
	host, err := resolveRDSHost(context.Background(), client, "aws-rds://cluster/restored")
	if err != nil {
		t.Fatal(err)
	}
	if host != "cluster.example.amazonaws.com" {
		t.Fatalf("host = %q", host)
	}
}

func TestParseRDSResourceRejectsForeignShape(t *testing.T) {
	for _, resource := range []string{"", "gcp-cloud-sql://projects/p/instances/i", "aws-rds://instance/foreign/name", "aws-rds://cluster/foreign/name"} {
		if _, err := parseRDSResource(resource); err == nil {
			t.Fatalf("parseRDSResource(%q) accepted invalid identity", resource)
		}
	}
}

func TestParseDatabaseResourceAcceptsAuroraCluster(t *testing.T) {
	kind, id, err := parseDatabaseResource("aws-rds://cluster/orders")
	if err != nil || kind != "cluster" || id != "orders" {
		t.Fatalf("parseDatabaseResource = %q, %q, %v", kind, id, err)
	}
}

func TestVerifyDatabaseCleanupAllowsAuroraSourceMembers(t *testing.T) {
	api := fakeRDSEndpointAPI{clusterOutput: &rds.DescribeDBClustersOutput{DBClusters: []rdstypes.DBCluster{{
		DBClusterIdentifier: aws.String("orders"),
		DBClusterMembers:    []rdstypes.DBClusterMember{{DBInstanceIdentifier: aws.String("orders-instance")}},
	}}}}
	resources := []sdk.ResilienceInventoryResource{
		{Identity: "aws-rds://cluster/orders", Owned: true, Live: true},
		{Identity: "aws-rds://instance/orders-instance", Owned: true, Live: true},
	}
	if err := verifyDatabaseCleanup(context.Background(), api, resources, "aws-rds://cluster/orders"); err != nil {
		t.Fatal(err)
	}
	resources = append(resources, sdk.ResilienceInventoryResource{Identity: "aws-rds://cluster/restore", Owned: true, Live: true})
	if err := verifyDatabaseCleanup(context.Background(), api, resources, "aws-rds://cluster/orders"); err == nil || !strings.Contains(err.Error(), "unexpected resource") {
		t.Fatalf("unexpected Aurora cleanup inventory error = %v", err)
	}
}

type recordingMySQLQueryer struct {
	queries []string
	output  map[string]string
}

func (queryer *recordingMySQLQueryer) Query(_ context.Context, _ string, _ string, _ string, _ string, statement string) (string, error) {
	queryer.queries = append(queryer.queries, statement)
	for prefix, output := range queryer.output {
		if strings.HasPrefix(statement, prefix) {
			return output, nil
		}
	}
	return "", nil
}

type fakeRDSEndpointAPI struct {
	output        *rds.DescribeDBInstancesOutput
	clusterOutput *rds.DescribeDBClustersOutput
}

func (fake fakeRDSEndpointAPI) DescribeDBInstances(context.Context, *rds.DescribeDBInstancesInput, ...func(*rds.Options)) (*rds.DescribeDBInstancesOutput, error) {
	return fake.output, nil
}

func (fake fakeRDSEndpointAPI) DescribeDBClusters(context.Context, *rds.DescribeDBClustersInput, ...func(*rds.Options)) (*rds.DescribeDBClustersOutput, error) {
	return fake.clusterOutput, nil
}

func containsArg(args []string, want string) bool {
	for _, arg := range args {
		if arg == want {
			return true
		}
	}
	return false
}
