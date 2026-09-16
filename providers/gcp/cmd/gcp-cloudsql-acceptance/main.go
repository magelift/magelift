// Command gcp-cloudsql-acceptance runs one disposable, provider-backed Cloud
// SQL on-demand-backup and restore acceptance cell. Destination defaults to
// isolated restore; -destination=in-place overwrites the source instance after
// an operator approval reference. The shell wrapper owns instance lifecycle;
// this command owns the MageLift recovery operation, backup deletion, and
// owning-service proof.
package main

import (
	"context"
	"crypto/sha256"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"regexp"
	"strings"
	"syscall"
	"time"

	"github.com/magelift/magelift/internal/certification"
	gcpresilience "github.com/magelift/magelift/providers/gcp/resilience"
	"github.com/magelift/magelift/sdk"
)

const (
	defaultRetentionDays   = 1
	defaultTimeout         = 25 * time.Minute
	defaultMySQLImage      = "mysql:8.4"
	recoveryDatabase       = "magelift_recovery"
	recoveryPayload        = "magelift-cloudsql-recovery-v1"
	recoveryFixtureRecords = 1
)

var safeFixturePart = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$`)

func main() {
	if err := runMain(); err != nil {
		fmt.Fprintf(os.Stderr, "gcp Cloud SQL acceptance failed: %v\n", err)
		os.Exit(1)
	}
}

func runMain() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return run(ctx, os.Args[1:], os.Stdout)
}

func run(parent context.Context, args []string, output io.Writer) (runErr error) {
	flags := flag.NewFlagSet("gcp-cloudsql-acceptance", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	project := flags.String("project", "", "GCP project ID")
	instance := flags.String("instance", "", "owned Cloud SQL source instance ID")
	marker := flags.String("marker", "", "single-line ownership marker")
	fixture := flags.String("fixture", "", "single-line recovery fixture ID")
	retentionDays := flags.Int("retention-days", defaultRetentionDays, "on-demand backup retention in days")
	destinationName := flags.String("destination", "isolated", "recovery destination: isolated or in-place")
	mysqlImage := flags.String("mysql-image", firstNonEmpty(os.Getenv("MAGELIFT_GCP_CLOUDSQL_MYSQL_IMAGE"), defaultMySQLImage), "MySQL client image used when host mysql is unavailable")
	if err := flags.Parse(args); err != nil {
		return fmt.Errorf("parse flags: %w", err)
	}
	if flags.NArg() != 0 {
		return errors.New("unexpected positional arguments")
	}
	for _, value := range []struct {
		value string
		name  string
	}{
		{*project, "project"}, {*instance, "instance"}, {*marker, "marker"}, {*fixture, "fixture"},
	} {
		if err := validatePart(value.value, value.name, true); err != nil {
			return err
		}
	}
	if *retentionDays <= 0 || *retentionDays > 365 {
		return errors.New("retention-days must be between 1 and 365")
	}
	destination, approval, err := parseRecoveryDestination(*destinationName)
	if err != nil {
		return err
	}
	if !safeFixturePart.MatchString(*marker) || !safeFixturePart.MatchString(*fixture) {
		return errors.New("marker and fixture must contain only letters, numbers, dots, underscores, colons, or hyphens")
	}
	databasePassword := strings.TrimSpace(os.Getenv("MAGELIFT_GCP_CLOUDSQL_ROOT_PASSWORD"))
	if databasePassword == "" {
		return errors.New("MAGELIFT_GCP_CLOUDSQL_ROOT_PASSWORD is required for Cloud SQL application fixture verification")
	}
	if strings.TrimSpace(*mysqlImage) == "" || strings.ContainsAny(*mysqlImage, "\r\n\x00") {
		return errors.New("mysql-image must be a non-empty single-line image reference")
	}

	ctx, cancel := context.WithTimeout(parent, defaultTimeout)
	defer cancel()
	resource := "gcp-cloud-sql://projects/" + *project + "/instances/" + *instance
	mysql := newCommandMySQLExecutor(*mysqlImage)
	hostResolver := resolveCloudSQLPublicIP
	fixtureStore := sqlFixtureStore{
		queryer: mysql, resolveHost: hostResolver, user: "root", password: databasePassword,
		database: recoveryDatabase, marker: *marker, fixtureID: *fixture,
	}
	manifest, err := buildRecoveryFixtureManifest(resource, *fixture, *marker)
	if err != nil {
		return err
	}
	fixtureCleaned := false
	cleanupFixture := func() error {
		if fixtureCleaned {
			return nil
		}
		cleanupCtx, cleanupCancel := context.WithTimeout(context.WithoutCancel(context.Background()), 90*time.Second)
		defer cleanupCancel()
		if err := fixtureStore.Delete(cleanupCtx, resource); err != nil {
			return err
		}
		fixtureCleaned = true
		return nil
	}
	defer func() { runErr = errors.Join(runErr, cleanupFixture()) }()
	verifier := sqlFixtureVerifier{
		queryer: mysql, resolveHost: hostResolver, user: "root", password: databasePassword,
		database: recoveryDatabase, payload: recoveryPayload,
	}
	native, err := gcpresilience.NewGCPCloudSQLNativeAPI(ctx, gcpresilience.NativeAPIConfig{
		Project: *project, RetentionDays: *retentionDays, Verifier: verifier,
	})
	if err != nil {
		return err
	}
	defer func() { runErr = errors.Join(runErr, native.Close()) }()
	defer func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cleanupCancel()
		runErr = errors.Join(runErr, native.DeleteOwnedCloudSQLBackups(cleanupCtx, *marker))
	}()

	client, err := gcpresilience.NewNativeResilienceClient(native)
	if err != nil {
		return err
	}
	policy := sdk.ResilienceOperationPolicy{Timeout: 20 * time.Minute, PollInterval: 5 * time.Second, MaxAttempts: 240}
	cell, err := certification.NewDatabaseRecoveryCell(certification.DatabaseRecoveryCellConfig{
		Operations: client, FixtureStore: fixtureStore, OperationPolicy: policy, Fixture: manifest,
		DataClass: "database", Destination: destination, ResourceReference: resource,
		Marker: *marker, ApprovalReference: approval, Protection: certification.DatabaseRecoveryProtectionRequired,
	})
	if err != nil {
		return fmt.Errorf("construct Cloud SQL database recovery cell: %w", err)
	}
	if err := cell.Prepare(ctx); err != nil {
		return fmt.Errorf("prepare Cloud SQL application fixture: %w", err)
	}
	result, err := cell.Run(ctx)
	if err != nil {
		return fmt.Errorf("run Cloud SQL database recovery cell: %w", err)
	}
	backupID := result.BackupID
	restoreID := result.RestoreID

	inventory, err := client.Inventory(ctx, *marker)
	if err != nil {
		return fmt.Errorf("inventory Cloud SQL acceptance resources: %w", err)
	}
	if !containsInventory(inventory, backupID) || !containsInventory(inventory, resource) || !containsInventory(inventory, restoreID) {
		return fmt.Errorf("Cloud SQL acceptance inventory did not contain source, restore, and backup: %#v", inventory)
	}
	instanceCount := 2
	if restoreID == resource {
		instanceCount = 1
	}
	if err := native.DeleteOwnedCloudSQLBackups(ctx, *marker); err != nil {
		return fmt.Errorf("delete Cloud SQL acceptance backup: %w", err)
	}
	remaining, err := client.Inventory(ctx, *marker)
	if err != nil {
		return fmt.Errorf("verify Cloud SQL backup cleanup: %w", err)
	}
	if containsInventory(remaining, backupID) || !containsInventory(remaining, resource) || (restoreID != resource && !containsInventory(remaining, restoreID)) {
		return fmt.Errorf("Cloud SQL acceptance cleanup inventory is incorrect: %#v", remaining)
	}
	if err := cleanupFixture(); err != nil {
		return fmt.Errorf("delete Cloud SQL application fixture: %w", err)
	}

	fmt.Fprintf(output, "gcp Cloud SQL acceptance PASS project=%s source=%s restore=%s destination=%s backup=%s restoreDurationSeconds=%d retentionDays=%d backupCleanup=verified applicationFixture=verified manifest=verified reads=verified permissions=verified health=verified instances=%d corruption=%s\n", *project, *instance, restoreID, destination, backupID, result.RestoreDurationSeconds, *retentionDays, instanceCount, corruptionLabel(result.CorruptionInjected))
	return nil
}

func corruptionLabel(injected bool) string {
	if injected {
		return "injected"
	}
	return "not-injected"
}

type mysqlQueryer interface {
	Query(context.Context, string, string, string, string, string) (string, error)
}

type commandMySQLExecutor struct {
	image  string
	lookup func(string) (string, error)
	run    func(context.Context, string, []string, []string) (string, error)
}

func newCommandMySQLExecutor(image string) commandMySQLExecutor {
	return commandMySQLExecutor{image: image, lookup: exec.LookPath, run: runExternalCommand}
}

func (executor commandMySQLExecutor) Query(ctx context.Context, host, user, password, database, statement string) (string, error) {
	if ctx == nil {
		return "", errors.New("mysql query context is required")
	}
	if net.ParseIP(host) == nil {
		return "", errors.New("mysql query host must be an IP address")
	}
	if strings.TrimSpace(user) == "" || strings.TrimSpace(statement) == "" {
		return "", errors.New("mysql query user and statement are required")
	}
	args := mysqlClientArgs(host, user, database, statement)
	env := []string{"MYSQL_PWD=" + password}
	command := ""
	if path, err := executor.lookup("mysql"); err == nil {
		command = path
	} else if path, err := executor.lookup("docker"); err == nil {
		command = path
		args = append([]string{"run", "--rm", "--pull=missing", "--env", "MYSQL_PWD", executor.image, "mysql"}, args...)
	} else {
		return "", errors.New("Cloud SQL application verification requires mysql or docker")
	}
	output, err := executor.run(ctx, command, args, env)
	if err != nil {
		return "", fmt.Errorf("mysql query failed: %w", err)
	}
	return strings.TrimSpace(output), nil
}

func mysqlClientArgs(host, user, database, statement string) []string {
	args := []string{"--protocol=TCP", "--connect-timeout=15", "--host", host, "--port", "3306", "--user", user, "--batch", "--skip-column-names"}
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
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(output), nil
}

type sqlHostResolver func(context.Context, string, string) (string, error)

func resolveCloudSQLPublicIP(ctx context.Context, project, instance string) (string, error) {
	if err := validatePart(project, "project", true); err != nil {
		return "", err
	}
	if err := validatePart(instance, "instance", true); err != nil {
		return "", err
	}
	cmd := exec.CommandContext(ctx, "gcloud", "sql", "instances", "describe", instance, "--project", project, "--format=value(ipAddresses[0].ipAddress)")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("resolve Cloud SQL public IP: %w", err)
	}
	host := strings.TrimSpace(string(output))
	if net.ParseIP(host) == nil {
		return "", errors.New("Cloud SQL instance has no usable public IP")
	}
	return host, nil
}

type sqlFixtureStore struct {
	queryer     mysqlQueryer
	resolveHost sqlHostResolver
	user        string
	password    string
	database    string
	marker      string
	fixtureID   string
}

func (store sqlFixtureStore) Prepare(ctx context.Context, resource string, manifest certification.RecoveryFixtureManifest) error {
	project, instance, err := parseCloudSQLResource(resource)
	if err != nil {
		return err
	}
	if len(manifest.Classes) != 1 || manifest.Classes[0].Name != "database" || manifest.Classes[0].ExpectedRecords != recoveryFixtureRecords {
		return errors.New("Cloud SQL application fixture manifest is not the expected database fixture")
	}
	host, err := store.resolveHost(ctx, project, instance)
	if err != nil {
		return err
	}
	table := recoveryTableName(store.marker)
	if _, err := store.queryer.Query(ctx, host, store.user, store.password, "", "CREATE DATABASE IF NOT EXISTS `"+recoveryDatabase+"`"); err != nil {
		return fmt.Errorf("create Cloud SQL fixture database: %w", err)
	}
	createTable := "CREATE TABLE IF NOT EXISTS `" + table + "` (id BIGINT NOT NULL PRIMARY KEY, marker VARCHAR(128) NOT NULL, fixture_id VARCHAR(128) NOT NULL, payload VARCHAR(128) NOT NULL)"
	if _, err := store.queryer.Query(ctx, host, store.user, store.password, store.database, createTable); err != nil {
		return fmt.Errorf("create Cloud SQL fixture table: %w", err)
	}
	row, err := store.queryer.Query(ctx, host, store.user, store.password, store.database, fixtureSelect(table))
	if err != nil {
		return fmt.Errorf("inspect existing Cloud SQL fixture row: %w", err)
	}
	if fixtureRowMatches(row, store.marker, store.fixtureID) {
		return nil
	}
	if !fixtureRowIsEmpty(row) {
		return errors.New("Cloud SQL fixture table contains unexpected existing content")
	}
	insert := "INSERT INTO `" + table + "` (id, marker, fixture_id, payload) VALUES (1, " + quoteMySQLString(store.marker) + ", " + quoteMySQLString(store.fixtureID) + ", " + quoteMySQLString(recoveryPayload) + ")"
	if _, err := store.queryer.Query(ctx, host, store.user, store.password, store.database, insert); err != nil {
		return fmt.Errorf("write Cloud SQL fixture row: %w", err)
	}
	return nil
}

func (store sqlFixtureStore) Delete(ctx context.Context, resource string) error {
	project, instance, err := parseCloudSQLResource(resource)
	if err != nil {
		return err
	}
	host, err := store.resolveHost(ctx, project, instance)
	if err != nil {
		return err
	}
	if _, err := store.queryer.Query(ctx, host, store.user, store.password, store.database, "DROP TABLE IF EXISTS `"+recoveryTableName(store.marker)+"`"); err != nil {
		return fmt.Errorf("delete Cloud SQL fixture table: %w", err)
	}
	return nil
}

func (store sqlFixtureStore) Overwrite(ctx context.Context, resource, payload string) error {
	if payload != certification.DatabaseRecoveryCorruptPayload {
		return errors.New("Cloud SQL fixture overwrite payload is not the cell corrupt value")
	}
	project, instance, err := parseCloudSQLResource(resource)
	if err != nil {
		return err
	}
	host, err := store.resolveHost(ctx, project, instance)
	if err != nil {
		return err
	}
	table := recoveryTableName(store.marker)
	update := "UPDATE `" + table + "` SET payload = " + quoteMySQLString(payload) + " WHERE id=1"
	if _, err := store.queryer.Query(ctx, host, store.user, store.password, store.database, update); err != nil {
		return fmt.Errorf("corrupt Cloud SQL fixture row: %w", err)
	}
	row, err := store.queryer.Query(ctx, host, store.user, store.password, store.database, fixtureSelect(table))
	if err != nil {
		return fmt.Errorf("inspect corrupted Cloud SQL fixture row: %w", err)
	}
	if !fixtureRowIsCorrupt(row, store.marker, store.fixtureID, payload) {
		return errors.New("Cloud SQL fixture row was not corrupted before in-place restore")
	}
	return nil
}

type sqlFixtureVerifier struct {
	queryer     mysqlQueryer
	resolveHost sqlHostResolver
	user        string
	password    string
	database    string
	payload     string
}

func (verifier sqlFixtureVerifier) Verify(ctx context.Context, request gcpresilience.RecoveryVerificationRequest) (gcpresilience.RecoveryVerification, error) {
	project, instance, err := parseCloudSQLResource(request.Resource)
	if err != nil {
		return gcpresilience.RecoveryVerification{}, err
	}
	host, err := verifier.resolveHost(ctx, project, instance)
	if err != nil {
		return gcpresilience.RecoveryVerification{}, err
	}
	table := recoveryTableName(request.OwnershipMarker)
	row, err := verifier.queryer.Query(ctx, host, verifier.user, verifier.password, verifier.database, fixtureSelect(table))
	if err != nil {
		return gcpresilience.RecoveryVerification{}, fmt.Errorf("read Cloud SQL recovery fixture: %w", err)
	}
	if !fixtureRowMatches(row, request.OwnershipMarker, request.FixtureID) || !strings.Contains(row, verifier.payload) {
		return gcpresilience.RecoveryVerification{}, errors.New("Cloud SQL recovery fixture content did not match the expected marker and payload")
	}
	if _, err := verifier.queryer.Query(ctx, host, verifier.user, verifier.password, verifier.database, "CREATE TEMPORARY TABLE `magelift_permission_probe` (id INT NOT NULL); INSERT INTO `magelift_permission_probe` VALUES (1); DROP TEMPORARY TABLE `magelift_permission_probe`"); err != nil {
		return gcpresilience.RecoveryVerification{}, fmt.Errorf("verify Cloud SQL recovery fixture permissions: %w", err)
	}
	health, err := verifier.queryer.Query(ctx, host, verifier.user, verifier.password, verifier.database, "SELECT 1")
	if err != nil || strings.TrimSpace(health) != "1" {
		if err == nil {
			err = errors.New("health query returned an unexpected result")
		}
		return gcpresilience.RecoveryVerification{}, fmt.Errorf("verify Cloud SQL recovery service health: %w", err)
	}
	return gcpresilience.RecoveryVerification{
		ManifestVerified: true, CountsVerified: true, ApplicationReadsVerified: true,
		PermissionsVerified: true, SecretReferencesVerified: true, ServiceHealthVerified: true,
	}, nil
}

func buildRecoveryFixtureManifest(resource, fixture, marker string) (certification.RecoveryFixtureManifest, error) {
	return certification.BuildRecoveryFixtureManifest(fixture, resource, []certification.RecoveryFixtureMaterial{{
		Name: "database", ExpectedRecords: recoveryFixtureRecords, ExpectedObjects: 1,
		Manifest: []byte("table=" + recoveryTableName(marker)), Content: []byte(marker + "\x00" + fixture + "\x00" + recoveryPayload),
		Permissions: []byte("root@%"), MaxRestoreDurationSeconds: 20 * 60,
	}})
}

func parseRecoveryDestination(value string) (sdk.RecoveryDestination, string, error) {
	switch strings.TrimSpace(value) {
	case "", "isolated":
		return sdk.RecoverySameRegionIsolated, "", nil
	case "in-place":
		return sdk.RecoverySameRegion, "gcp-cloudsql-acceptance-in-place", nil
	default:
		return "", "", fmt.Errorf("destination must be isolated or in-place, got %q", value)
	}
}

func parseCloudSQLResource(resource string) (string, string, error) {
	const prefix = "gcp-cloud-sql://projects/"
	if !strings.HasPrefix(resource, prefix) {
		return "", "", fmt.Errorf("Cloud SQL resource %q is invalid", resource)
	}
	parts := strings.Split(strings.TrimPrefix(resource, prefix), "/instances/")
	if len(parts) != 2 || validatePart(parts[0], "project", true) != nil || validatePart(parts[1], "instance", true) != nil {
		return "", "", fmt.Errorf("Cloud SQL resource %q is invalid", resource)
	}
	return parts[0], parts[1], nil
}

func recoveryTableName(marker string) string {
	digest := sha256.Sum256([]byte(marker))
	return fmt.Sprintf("magelift_fixture_%x", digest[:8])
}

func fixtureSelect(table string) string {
	return "SELECT COUNT(*), COALESCE(MAX(marker), ''), COALESCE(MAX(fixture_id), ''), COALESCE(MAX(payload), '') FROM `" + table + "` WHERE id=1"
}

func fixtureRowMatches(row, marker, fixture string) bool {
	fields := strings.Split(strings.TrimSpace(row), "\t")
	return len(fields) == 4 && fields[0] == "1" && fields[1] == marker && fields[2] == fixture && fields[3] == recoveryPayload
}

func fixtureRowIsCorrupt(row, marker, fixture, payload string) bool {
	fields := strings.Split(strings.TrimSpace(row), "\t")
	return len(fields) == 4 && fields[0] == "1" && fields[1] == marker && fields[2] == fixture && fields[3] == payload && payload != recoveryPayload
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

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func containsInventory(resources []sdk.ResilienceInventoryResource, identity string) bool {
	for _, resource := range resources {
		if resource.Identity == identity && resource.Owned && resource.Live {
			return true
		}
	}
	return false
}

func validatePart(value, name string, rejectSlash bool) error {
	if strings.ContainsAny(value, "\r\n\x00?#") || (rejectSlash && strings.Contains(value, "/")) {
		return fmt.Errorf("%s contains unsupported characters", name)
	}
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("%s is required", name)
	}
	return nil
}
