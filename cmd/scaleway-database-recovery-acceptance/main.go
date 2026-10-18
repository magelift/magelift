// Command scaleway-database-recovery-acceptance runs one bounded Scaleway
// Managed Database snapshot, isolated restore, and known-content
// application-fixture cell. The shell wrapper owns the generated source
// instance; this command owns the provider recovery operation, polling, and
// output cleanup.
package main

import (
	"context"
	"crypto/sha256"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/aws/aws-sdk-go-v2/credentials"

	"github.com/magelift/magelift/internal/certification"
	"github.com/magelift/magelift/internal/cloud/scaleway/resilience"
	"github.com/magelift/magelift/sdk"
	rdb "github.com/scaleway/scaleway-sdk-go/api/rdb/v1"
	"github.com/scaleway/scaleway-sdk-go/scw"
)

const (
	defaultTimeout          = 45 * time.Minute
	defaultOperationTimeout = 35 * time.Minute
	defaultPollInterval     = 15 * time.Second
	defaultMaxAttempts      = 140
	defaultMySQLImage       = "mysql:8.4"
	recoveryDatabase        = "magelift_recovery"
	recoveryPayload         = "magelift-scaleway-rdb-recovery-v1"
	recoveryFixtureRecords  = 1
)

var (
	safeIdentifier  = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)
	safeFixturePart = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:/-]{0,127}$`)
)

func main() {
	if err := runMain(); err != nil {
		fmt.Fprintf(os.Stderr, "Scaleway Managed Database recovery acceptance failed: %v\n", err)
		os.Exit(1)
	}
}

func runMain() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return run(ctx, os.Args[1:], os.Stdout)
}

func run(parent context.Context, args []string, output io.Writer) (runErr error) {
	flags := flag.NewFlagSet("scaleway-database-recovery-acceptance", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	profile := flags.String("profile", "default", "Scaleway profile name")
	project := flags.String("project", "", "Scaleway project ID")
	region := flags.String("region", "", "Scaleway RDB region")
	instance := flags.String("instance", "", "owned generated RDB source instance ID")
	marker := flags.String("marker", "", "single-line MageLift ownership marker")
	fixture := flags.String("fixture", "", "single-line recovery fixture ID")
	nodeType := flags.String("restore-node-type", "DB-DEV-S", "node type for the isolated restore")
	mysqlImage := flags.String("mysql-image", firstNonEmpty(os.Getenv("MAGELIFT_SCALEWAY_DATABASE_MYSQL_IMAGE"), defaultMySQLImage), "MySQL client image used when host mysql is unavailable")
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
		{*profile, "profile"}, {*project, "project"}, {*region, "region"}, {*instance, "instance"}, {*marker, "marker"}, {*fixture, "fixture"}, {*nodeType, "restore-node-type"},
	} {
		if err := validatePart(part.value, part.name); err != nil {
			return err
		}
	}
	if !safeIdentifier.MatchString(*profile) || !safeIdentifier.MatchString(*instance) || !safeIdentifier.MatchString(*nodeType) {
		return errors.New("profile, instance, and restore-node-type contain unsupported characters")
	}
	if !regexp.MustCompile(`^[0-9a-fA-F-]{36}$`).MatchString(*project) {
		return errors.New("project must be a Scaleway UUID")
	}
	if !regexp.MustCompile(`^(fr-par|nl-ams|pl-waw)$`).MatchString(*region) {
		return errors.New("region must be fr-par, nl-ams, or pl-waw")
	}
	if !safeFixturePart.MatchString(*marker) || !safeFixturePart.MatchString(*fixture) {
		return errors.New("marker and fixture must contain only letters, numbers, dots, underscores, colons, slashes, or hyphens")
	}
	databasePassword := strings.TrimSpace(os.Getenv("MAGELIFT_SCALEWAY_DATABASE_ROOT_PASSWORD"))
	if databasePassword == "" {
		return errors.New("MAGELIFT_SCALEWAY_DATABASE_ROOT_PASSWORD is required for Scaleway Managed Database application fixture verification")
	}
	if strings.TrimSpace(*mysqlImage) == "" || strings.ContainsAny(*mysqlImage, "\r\n\x00") {
		return errors.New("mysql-image must be a non-empty single-line image reference")
	}

	ctx, cancel := context.WithTimeout(parent, defaultTimeout)
	defer cancel()
	config, err := scw.LoadConfig()
	if err != nil {
		return fmt.Errorf("load Scaleway profile configuration: %w", err)
	}
	profileConfig, err := config.GetProfile(*profile)
	if err != nil {
		return fmt.Errorf("load Scaleway profile %q: %w", *profile, err)
	}
	profileClient, err := scw.NewClient(scw.WithProfile(profileConfig))
	if err != nil {
		return fmt.Errorf("construct Scaleway profile client: %w", err)
	}
	accessKey, accessKeyOK := profileClient.GetAccessKey()
	secretKey, secretKeyOK := profileClient.GetSecretKey()
	if !accessKeyOK || !secretKeyOK || strings.TrimSpace(accessKey) == "" || strings.TrimSpace(secretKey) == "" {
		return errors.New("selected Scaleway profile does not contain an access key and secret key")
	}
	resource := "scaleway-rdb://" + *instance
	rdbAPI := rdb.NewAPI(profileClient)
	endpointResolver := rdbEndpointResolver{api: rdbAPI, region: scw.Region(*region)}
	mysql := newCommandMySQLExecutor(*mysqlImage)
	resolveEndpoint := func(resolveCtx context.Context, resource string) (mysqlEndpoint, error) {
		return endpointResolver.Resolve(resolveCtx, resource)
	}
	fixtureStore := sqlFixtureStore{
		queryer: mysql, resolveEndpoint: resolveEndpoint, user: "magelift", password: databasePassword,
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
		queryer: mysql, resolveEndpoint: resolveEndpoint, user: "magelift", password: databasePassword,
		database: recoveryDatabase, payload: recoveryPayload,
	}
	native, err := resilience.NewScalewayDatabaseNativeAPI(ctx, resilience.NativeAPIConfig{
		Region:                  *region,
		DatabaseRegion:          *region,
		DatabaseProjectID:       *project,
		Credentials:             credentials.NewStaticCredentialsProvider(accessKey, secretKey, ""),
		RetentionDays:           1,
		RestoreDatabaseNodeType: *nodeType,
		Verifier:                verifier,
	}, scw.WithProfile(profileConfig), scw.WithDefaultProjectID(*project), scw.WithDefaultRegion(scw.Region(*region)))
	if err != nil {
		return fmt.Errorf("construct Scaleway Managed Database recovery translator: %w", err)
	}
	client, err := resilience.NewNativeResilienceClient(native)
	if err != nil {
		return fmt.Errorf("construct Scaleway recovery operation client: %w", err)
	}

	policy := sdk.ResilienceOperationPolicy{Timeout: defaultOperationTimeout, PollInterval: defaultPollInterval, MaxAttempts: defaultMaxAttempts}
	cell, err := certification.NewDatabaseRecoveryCell(certification.DatabaseRecoveryCellConfig{
		Operations: client, FixtureStore: fixtureStore, OperationPolicy: policy, Fixture: manifest,
		DataClass: "database", Destination: sdk.RecoverySameRegionIsolated, ResourceReference: resource,
		Marker: *marker, Protection: certification.DatabaseRecoveryProtectionOptional,
	})
	if err != nil {
		return fmt.Errorf("construct Scaleway Managed Database recovery cell: %w", err)
	}
	fixtureAttempted = true
	if err := cell.Prepare(ctx); err != nil {
		return fmt.Errorf("prepare Scaleway Managed Database application fixture: %w", err)
	}
	result, err := cell.Run(ctx)
	if err != nil {
		return fmt.Errorf("run Scaleway Managed Database recovery cell: %w", err)
	}
	backupID := result.BackupID
	restoreID := result.RestoreID
	inventory, err := client.Inventory(ctx, *marker)
	if err != nil {
		return fmt.Errorf("inventory Scaleway Managed Database recovery resources: %w", err)
	}
	if !containsInventory(inventory, resource) || !containsInventory(inventory, backupID) || !containsInventory(inventory, restoreID) {
		return fmt.Errorf("Scaleway Managed Database inventory did not contain source, snapshot, and isolated restore: %#v", inventory)
	}
	if err := cell.Cleanup(ctx); err != nil {
		return fmt.Errorf("clean Scaleway Managed Database recovery outputs: %w", err)
	}
	if err := cleanupFixture(); err != nil {
		return fmt.Errorf("delete Scaleway Managed Database application fixture: %w", err)
	}
	remaining, err := client.Inventory(ctx, *marker)
	if err != nil {
		return fmt.Errorf("verify Scaleway Managed Database recovery cleanup: %w", err)
	}
	if len(remaining) != 1 || !containsInventory(remaining, resource) {
		return fmt.Errorf("Scaleway Managed Database cleanup must preserve only the source instance: %#v", remaining)
	}

	fmt.Fprintf(output, "Scaleway Managed Database recovery acceptance PASS region=%s source=%s backup=%s restore=%s restoreDurationSeconds=%d cleanup=verified applicationFixture=verified manifest=verified reads=verified permissions=verified health=verified protection=provider-optional\n", *region, resource, backupID, restoreID, result.RestoreDurationSeconds)
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
		return "", errors.New("mysql query host must be a valid Scaleway RDB endpoint")
	}
	if endpoint.Port == 0 || endpoint.Port > 65535 {
		return "", errors.New("mysql query port must be between 1 and 65535")
	}
	if strings.TrimSpace(user) == "" || strings.TrimSpace(statement) == "" {
		return "", errors.New("mysql query user and statement are required")
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
		return "", errors.New("Scaleway Managed Database application verification requires mysql or docker")
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
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(output), nil
}

type rdbInstanceAPI interface {
	GetInstance(*rdb.GetInstanceRequest, ...scw.RequestOption) (*rdb.Instance, error)
}

type rdbEndpointResolver struct {
	api    rdbInstanceAPI
	region scw.Region
}

func (resolver rdbEndpointResolver) Resolve(ctx context.Context, resource string) (mysqlEndpoint, error) {
	instanceID, err := parseScalewayResource(resource)
	if err != nil {
		return mysqlEndpoint{}, err
	}
	if resolver.api == nil {
		return mysqlEndpoint{}, errors.New("Scaleway RDB endpoint API is required")
	}
	instance, err := resolver.api.GetInstance(&rdb.GetInstanceRequest{Region: resolver.region, InstanceID: instanceID}, scw.WithContext(ctx))
	if err != nil {
		return mysqlEndpoint{}, fmt.Errorf("resolve Scaleway RDB endpoint: %w", err)
	}
	return selectPublicRDBEndpoint(instance)
}

func selectPublicRDBEndpoint(instance *rdb.Instance) (mysqlEndpoint, error) {
	if instance == nil {
		return mysqlEndpoint{}, errors.New("Scaleway RDB endpoint response is empty")
	}
	candidates := append([]*rdb.Endpoint(nil), instance.Endpoints...)
	if instance.Endpoint != nil {
		candidates = append(candidates, instance.Endpoint)
	}
	for pass := 0; pass < 2; pass++ {
		for _, endpoint := range candidates {
			if endpoint == nil || endpoint.Port == 0 || (pass == 0 && endpoint.LoadBalancer == nil) || (pass == 1 && (endpoint.PrivateNetwork != nil || endpoint.DirectAccess != nil)) {
				continue
			}
			host := ""
			if endpoint.Hostname != nil {
				host = strings.TrimSpace(*endpoint.Hostname)
			}
			if host == "" && endpoint.IP != nil {
				host = endpoint.IP.String()
			}
			if host == "" || strings.ContainsAny(host, "/\r\n\x00") || len(host) > 253 {
				continue
			}
			return mysqlEndpoint{Host: host, Port: endpoint.Port}, nil
		}
	}
	return mysqlEndpoint{}, errors.New("Scaleway RDB instance has no usable public load-balancer endpoint")
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
		return errors.New("Scaleway RDB application fixture manifest is not the expected database fixture")
	}
	endpoint, err := store.resolveEndpoint(ctx, resource)
	if err != nil {
		return err
	}
	table := recoveryTableName(store.marker)
	if _, err := store.queryer.Query(ctx, endpoint, store.user, store.password, "", "CREATE DATABASE IF NOT EXISTS `"+recoveryDatabase+"`"); err != nil {
		return fmt.Errorf("create Scaleway RDB fixture database: %w", err)
	}
	createTable := "CREATE TABLE IF NOT EXISTS `" + table + "` (id BIGINT NOT NULL PRIMARY KEY, marker VARCHAR(128) NOT NULL, fixture_id VARCHAR(128) NOT NULL, payload VARCHAR(128) NOT NULL)"
	if _, err := store.queryer.Query(ctx, endpoint, store.user, store.password, store.database, createTable); err != nil {
		return fmt.Errorf("create Scaleway RDB fixture table: %w", err)
	}
	row, err := store.queryer.Query(ctx, endpoint, store.user, store.password, store.database, fixtureSelect(table))
	if err != nil {
		return fmt.Errorf("inspect existing Scaleway RDB fixture row: %w", err)
	}
	if fixtureRowMatches(row, store.marker, store.fixtureID) {
		return nil
	}
	if !fixtureRowIsEmpty(row) {
		return errors.New("Scaleway RDB fixture table contains unexpected existing content")
	}
	insert := "INSERT INTO `" + table + "` (id, marker, fixture_id, payload) VALUES (1, " + quoteMySQLString(store.marker) + ", " + quoteMySQLString(store.fixtureID) + ", " + quoteMySQLString(recoveryPayload) + ")"
	if _, err := store.queryer.Query(ctx, endpoint, store.user, store.password, store.database, insert); err != nil {
		return fmt.Errorf("write Scaleway RDB fixture row: %w", err)
	}
	return nil
}

func (store sqlFixtureStore) Delete(ctx context.Context, resource string) error {
	endpoint, err := store.resolveEndpoint(ctx, resource)
	if err != nil {
		return err
	}
	if _, err := store.queryer.Query(ctx, endpoint, store.user, store.password, store.database, "DROP TABLE IF EXISTS `"+recoveryTableName(store.marker)+"`"); err != nil {
		return fmt.Errorf("delete Scaleway RDB fixture table: %w", err)
	}
	return nil
}

type sqlFixtureVerifier struct {
	queryer         mysqlQueryer
	resolveEndpoint func(context.Context, string) (mysqlEndpoint, error)
	user            string
	password        string
	database        string
	payload         string
}

func (verifier sqlFixtureVerifier) Verify(ctx context.Context, request resilience.RecoveryVerificationRequest) (resilience.RecoveryVerification, error) {
	endpoint, err := verifier.resolveEndpoint(ctx, request.Resource)
	if err != nil {
		return resilience.RecoveryVerification{}, err
	}
	table := recoveryTableName(request.OwnershipMarker)
	row, err := verifier.queryer.Query(ctx, endpoint, verifier.user, verifier.password, verifier.database, fixtureSelect(table))
	if err != nil {
		return resilience.RecoveryVerification{}, fmt.Errorf("read Scaleway RDB recovery fixture: %w", err)
	}
	if !fixtureRowMatches(row, request.OwnershipMarker, request.FixtureID) || !strings.Contains(row, verifier.payload) {
		return resilience.RecoveryVerification{}, errors.New("Scaleway RDB recovery fixture content did not match the expected marker and payload")
	}
	if _, err := verifier.queryer.Query(ctx, endpoint, verifier.user, verifier.password, verifier.database, "CREATE TEMPORARY TABLE `magelift_permission_probe` (id INT NOT NULL); INSERT INTO `magelift_permission_probe` VALUES (1); DROP TEMPORARY TABLE `magelift_permission_probe`"); err != nil {
		return resilience.RecoveryVerification{}, fmt.Errorf("verify Scaleway RDB recovery fixture permissions: %w", err)
	}
	health, err := verifier.queryer.Query(ctx, endpoint, verifier.user, verifier.password, verifier.database, "SELECT 1")
	if err != nil || strings.TrimSpace(health) != "1" {
		if err == nil {
			err = errors.New("health query returned an unexpected result")
		}
		return resilience.RecoveryVerification{}, fmt.Errorf("verify Scaleway RDB recovery service health: %w", err)
	}
	return resilience.RecoveryVerification{
		ManifestVerified: true, CountsVerified: true, ApplicationReadsVerified: true,
		PermissionsVerified: true, SecretReferencesVerified: true, ServiceHealthVerified: true,
	}, nil
}

func buildRecoveryFixtureManifest(resource, fixture, marker string) (certification.RecoveryFixtureManifest, error) {
	return certification.BuildRecoveryFixtureManifest(fixture, resource, []certification.RecoveryFixtureMaterial{{
		Name: "database", ExpectedRecords: recoveryFixtureRecords, ExpectedObjects: 1,
		Manifest: []byte("table=" + recoveryTableName(marker)), Content: []byte(marker + "\x00" + fixture + "\x00" + recoveryPayload),
		Permissions: []byte("magelift@%"), MaxRestoreDurationSeconds: 30 * 60,
	}})
}

func parseScalewayResource(resource string) (string, error) {
	const prefix = "scaleway-rdb://"
	if !strings.HasPrefix(resource, prefix) {
		return "", fmt.Errorf("Scaleway RDB resource %q is invalid", resource)
	}
	instanceID := strings.TrimPrefix(resource, prefix)
	if instanceID == "" || strings.ContainsAny(instanceID, "/\\?#\r\n\x00") || !safeIdentifier.MatchString(instanceID) {
		return "", fmt.Errorf("Scaleway RDB resource %q is invalid", resource)
	}
	return instanceID, nil
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

func startAndAwait(ctx context.Context, client sdk.ResilienceOperationClient, request sdk.ResilienceOperationRequest, policy sdk.ResilienceOperationPolicy, output io.Writer) (sdk.ResilienceOperationObservation, error) {
	if ctx == nil {
		return sdk.ResilienceOperationObservation{}, errors.New("resilience operation context is required")
	}
	operationCtx, cancel := context.WithTimeout(ctx, policy.Timeout)
	defer cancel()
	observation, err := client.Start(operationCtx, request)
	if err != nil {
		return sdk.ResilienceOperationObservation{}, err
	}
	return await(operationCtx, client, observation, policy, request.Action, output)
}

func await(ctx context.Context, client sdk.ResilienceOperationClient, observation sdk.ResilienceOperationObservation, policy sdk.ResilienceOperationPolicy, action sdk.ResilienceAction, output io.Writer) (sdk.ResilienceOperationObservation, error) {
	fmt.Fprintf(output, "+ scaleway-rdb: %s status=%s operation=%s\n", action, observation.Status, observation.OperationID)
	if syncer, ok := output.(interface{ Sync() error }); ok {
		_ = syncer.Sync()
	}
	if observation.Status == sdk.ResilienceOperationSucceeded {
		return observation, nil
	}
	waited, err := sdk.WaitForResilienceOperation(ctx, client, observation.OperationID, policy)
	if err != nil {
		return sdk.ResilienceOperationObservation{}, err
	}
	return waited, nil
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
