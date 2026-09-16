// Command aws-database-recovery-acceptance runs one bounded AWS RDS/Aurora
// backup, isolated-restore, and known-content application-fixture cell. The
// shell wrapper owns the generated source and subnet group; this command owns
// the recovery snapshot, restored resource, polling, and ownership-scoped
// cleanup.
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
	"strings"
	"syscall"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/rds"

	"github.com/magelift/magelift/internal/certification"
	awsresilience "github.com/magelift/magelift/internal/cloud/aws/resilience"
	"github.com/magelift/magelift/sdk"
)

const (
	defaultTimeout          = 50 * time.Minute
	defaultOperationTimeout = 35 * time.Minute
	defaultPollInterval     = 10 * time.Second
	defaultMaxAttempts      = 210
	defaultRetentionDays    = 1
	defaultRestoreClass     = "db.t3.micro"
	defaultMySQLImage       = "mysql:8.4"
	recoveryDatabase        = "magelift_recovery"
	recoveryPayload         = "magelift-aws-rds-recovery-v1"
	recoveryFixtureRecords  = 1
)

var (
	safeAWSProfile    = regexp.MustCompile(`^[A-Za-z0-9._-]{1,64}$`)
	safeAWSRegion     = regexp.MustCompile(`^[A-Za-z0-9-]{1,32}$`)
	safeRDSIdentifier = regexp.MustCompile(`^[a-z][a-z0-9-]{0,62}$`)
	safeRDSClass      = regexp.MustCompile(`^[A-Za-z0-9._-]{1,64}$`)
	safeSecurityGroup = regexp.MustCompile(`^sg-[A-Za-z0-9]+$`)
	safeFixturePart   = regexp.MustCompile(`^[A-Za-z0-9._:/-]{1,128}$`)
)

func main() {
	if err := runMain(); err != nil {
		fmt.Fprintf(os.Stderr, "AWS database recovery acceptance failed: %v\n", err)
		os.Exit(1)
	}
}

func runMain() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return run(ctx, os.Args[1:], os.Stdout)
}

func run(parent context.Context, args []string, output io.Writer) (runErr error) {
	flags := flag.NewFlagSet("aws-database-recovery-acceptance", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	profile := flags.String("profile", "default", "AWS shared-config profile")
	region := flags.String("region", "", "AWS region")
	sourceKind := flags.String("source-kind", "instance", "owned source resource kind: instance or cluster")
	instance := flags.String("instance", "", "owned generated RDS source instance identifier")
	restoreSubnetGroup := flags.String("restore-subnet-group", "", "owned RDS DB subnet group for the isolated restore")
	restoreSecurityGroupID := flags.String("restore-security-group-id", "", "owned EC2 security group for source and isolated restore connectivity")
	restoreClass := flags.String("restore-class", defaultRestoreClass, "RDS instance class for the isolated restore")
	marker := flags.String("marker", "", "single-line MageLift ownership marker")
	fixture := flags.String("fixture", "", "single-line recovery fixture ID")
	retentionDays := flags.Int("retention-days", defaultRetentionDays, "manual snapshot retention policy in days")
	mysqlImage := flags.String("mysql-image", firstNonEmpty(os.Getenv("MAGELIFT_AWS_RDS_MYSQL_IMAGE"), defaultMySQLImage), "MySQL client image used when host mysql is unavailable")
	if err := flags.Parse(args); err != nil {
		return fmt.Errorf("parse flags: %w", err)
	}
	if flags.NArg() != 0 {
		return errors.New("unexpected positional arguments")
	}
	for _, value := range []struct {
		value string
		name  string
		check *regexp.Regexp
	}{
		{value: *profile, name: "profile", check: safeAWSProfile},
		{value: *region, name: "region", check: safeAWSRegion},
		{value: *sourceKind, name: "source-kind", check: safeRDSIdentifier},
		{value: *instance, name: "instance", check: safeRDSIdentifier},
		{value: *restoreSubnetGroup, name: "restore-subnet-group", check: safeRDSIdentifier},
		{value: *restoreSecurityGroupID, name: "restore-security-group-id", check: safeSecurityGroup},
		{value: *restoreClass, name: "restore-class", check: safeRDSClass},
		{value: *marker, name: "marker", check: safeFixturePart},
		{value: *fixture, name: "fixture", check: safeFixturePart},
	} {
		if err := validatePart(value.value, value.name, value.check); err != nil {
			return err
		}
	}
	if *sourceKind != "instance" && *sourceKind != "cluster" {
		return errors.New("source-kind must be instance or cluster")
	}
	if *retentionDays <= 0 || *retentionDays > 35 {
		return errors.New("retention-days must be between 1 and 35")
	}
	if strings.TrimSpace(os.Getenv("MAGELIFT_AWS_RDS_ROOT_PASSWORD")) == "" {
		return errors.New("MAGELIFT_AWS_RDS_ROOT_PASSWORD is required for RDS application fixture verification")
	}
	if strings.TrimSpace(*mysqlImage) == "" || strings.ContainsAny(*mysqlImage, "\r\n\x00") {
		return errors.New("mysql-image must be a non-empty single-line image reference")
	}

	ctx, cancel := context.WithTimeout(parent, defaultTimeout)
	defer cancel()
	loadOptions := []func(*awsconfig.LoadOptions) error{awsconfig.WithRegion(*region)}
	if strings.TrimSpace(*profile) != "" {
		loadOptions = append(loadOptions, awsconfig.WithSharedConfigProfile(*profile))
	}
	awsConfig, err := awsconfig.LoadDefaultConfig(ctx, loadOptions...)
	if err != nil {
		return fmt.Errorf("load AWS SDK configuration: %w", err)
	}
	rdsClient := rds.NewFromConfig(awsConfig)
	resource := "aws-rds://" + *sourceKind + "/" + *instance
	databasePassword := strings.TrimSpace(os.Getenv("MAGELIFT_AWS_RDS_ROOT_PASSWORD"))
	mysql := newCommandMySQLExecutor(*mysqlImage)
	hostResolver := func(resolveCtx context.Context, resource string) (string, error) {
		return resolveRDSHost(resolveCtx, rdsClient, resource)
	}
	fixtureStore := sqlFixtureStore{
		queryer: mysql, resolveHost: hostResolver, user: "magelift", password: databasePassword,
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
		queryer: mysql, resolveHost: hostResolver, user: "magelift", password: databasePassword,
		database: recoveryDatabase, payload: recoveryPayload,
	}
	native, err := awsresilience.NewAWSDatabaseNativeAPI(ctx, awsresilience.NativeAPIConfig{
		RestoreDBInstanceClass:     *restoreClass,
		RestoreDBSubnetGroup:       *restoreSubnetGroup,
		RestorePubliclyAccessible:  true,
		RestoreVPCSecurityGroupIDs: []string{*restoreSecurityGroupID},
		RetentionDays:              *retentionDays,
		Verifier:                   verifier,
	}, loadOptions...)
	if err != nil {
		return fmt.Errorf("construct AWS database recovery translator: %w", err)
	}
	client, err := awsresilience.NewNativeResilienceClient(native)
	if err != nil {
		return fmt.Errorf("construct AWS RDS recovery operation client: %w", err)
	}

	policy := sdk.ResilienceOperationPolicy{
		Timeout: defaultOperationTimeout, PollInterval: defaultPollInterval, MaxAttempts: defaultMaxAttempts,
	}
	cell, err := certification.NewDatabaseRecoveryCell(certification.DatabaseRecoveryCellConfig{
		Operations: client, FixtureStore: fixtureStore, OperationPolicy: policy, Fixture: manifest,
		DataClass: "database", Destination: sdk.RecoverySameRegionIsolated, ResourceReference: resource,
		Marker: *marker, Protection: certification.DatabaseRecoveryProtectionRequired,
	})
	if err != nil {
		return fmt.Errorf("construct AWS database recovery cell: %w", err)
	}
	fixtureAttempted = true
	if err := cell.Prepare(ctx); err != nil {
		return fmt.Errorf("prepare AWS application fixture: %w", err)
	}
	result, err := cell.Run(ctx)
	if err != nil {
		return fmt.Errorf("run AWS database recovery cell: %w", err)
	}
	backupID := result.BackupID
	restoreID := result.RestoreID

	inventory, err := client.Inventory(ctx, *marker)
	if err != nil {
		return fmt.Errorf("inventory AWS database recovery resources: %w", err)
	}
	if !containsInventory(inventory, resource) || !containsInventory(inventory, backupID) || !containsInventory(inventory, restoreID) {
		return fmt.Errorf("AWS database inventory did not contain source, backup, and restored resource: %#v", inventory)
	}

	if err := cell.Cleanup(ctx); err != nil {
		return fmt.Errorf("clean AWS database recovery outputs: %w", err)
	}
	if err := cleanupFixture(); err != nil {
		return fmt.Errorf("delete AWS application fixture: %w", err)
	}
	remaining, err := client.Inventory(ctx, *marker)
	if err != nil {
		return fmt.Errorf("verify AWS database recovery cleanup: %w", err)
	}
	if err := verifyDatabaseCleanup(ctx, rdsClient, remaining, resource); err != nil {
		return fmt.Errorf("verify AWS database recovery cleanup: %w", err)
	}

	label := "AWS RDS"
	if *sourceKind == "cluster" {
		label = "AWS Aurora"
	}
	fmt.Fprintf(output, "%s database recovery acceptance PASS profile=%s region=%s source=%s backup=%s restore=%s restoreDurationSeconds=%d retentionDays=%d cleanup=verified applicationFixture=verified manifest=verified reads=verified permissions=verified health=verified\n", label, *profile, *region, resource, backupID, restoreID, result.RestoreDurationSeconds, *retentionDays)
	return nil
}

type mysqlQueryer interface {
	Query(context.Context, string, string, string, string, string) (string, error)
}

type rdsEndpointAPI interface {
	DescribeDBInstances(context.Context, *rds.DescribeDBInstancesInput, ...func(*rds.Options)) (*rds.DescribeDBInstancesOutput, error)
	DescribeDBClusters(context.Context, *rds.DescribeDBClustersInput, ...func(*rds.Options)) (*rds.DescribeDBClustersOutput, error)
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
	if strings.TrimSpace(host) == "" || strings.ContainsAny(host, "/\r\n\x00") || len(host) > 253 {
		return "", errors.New("mysql query host must be a valid RDS endpoint")
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
		return "", errors.New("AWS database application verification requires mysql or docker")
	}
	output, err := executor.run(ctx, command, args, env)
	if err != nil {
		return "", fmt.Errorf("mysql query failed: %w", err)
	}
	return strings.TrimSpace(output), nil
}

func mysqlClientArgs(host, user, database, statement string) []string {
	args := []string{"--protocol=TCP", "--connect-timeout=15", "--ssl-mode=REQUIRED", "--host", host, "--port", "3306", "--user", user, "--batch", "--skip-column-names"}
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

func resolveRDSHost(ctx context.Context, client rdsEndpointAPI, resource string) (string, error) {
	kind, instance, err := parseDatabaseResource(resource)
	if err != nil {
		return "", err
	}
	var host string
	switch kind {
	case "instance":
		output, err := client.DescribeDBInstances(ctx, &rds.DescribeDBInstancesInput{DBInstanceIdentifier: aws.String(instance)})
		if err != nil {
			return "", fmt.Errorf("describe AWS RDS endpoint for %q: %w", instance, err)
		}
		if output == nil || len(output.DBInstances) != 1 || output.DBInstances[0].Endpoint == nil {
			return "", fmt.Errorf("AWS RDS instance %q did not expose one endpoint", instance)
		}
		host = strings.TrimSpace(aws.ToString(output.DBInstances[0].Endpoint.Address))
	case "cluster":
		output, err := client.DescribeDBClusters(ctx, &rds.DescribeDBClustersInput{DBClusterIdentifier: aws.String(instance)})
		if err != nil {
			return "", fmt.Errorf("describe AWS Aurora endpoint for %q: %w", instance, err)
		}
		if output == nil || len(output.DBClusters) != 1 || output.DBClusters[0].Endpoint == nil {
			return "", fmt.Errorf("AWS Aurora cluster %q did not expose one endpoint", instance)
		}
		host = strings.TrimSpace(aws.ToString(output.DBClusters[0].Endpoint))
	default:
		return "", fmt.Errorf("AWS database resource kind %q is unsupported", kind)
	}
	if host == "" || strings.ContainsAny(host, "/\r\n\x00") || len(host) > 253 {
		return "", fmt.Errorf("AWS database resource %q exposed an invalid endpoint", instance)
	}
	return host, nil
}

type sqlFixtureStore struct {
	queryer     mysqlQueryer
	resolveHost func(context.Context, string) (string, error)
	user        string
	password    string
	database    string
	marker      string
	fixtureID   string
}

func (store sqlFixtureStore) Prepare(ctx context.Context, resource string, manifest certification.RecoveryFixtureManifest) error {
	if _, _, err := parseDatabaseResource(resource); err != nil {
		return err
	}
	if len(manifest.Classes) != 1 || manifest.Classes[0].Name != "database" || manifest.Classes[0].ExpectedRecords != recoveryFixtureRecords {
		return errors.New("AWS database application fixture manifest is not the expected database fixture")
	}
	host, err := store.resolveHost(ctx, resource)
	if err != nil {
		return err
	}
	table := recoveryTableName(store.marker)
	if _, err := store.queryer.Query(ctx, host, store.user, store.password, "", "CREATE DATABASE IF NOT EXISTS `"+recoveryDatabase+"`"); err != nil {
		return fmt.Errorf("create AWS database fixture database: %w", err)
	}
	createTable := "CREATE TABLE IF NOT EXISTS `" + table + "` (id BIGINT NOT NULL PRIMARY KEY, marker VARCHAR(128) NOT NULL, fixture_id VARCHAR(128) NOT NULL, payload VARCHAR(128) NOT NULL)"
	if _, err := store.queryer.Query(ctx, host, store.user, store.password, store.database, createTable); err != nil {
		return fmt.Errorf("create AWS database fixture table: %w", err)
	}
	row, err := store.queryer.Query(ctx, host, store.user, store.password, store.database, fixtureSelect(table))
	if err != nil {
		return fmt.Errorf("inspect existing AWS database fixture row: %w", err)
	}
	if fixtureRowMatches(row, store.marker, store.fixtureID) {
		return nil
	}
	if !fixtureRowIsEmpty(row) {
		return errors.New("AWS database fixture table contains unexpected existing content")
	}
	insert := "INSERT INTO `" + table + "` (id, marker, fixture_id, payload) VALUES (1, " + quoteMySQLString(store.marker) + ", " + quoteMySQLString(store.fixtureID) + ", " + quoteMySQLString(recoveryPayload) + ")"
	if _, err := store.queryer.Query(ctx, host, store.user, store.password, store.database, insert); err != nil {
		return fmt.Errorf("write AWS database fixture row: %w", err)
	}
	return nil
}

func (store sqlFixtureStore) Delete(ctx context.Context, resource string) error {
	if _, _, err := parseDatabaseResource(resource); err != nil {
		return err
	}
	host, err := store.resolveHost(ctx, resource)
	if err != nil {
		return err
	}
	if _, err := store.queryer.Query(ctx, host, store.user, store.password, store.database, "DROP TABLE IF EXISTS `"+recoveryTableName(store.marker)+"`"); err != nil {
		return fmt.Errorf("delete AWS database fixture table: %w", err)
	}
	return nil
}

type sqlFixtureVerifier struct {
	queryer     mysqlQueryer
	resolveHost func(context.Context, string) (string, error)
	user        string
	password    string
	database    string
	payload     string
}

func (verifier sqlFixtureVerifier) Verify(ctx context.Context, request awsresilience.RecoveryVerificationRequest) (awsresilience.RecoveryVerification, error) {
	if _, _, err := parseDatabaseResource(request.Resource); err != nil {
		return awsresilience.RecoveryVerification{}, err
	}
	host, err := verifier.resolveHost(ctx, request.Resource)
	if err != nil {
		return awsresilience.RecoveryVerification{}, err
	}
	table := recoveryTableName(request.OwnershipMarker)
	row, err := verifier.queryer.Query(ctx, host, verifier.user, verifier.password, verifier.database, fixtureSelect(table))
	if err != nil {
		return awsresilience.RecoveryVerification{}, fmt.Errorf("read AWS database recovery fixture: %w", err)
	}
	if !fixtureRowMatches(row, request.OwnershipMarker, request.FixtureID) || !strings.Contains(row, verifier.payload) {
		return awsresilience.RecoveryVerification{}, errors.New("AWS database recovery fixture content did not match the expected marker and payload")
	}
	if _, err := verifier.queryer.Query(ctx, host, verifier.user, verifier.password, verifier.database, "CREATE TEMPORARY TABLE `magelift_permission_probe` (id INT NOT NULL PRIMARY KEY); INSERT INTO `magelift_permission_probe` VALUES (1); DROP TEMPORARY TABLE `magelift_permission_probe`"); err != nil {
		return awsresilience.RecoveryVerification{}, fmt.Errorf("verify AWS database recovery fixture permissions: %w", err)
	}
	health, err := verifier.queryer.Query(ctx, host, verifier.user, verifier.password, verifier.database, "SELECT 1")
	if err != nil || strings.TrimSpace(health) != "1" {
		if err == nil {
			err = errors.New("health query returned an unexpected result")
		}
		return awsresilience.RecoveryVerification{}, fmt.Errorf("verify AWS database recovery service health: %w", err)
	}
	return awsresilience.RecoveryVerification{
		ManifestVerified: true, CountsVerified: true, ApplicationReadsVerified: true,
		PermissionsVerified: true, SecretReferencesVerified: true, ServiceHealthVerified: true,
	}, nil
}

func buildRecoveryFixtureManifest(resource, fixture, marker string) (certification.RecoveryFixtureManifest, error) {
	return certification.BuildRecoveryFixtureManifest(fixture, resource, []certification.RecoveryFixtureMaterial{{
		Name: "database", ExpectedRecords: recoveryFixtureRecords, ExpectedObjects: 1,
		Manifest: []byte("table=" + recoveryTableName(marker)), Content: []byte(marker + "\x00" + fixture + "\x00" + recoveryPayload),
		Permissions: []byte("magelift@%"), MaxRestoreDurationSeconds: 35 * 60,
	}})
}

func parseRDSResource(resource string) (string, error) {
	kind, id, err := parseDatabaseResource(resource)
	if err != nil {
		return "", err
	}
	if kind != "instance" {
		return "", fmt.Errorf("AWS RDS resource %q must identify an instance", resource)
	}
	return id, nil
}

func parseDatabaseResource(resource string) (string, string, error) {
	for _, kind := range []string{"instance", "cluster"} {
		prefix := "aws-rds://" + kind + "/"
		if strings.HasPrefix(resource, prefix) {
			id := strings.TrimPrefix(resource, prefix)
			if safeRDSIdentifier.MatchString(id) {
				return kind, id, nil
			}
		}
	}
	return "", "", fmt.Errorf("AWS database resource %q is invalid", resource)
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

func containsInventory(resources []sdk.ResilienceInventoryResource, identity string) bool {
	for _, resource := range resources {
		if resource.Identity == identity && resource.Owned && resource.Live {
			return true
		}
	}
	return false
}

func verifyDatabaseCleanup(ctx context.Context, api rdsEndpointAPI, resources []sdk.ResilienceInventoryResource, source string) error {
	kind, id, err := parseDatabaseResource(source)
	if err != nil {
		return err
	}
	allowed := map[string]struct{}{source: {}}
	if kind == "cluster" {
		output, err := api.DescribeDBClusters(ctx, &rds.DescribeDBClustersInput{DBClusterIdentifier: aws.String(id)})
		if err != nil {
			return fmt.Errorf("describe preserved AWS Aurora source cluster %q: %w", id, err)
		}
		if output == nil || len(output.DBClusters) != 1 {
			return fmt.Errorf("preserved AWS Aurora source cluster %q did not expose one cluster", id)
		}
		for _, member := range output.DBClusters[0].DBClusterMembers {
			memberID := strings.TrimSpace(aws.ToString(member.DBInstanceIdentifier))
			if memberID != "" {
				allowed["aws-rds://instance/"+memberID] = struct{}{}
			}
		}
	}
	if !containsInventory(resources, source) {
		return fmt.Errorf("AWS database cleanup did not preserve source resource %q: %#v", source, resources)
	}
	for _, resource := range resources {
		if !resource.Owned || !resource.Live {
			return fmt.Errorf("AWS database cleanup returned an unowned or non-live resource %q: %#v", resource.Identity, resources)
		}
		if _, ok := allowed[resource.Identity]; !ok {
			return fmt.Errorf("AWS database cleanup left unexpected resource %q: %#v", resource.Identity, resources)
		}
	}
	return nil
}

func validatePart(value, name string, pattern *regexp.Regexp) error {
	if strings.TrimSpace(value) == "" || strings.ContainsAny(value, "\r\n\x00") {
		return fmt.Errorf("%s is required and must be single-line", name)
	}
	if pattern != nil && !pattern.MatchString(value) {
		return fmt.Errorf("%s contains unsupported characters", name)
	}
	return nil
}
