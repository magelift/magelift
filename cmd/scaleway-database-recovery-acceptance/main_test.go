package main

import (
	"context"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/magelift/magelift/internal/cloud/scaleway/resilience"
	sdk "github.com/magelift/magelift/sdk/v1"
	rdb "github.com/scaleway/scaleway-sdk-go/api/rdb/v1"
)

type blockingResilienceClient struct {
	startDeadline time.Time
}

func (client *blockingResilienceClient) Start(ctx context.Context, _ sdk.ResilienceOperationRequest) (sdk.ResilienceOperationObservation, error) {
	client.startDeadline, _ = ctx.Deadline()
	<-ctx.Done()
	return sdk.ResilienceOperationObservation{}, ctx.Err()
}

func (*blockingResilienceClient) Poll(context.Context, string) (sdk.ResilienceOperationObservation, error) {
	return sdk.ResilienceOperationObservation{}, nil
}

func (*blockingResilienceClient) Inventory(context.Context, string) ([]sdk.ResilienceInventoryResource, error) {
	return nil, nil
}

func TestStartAndAwaitBoundsProviderStart(t *testing.T) {
	client := &blockingResilienceClient{}
	policy := sdk.ResilienceOperationPolicy{Timeout: 20 * time.Millisecond, PollInterval: time.Millisecond, MaxAttempts: 1}
	started := time.Now()
	_, err := startAndAwait(context.Background(), client, sdk.ResilienceOperationRequest{Action: sdk.ResilienceBackup}, policy, io.Discard)
	if err == nil {
		t.Fatal("startAndAwait() error = nil, want deadline error")
	}
	if client.startDeadline.IsZero() || client.startDeadline.After(started.Add(250*time.Millisecond)) {
		t.Fatalf("provider start deadline = %v, want a short operation deadline", client.startDeadline)
	}
}

func TestValidatePart(t *testing.T) {
	for _, test := range []struct {
		name    string
		value   string
		wantErr bool
	}{
		{name: "valid", value: "value"},
		{name: "empty", wantErr: true},
		{name: "newline", value: "value\nwith-control", wantErr: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := validatePart(test.value, "test")
			if (err != nil) != test.wantErr {
				t.Fatalf("validatePart() error = %v, wantErr=%t", err, test.wantErr)
			}
		})
	}
}

func TestContainsInventoryRequiresOwnedLiveResource(t *testing.T) {
	resources := []sdk.ResilienceInventoryResource{
		{Identity: "foreign", Owned: false, Live: true},
		{Identity: "gone", Owned: true, Live: false},
		{Identity: "owned", Owned: true, Live: true},
	}
	if !containsInventory(resources, "owned") {
		t.Fatal("owned live resource was not found")
	}
	if containsInventory(resources, "foreign") || containsInventory(resources, "gone") {
		t.Fatal("foreign or non-live resource was treated as an owned live resource")
	}
}

func TestCommandMySQLExecutorKeepsPasswordOutOfArguments(t *testing.T) {
	var gotArgs []string
	var gotEnv []string
	executor := commandMySQLExecutor{
		image: "mysql:8.4",
		lookup: func(name string) (string, error) {
			if name == "mysql" {
				return "/usr/bin/mysql", nil
			}
			return "", errors.New("not found")
		},
		run: func(_ context.Context, command string, args, env []string) (string, error) {
			if command != "/usr/bin/mysql" {
				t.Fatalf("command = %q, want mysql", command)
			}
			gotArgs = append([]string(nil), args...)
			gotEnv = append([]string(nil), env...)
			return "ok", nil
		},
	}
	if _, err := executor.Query(context.Background(), mysqlEndpoint{Host: "db.example", Port: 4556}, "magelift", "secret-value", recoveryDatabase, "SELECT 1"); err != nil {
		t.Fatal(err)
	}
	if containsArg(gotArgs, "secret-value") {
		t.Fatalf("password leaked into mysql arguments: %v", gotArgs)
	}
	if !reflect.DeepEqual(gotEnv, []string{"MYSQL_PWD=secret-value"}) {
		t.Fatalf("environment = %v, want MYSQL_PWD only", gotEnv)
	}
	if !containsArg(gotArgs, "4556") || !containsArg(gotArgs, "--ssl-mode=REQUIRED") {
		t.Fatalf("mysql arguments = %v, want explicit port and TLS", gotArgs)
	}
}

func TestSelectPublicRDBEndpoint(t *testing.T) {
	privateHost := "private.example"
	publicHost := "public.example"
	endpoint, err := selectPublicRDBEndpoint(&rdb.Instance{Endpoints: []*rdb.Endpoint{
		{Hostname: &privateHost, Port: 3306, PrivateNetwork: &rdb.EndpointPrivateNetworkDetails{}},
		{Hostname: &publicHost, Port: 4556, LoadBalancer: &rdb.EndpointLoadBalancerDetails{}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if endpoint != (mysqlEndpoint{Host: publicHost, Port: 4556}) {
		t.Fatalf("endpoint = %#v, want public load-balancer endpoint", endpoint)
	}
}

func TestParseScalewayResourceRejectsForeignShape(t *testing.T) {
	if got, err := parseScalewayResource("scaleway-rdb://instance-1"); err != nil || got != "instance-1" {
		t.Fatalf("parseScalewayResource() = %q, %v", got, err)
	}
	for _, resource := range []string{"gcp-cloud-sql://instance-1", "scaleway-rdb://instances/instance-1", "scaleway-rdb://instance-1?secret"} {
		if _, err := parseScalewayResource(resource); err == nil {
			t.Fatalf("parseScalewayResource(%q) error = nil", resource)
		}
	}
}

func TestSQLFixtureVerifierUsesRestoredResourceIdentity(t *testing.T) {
	queryer := recordingMySQLQueryer{}
	var resolved string
	verifier := sqlFixtureVerifier{
		queryer: &queryer,
		resolveEndpoint: func(_ context.Context, resource string) (mysqlEndpoint, error) {
			resolved = resource
			return mysqlEndpoint{Host: "restored.example", Port: 4556}, nil
		},
		user: "magelift", password: "secret-value", database: recoveryDatabase, payload: recoveryPayload,
	}
	request := resilience.RecoveryVerificationRequest{
		Resource: "scaleway-rdb://restored-instance", FixtureID: "fixture-1", OwnershipMarker: "owner/1",
	}
	verification, err := verifier.Verify(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if resolved != request.Resource {
		t.Fatalf("resolved resource = %q, want %q", resolved, request.Resource)
	}
	if !verification.ManifestVerified || !verification.CountsVerified || !verification.ApplicationReadsVerified || !verification.PermissionsVerified || !verification.ServiceHealthVerified {
		t.Fatalf("verification = %#v", verification)
	}
	if len(queryer.calls) != 3 || !strings.Contains(queryer.calls[0].statement, "SELECT COUNT") || queryer.calls[2].statement != "SELECT 1" {
		t.Fatalf("query calls = %#v", queryer.calls)
	}
}

type recordedMySQLQuery struct {
	endpoint  mysqlEndpoint
	statement string
}

type recordingMySQLQueryer struct {
	calls []recordedMySQLQuery
}

func (queryer *recordingMySQLQueryer) Query(_ context.Context, endpoint mysqlEndpoint, _, _, _, statement string) (string, error) {
	queryer.calls = append(queryer.calls, recordedMySQLQuery{endpoint: endpoint, statement: statement})
	switch {
	case strings.Contains(statement, "SELECT COUNT"):
		return "1\towner/1\tfixture-1\t" + recoveryPayload, nil
	case statement == "SELECT 1":
		return "1", nil
	default:
		return "", nil
	}
}

func containsArg(args []string, wanted string) bool {
	for _, arg := range args {
		if arg == wanted {
			return true
		}
	}
	return false
}
