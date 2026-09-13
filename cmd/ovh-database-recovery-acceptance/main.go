// Command ovh-database-recovery-acceptance runs one bounded OVHcloud Public
// Cloud Database provider-backup, isolated-restore, and known-content
// application-fixture cell. The shell wrapper owns the generated encrypted
// MySQL source, its runner CIDR, and the one-time primary credential; this
// command owns the provider recovery operation, polling, endpoint resolution,
// fixture verification, and output cleanup.
package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/magelift/magelift/internal/certification"
	ovhresilience "github.com/magelift/magelift/internal/cloud/ovh/resilience"
	sdk "github.com/magelift/magelift/sdk/v1"
)

const (
	defaultTimeout          = 45 * time.Minute
	defaultOperationTimeout = 35 * time.Minute
	defaultPollInterval     = 15 * time.Second
	defaultMaxAttempts      = 140
	defaultMySQLImage       = "mysql:8.4"
	recoveryDatabase        = "defaultdb"
	recoveryPayload         = "magelift-ovh-database-recovery-v1"
	recoveryFixtureRecords  = 1
	databaseUser            = "avnadmin"
)

var (
	safeIdentifier  = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)
	safeFixturePart = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:/-]{0,127}$`)
)

func main() {
	if err := runMain(); err != nil {
		fmt.Fprintf(os.Stderr, "OVHcloud Public Cloud Database recovery acceptance failed: %v\n", err)
		os.Exit(1)
	}
}

func runMain() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return run(ctx, os.Args[1:], os.Stdout)
}

func run(parent context.Context, args []string, output io.Writer) (runErr error) {
	flags := flag.NewFlagSet("ovh-database-recovery-acceptance", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	profile := flags.String("profile", "default", "OVHcloud profile name")
	project := flags.String("project", "", "OVHcloud Public Cloud project ID")
	region := flags.String("region", "", "OVHcloud managed database region")
	engine := flags.String("engine", "mysql", "managed database engine")
	instance := flags.String("instance", "", "owned generated database source service ID")
	marker := flags.String("marker", "", "single-line MageLift ownership marker")
	fixture := flags.String("fixture", "", "single-line recovery fixture ID")
	plan := flags.String("restore-plan", "essential", "plan for the isolated restore")
	flavor := flags.String("restore-flavor", "db1-4", "flavor for the isolated restore")
	version := flags.String("restore-version", "8.4", "version for the isolated restore")
	diskGB := flags.Int("disk-gb", 80, "disk size in GB for the isolated restore when forked")
	image := flags.String("mysql-image", firstNonEmpty(os.Getenv("MAGELIFT_OVH_DATABASE_MYSQL_IMAGE"), defaultMySQLImage), "MySQL client image used when host mysql is unavailable")
	restoreIPRestriction := flags.String("restore-ip-restriction", strings.TrimSpace(os.Getenv("MAGELIFT_OVH_DATABASE_RESTORE_IP_RESTRICTION")), "CIDR allowed to reach the source and isolated restore")
	if err := flags.Parse(args); err != nil {
		return fmt.Errorf("parse flags: %w", err)
	}
	if flags.NArg() != 0 {
		return errors.New("unexpected positional arguments")
	}
	for _, part := range []struct {
		value string
		name  string
	}{
		{*profile, "profile"}, {*project, "project"}, {*region, "region"}, {*engine, "engine"},
		{*instance, "instance"}, {*marker, "marker"}, {*fixture, "fixture"},
		{*plan, "restore-plan"}, {*flavor, "restore-flavor"}, {*version, "restore-version"},
	} {
		if err := validatePart(part.value, part.name); err != nil {
			return err
		}
	}
	if !safeIdentifier.MatchString(*profile) || !safeIdentifier.MatchString(*instance) || !safeIdentifier.MatchString(*plan) || !safeIdentifier.MatchString(*flavor) || !safeIdentifier.MatchString(*version) {
		return errors.New("profile, instance, restore-plan, restore-flavor, and restore-version contain unsupported characters")
	}
	if !regexp.MustCompile(`^[0-9a-fA-F]{32}$`).MatchString(*project) {
		return errors.New("project must be a 32-character OVHcloud Public Cloud project ID")
	}
	if !regexp.MustCompile(`^[A-Z0-9-]{2,16}$`).MatchString(*region) {
		return errors.New("region must be an OVHcloud managed database region code")
	}
	if !strings.EqualFold(*engine, "mysql") {
		return errors.New("the OVHcloud application-fixture acceptance cell supports only mysql")
	}
	if *diskGB <= 0 || *diskGB > 1024 {
		return errors.New("disk-gb must be between 1 and 1024")
	}
	if err := validateCIDR(*restoreIPRestriction); err != nil {
		return fmt.Errorf("restore-ip-restriction: %w", err)
	}
	if strings.TrimSpace(*image) == "" || strings.ContainsAny(*image, " \t\r\n\x00") {
		return errors.New("mysql-image must be a non-empty single-line image reference without whitespace")
	}
	databasePassword := strings.TrimSpace(os.Getenv("MAGELIFT_OVH_DATABASE_ROOT_PASSWORD"))
	if databasePassword == "" {
		return errors.New("MAGELIFT_OVH_DATABASE_ROOT_PASSWORD is required for OVHcloud Managed Database application fixture verification")
	}
	if !safeFixturePart.MatchString(*marker) || !safeFixturePart.MatchString(*fixture) {
		return errors.New("marker and fixture must contain only letters, numbers, dots, underscores, colons, slashes, or hyphens")
	}

	ctx, cancel := context.WithTimeout(parent, defaultTimeout)
	defer cancel()
	ovhClient, err := ovhresilience.NewOVHClientFromProfile(*profile)
	if err != nil {
		return fmt.Errorf("load OVHcloud profile %q: %w", *profile, err)
	}
	resource := "ovh-database://" + strings.ToLower(*engine) + "/" + *instance
	endpointResolver := ovhEndpointResolver{client: ovhClient, project: *project, engine: strings.ToLower(*engine)}
	mysql := newCommandMySQLExecutor(*image)
	resolveEndpoint := func(resolveCtx context.Context, resolveResource string) (mysqlEndpoint, error) {
		return endpointResolver.Resolve(resolveCtx, resolveResource)
	}
	fixtureStore := sqlFixtureStore{
		queryer: mysql, resolveEndpoint: resolveEndpoint, user: databaseUser, password: databasePassword,
		database: recoveryDatabase, marker: *marker, fixtureID: *fixture,
	}
	manifest, err := buildRecoveryFixtureManifest(resource, *fixture, *marker)
	if err != nil {
		return err
	}
	fixtureAttempted := false
	cleanupFixture := func() error {
		if !fixtureAttempted {
			return nil
		}
		cleanupCtx, cleanupCancel := context.WithTimeout(context.WithoutCancel(context.Background()), 90*time.Second)
		defer cleanupCancel()
		if err := fixtureStore.Delete(cleanupCtx, resource); err != nil {
			return err
		}
		fixtureAttempted = false
		return nil
	}
	defer func() { runErr = errors.Join(runErr, cleanupFixture()) }()
	verifier := sqlFixtureVerifier{
		queryer: mysql, resolveEndpoint: resolveEndpoint, user: databaseUser, password: databasePassword,
		database: recoveryDatabase, payload: recoveryPayload,
		resolvePassword: ovhDatabaseCredentialResolver{client: ovhClient, project: *project, engine: strings.ToLower(*engine), user: databaseUser}.Resolve,
	}
	native, err := ovhresilience.NewOVHDatabaseNativeAPI(ctx, ovhresilience.NativeAPIConfig{
		DatabaseProjectID:          *project,
		DatabaseEngine:             *engine,
		DatabaseRegion:             *region,
		DatabaseSourceID:           *instance,
		DatabasePlan:               *plan,
		DatabaseFlavor:             *flavor,
		DatabaseVersion:            *version,
		DatabaseDiskGB:             *diskGB,
		DatabaseIPRestrictions:     []string{*restoreIPRestriction},
		RestoreDatabaseRegion:      *region,
		RestoreDatabasePlan:        *plan,
		RestoreDatabaseFlavor:      *flavor,
		RestoreDatabaseVersion:     *version,
		MaxDatabaseBackupAge:       24 * time.Hour,
		DatabaseBackupNotBefore:    time.Now().UTC(),
		DatabaseUsePITR:            true,
		DatabasePITRSettleDuration: 5 * time.Minute,
		Verifier:                   verifier,
	}, ovhClient)
	if err != nil {
		return fmt.Errorf("construct OVHcloud Public Cloud Database recovery translator: %w", err)
	}
	client, err := ovhresilience.NewNativeResilienceClient(native)
	if err != nil {
		return fmt.Errorf("construct OVHcloud recovery operation client: %w", err)
	}

	policy := sdk.ResilienceOperationPolicy{Timeout: defaultOperationTimeout, PollInterval: defaultPollInterval, MaxAttempts: defaultMaxAttempts}
	cell, err := certification.NewDatabaseRecoveryCell(certification.DatabaseRecoveryCellConfig{
		Operations: client, FixtureStore: fixtureStore, OperationPolicy: policy, Fixture: manifest,
		DataClass: "database", Destination: sdk.RecoverySameRegionIsolated, ResourceReference: resource,
		Marker: *marker, Protection: certification.DatabaseRecoveryProtectionOptional,
	})
	if err != nil {
		return fmt.Errorf("construct OVHcloud Public Cloud Database recovery cell: %w", err)
	}
	fixtureAttempted = true
	if err := cell.Prepare(ctx); err != nil {
		return fmt.Errorf("prepare OVHcloud Public Cloud Database application fixture: %w", err)
	}
	result, err := cell.Run(ctx)
	if err != nil {
		return fmt.Errorf("run OVHcloud Public Cloud Database recovery cell: %w", err)
	}
	backupID := result.BackupID
	restoreID := result.RestoreID
	inventory, err := client.Inventory(ctx, *marker)
	if err != nil {
		return fmt.Errorf("inventory OVHcloud Public Cloud Database recovery resources: %w", err)
	}
	if !containsInventory(inventory, resource) || !containsInventory(inventory, restoreID) {
		return fmt.Errorf("OVHcloud Public Cloud Database inventory did not contain source and isolated restore: %#v", inventory)
	}
	if err := cell.Cleanup(ctx); err != nil {
		return fmt.Errorf("clean OVHcloud Public Cloud Database recovery outputs: %w", err)
	}
	if err := cleanupFixture(); err != nil {
		return fmt.Errorf("delete OVHcloud Public Cloud Database application fixture: %w", err)
	}
	remaining, err := client.Inventory(ctx, *marker)
	if err != nil {
		return fmt.Errorf("verify OVHcloud Public Cloud Database recovery cleanup: %w", err)
	}
	if len(remaining) != 1 || !containsInventory(remaining, resource) {
		return fmt.Errorf("OVHcloud Public Cloud Database cleanup must preserve only the source service: %#v", remaining)
	}

	fmt.Fprintf(output, "OVHcloud Public Cloud Database recovery acceptance PASS region=%s source=%s backup=%s restore=%s restoreDurationSeconds=%d cleanup=verified applicationFixture=verified manifest=verified reads=verified permissions=verified health=verified protection=provider-optional\n", *region, resource, backupID, restoreID, result.RestoreDurationSeconds)
	return nil
}

type mysqlEndpoint struct {
	Host string
	Port uint32
}

type mysqlQueryer interface {
	Query(context.Context, mysqlEndpoint, string, string, string, string) (string, error)
}

type commandMySQLExecutor struct {
	image  string
	lookup func(string) (string, error)
	run    func(context.Context, string, []string, []string) (string, error)
}

func newCommandMySQLExecutor(image string) commandMySQLExecutor {
	return commandMySQLExecutor{image: image, lookup: exec.LookPath, run: runExternalCommand}
}

func (executor commandMySQLExecutor) Query(ctx context.Context, endpoint mysqlEndpoint, user, password, database, statement string) (string, error) {
	if ctx == nil {
		return "", errors.New("mysql query context is required")
	}
	if strings.TrimSpace(endpoint.Host) == "" || strings.ContainsAny(endpoint.Host, "/\r\n\x00") || len(endpoint.Host) > 253 {
		return "", errors.New("mysql query host must be a valid OVHcloud database endpoint")
	}
	if endpoint.Port == 0 || endpoint.Port > 65535 {
		return "", errors.New("mysql query port must be between 1 and 65535")
	}
	if strings.TrimSpace(user) == "" || strings.TrimSpace(password) == "" || strings.TrimSpace(statement) == "" {
		return "", errors.New("mysql query user, password, and statement are required")
	}
	args := mysqlClientArgs(endpoint, user, database, statement)
	env := []string{"MYSQL_PWD=" + password}
	command := ""
	if path, err := executor.lookup("mysql"); err == nil {
		command = path
	} else if path, err := executor.lookup("docker"); err == nil {
		command = path
		args = append([]string{"run", "--rm", "--pull=missing", "--env", "MYSQL_PWD", executor.image, "mysql"}, args...)
	} else {
		return "", errors.New("OVHcloud Managed Database application verification requires mysql or docker")
	}
	output, err := executor.run(ctx, command, args, env)
	if err != nil {
		return "", fmt.Errorf("mysql query failed: %w", err)
	}
	return strings.TrimSpace(output), nil
}

func mysqlClientArgs(endpoint mysqlEndpoint, user, database, statement string) []string {
	args := []string{
		"--protocol=TCP", "--connect-timeout=15", "--ssl-mode=REQUIRED",
		"--host", endpoint.Host, "--port", strconv.FormatUint(uint64(endpoint.Port), 10),
		"--user", user, "--batch", "--skip-column-names",
	}
	if strings.TrimSpace(database) != "" {
		args = append(args, database)
	}
	return append(args, "--execute", statement)
}

func runExternalCommand(ctx context.Context, command string, args, env []string) (string, error) {
	cmd := exec.CommandContext(ctx, command, args...)
	baseEnv := make([]string, 0, len(os.Environ())+len(env))
	for _, entry := range os.Environ() {
		if !strings.HasPrefix(entry, "MYSQL_PWD=") {
			baseEnv = append(baseEnv, entry)
		}
	}
	cmd.Env = append(baseEnv, env...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	output, err := cmd.Output()
	if err != nil {
		if detail := strings.TrimSpace(stderr.String()); detail != "" {
			return "", fmt.Errorf("%w: %s", err, detail)
		}
		return "", err
	}
	return string(output), nil
}

type ovhAPIClient interface {
	GetWithContext(context.Context, string, interface{}) error
}

type ovhCredentialAPI interface {
	GetWithContext(context.Context, string, interface{}) error
	PostWithContext(context.Context, string, interface{}, interface{}) error
}

type ovhEndpointResolver struct {
	client  ovhAPIClient
	project string
	engine  string
}

type ovhDatabaseServiceResponse struct {
	Endpoints []ovhDatabaseEndpoint `json:"endpoints"`
}

type ovhDatabaseEndpoint struct {
	Component string `json:"component"`
	Domain    string `json:"domain"`
	Port      int    `json:"port"`
	Scheme    string `json:"scheme"`
	SSL       bool   `json:"ssl"`
	SSLMode   string `json:"sslMode"`
	URI       string `json:"uri"`
}

type ovhDatabaseUser struct {
	ID       string `json:"id"`
	Username string `json:"username"`
}

type ovhDatabaseCredentialReset struct {
	Password string `json:"password"`
}

type ovhDatabaseCredentialResolver struct {
	client  ovhCredentialAPI
	project string
	engine  string
	user    string
}

func (resolver ovhDatabaseCredentialResolver) Resolve(ctx context.Context, resource string) (string, error) {
	if ctx == nil {
		return "", errors.New("OVHcloud database credential context is required")
	}
	if resolver.client == nil {
		return "", errors.New("OVHcloud database credential API is required")
	}
	engine, instanceID, err := parseOVHResource(resource)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(resolver.project) == "" || strings.TrimSpace(resolver.user) == "" {
		return "", errors.New("OVHcloud database credential project and user are required")
	}
	if strings.TrimSpace(resolver.engine) != "" && !strings.EqualFold(resolver.engine, engine) {
		return "", errors.New("OVHcloud database credential engine does not match the resource")
	}
	basePath := "/cloud/project/" + url.PathEscape(resolver.project) + "/database/" + url.PathEscape(engine) + "/" + url.PathEscape(instanceID) + "/user"
	var rawUsers json.RawMessage
	if err := resolver.client.GetWithContext(ctx, basePath, &rawUsers); err != nil {
		return "", fmt.Errorf("list OVHcloud restored database users: %w", err)
	}
	users, err := decodeOVHDatabaseUsers(rawUsers)
	if err != nil {
		return "", fmt.Errorf("decode OVHcloud restored database users: %w", err)
	}
	var userID string
	for _, candidate := range users {
		if candidate.Username == "" {
			var details ovhDatabaseUser
			if err := resolver.client.GetWithContext(ctx, basePath+"/"+url.PathEscape(candidate.ID), &details); err != nil {
				return "", fmt.Errorf("get OVHcloud restored database user %q: %w", candidate.ID, err)
			}
			candidate = details
		}
		if candidate.Username != resolver.user {
			continue
		}
		if strings.TrimSpace(candidate.ID) == "" || userID != "" {
			return "", errors.New("OVHcloud restored database primary user identity is ambiguous")
		}
		userID = candidate.ID
	}
	if userID == "" {
		return "", fmt.Errorf("OVHcloud restored database primary user %q was not found", resolver.user)
	}
	var reset ovhDatabaseCredentialReset
	resetPath := basePath + "/" + url.PathEscape(userID) + "/credentials/reset"
	if err := resolver.client.PostWithContext(ctx, resetPath, nil, &reset); err != nil {
		return "", fmt.Errorf("reset OVHcloud restored database credentials: %w", err)
	}
	password := strings.TrimSpace(reset.Password)
	if password == "" || strings.ContainsAny(password, "\r\n\x00") || len(password) > 128 {
		return "", errors.New("OVHcloud restored database credential reset returned an unusable password")
	}
	return password, nil
}

func decodeOVHDatabaseUsers(raw []byte) ([]ovhDatabaseUser, error) {
	var entries []json.RawMessage
	if err := json.Unmarshal(raw, &entries); err != nil {
		return nil, err
	}
	users := make([]ovhDatabaseUser, 0, len(entries))
	for _, entry := range entries {
		var id string
		if err := json.Unmarshal(entry, &id); err == nil {
			users = append(users, ovhDatabaseUser{ID: id})
			continue
		}
		var user ovhDatabaseUser
		if err := json.Unmarshal(entry, &user); err != nil {
			return nil, err
		}
		users = append(users, user)
	}
	return users, nil
}

func (resolver ovhEndpointResolver) Resolve(ctx context.Context, resource string) (mysqlEndpoint, error) {
	if ctx == nil {
		return mysqlEndpoint{}, errors.New("OVHcloud database endpoint context is required")
	}
	if err := ctx.Err(); err != nil {
		return mysqlEndpoint{}, err
	}
	engine, instanceID, err := parseOVHResource(resource)
	if err != nil {
		return mysqlEndpoint{}, err
	}
	if resolver.client == nil {
		return mysqlEndpoint{}, errors.New("OVHcloud database endpoint API is required")
	}
	if strings.TrimSpace(resolver.project) == "" {
		return mysqlEndpoint{}, errors.New("OVHcloud database endpoint project is required")
	}
	if strings.TrimSpace(resolver.engine) != "" && !strings.EqualFold(resolver.engine, engine) {
		return mysqlEndpoint{}, errors.New("OVHcloud database endpoint engine does not match the resource")
	}
	var response ovhDatabaseServiceResponse
	path := "/cloud/project/" + url.PathEscape(resolver.project) + "/database/" + url.PathEscape(engine) + "/" + url.PathEscape(instanceID)
	if err := resolver.client.GetWithContext(ctx, path, &response); err != nil {
		return mysqlEndpoint{}, fmt.Errorf("resolve OVHcloud database endpoint: %w", err)
	}
	return selectOVHMySQLEndpoint(response.Endpoints)
}

func selectOVHMySQLEndpoint(endpoints []ovhDatabaseEndpoint) (mysqlEndpoint, error) {
	for _, endpoint := range endpoints {
		if endpoint.Component != "" && !strings.EqualFold(endpoint.Component, "mysql") {
			continue
		}
		host := strings.TrimSpace(endpoint.Domain)
		port := endpoint.Port
		if (host == "" || port == 0) && strings.TrimSpace(endpoint.URI) != "" {
			parsed, err := url.Parse(endpoint.URI)
			if err == nil {
				if host == "" {
					host = parsed.Hostname()
				}
				if port == 0 {
					port, _ = strconv.Atoi(parsed.Port())
				}
			}
		}
		if host == "" || strings.ContainsAny(host, "/\r\n\x00") || len(host) > 253 || port <= 0 || port > 65535 {
			continue
		}
		secure := endpoint.SSL || strings.EqualFold(strings.TrimSpace(endpoint.SSLMode), "REQUIRED") || strings.EqualFold(strings.TrimSpace(endpoint.Scheme), "mysqls")
		if !secure {
			continue
		}
		return mysqlEndpoint{Host: host, Port: uint32(port)}, nil
	}
	return mysqlEndpoint{}, errors.New("OVHcloud database service has no usable TLS MySQL endpoint")
}

type sqlFixtureStore struct {
	queryer         mysqlQueryer
	resolveEndpoint func(context.Context, string) (mysqlEndpoint, error)
	user            string
	password        string
	database        string
	marker          string
	fixtureID       string
}

func (store sqlFixtureStore) Prepare(ctx context.Context, resource string, manifest certification.RecoveryFixtureManifest) error {
	if len(manifest.Classes) != 1 || manifest.Classes[0].Name != "database" || manifest.Classes[0].ExpectedRecords != recoveryFixtureRecords {
		return errors.New("OVHcloud database application fixture manifest is not the expected database fixture")
	}
	endpoint, err := store.resolveEndpoint(ctx, resource)
	if err != nil {
		return err
	}
	table := recoveryTableName(store.marker)
	createTable := "CREATE TABLE IF NOT EXISTS `" + table + "` (id BIGINT NOT NULL PRIMARY KEY, marker VARCHAR(128) NOT NULL, fixture_id VARCHAR(128) NOT NULL, payload VARCHAR(128) NOT NULL)"
	if _, err := store.queryer.Query(ctx, endpoint, store.user, store.password, store.database, createTable); err != nil {
		return fmt.Errorf("create OVHcloud database fixture table: %w", err)
	}
	row, err := store.queryer.Query(ctx, endpoint, store.user, store.password, store.database, fixtureSelect(table))
	if err != nil {
		return fmt.Errorf("inspect existing OVHcloud database fixture row: %w", err)
	}
	if fixtureRowMatches(row, store.marker, store.fixtureID) {
		return nil
	}
	if !fixtureRowIsEmpty(row) {
		return errors.New("OVHcloud database fixture table contains unexpected existing content")
	}
	insert := "INSERT INTO `" + table + "` (id, marker, fixture_id, payload) VALUES (1, " + quoteMySQLString(store.marker) + ", " + quoteMySQLString(store.fixtureID) + ", " + quoteMySQLString(recoveryPayload) + ")"
	if _, err := store.queryer.Query(ctx, endpoint, store.user, store.password, store.database, insert); err != nil {
		return fmt.Errorf("write OVHcloud database fixture row: %w", err)
	}
	return nil
}

func (store sqlFixtureStore) Delete(ctx context.Context, resource string) error {
	endpoint, err := store.resolveEndpoint(ctx, resource)
	if err != nil {
		return err
	}
	if _, err := store.queryer.Query(ctx, endpoint, store.user, store.password, store.database, "DROP TABLE IF EXISTS `"+recoveryTableName(store.marker)+"`"); err != nil {
		return fmt.Errorf("delete OVHcloud database fixture table: %w", err)
	}
	return nil
}

type sqlFixtureVerifier struct {
	queryer         mysqlQueryer
	resolveEndpoint func(context.Context, string) (mysqlEndpoint, error)
	resolvePassword func(context.Context, string) (string, error)
	user            string
	password        string
	database        string
	payload         string
}

func (verifier sqlFixtureVerifier) Verify(ctx context.Context, request ovhresilience.RecoveryVerificationRequest) (ovhresilience.RecoveryVerification, error) {
	endpoint, err := verifier.resolveEndpoint(ctx, request.Resource)
	if err != nil {
		return ovhresilience.RecoveryVerification{}, err
	}
	password := verifier.password
	if verifier.resolvePassword != nil {
		password, err = verifier.resolvePassword(ctx, request.Resource)
		if err != nil {
			return ovhresilience.RecoveryVerification{}, fmt.Errorf("resolve OVHcloud restored database credentials: %w", err)
		}
	}
	table := recoveryTableName(request.OwnershipMarker)
	row, err := verifier.queryer.Query(ctx, endpoint, verifier.user, password, verifier.database, fixtureSelect(table))
	if err != nil {
		return ovhresilience.RecoveryVerification{}, fmt.Errorf("read OVHcloud database recovery fixture: %w", err)
	}
	if !fixtureRowMatchesPayload(row, request.OwnershipMarker, request.FixtureID, verifier.payload) {
		return ovhresilience.RecoveryVerification{}, errors.New("OVHcloud database recovery fixture content did not match the expected marker and payload")
	}
	if _, err := verifier.queryer.Query(ctx, endpoint, verifier.user, password, verifier.database, "CREATE TEMPORARY TABLE `magelift_permission_probe` (id INT NOT NULL PRIMARY KEY); INSERT INTO `magelift_permission_probe` VALUES (1); DROP TEMPORARY TABLE `magelift_permission_probe`"); err != nil {
		return ovhresilience.RecoveryVerification{}, fmt.Errorf("verify OVHcloud database recovery fixture permissions: %w", err)
	}
	health, err := verifier.queryer.Query(ctx, endpoint, verifier.user, password, verifier.database, "SELECT 1")
	if err != nil || strings.TrimSpace(health) != "1" {
		if err == nil {
			err = errors.New("health query returned an unexpected result")
		}
		return ovhresilience.RecoveryVerification{}, fmt.Errorf("verify OVHcloud database recovery service health: %w", err)
	}
	return ovhresilience.RecoveryVerification{
		ManifestVerified: true, CountsVerified: true, ApplicationReadsVerified: true,
		PermissionsVerified: true, SecretReferencesVerified: true, ServiceHealthVerified: true,
	}, nil
}

func buildRecoveryFixtureManifest(resource, fixture, marker string) (certification.RecoveryFixtureManifest, error) {
	return certification.BuildRecoveryFixtureManifest(fixture, resource, []certification.RecoveryFixtureMaterial{{
		Name: "database", ExpectedRecords: recoveryFixtureRecords, ExpectedObjects: 1,
		Manifest: []byte("table=" + recoveryTableName(marker)), Content: []byte(marker + "\x00" + fixture + "\x00" + recoveryPayload),
		Permissions: []byte(databaseUser + "@%"), MaxRestoreDurationSeconds: 30 * 60,
	}})
}

func parseOVHResource(resource string) (string, string, error) {
	parsed, err := url.Parse(strings.TrimSpace(resource))
	if err != nil || (parsed.Scheme != "ovh-database" && parsed.Scheme != "ovh-db" && parsed.Scheme != "ovhcloud-database") || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", "", fmt.Errorf("OVHcloud database resource %q is invalid", resource)
	}
	parts := strings.Split(strings.Trim(parsed.Host+parsed.EscapedPath(), "/"), "/")
	if len(parts) != 2 {
		return "", "", fmt.Errorf("OVHcloud database resource %q must identify engine and instance", resource)
	}
	for i := range parts {
		parts[i], err = url.PathUnescape(parts[i])
		if err != nil || !safeIdentifier.MatchString(parts[i]) {
			return "", "", fmt.Errorf("OVHcloud database resource %q contains invalid identity data", resource)
		}
	}
	return strings.ToLower(parts[0]), parts[1], nil
}

func recoveryTableName(marker string) string {
	digest := sha256.Sum256([]byte(marker))
	return fmt.Sprintf("magelift_fixture_%x", digest[:8])
}

func fixtureSelect(table string) string {
	return "SELECT COUNT(*), COALESCE(MAX(marker), ''), COALESCE(MAX(fixture_id), ''), COALESCE(MAX(payload), '') FROM `" + table + "` WHERE id=1"
}

func fixtureRowMatches(row, marker, fixture string) bool {
	return fixtureRowMatchesPayload(row, marker, fixture, recoveryPayload)
}

func fixtureRowMatchesPayload(row, marker, fixture, payload string) bool {
	fields := strings.Split(strings.TrimSpace(row), "\t")
	return len(fields) == 4 && fields[0] == "1" && fields[1] == marker && fields[2] == fixture && fields[3] == payload
}

func fixtureRowIsEmpty(row string) bool {
	trimmed := strings.TrimSpace(row)
	if trimmed == "0" {
		return true
	}
	fields := strings.Split(trimmed, "\t")
	return len(fields) == 4 && fields[0] == "0" && fields[1] == "" && fields[2] == "" && fields[3] == ""
}

func quoteMySQLString(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, "'", `\'`)
	return "'" + value + "'"
}

func validateCIDR(value string) error {
	if strings.TrimSpace(value) == "" {
		return errors.New("a non-empty CIDR is required")
	}
	if _, _, err := net.ParseCIDR(strings.TrimSpace(value)); err != nil {
		return fmt.Errorf("%q is not a valid CIDR", value)
	}
	return nil
}

func containsInventory(resources []sdk.ResilienceInventoryResource, identity string) bool {
	for _, resource := range resources {
		if resource.Identity == identity && resource.Owned && resource.Live {
			return true
		}
	}
	return false
}

func validatePart(value, name string) error {
	if strings.TrimSpace(value) == "" || strings.ContainsAny(value, "\r\n\x00") {
		return fmt.Errorf("%s is required and must be single-line", name)
	}
	return nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
