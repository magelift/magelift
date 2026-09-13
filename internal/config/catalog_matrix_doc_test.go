package config

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestUnaimedAurora84IsRejected(t *testing.T) {
	t.Parallel()
	input := strings.Replace(base,
		"target: {provider: aws, runtime: ecs-fargate}",
		"target: {provider: aws, runtime: ecs-fargate, aws: {catalog: {databaseEngine: aurora-mysql, versions: {auroraMysql: 8.4.mysql_aurora.8.4.7}}}}",
		1)
	file, err := Load([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	_, err = file.Resolve("staging", ResolveOptions{})
	if err == nil || !strings.Contains(err.Error(), "Aurora MySQL 3.11 or 3.12") {
		t.Fatalf("unaimed Aurora 8.4 error = %v", err)
	}
}

func TestCapabilityMatrixDocumentsFirstPartyCatalogs(t *testing.T) {
	t.Parallel()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	matrixPath := filepath.Join(filepath.Dir(thisFile), "..", "..", "docs", "capability-matrix.md")
	data, err := os.ReadFile(matrixPath)
	if err != nil {
		t.Fatal(err)
	}
	matrix := string(data)
	for _, want := range []string{
		"certification-matrix",
		"certification-aws",
		"certification-gcp",
		"certification-ovh",
		"certification-scaleway",
		"`ecs-fargate`",
		"`eks`",
		"`fargate-spot`",
		"`ec2-asg`",
		"`managed-instances`",
		"`auto-mode`",
		"`managed-node-groups`",
		"`self-managed`",
		"`rds-mysql`",
		"`aurora-mysql`",
		"`rds-mariadb`",
		"`serverless`",
		"`provisioned`",
		"`ecs-rabbitmq`",
		"`amazon-mq`",
		"`ecs-artemis`",
		"`nat-gateway`",
		"`fck-nat`",
		"`nginx-fpm`",
		"`gke-autopilot`",
		"`gke-standard`",
		"`ZONAL`",
		"`REGIONAL`",
		"`openSearchMode`",
		"`queueMode`",
		"Cloud SQL attach",
		"Pub/Sub",
		"requestBodiesToExclude",
		"vm.max_map_count",
		"`mks`",
		"`free`",
		"`standard`",
		"`attachFloatingIps`",
		"`privateNetworkRoutingAsDefault`",
		"ErrNotSupported",
		"`kapsule`",
		"`cacheMode: redis`",
		"`redisClusterSize`",
		"Redis",
	} {
		if !strings.Contains(matrix, want) {
			t.Errorf("capability-matrix.md missing %q", want)
		}
	}
	if strings.Contains(matrix, "preset-derived") {
		t.Fatal("capability-matrix.md still claims GCP cells are preset-derived")
	}
}

func TestAutopilotRegionalRabbitMQDoesNotRequireStandardPreset(t *testing.T) {
	t.Parallel()
	input := `schemaVersion: 1
project: {name: shop}
application: {edition: open-source, version: 2.4.9, mode: integrated}
build:
  php: "8.5"
  composer: {credentials: gcp-secret-manager://composer/auth}
target:
  provider: gcp
  runtime: gke-autopilot
  gcp:
    project: example-gcp-project
    cloudSqlAvailability: REGIONAL
    cloudSqlBackupEnabled: true
    cloudSqlBinaryLogEnabled: true
    queueMode: rabbitmq
defaults: {region: europe-west1, preset: preview}
environments: {staging: {}}
`
	file, err := Load([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	effective, err := file.Resolve("staging", ResolveOptions{})
	if err != nil {
		t.Fatal(err)
	}
	gcp := effective.Config.Target.GCP
	if gcp == nil {
		t.Fatal("resolved GCP target is nil")
	}
	if gcp.CloudSQLAvailability != "REGIONAL" {
		t.Fatalf("cloudSqlAvailability = %q, want REGIONAL", gcp.CloudSQLAvailability)
	}
	if gcp.QueueMode != "rabbitmq" {
		t.Fatalf("queueMode = %q, want rabbitmq", gcp.QueueMode)
	}
	if effective.Config.Target.Runtime != "gke-autopilot" {
		t.Fatalf("runtime = %q, want gke-autopilot", effective.Config.Target.Runtime)
	}
}
