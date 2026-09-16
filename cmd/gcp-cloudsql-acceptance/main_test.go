package main

import (
	"context"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"

	"github.com/magelift/magelift/internal/certification"
	gcpresilience "github.com/magelift/magelift/internal/cloud/gcp/resilience"
	"github.com/magelift/magelift/sdk"
)

func TestParseRecoveryDestination(t *testing.T) {
	destination, approval, err := parseRecoveryDestination("in-place")
	if err != nil || destination != sdk.RecoverySameRegion || approval != "gcp-cloudsql-acceptance-in-place" {
		t.Fatalf("in-place destination = %q approval=%q err=%v", destination, approval, err)
	}
	destination, approval, err = parseRecoveryDestination("isolated")
	if err != nil || destination != sdk.RecoverySameRegionIsolated || approval != "" {
		t.Fatalf("isolated destination = %q approval=%q err=%v", destination, approval, err)
	}
	if _, _, err := parseRecoveryDestination("somewhere"); err == nil {
		t.Fatal("unknown destination was accepted")
	}
}

func TestRunRejectsUnknownDestinationBeforePassword(t *testing.T) {
	err := run(context.Background(), []string{
		"--project=demo", "--instance=orders", "--marker=marker", "--fixture=fixture", "--destination=somewhere",
	}, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "destination must be isolated or in-place") {
		t.Fatalf("unknown destination error = %v", err)
	}
}

func TestValidatePart(t *testing.T) {
	for _, test := range []struct {
		name      string
		value     string
		reject    bool
		wantError bool
	}{
		{name: "valid", value: "magelift-live-abc", reject: true},
		{name: "slash", value: "projects/demo", reject: true, wantError: true},
		{name: "query", value: "magelift-live-abc?x=1", reject: true, wantError: true},
		{name: "newline", value: "magelift-live-abc\n", reject: true, wantError: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := validatePart(test.value, "value", test.reject)
			if (err != nil) != test.wantError {
				t.Fatalf("validatePart(%q) error = %v, wantError=%v", test.value, err, test.wantError)
			}
		})
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
	output, err := executor.Query(context.Background(), "192.0.2.10", "root", "secret-value", "magelift_recovery", "SELECT 1")
	if err != nil {
		t.Fatal(err)
	}
	if output != "1" || command != "/usr/bin/docker" {
		t.Fatalf("query output or command = %q, %q", output, command)
	}
	joined := strings.Join(args, " ")
	if strings.Contains(joined, "secret-value") {
		t.Fatalf("password leaked into command arguments: %v", args)
	}
	if !reflect.DeepEqual(env, []string{"MYSQL_PWD=secret-value"}) {
		t.Fatalf("command environment = %v", env)
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

func TestSQLFixtureVerifierUsesRestoredResourceIdentity(t *testing.T) {
	queryer := &recordingMySQLQueryer{output: map[string]string{
		"SELECT COUNT(*)": "1\tmarker\tfixture\t" + recoveryPayload,
		"SELECT 1":        "1",
	}}
	var gotProject, gotInstance string
	verifier := sqlFixtureVerifier{
		queryer: queryer,
		resolveHost: func(_ context.Context, project, instance string) (string, error) {
			gotProject, gotInstance = project, instance
			return "192.0.2.20", nil
		},
		user: "root", password: "secret-value", database: recoveryDatabase, payload: recoveryPayload,
	}
	verification, err := verifier.Verify(context.Background(), gcpRecoveryVerificationRequest("marker", "fixture", "gcp-cloud-sql://projects/recovery/instances/restored"))
	if err != nil {
		t.Fatal(err)
	}
	if gotProject != "recovery" || gotInstance != "restored" {
		t.Fatalf("resolved resource = %s/%s, want recovery/restored", gotProject, gotInstance)
	}
	if !verification.ManifestVerified || !verification.CountsVerified || !verification.ApplicationReadsVerified || !verification.PermissionsVerified || !verification.SecretReferencesVerified || !verification.ServiceHealthVerified {
		t.Fatalf("verification = %#v", verification)
	}
	if len(queryer.queries) != 3 {
		t.Fatalf("queries = %v, want fixture, permissions, and health queries", queryer.queries)
	}
}

func gcpRecoveryVerificationRequest(marker, fixture, resource string) gcpresilience.RecoveryVerificationRequest {
	return gcpresilience.RecoveryVerificationRequest{DataClass: "database", Resource: resource, FixtureID: fixture, OwnershipMarker: marker}
}

func TestFixtureRowMatchingRejectsUnexpectedContent(t *testing.T) {
	if !fixtureRowMatches("1\tmarker\tfixture\t"+recoveryPayload, "marker", "fixture") {
		t.Fatal("expected known fixture row to match")
	}
	if fixtureRowMatches("1\tforeign\tfixture\t"+recoveryPayload, "marker", "fixture") {
		t.Fatal("foreign fixture row matched")
	}
	if fixtureRowMatches("1\tmarker\tfixture\t"+certification.DatabaseRecoveryCorruptPayload, "marker", "fixture") {
		t.Fatal("corrupted fixture row matched the known payload")
	}
	if !fixtureRowIsCorrupt("1\tmarker\tfixture\t"+certification.DatabaseRecoveryCorruptPayload, "marker", "fixture", certification.DatabaseRecoveryCorruptPayload) {
		t.Fatal("expected corrupted fixture row to match")
	}
	if fixtureRowIsCorrupt("1\tmarker\tfixture\t"+recoveryPayload, "marker", "fixture", recoveryPayload) {
		t.Fatal("known fixture row was classified as corrupt")
	}
	if !fixtureRowIsEmpty("0\t\t\t") {
		t.Fatal("empty fixture row was not recognized")
	}
}

func TestSQLFixtureStoreCorruptsExistingFixtureRow(t *testing.T) {
	queryer := &recordingMySQLQueryer{output: map[string]string{
		"UPDATE":          "",
		"SELECT COUNT(*)": "1\tmarker\tfixture\t" + certification.DatabaseRecoveryCorruptPayload,
	}}
	store := sqlFixtureStore{
		queryer: queryer,
		resolveHost: func(context.Context, string, string) (string, error) {
			return "192.0.2.20", nil
		},
		user: "root", password: "secret-value", database: recoveryDatabase, marker: "marker", fixtureID: "fixture",
	}
	if err := store.Overwrite(context.Background(), "gcp-cloud-sql://projects/recovery/instances/source", certification.DatabaseRecoveryCorruptPayload); err != nil {
		t.Fatal(err)
	}
	if len(queryer.queries) != 2 || !strings.Contains(queryer.queries[0], "UPDATE") || !strings.Contains(queryer.queries[1], "SELECT COUNT(*)") {
		t.Fatalf("queries = %v", queryer.queries)
	}
	if err := store.Overwrite(context.Background(), "gcp-cloud-sql://projects/recovery/instances/source", recoveryPayload); err == nil {
		t.Fatal("non-corrupt overwrite payload was accepted")
	}
}
