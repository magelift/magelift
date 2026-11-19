package main

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	ovhresilience "github.com/magelift/magelift/internal/cloud/ovh/resilience"
)

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
				t.Fatalf("command = %q", command)
			}
			gotArgs = append([]string(nil), args...)
			gotEnv = append([]string(nil), env...)
			return "1\n", nil
		},
	}
	if _, err := executor.Query(context.Background(), mysqlEndpoint{Host: "mysql.example", Port: 20184}, "avnadmin", "secret-value", "defaultdb", "SELECT 1"); err != nil {
		t.Fatal(err)
	}
	if containsArg(gotArgs, "secret-value") || containsArg(gotArgs, "MYSQL_PWD") {
		t.Fatalf("password leaked into mysql arguments: %#v", gotArgs)
	}
	if len(gotEnv) != 1 || gotEnv[0] != "MYSQL_PWD=secret-value" {
		t.Fatalf("environment = %#v", gotEnv)
	}
	if !containsArg(gotArgs, "--ssl-mode=REQUIRED") || !containsArg(gotArgs, "mysql.example") || !containsArg(gotArgs, "20184") {
		t.Fatalf("mysql arguments do not prove TLS endpoint selection: %#v", gotArgs)
	}
}

func TestRunExternalCommandIncludesStderr(t *testing.T) {
	t.Parallel()
	output, err := runExternalCommand(context.Background(), "/bin/sh", []string{"-c", "printf 'diagnostic\\n' >&2; exit 7"}, []string{"MYSQL_PWD=secret-value"})
	if err == nil || !strings.Contains(err.Error(), "diagnostic") {
		t.Fatalf("error = %v, want command stderr", err)
	}
	if output != "" {
		t.Fatalf("output = %q, want empty output", output)
	}
}

func TestSelectOVHMySQLEndpoint(t *testing.T) {
	endpoint, err := selectOVHMySQLEndpoint([]ovhDatabaseEndpoint{
		{Component: "postgresql", Domain: "postgres.example", Port: 5432, SSL: true},
		{Component: "mysql", Domain: "mysql.example", Port: 20184, SSLMode: "REQUIRED"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if endpoint.Host != "mysql.example" || endpoint.Port != 20184 {
		t.Fatalf("endpoint = %#v", endpoint)
	}

	fromURI, err := selectOVHMySQLEndpoint([]ovhDatabaseEndpoint{{Component: "mysql", URI: "mysql://mysql-uri.example:20185/defaultdb", Scheme: "mysqls"}})
	if err != nil {
		t.Fatal(err)
	}
	if fromURI.Host != "mysql-uri.example" || fromURI.Port != 20185 {
		t.Fatalf("URI endpoint = %#v", fromURI)
	}

	if _, err := selectOVHMySQLEndpoint([]ovhDatabaseEndpoint{{Component: "mysql", Domain: "mysql.example", Port: 20184}}); err == nil || !strings.Contains(err.Error(), "TLS") {
		t.Fatalf("insecure endpoint error = %v", err)
	}
}

func TestOVHEndpointResolverUsesRestoredResourceIdentity(t *testing.T) {
	api := &recordingOVHAPI{response: ovhDatabaseServiceResponse{Endpoints: []ovhDatabaseEndpoint{{Component: "mysql", Domain: "restore.example", Port: 20184, SSL: true}}}}
	resolver := ovhEndpointResolver{client: api, project: "project", engine: "mysql"}
	endpoint, err := resolver.Resolve(context.Background(), "ovh-database://mysql/restore")
	if err != nil {
		t.Fatal(err)
	}
	if endpoint.Host != "restore.example" || endpoint.Port != 20184 || api.path != "/cloud/project/project/database/mysql/restore" {
		t.Fatalf("endpoint = %#v path=%q", endpoint, api.path)
	}
}

func TestOVHDatabaseCredentialResolverResetsRestoredPrimaryUser(t *testing.T) {
	api := &recordingCredentialAPI{}
	resolver := ovhDatabaseCredentialResolver{client: api, project: "project", engine: "mysql", user: "avnadmin"}
	password, err := resolver.Resolve(context.Background(), "ovh-database://mysql/restore")
	if err != nil {
		t.Fatal(err)
	}
	if password != "restored-secret" || api.getPath != "/cloud/project/project/database/mysql/restore/user/user-1" || api.postPath != "/cloud/project/project/database/mysql/restore/user/user-1/credentials/reset" {
		t.Fatalf("password=%q get=%q post=%q", password, api.getPath, api.postPath)
	}
}

func TestParseOVHResourceRejectsForeignShape(t *testing.T) {
	engine, instance, err := parseOVHResource("ovh-database://mysql/service-1")
	if err != nil || engine != "mysql" || instance != "service-1" {
		t.Fatalf("valid resource = %q/%q err=%v", engine, instance, err)
	}
	for _, resource := range []string{
		"scaleway-rdb://service-1",
		"ovh-database://mysql/service-1?foreign=true",
		"ovh-database://mysql/service/extra",
	} {
		if _, _, err := parseOVHResource(resource); err == nil {
			t.Fatalf("parseOVHResource(%q) error = nil", resource)
		}
	}
}

func TestSQLFixtureVerifierUsesRestoredResourceIdentity(t *testing.T) {
	queryer := &recordingQueryer{responses: []string{
		"1\tmarker\tfixture\t" + recoveryPayload,
		"",
		"1",
	}}
	var resolved []string
	verifier := sqlFixtureVerifier{
		queryer: queryer,
		resolveEndpoint: func(_ context.Context, resource string) (mysqlEndpoint, error) {
			resolved = append(resolved, resource)
			return mysqlEndpoint{Host: "restore.example", Port: 20184}, nil
		},
		resolvePassword: func(_ context.Context, resource string) (string, error) {
			if resource != "ovh-database://mysql/restore" {
				t.Fatalf("password resource = %q", resource)
			}
			return "restored-secret", nil
		},
		user: "avnadmin", password: "secret", database: recoveryDatabase, payload: recoveryPayload,
	}
	result, err := verifier.Verify(context.Background(), ovhresilience.RecoveryVerificationRequest{Resource: "ovh-database://mysql/restore", FixtureID: "fixture", OwnershipMarker: "marker"})
	if err != nil {
		t.Fatal(err)
	}
	if !result.ApplicationReadsVerified || !result.PermissionsVerified || !result.ServiceHealthVerified {
		t.Fatalf("verification = %#v", result)
	}
	if len(resolved) != 1 || resolved[0] != "ovh-database://mysql/restore" {
		t.Fatalf("resolved resources = %#v", resolved)
	}
	if len(queryer.statements) != 3 || !strings.Contains(queryer.statements[0], recoveryTableName("marker")) || !strings.Contains(queryer.statements[1], "PRIMARY KEY") {
		t.Fatalf("statements = %#v", queryer.statements)
	}
	for _, password := range queryer.passwords {
		if password != "restored-secret" {
			t.Fatalf("query password = %q, want restored credential", password)
		}
	}
}

func TestSQLFixtureStoreUsesDefaultDatabaseAndDropsOnlyFixtureTable(t *testing.T) {
	queryer := &recordingQueryer{responses: []string{"", "0\t\t\t"}}
	store := sqlFixtureStore{
		queryer: queryer,
		resolveEndpoint: func(context.Context, string) (mysqlEndpoint, error) {
			return mysqlEndpoint{Host: "source.example", Port: 20184}, nil
		},
		user: "avnadmin", password: "secret", database: recoveryDatabase, marker: "marker", fixtureID: "fixture",
	}
	manifest, err := buildRecoveryFixtureManifest("ovh-database://mysql/source", "fixture", "marker")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Prepare(context.Background(), "ovh-database://mysql/source", manifest); err != nil {
		t.Fatal(err)
	}
	if err := store.Delete(context.Background(), "ovh-database://mysql/source"); err != nil {
		t.Fatal(err)
	}
	if len(queryer.statements) != 4 {
		t.Fatalf("statements = %#v", queryer.statements)
	}
	if strings.Contains(strings.Join(queryer.statements, "\n"), "DATABASE") {
		t.Fatalf("fixture touched database-level DDL: %#v", queryer.statements)
	}
	if !strings.Contains(queryer.statements[len(queryer.statements)-1], "DROP TABLE IF EXISTS") || !strings.Contains(queryer.statements[len(queryer.statements)-1], recoveryTableName("marker")) {
		t.Fatalf("cleanup statement = %q", queryer.statements[len(queryer.statements)-1])
	}
}

func TestValidateCIDR(t *testing.T) {
	if err := validateCIDR("198.51.100.7/32"); err != nil {
		t.Fatal(err)
	}
	if err := validateCIDR("198.51.100.7"); err == nil {
		t.Fatal("bare IP was accepted as a CIDR")
	}
}

type recordingQueryer struct {
	statements []string
	responses  []string
	passwords  []string
}

type recordingOVHAPI struct {
	path     string
	response ovhDatabaseServiceResponse
}

func (api *recordingOVHAPI) GetWithContext(_ context.Context, path string, result interface{}) error {
	api.path = path
	response, ok := result.(*ovhDatabaseServiceResponse)
	if !ok {
		return errors.New("unexpected OVH response type")
	}
	*response = api.response
	return nil
}

type recordingCredentialAPI struct {
	getPath  string
	postPath string
}

func (api *recordingCredentialAPI) GetWithContext(_ context.Context, path string, result interface{}) error {
	api.getPath = path
	if raw, ok := result.(*json.RawMessage); ok {
		*raw = json.RawMessage(`["user-1"]`)
		return nil
	}
	user, ok := result.(*ovhDatabaseUser)
	if !ok {
		return errors.New("unexpected OVH credential GET response type")
	}
	*user = ovhDatabaseUser{ID: "user-1", Username: "avnadmin"}
	return nil
}

func (api *recordingCredentialAPI) PostWithContext(_ context.Context, path string, _ interface{}, result interface{}) error {
	api.postPath = path
	reset, ok := result.(*ovhDatabaseCredentialReset)
	if !ok {
		return errors.New("unexpected OVH credential POST response type")
	}
	reset.Password = "restored-secret"
	return nil
}

func (queryer *recordingQueryer) Query(_ context.Context, _ mysqlEndpoint, _, password, _, statement string) (string, error) {
	queryer.statements = append(queryer.statements, statement)
	queryer.passwords = append(queryer.passwords, password)
	if len(queryer.responses) == 0 {
		return "", nil
	}
	response := queryer.responses[0]
	queryer.responses = queryer.responses[1:]
	return response, nil
}

func containsArg(args []string, wanted string) bool {
	for _, arg := range args {
		if arg == wanted {
			return true
		}
	}
	return false
}
