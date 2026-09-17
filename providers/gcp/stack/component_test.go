package stack

import (
	"encoding/json"
	"strings"
	"sync"
	"testing"

	"github.com/magelift/magelift/sdk"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

func TestPlanFromInputsRejectsUnknownRuntime(t *testing.T) {
	in := gcpDeploymentInputs()
	in.Runtime = "ecs-fargate"
	if _, err := PlanFromInputs(in); err == nil || !strings.Contains(err.Error(), "unsupported GCP runtime") {
		t.Fatalf("expected GCP runtime rejection, got %v", err)
	}
}

func TestProgramBuildsMockGraph(t *testing.T) {
	spec := Spec{
		Identity: Identity{
			Project: "shop", GCPProject: "example-gcp-project", Environment: "preview",
			Region: "europe-west1", EnvironmentClass: "preview", Preset: "preview",
			Labels: map[string]string{"magelift-managed-by": "magelift"},
		},
		Application: Application{Edition: "open-source", Version: "2.4.8", Mode: "integrated", WebRuntime: "nginx-fpm"},
		Artifact:    Artifact{ImageDigest: "ghcr.io/magelift/magento@sha256:abcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcd"},
		Policy:      NetworkPolicy{NetworkCIDR: "10.20.0.0/16", Zones: []string{"europe-west1-b", "europe-west1-c"}},
		Catalog: CatalogSelection{
			CloudSQLTier: "db-perf-optimized-N-2", CloudSQLAvailability: "ZONAL", ValkeyRequirement: "8.1",
			MemorystoreNodeType: "SHARED_CORE_NANO", AutopilotCPURequest: "500m", AutopilotMemoryRequest: "1Gi",
			DesiredWebReplicas: 1, SearchMode: "opensearch", SearchReplicas: 1, QueueMode: "database",
		},
		Dependencies: Dependencies{DatabaseName: "magento", MasterUsername: "magento", EncryptionKeySecret: "magento-crypt-key"},
	}
	if err := spec.Validate(); err != nil {
		t.Fatal(err)
	}
	mocks := &stackMocks{}
	err := pulumi.RunErr(Program(spec), pulumi.WithMocks("magelift", "shop-preview", mocks))
	if err != nil {
		t.Fatal(err)
	}
	if len(mocks.resources) == 0 {
		t.Fatal("expected mock resources")
	}
	for _, res := range mocks.resources {
		if res.TypeToken != "gcp:sql/databaseInstance:DatabaseInstance" {
			continue
		}
		if _, ok := res.Inputs["settings"].ObjectValue()["finalBackupConfig"]; ok {
			t.Fatal("preview Cloud SQL with backups disabled must not enable a final backup")
		}
	}
}

func TestSpecRefusesInPlaceRabbitMQQuorum(t *testing.T) {
	spec := Spec{
		Identity: Identity{
			Project: "shop", GCPProject: "example-gcp-project", Environment: "staging",
			Region: "europe-west1", EnvironmentClass: "staging", Preset: "standard",
			Labels: map[string]string{"magelift-managed-by": "magelift"},
		},
		Application: Application{Edition: "open-source", Version: "2.4.8", Mode: "integrated", WebRuntime: "nginx-fpm"},
		Artifact:    Artifact{ImageDigest: "ghcr.io/magelift/magento@sha256:abcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcd"},
		Policy:      NetworkPolicy{NetworkCIDR: "10.20.0.0/16", Zones: []string{"europe-west1-b", "europe-west1-c"}},
		Catalog: CatalogSelection{
			CloudSQLTier: "db-perf-optimized-N-2", CloudSQLAvailability: "ZONAL", ValkeyRequirement: "8.1",
			MemorystoreNodeType: "STANDARD_SMALL", AutopilotCPURequest: "500m", AutopilotMemoryRequest: "1Gi",
			DesiredWebReplicas: 1, SearchMode: "disabled", QueueMode: "rabbitmq", QueueReplicas: 3, LiveQueueReplicas: 1,
		},
		Dependencies: Dependencies{DatabaseName: "magento", MasterUsername: "magento", EncryptionKeySecret: "magento-crypt-key"},
	}
	err := spec.Validate()
	if err == nil || !strings.Contains(err.Error(), "in-place RabbitMQ") {
		t.Fatalf("expected in-place RabbitMQ refusal, got %v", err)
	}
}

func TestProgramUsesValkeyNineForMagento249(t *testing.T) {
	in := gcpDeploymentInputs()
	in.Application.Version = "2.4.9"
	in.ValkeyRequirement = "9"
	spec, err := PlanFromInputs(in)
	if err != nil {
		t.Fatal(err)
	}
	mocks := &stackMocks{}
	if err := pulumi.RunErr(Program(spec), pulumi.WithMocks("magelift", "shop-staging", mocks)); err != nil {
		t.Fatal(err)
	}
	for _, res := range mocks.resources {
		if res.TypeToken != "gcp:memorystore/instance:Instance" {
			continue
		}
		if got := res.Inputs["engineVersion"].StringValue(); got != "VALKEY_9_0" {
			t.Fatalf("Memorystore engine version = %q, want VALKEY_9_0", got)
		}
		return
	}
	t.Fatal("mock graph did not contain a Memorystore instance")
}

func TestProgramBuildsStandardPresetGraph(t *testing.T) {
	spec := Spec{
		Identity: Identity{
			Project: "shop", GCPProject: "example-gcp-project", Environment: "staging",
			Region: "europe-west1", EnvironmentClass: "staging", Preset: "standard",
			Labels: map[string]string{"magelift-managed-by": "magelift"},
		},
		Application: Application{Edition: "open-source", Version: "2.4.8", Mode: "integrated", WebRuntime: "nginx-fpm"},
		Artifact:    Artifact{ImageDigest: "ghcr.io/magelift/magento@sha256:abcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcd"},
		Policy:      NetworkPolicy{NetworkCIDR: "10.20.0.0/16", Zones: []string{"europe-west1-b", "europe-west1-c"}},
		Catalog: CatalogSelection{
			CloudSQLTier: "db-perf-optimized-N-2", CloudSQLAvailability: "REGIONAL", ValkeyRequirement: "8.1",
			CloudSQLBackupEnabled: true, CloudSQLBinaryLogEnabled: true, CloudSQLBackupRetentionCount: 14, CloudSQLTransactionLogRetention: 7,
			CloudSQLBackupStartTime: "03:30", CloudSQLBackupLocation: "europe-west1", CloudSQLDeletionProtection: true,
			MemorystoreNodeType: "STANDARD_SMALL", MemorystoreReplicas: 1,
			MemorystoreDeletionProtection: true, MemorystorePSCConnectionLimit: 4,
			AutopilotCPURequest: "500m", AutopilotMemoryRequest: "1Gi", DesiredWebReplicas: 2,
			SearchMode: "opensearch", SearchReplicas: 1, QueueMode: "rabbitmq", QueueReplicas: 1,
			QueueConsumerCount: 1, EnableCloudArmor: true,
		},
		Dependencies: Dependencies{DatabaseName: "magento", MasterUsername: "magento", EncryptionKeySecret: "magento-crypt-key"},
	}
	if err := spec.Validate(); err != nil {
		t.Fatal(err)
	}
	mocks := &stackMocks{}
	if err := pulumi.RunErr(Program(spec), pulumi.WithMocks("magelift", "shop-staging", mocks)); err != nil {
		t.Fatal(err)
	}
	var sawBucket, sawArmor, sawArmorRules, sawArmorSQLi, sawArmorScanner, sawArmorStatic, sawSQL, sawSearchStatefulSet, sawSearchHeadless, sawSearchReadiness, sawSearchProbeBudget, sawSearchBootstrap, sawQueuePasswordSecret, sawQueueStatefulSet, sawQueueHeadless, sawQueueRole, sawQueueRoleBinding, sawQueueConfig, sawQueueBrokerSecret, sawQueueSpread, sawQueueReadiness, sawQueueProbeBudget, sawQueueInitRoot bool
	var sawGrantJob, sawGrantCommand, sawGrantResources, sawGrantRetryPolicy, sawQueueDataOwnership bool
	sawSQLUsers := 0
	for _, res := range mocks.resources {
		switch res.TypeToken {
		case "gcp:storage/bucket:Bucket":
			sawBucket = true
		case "gcp:compute/securityPolicy:SecurityPolicy":
			sawArmor = true
			encoded, err := json.Marshal(res.Inputs.Mappable())
			if err != nil {
				t.Fatal(err)
			}
			armor := string(encoded)
			for _, required := range []string{"64KB", "STANDARD_WITH_GRAPHQL"} {
				if !strings.Contains(armor, required) {
					t.Fatalf("Cloud Armor lacks Magento-safe %q: %s", required, armor)
				}
			}
		case "gcp:compute/securityPolicyRule:SecurityPolicyRule":
			sawArmorRules = true
			encoded, err := json.Marshal(res.Inputs.Mappable())
			if err != nil {
				t.Fatal(err)
			}
			armor := string(encoded)
			sawArmorSQLi = sawArmorSQLi || strings.Contains(armor, "sqli-v33-stable")
			sawArmorScanner = sawArmorScanner || strings.Contains(armor, "scannerdetection-v33-stable")
			sawArmorStatic = sawArmorStatic || strings.Contains(armor, "/static/")
		case "gcp:sql/databaseInstance:DatabaseInstance":
			sawSQL = true
			if got := res.Inputs["databaseVersion"].StringValue(); got != "MYSQL_8_4" {
				t.Fatalf("standard Cloud SQL database version = %q", got)
			}
			settings := res.Inputs["settings"].ObjectValue()
			if got := settings["tier"].StringValue(); got != "db-perf-optimized-N-2" {
				t.Fatalf("standard Cloud SQL tier = %q", got)
			}
			if got := settings["edition"].StringValue(); got != "ENTERPRISE_PLUS" {
				t.Fatalf("standard Cloud SQL edition = %q", got)
			}
			if _, ok := res.Inputs["rootPassword"]; ok {
				t.Fatal("Cloud SQL instance must not rely on the provider-deleted root user")
			}
			if got := res.Inputs["settings"].ObjectValue()["availabilityType"].StringValue(); got != "REGIONAL" {
				t.Fatalf("standard Cloud SQL availability = %q", got)
			}
			backup := res.Inputs["settings"].ObjectValue()["backupConfiguration"].ObjectValue()
			if !backup["enabled"].BoolValue() || !backup["binaryLogEnabled"].BoolValue() {
				t.Fatalf("standard Cloud SQL must enable backups and binary logging: %v", backup)
			}
			if backup["startTime"].StringValue() != "03:30" || backup["location"].StringValue() != "europe-west1" || backup["transactionLogRetentionDays"].NumberValue() != 7 || backup["backupRetentionSettings"].ObjectValue()["retainedBackups"].NumberValue() != 14 || !res.Inputs["settings"].ObjectValue()["deletionProtectionEnabled"].BoolValue() || !res.Inputs["deletionProtection"].BoolValue() {
				t.Fatalf("Cloud SQL protection settings were not propagated: %v", res.Inputs)
			}
			finalBackup := settings["finalBackupConfig"].ObjectValue()
			if !finalBackup["enabled"].BoolValue() || finalBackup["retentionDays"].NumberValue() != 14 {
				t.Fatalf("standard Cloud SQL must keep a final backup for 14 days on destroy: %v", finalBackup)
			}
		case "gcp:sql/user:User":
			sawSQLUsers++
			if _, ok := res.Inputs["databaseRoles"]; ok {
				t.Fatal("Cloud SQL users must use the managed built-in role default before the direct schema grant")
			}
		case "kubernetes:batch/v1:Job":
			if strings.HasSuffix(res.Name, "-db-grant") {
				sawGrantJob = true
				spec, ok := res.Inputs["spec"]
				if !ok {
					t.Fatal("database grant Job is missing its spec")
				}
				template, ok := spec.ObjectValue()["template"]
				if !ok {
					t.Fatal("database grant Job is missing its pod template")
				}
				podSpec, ok := template.ObjectValue()["spec"]
				if !ok {
					t.Fatal("database grant Job is missing its pod spec")
				}
				containers, ok := podSpec.ObjectValue()["containers"]
				if !ok || len(containers.ArrayValue()) != 1 {
					t.Fatal("database grant Job must have one container")
				}
				if spec.ObjectValue()["activeDeadlineSeconds"].NumberValue() == 900 && spec.ObjectValue()["backoffLimit"].NumberValue() == 2 {
					sawGrantRetryPolicy = true
				}
				command, ok := containers.ArrayValue()[0].ObjectValue()["command"]
				if !ok {
					t.Fatal("database grant Job is missing its command")
				}
				values := command.ArrayValue()
				if len(values) == 3 && values[0].StringValue() == "/bin/sh" && values[1].StringValue() == "-ec" {
					sawGrantCommand = true
				}
				resources, ok := containers.ArrayValue()[0].ObjectValue()["resources"]
				if ok {
					requests, ok := resources.ObjectValue()["requests"]
					if ok && requests.ObjectValue()["memory"].StringValue() == "1Gi" {
						sawGrantResources = true
					}
				}
			}
		case "kubernetes:core/v1:Secret":
			if strings.HasSuffix(res.Name, "-queue-password") {
				sawQueuePasswordSecret = true
			}
			if strings.HasSuffix(res.Name, "-rabbitmq-credentials") {
				sawQueueBrokerSecret = true
			}
		case "kubernetes:core/v1:ConfigMap":
			if strings.HasSuffix(res.Name, "-rabbitmq-config") {
				sawQueueConfig = true
			}
		case "kubernetes:core/v1:Service":
			if strings.HasSuffix(res.Name, "-rabbitmq-headless") {
				sawQueueHeadless = true
				serviceSpec, ok := res.Inputs["spec"]
				if !ok || serviceSpec.ObjectValue()["clusterIP"].StringValue() != "None" || !serviceSpec.ObjectValue()["publishNotReadyAddresses"].BoolValue() {
					t.Fatalf("RabbitMQ headless Service must publish not-ready addresses")
				}
			}
			if strings.HasSuffix(res.Name, "-search-headless") {
				sawSearchHeadless = true
				serviceSpec, ok := res.Inputs["spec"]
				if !ok || serviceSpec.ObjectValue()["clusterIP"].StringValue() != "None" || !serviceSpec.ObjectValue()["publishNotReadyAddresses"].BoolValue() {
					t.Fatalf("OpenSearch headless Service must publish not-ready addresses")
				}
			}
		case "kubernetes:apps/v1:StatefulSet":
			if strings.HasSuffix(res.Name, "-search") {
				sawSearchStatefulSet = true
				statefulSpec, ok := res.Inputs["spec"]
				if !ok {
					t.Fatal("OpenSearch StatefulSet is missing its spec")
				}
				statefulSpecValue := statefulSpec.ObjectValue()
				if statefulSpecValue["podManagementPolicy"].StringValue() != "Parallel" || statefulSpecValue["replicas"].NumberValue() != 1 || statefulSpecValue["serviceName"].StringValue() == "" {
					t.Fatalf("OpenSearch StatefulSet must use Parallel management and a headless service: %v", statefulSpecValue)
				}
				if len(statefulSpecValue["volumeClaimTemplates"].ArrayValue()) != 1 {
					t.Fatal("OpenSearch StatefulSet must have one persistent volume claim template")
				}
				template, ok := statefulSpecValue["template"]
				if !ok {
					t.Fatal("OpenSearch StatefulSet is missing its pod template")
				}
				podSpec := template.ObjectValue()["spec"].ObjectValue()
				containers := podSpec["containers"].ArrayValue()
				if len(containers) != 1 {
					t.Fatal("OpenSearch StatefulSet must have one container")
				}
				containerValue := containers[0].ObjectValue()
				if !strings.Contains(containerValue["image"].StringValue(), "opensearchproject/opensearch:3@sha256:") {
					t.Fatalf("OpenSearch StatefulSet must use the Adobe-compatible default image: %v", containerValue["image"])
				}
				readiness, ok := containerValue["readinessProbe"]
				if !ok {
					t.Fatal("OpenSearch StatefulSet is missing its readiness probe")
				}
				readinessValue := readiness.ObjectValue()
				if readinessValue["initialDelaySeconds"].NumberValue() != 30 || readinessValue["periodSeconds"].NumberValue() != 10 || readinessValue["timeoutSeconds"].NumberValue() != 10 || readinessValue["failureThreshold"].NumberValue() != 30 {
					t.Fatalf("OpenSearch readiness probe must allow a slow startup: %v", readinessValue)
				}
				readinessHTTP, ok := readinessValue["httpGet"]
				if !ok || readinessHTTP.ObjectValue()["path"].StringValue() != "/_cluster/health?wait_for_status=yellow&timeout=5s" {
					t.Fatalf("OpenSearch readiness probe must check cluster health: %v", readinessValue)
				}
				sawSearchReadiness = true
				startup, ok := containerValue["startupProbe"]
				if !ok {
					t.Fatal("OpenSearch StatefulSet is missing its startup probe")
				}
				startupValue := startup.ObjectValue()
				if startupValue["initialDelaySeconds"].NumberValue() != 30 || startupValue["periodSeconds"].NumberValue() != 10 || startupValue["timeoutSeconds"].NumberValue() != 10 || startupValue["failureThreshold"].NumberValue() != 60 {
					t.Fatalf("OpenSearch startup probe must allow a slow startup: %v", startupValue)
				}
				liveness, ok := containerValue["livenessProbe"]
				if !ok {
					t.Fatal("OpenSearch StatefulSet is missing its liveness probe")
				}
				livenessValue := liveness.ObjectValue()
				if livenessValue["initialDelaySeconds"].NumberValue() != 120 || livenessValue["periodSeconds"].NumberValue() != 30 || livenessValue["timeoutSeconds"].NumberValue() != 10 || livenessValue["failureThreshold"].NumberValue() != 5 {
					t.Fatalf("OpenSearch liveness probe must tolerate warmup: %v", livenessValue)
				}
				sawSearchProbeBudget = true
				env := containerValue["env"].ArrayValue()
				for _, item := range env {
					value := item.ObjectValue()
					if value["name"].StringValue() == "DISABLE_INSTALL_DEMO_CONFIG" && value["value"].StringValue() == "true" {
						sawSearchBootstrap = true
					}
				}
			}
			if strings.HasSuffix(res.Name, "-rabbitmq") {
				sawQueueStatefulSet = true
				statefulSpec, ok := res.Inputs["spec"]
				if !ok {
					t.Fatal("RabbitMQ StatefulSet is missing its spec")
				}
				statefulSpecValue := statefulSpec.ObjectValue()
				if statefulSpecValue["podManagementPolicy"].StringValue() != "Parallel" || statefulSpecValue["replicas"].NumberValue() != 1 || statefulSpecValue["serviceName"].StringValue() == "" {
					t.Fatalf("RabbitMQ StatefulSet must use Parallel management and a headless service: %v", statefulSpecValue)
				}
				if len(statefulSpecValue["volumeClaimTemplates"].ArrayValue()) != 1 {
					t.Fatal("RabbitMQ StatefulSet must have one persistent volume claim template")
				}
				template, ok := statefulSpecValue["template"]
				if !ok {
					t.Fatal("RabbitMQ StatefulSet is missing its pod template")
				}
				podSpec, ok := template.ObjectValue()["spec"]
				if !ok {
					t.Fatal("RabbitMQ StatefulSet is missing its pod spec")
				}
				initContainers, ok := podSpec.ObjectValue()["initContainers"]
				if !ok {
					t.Fatal("RabbitMQ StatefulSet is missing its configuration init container")
				}
				for _, initContainer := range initContainers.ArrayValue() {
					initValue := initContainer.ObjectValue()
					if initValue["name"].StringValue() != "rabbitmq-configure" {
						continue
					}
					initCommand := initValue["args"].ArrayValue()[0].StringValue()
					if !strings.Contains(initCommand, "chown -R 100:101 /etc/rabbitmq /var/lib/rabbitmq") || !strings.Contains(initCommand, "chmod 600 /var/lib/rabbitmq/.erlang.cookie") {
						t.Fatal("RabbitMQ configuration init container must repair the persistent data volume ownership")
					}
					securityContext, ok := initValue["securityContext"]
					if !ok {
						t.Fatal("RabbitMQ configuration init container must run with an explicit root security context")
					}
					security := securityContext.ObjectValue()
					if _, userSet := security["runAsUser"]; !userSet {
						t.Fatalf("RabbitMQ configuration init container must set runAsUser=0: %v", security)
					}
					if _, groupSet := security["runAsGroup"]; !groupSet {
						t.Fatalf("RabbitMQ configuration init container must set runAsGroup=0: %v", security)
					}
					if _, nonRootSet := security["runAsNonRoot"]; !nonRootSet {
						t.Fatalf("RabbitMQ configuration init container must set runAsNonRoot=false: %v", security)
					}
					if security["runAsUser"].NumberValue() != 0 || security["runAsGroup"].NumberValue() != 0 || security["runAsNonRoot"].BoolValue() {
						t.Fatalf("RabbitMQ configuration init container must run as root: %v", security)
					}
					sawQueueInitRoot = true
					for _, mount := range initValue["volumeMounts"].ArrayValue() {
						if mount.ObjectValue()["name"].StringValue() == "data" && mount.ObjectValue()["mountPath"].StringValue() == "/var/lib/rabbitmq" {
							sawQueueDataOwnership = true
						}
					}
				}
				containers, ok := podSpec.ObjectValue()["containers"]
				if !ok {
					t.Fatal("RabbitMQ StatefulSet is missing its containers")
				}
				for _, container := range containers.ArrayValue() {
					containerValue := container.ObjectValue()
					if containerValue["name"].StringValue() != "rabbitmq" {
						continue
					}
					if !strings.Contains(containerValue["image"].StringValue(), "rabbitmq:4.2-management-alpine@sha256:") {
						t.Fatalf("RabbitMQ StatefulSet must use the Adobe-compatible default image, got %q", containerValue["image"].StringValue())
					}
					readiness, ok := containerValue["readinessProbe"]
					if !ok {
						t.Fatal("RabbitMQ StatefulSet is missing its readiness probe")
					}
					readinessValue := readiness.ObjectValue()
					if readinessValue["initialDelaySeconds"].NumberValue() != 30 || readinessValue["periodSeconds"].NumberValue() != 10 || readinessValue["timeoutSeconds"].NumberValue() != 10 || readinessValue["failureThreshold"].NumberValue() != 30 {
						t.Fatalf("RabbitMQ readiness probe must allow a slow 4.2 startup: %v", readinessValue)
					}
					execProbe, ok := readiness.ObjectValue()["exec"]
					if !ok {
						t.Fatal("RabbitMQ readiness probe is missing its exec action")
					}
					command, ok := execProbe.ObjectValue()["command"]
					if !ok {
						t.Fatal("RabbitMQ readiness probe is missing its command")
					}
					values := command.ArrayValue()
					if len(values) == 3 && values[0].StringValue() == "rabbitmq-diagnostics" && values[1].StringValue() == "-q" && values[2].StringValue() == "ping" {
						sawQueueReadiness = true
					}
					startup, ok := containerValue["startupProbe"]
					if !ok {
						t.Fatal("RabbitMQ StatefulSet is missing its startup probe")
					}
					startupValue := startup.ObjectValue()
					if startupValue["initialDelaySeconds"].NumberValue() != 30 || startupValue["periodSeconds"].NumberValue() != 10 || startupValue["timeoutSeconds"].NumberValue() != 10 || startupValue["failureThreshold"].NumberValue() != 60 {
						t.Fatalf("RabbitMQ startup probe must allow a slow 4.2 startup: %v", startupValue)
					}
					startupExec, ok := startup.ObjectValue()["exec"]
					if !ok {
						t.Fatal("RabbitMQ startup probe is missing its exec action")
					}
					startupCommand, ok := startupExec.ObjectValue()["command"]
					if !ok {
						t.Fatal("RabbitMQ startup probe is missing its command")
					}
					startupValues := startupCommand.ArrayValue()
					if len(startupValues) != 3 || startupValues[0].StringValue() != "rabbitmq-diagnostics" || startupValues[1].StringValue() != "-q" || startupValues[2].StringValue() != "ping" {
						sawQueueReadiness = false
					}
					liveness, ok := containerValue["livenessProbe"]
					if !ok {
						t.Fatal("RabbitMQ StatefulSet is missing its liveness probe")
					}
					livenessValue := liveness.ObjectValue()
					if livenessValue["initialDelaySeconds"].NumberValue() != 120 || livenessValue["periodSeconds"].NumberValue() != 30 || livenessValue["timeoutSeconds"].NumberValue() != 10 || livenessValue["failureThreshold"].NumberValue() != 5 {
						t.Fatalf("RabbitMQ liveness probe must tolerate broker warmup: %v", livenessValue)
					}
					sawQueueProbeBudget = true
				}
				spreads := podSpec.ObjectValue()["topologySpreadConstraints"].ArrayValue()
				spreadIsStrict := len(spreads) == 1 &&
					spreads[0].ObjectValue()["topologyKey"].StringValue() == "topology.kubernetes.io/zone" &&
					spreads[0].ObjectValue()["maxSkew"].NumberValue() == 1 &&
					spreads[0].ObjectValue()["whenUnsatisfiable"].StringValue() == "DoNotSchedule"
				if spreadIsStrict {
					sawQueueSpread = true
				}
			}
		case "kubernetes:rbac.authorization.k8s.io/v1:Role":
			if strings.HasSuffix(res.Name, "-rabbitmq-peer-discovery") {
				sawQueueRole = true
			}
		case "kubernetes:rbac.authorization.k8s.io/v1:RoleBinding":
			if strings.HasSuffix(res.Name, "-rabbitmq-peer-discovery-binding") {
				sawQueueRoleBinding = true
			}
		}
	}
	if !sawBucket || !sawArmor || !sawArmorRules || !sawArmorSQLi || !sawArmorScanner || !sawArmorStatic || !sawSQL || !sawSearchStatefulSet || !sawSearchHeadless || !sawSearchReadiness || !sawSearchProbeBudget || !sawSearchBootstrap || !sawQueuePasswordSecret || !sawQueueStatefulSet || !sawQueueHeadless || !sawQueueRole || !sawQueueRoleBinding || !sawQueueConfig || !sawQueueBrokerSecret || !sawQueueSpread || !sawQueueReadiness || !sawQueueProbeBudget || !sawQueueInitRoot || !sawQueueDataOwnership || sawSQLUsers != 2 || !sawGrantJob || !sawGrantCommand || !sawGrantResources || !sawGrantRetryPolicy {
		t.Fatalf("missing production resources bucket=%v armor=%v armorRules=%v armorSQLi=%v armorScanner=%v armorStatic=%v sql=%v searchStatefulSet=%v searchHeadless=%v searchReadiness=%v searchProbeBudget=%v searchBootstrap=%v queuePasswordSecret=%v queueStatefulSet=%v queueHeadless=%v queueRole=%v queueRoleBinding=%v queueConfig=%v queueBrokerSecret=%v queueSpread=%v queueReadiness=%v queueProbeBudget=%v queueInitRoot=%v queueDataOwnership=%v sqlUsers=%d grantJob=%v grantCommand=%v grantResources=%v grantRetryPolicy=%v (n=%d)", sawBucket, sawArmor, sawArmorRules, sawArmorSQLi, sawArmorScanner, sawArmorStatic, sawSQL, sawSearchStatefulSet, sawSearchHeadless, sawSearchReadiness, sawSearchProbeBudget, sawSearchBootstrap, sawQueuePasswordSecret, sawQueueStatefulSet, sawQueueHeadless, sawQueueRole, sawQueueRoleBinding, sawQueueConfig, sawQueueBrokerSecret, sawQueueSpread, sawQueueReadiness, sawQueueProbeBudget, sawQueueInitRoot, sawQueueDataOwnership, sawSQLUsers, sawGrantJob, sawGrantCommand, sawGrantResources, sawGrantRetryPolicy, len(mocks.resources))
	}
}

func TestProgramBuildsGCPNativeEdgeGraph(t *testing.T) {
	in := gcpDeploymentInputs()
	in.Envelope.Domain = "shop.example.com"
	in.Edge = sdk.EdgeIntent{
		Mode: "native", NativeProvider: "cloud-cdn", Domains: []string{"shop.example.com"},
		TLS: true, TLSMode: "gke-managed", DNSMode: "customer-managed", OriginHealthRef: "health/shop",
		OwnershipMarker: "shop/staging",
	}
	spec, err := PlanFromInputs(in)
	if err != nil {
		t.Fatal(err)
	}
	mocks := &stackMocks{}
	if err := pulumi.RunErr(Program(spec), pulumi.WithMocks("magelift", "shop-staging", mocks)); err != nil {
		t.Fatal(err)
	}
	sawBackend, sawCertificate, sawIngress, sawClusterIPService := false, false, false, false
	for _, res := range mocks.resources {
		switch {
		case strings.Contains(res.TypeToken, "cloud.google.com/v1") && strings.HasSuffix(res.Name, "-edge-backend"):
			sawBackend = true
			backendSpec := res.Inputs["spec"].ObjectValue()
			if !backendSpec["cdn"].ObjectValue()["enabled"].BoolValue() {
				t.Fatal("GCP Cloud CDN BackendConfig must enable CDN")
			}
		case strings.Contains(res.TypeToken, "networking.gke.io/v1") && strings.HasSuffix(res.Name, "-edge-certificate"):
			sawCertificate = true
		case res.TypeToken == "kubernetes:networking.k8s.io/v1:Ingress":
			sawIngress = true
			annotations := res.Inputs["metadata"].ObjectValue()["annotations"].ObjectValue()
			if annotations["kubernetes.io/ingress.class"].StringValue() != "gce" {
				t.Fatalf("GCP native edge must select the GCE Ingress controller with its supported annotation: %v", annotations)
			}
			spec := res.Inputs["spec"].ObjectValue()
			if _, hasIngressClassName := spec["ingressClassName"]; hasIngressClassName {
				t.Fatalf("GCP native edge must not rely on unsupported IngressClassName field: %v", spec)
			}
			if _, hasSecretTLS := spec["tls"]; hasSecretTLS {
				t.Fatalf("GCP managed certificates must use the managed-certificates annotation, not Secret-backed spec.tls: %v", spec)
			}
		case res.TypeToken == "kubernetes:core/v1:Service" && strings.HasSuffix(res.Name, "-web-svc"):
			sawClusterIPService = res.Inputs["spec"].ObjectValue()["type"].StringValue() == "ClusterIP"
		}
	}
	if !sawBackend || !sawCertificate || !sawIngress || !sawClusterIPService {
		t.Fatalf("native GCP edge graph incomplete: backend=%v certificate=%v ingress=%v clusterIPService=%v", sawBackend, sawCertificate, sawIngress, sawClusterIPService)
	}
}

func TestProgramBuildsHighAvailabilityPresetGraph(t *testing.T) {
	spec := Spec{
		Identity: Identity{
			Project: "shop", GCPProject: "example-gcp-project", Environment: "prod",
			Region: "europe-west1", Runtime: "gke-standard", EnvironmentClass: "production", Preset: "high-availability",
			Labels: map[string]string{"magelift-managed-by": "magelift"},
		},
		Application: Application{Edition: "open-source", Version: "2.4.8", Mode: "integrated", WebRuntime: "nginx-fpm"},
		Artifact:    Artifact{ImageDigest: "ghcr.io/magelift/magento@sha256:abcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcd"},
		Policy:      NetworkPolicy{NetworkCIDR: "10.20.0.0/16", Zones: []string{"europe-west1-b", "europe-west1-c", "europe-west1-d"}},
		Catalog: CatalogSelection{
			CloudSQLTier: "db-perf-optimized-N-2", CloudSQLAvailability: "REGIONAL", ValkeyRequirement: "8.1",
			CloudSQLDeletionProtection: true,
			MemorystoreNodeType:        "STANDARD_SMALL", MemorystoreShardCount: 4, MemorystoreReplicas: 2,
			MemorystoreMode: "CLUSTER", MemorystoreZoneDistributionMode: "SINGLE_ZONE", MemorystoreZone: "europe-west1-b",
			MemorystoreDeletionProtection: true, MemorystorePSCConnectionLimit: 4,
			KubernetesVersion: "1.32", ReleaseChannel: "STABLE", ClusterIPv4CIDR: "10.64.0.0/16", ServicesIPv4CIDR: "10.65.0.0/20",
			StandardNodeType: "n2-standard-8", StandardNodeCount: 4, StandardNodeMinCount: 3, StandardNodeMaxCount: 6,
			StandardNodeDiskType: "pd-ssd", StandardNodeDiskSizeGiB: 200, StandardNodeImageType: "UBUNTU_CONTAINERD", StandardNodeSpot: true,
			AutopilotCPURequest: "1", AutopilotMemoryRequest: "2Gi", DesiredWebReplicas: 3,
			SearchMode: "opensearch", SearchReplicas: 3, QueueMode: "rabbitmq", QueueReplicas: 2,
			QueueConsumerCount: 2, EnableCloudArmor: true,
		},
		Dependencies: Dependencies{DatabaseName: "magento", MasterUsername: "magento", EncryptionKeySecret: "magento-crypt-key"},
	}
	if err := spec.Validate(); err != nil {
		t.Fatal(err)
	}
	mocks := &stackMocks{}
	if err := pulumi.RunErr(Program(spec), pulumi.WithMocks("magelift", "shop-prod", mocks)); err != nil {
		t.Fatal(err)
	}
	if len(mocks.resources) < 20 {
		t.Fatalf("expected a dense HA graph, got %d resources", len(mocks.resources))
	}
	standardClusterValidated := false
	standardNodePoolValidated := false
	memorystoreValidated := false
	searchHAValidated := false
	for _, res := range mocks.resources {
		if res.TypeToken == "gcp:memorystore/instance:Instance" {
			if res.Inputs["shardCount"].NumberValue() != 4 || res.Inputs["replicaCount"].NumberValue() != 2 || res.Inputs["mode"].StringValue() != "CLUSTER" {
				t.Fatalf("Memorystore topology settings were not propagated: %v", res.Inputs)
			}
			if !res.Inputs["deletionProtectionEnabled"].BoolValue() {
				t.Fatalf("Memorystore deletion protection was not propagated: %v", res.Inputs)
			}
			zoneDistribution := res.Inputs["zoneDistributionConfig"].ObjectValue()
			if zoneDistribution["mode"].StringValue() != "SINGLE_ZONE" || zoneDistribution["zone"].StringValue() != "europe-west1-b" {
				t.Fatalf("Memorystore zone distribution was not propagated: %v", zoneDistribution)
			}
			memorystoreValidated = true
		}
		if res.TypeToken == "gcp:networkconnectivity/serviceConnectionPolicy:ServiceConnectionPolicy" {
			psc := res.Inputs["pscConfig"].ObjectValue()
			if psc["limit"].StringValue() != "4" {
				t.Fatalf("Memorystore PSC connection limit was not propagated: %v", res.Inputs)
			}
		}
		if res.TypeToken == "gcp:container/cluster:Cluster" {
			clusterArgs := res.Inputs
			autopilot, hasAutopilot := clusterArgs["enableAutopilot"]
			removeDefaultPool, hasRemoveDefaultPool := clusterArgs["removeDefaultNodePool"]
			if (hasAutopilot && autopilot.BoolValue()) || !hasRemoveDefaultPool || !removeDefaultPool.BoolValue() {
				t.Fatalf("GKE Standard HA cluster must disable Autopilot and remove the default node pool: %v", clusterArgs)
			}
			if clusterArgs["minMasterVersion"].StringValue() != "1.32" || clusterArgs["releaseChannel"].ObjectValue()["channel"].StringValue() != "STABLE" {
				t.Fatalf("GKE cluster version and release channel were not propagated: %v", clusterArgs)
			}
			ipPolicy := clusterArgs["ipAllocationPolicy"].ObjectValue()
			if ipPolicy["clusterIpv4CidrBlock"].StringValue() != "10.64.0.0/16" || ipPolicy["servicesIpv4CidrBlock"].StringValue() != "10.65.0.0/20" {
				t.Fatalf("GKE cluster CIDRs were not propagated: %v", ipPolicy)
			}
			standardClusterValidated = true
		}
		if res.TypeToken == "gcp:container/nodePool:NodePool" {
			nodeConfig := res.Inputs["nodeConfig"].ObjectValue()
			if res.Inputs["initialNodeCount"].NumberValue() != 4 || nodeConfig["machineType"].StringValue() != "n2-standard-8" || nodeConfig["diskType"].StringValue() != "pd-ssd" || nodeConfig["diskSizeGb"].NumberValue() != 200 || nodeConfig["imageType"].StringValue() != "UBUNTU_CONTAINERD" || !nodeConfig["spot"].BoolValue() {
				t.Fatalf("GKE Standard node settings were not propagated: %v", res.Inputs)
			}
			autoscaling := res.Inputs["autoscaling"].ObjectValue()
			if autoscaling["minNodeCount"].NumberValue() != 3 || autoscaling["maxNodeCount"].NumberValue() != 6 {
				t.Fatalf("GKE Standard autoscaling bounds were not propagated: %v", autoscaling)
			}
			linux := nodeConfig["linuxNodeConfig"].ObjectValue()
			sysctls := linux["sysctls"].ObjectValue()
			if sysctls["vm.max_map_count"].StringValue() != "262144" {
				t.Fatalf("GKE Standard OpenSearch node pool must set vm.max_map_count: %v", res.Inputs)
			}
			if !res.Inputs["networkConfig"].ObjectValue()["enablePrivateNodes"].BoolValue() {
				t.Fatal("GKE Standard node pool must use private nodes")
			}
			standardNodePoolValidated = true
		}
		if res.TypeToken != "kubernetes:apps/v1:StatefulSet" || !strings.HasSuffix(res.Name, "-search") {
			continue
		}
		spec := res.Inputs["spec"].ObjectValue()
		if spec["replicas"].NumberValue() != 3 || spec["serviceName"].StringValue() == "" {
			t.Fatalf("HA OpenSearch StatefulSet must have three replicas and a headless service: %v", spec)
		}
		template := spec["template"].ObjectValue()
		podSpec := template["spec"].ObjectValue()
		spreads := podSpec["topologySpreadConstraints"].ArrayValue()
		if len(spreads) != 1 || spreads[0].ObjectValue()["whenUnsatisfiable"].StringValue() != "DoNotSchedule" {
			t.Fatalf("HA OpenSearch StatefulSet must use strict zone spreading: %v", spreads)
		}
		containers := podSpec["containers"].ArrayValue()
		if len(containers) != 1 {
			t.Fatal("HA OpenSearch StatefulSet must have one container")
		}
		seenInitialManagers := false
		seenSingleNode := false
		for _, item := range containers[0].ObjectValue()["env"].ArrayValue() {
			env := item.ObjectValue()
			switch env["name"].StringValue() {
			case "cluster.initial_cluster_manager_nodes":
				seenInitialManagers = env["value"].StringValue() == "shop-prod-search-0,shop-prod-search-1,shop-prod-search-2"
			case "discovery.type":
				seenSingleNode = true
			}
		}
		if !seenInitialManagers || seenSingleNode {
			t.Fatalf("HA OpenSearch must configure multi-node discovery: %v", containers[0].ObjectValue()["env"])
		}
		searchHAValidated = true
	}
	if !standardClusterValidated || !standardNodePoolValidated || !memorystoreValidated {
		t.Fatalf("GKE Standard HA graph is missing its cluster, sysctl node pool, or Memorystore topology: cluster=%v nodePool=%v memorystore=%v", standardClusterValidated, standardNodePoolValidated, memorystoreValidated)
	}
	if !searchHAValidated {
		t.Fatal("HA graph is missing the OpenSearch StatefulSet")
	}
}

type stackMocks struct {
	mu        sync.Mutex
	resources []pulumi.MockResourceArgs
}

func (m *stackMocks) NewResource(args pulumi.MockResourceArgs) (string, resource.PropertyMap, error) {
	m.mu.Lock()
	m.resources = append(m.resources, args)
	m.mu.Unlock()
	state := args.Inputs.Copy()
	switch args.TypeToken {
	case "gcp:compute/network:Network":
		state["selfLink"] = resource.NewStringProperty("https://www.googleapis.com/compute/v1/projects/p/global/networks/n")
		state["name"] = resource.NewStringProperty(args.Name)
	case "gcp:compute/subnetwork:Subnetwork":
		state["name"] = resource.NewStringProperty(args.Name)
		state["selfLink"] = resource.NewStringProperty("https://www.googleapis.com/compute/v1/projects/p/regions/r/subnetworks/" + args.Name)
	case "gcp:sql/databaseInstance:DatabaseInstance":
		state["privateIpAddress"] = resource.NewStringProperty("10.20.1.5")
		state["connectionName"] = resource.NewStringProperty("p:europe-west1:sql")
		state["name"] = resource.NewStringProperty(args.Name)
	case "gcp:networkconnectivity/serviceConnectionPolicy:ServiceConnectionPolicy":
		state["name"] = resource.NewStringProperty(args.Name)
	case "gcp:memorystore/instance:Instance":
		state["instanceId"] = resource.NewStringProperty(args.Name)
		state["endpoints"] = resource.NewArrayProperty([]resource.PropertyValue{
			resource.NewObjectProperty(resource.PropertyMap{
				"connections": resource.NewArrayProperty([]resource.PropertyValue{
					resource.NewObjectProperty(resource.PropertyMap{
						"pscAutoConnection": resource.NewObjectProperty(resource.PropertyMap{
							"ipAddress": resource.NewStringProperty("10.20.2.8"),
						}),
					}),
				}),
			}),
		})
	case "gcp:container/cluster:Cluster":
		state["name"] = resource.NewStringProperty(args.Name)
		state["endpoint"] = resource.NewStringProperty("1.2.3.4")
		state["masterAuth"] = resource.NewObjectProperty(resource.PropertyMap{
			"clusterCaCertificate": resource.NewStringProperty("Y2E="),
		})
	case "gcp:storage/bucket:Bucket":
		state["name"] = resource.NewStringProperty(args.Name)
		state["url"] = resource.NewStringProperty("gs://" + args.Name)
	case "gcp:storage/hmacKey:HmacKey":
		state["accessId"] = resource.NewStringProperty("GOOGMOCKACCESSID")
		state["secret"] = resource.MakeSecret(resource.NewStringProperty("mock-hmac-secret"))
	case "gcp:serviceaccount/account:Account":
		state["email"] = resource.NewStringProperty(args.Name + "@example-gcp-project.iam.gserviceaccount.com")
	case "gcp:compute/securityPolicy:SecurityPolicy":
		state["name"] = resource.NewStringProperty(args.Name)
	case "kubernetes:core/v1:Service":
		state["status"] = resource.NewObjectProperty(resource.PropertyMap{
			"loadBalancer": resource.NewObjectProperty(resource.PropertyMap{
				"ingress": resource.NewArrayProperty([]resource.PropertyValue{
					resource.NewObjectProperty(resource.PropertyMap{"ip": resource.NewStringProperty("35.1.2.3")}),
				}),
			}),
		})
		state["metadata"] = resource.NewObjectProperty(resource.PropertyMap{"name": resource.NewStringProperty(args.Name)})
	case "kubernetes:apps/v1:Deployment":
		state["metadata"] = resource.NewObjectProperty(resource.PropertyMap{"name": resource.NewStringProperty(args.Name)})
	case "random:index/randomPassword:RandomPassword":
		state["result"] = resource.NewStringProperty("generated-password-value-32chars!!")
	case "gcp:secretmanager/secret:Secret":
		state["secretId"] = resource.NewStringProperty(args.Name)
	case "gcp:secretmanager/secretVersion:SecretVersion":
		state["secretData"] = resource.MakeSecret(resource.NewStringProperty("mock-magento-crypt-key"))
	}
	return args.Name + "-id", state, nil
}

func (m *stackMocks) Call(args pulumi.MockCallArgs) (resource.PropertyMap, error) {
	switch args.Token {
	case "gcp:organizations/getClientConfig:getClientConfig":
		return resource.PropertyMap{
			"accessToken": resource.MakeSecret(resource.NewStringProperty("mock-gcp-access-token")),
			"project":     resource.NewStringProperty("example-gcp-project"),
		}, nil
	default:
		return resource.PropertyMap{}, nil
	}
}

func TestProgramWiresSmtpRelay(t *testing.T) {
	spec := Spec{
		Identity: Identity{
			Project: "shop", GCPProject: "example-gcp-project", Environment: "preview",
			Region: "europe-west1", EnvironmentClass: "preview", Preset: "preview",
			Labels: map[string]string{"magelift-managed-by": "magelift"},
		},
		Application: Application{Edition: "open-source", Version: "2.4.8", Mode: "integrated", WebRuntime: "nginx-fpm"},
		Artifact:    Artifact{ImageDigest: "ghcr.io/magelift/magento@sha256:abcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcd"},
		Policy:      NetworkPolicy{NetworkCIDR: "10.20.0.0/16", Zones: []string{"europe-west1-b", "europe-west1-c"}},
		Catalog: CatalogSelection{
			CloudSQLTier: "db-perf-optimized-N-2", CloudSQLAvailability: "ZONAL", ValkeyRequirement: "8.1",
			MemorystoreNodeType: "SHARED_CORE_NANO", AutopilotCPURequest: "500m", AutopilotMemoryRequest: "1Gi",
			DesiredWebReplicas: 1, SearchMode: "opensearch", SearchReplicas: 1, QueueMode: "database",
		},
		Dependencies: Dependencies{DatabaseName: "magento", MasterUsername: "magento", EncryptionKeySecret: "magento-crypt-key"},
		Email: EmailSelection{
			Mode: "smtp", Host: "smtp.example.com", Port: 587, Username: "mailer",
			From: "shop@example.com", Credential: "gcp-secret-manager://projects/example-gcp-project/secrets/smtp-password/versions/latest",
		},
	}
	if err := spec.Validate(); err != nil {
		t.Fatal(err)
	}
	mocks := &stackMocks{}
	if err := pulumi.RunErr(Program(spec), pulumi.WithMocks("magelift", "shop-preview", mocks)); err != nil {
		t.Fatal(err)
	}
	sawSecret, sawHost, sawPasswordRef := false, false, false
	for _, res := range mocks.resources {
		if res.TypeToken == "kubernetes:core/v1:Secret" && strings.HasSuffix(res.Name, "-smtp-password") {
			sawSecret = true
		}
		if res.TypeToken != "kubernetes:apps/v1:Deployment" {
			continue
		}
		containers := res.Inputs["spec"].ObjectValue()["template"].ObjectValue()["spec"].ObjectValue()["containers"].ArrayValue()
		for _, container := range containers {
			envValue, ok := container.ObjectValue()["env"]
			if !ok || envValue.IsNull() {
				continue
			}
			for _, env := range envValue.ArrayValue() {
				entry := env.ObjectValue()
				name := entry["name"].StringValue()
				switch name {
				case "CONFIG__DEFAULT__SYSTEM__SMTP__HOST":
					if value := entry["value"].StringValue(); value != "smtp.example.com" {
						t.Fatalf("smtp host = %q", value)
					}
					sawHost = true
				case "CONFIG__DEFAULT__SYSTEM__SMTP__PASSWORD":
					ref := entry["valueFrom"].ObjectValue()["secretKeyRef"].ObjectValue()
					if key := ref["key"].StringValue(); key != "password" {
						t.Fatalf("smtp password ref key = %q", key)
					}
					if value, ok := entry["value"]; ok && strings.TrimSpace(value.StringValue()) != "" {
						t.Fatalf("smtp password must not appear as plaintext env: %q", value.StringValue())
					}
					sawPasswordRef = true
				}
			}
		}
	}
	if !sawSecret || !sawHost || !sawPasswordRef {
		t.Fatalf("smtp wiring incomplete: secret=%v host=%v passwordRef=%v", sawSecret, sawHost, sawPasswordRef)
	}
}

func TestProgramWiresMediaRemoteStorage(t *testing.T) {
	spec := Spec{
		Identity: Identity{
			Project: "shop", GCPProject: "example-gcp-project", Environment: "preview",
			Region: "europe-west1", EnvironmentClass: "preview", Preset: "preview",
			Labels: map[string]string{"magelift-managed-by": "magelift"},
		},
		Application: Application{Edition: "open-source", Version: "2.4.9", Mode: "integrated", WebRuntime: "nginx-fpm"},
		Artifact:    Artifact{ImageDigest: "ghcr.io/magelift/magento@sha256:abcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcd"},
		Policy:      NetworkPolicy{NetworkCIDR: "10.20.0.0/16", Zones: []string{"europe-west1-b", "europe-west1-c"}},
		Catalog: CatalogSelection{
			CloudSQLTier: "db-perf-optimized-N-2", CloudSQLAvailability: "ZONAL", ValkeyRequirement: "9",
			MemorystoreEngineVersion: "VALKEY_9_0", MemorystoreNodeType: "SHARED_CORE_NANO",
			AutopilotCPURequest: "500m", AutopilotMemoryRequest: "1Gi",
			DesiredWebReplicas: 1, SearchMode: "opensearch", SearchReplicas: 1, QueueMode: "database",
		},
		Dependencies: Dependencies{DatabaseName: "magento", MasterUsername: "magento", EncryptionKeySecret: "magento-crypt-key"},
	}
	if err := spec.Validate(); err != nil {
		t.Fatal(err)
	}
	mocks := &stackMocks{}
	if err := pulumi.RunErr(Program(spec), pulumi.WithMocks("magelift", "shop-preview", mocks)); err != nil {
		t.Fatal(err)
	}
	sawHMAC, sawSA, sawWriterGrant, sawPublicGrant := false, false, false, false
	sawSecret, sawKey, sawEndpoint, sawSecretRef, sawDriver := false, false, false, false, false
	for _, res := range mocks.resources {
		switch res.TypeToken {
		case "gcp:storage/hmacKey:HmacKey":
			sawHMAC = true
		case "gcp:serviceaccount/account:Account":
			if strings.Contains(res.Name, "media") {
				sawSA = true
			}
		case "gcp:storage/bucketIAMMember:BucketIAMMember":
			role := res.Inputs["role"].StringValue()
			member := res.Inputs["member"].StringValue()
			if role == "roles/storage.objectAdmin" && strings.HasPrefix(member, "serviceAccount:") {
				sawWriterGrant = true
			}
			if role == "roles/storage.objectViewer" && member == "allUsers" {
				condition := res.Inputs["condition"].ObjectValue()["expression"].StringValue()
				if !strings.Contains(condition, "/objects/media/") {
					t.Fatalf("public grant is not prefix-scoped: %q", condition)
				}
				sawPublicGrant = true
			}
		case "kubernetes:core/v1:Secret":
			if strings.HasSuffix(res.Name, "-media-hmac") {
				sawSecret = true
			}
		case "kubernetes:apps/v1:Deployment":
			containers := res.Inputs["spec"].ObjectValue()["template"].ObjectValue()["spec"].ObjectValue()["containers"].ArrayValue()
			for _, container := range containers {
				envValue, ok := container.ObjectValue()["env"]
				if !ok || envValue.IsNull() {
					continue
				}
				for _, env := range envValue.ArrayValue() {
					entry := env.ObjectValue()
					switch entry["name"].StringValue() {
					case "MAGELIFT_MEDIA_S3_KEY":
						if value := entry["value"].StringValue(); value != "GOOGMOCKACCESSID" {
							t.Fatalf("media key = %q", value)
						}
						sawKey = true
					case "MAGELIFT_MEDIA_S3_ENDPOINT":
						sawEndpoint = true
					case "MAGELIFT_MEDIA_DRIVER":
						if value := entry["value"].StringValue(); value != "aws-s3" {
							t.Fatalf("media driver = %q", value)
						}
						sawDriver = true
					case "MAGELIFT_MEDIA_S3_SECRET":
						ref := entry["valueFrom"].ObjectValue()["secretKeyRef"].ObjectValue()
						if key := ref["key"].StringValue(); key != "secret" {
							t.Fatalf("media secret ref key = %q", key)
						}
						if value, ok := entry["value"]; ok && strings.TrimSpace(value.StringValue()) != "" {
							t.Fatalf("media secret must not appear as plaintext env: %q", value.StringValue())
						}
						sawSecretRef = true
					}
				}
			}
		}
	}
	if !sawHMAC || !sawSA || !sawWriterGrant || !sawPublicGrant {
		t.Fatalf("media IAM incomplete: hmac=%v sa=%v writer=%v public=%v", sawHMAC, sawSA, sawWriterGrant, sawPublicGrant)
	}
	if !sawSecret || !sawKey || !sawEndpoint || !sawSecretRef || !sawDriver {
		t.Fatalf("media env incomplete: secret=%v key=%v endpoint=%v secretRef=%v driver=%v", sawSecret, sawKey, sawEndpoint, sawSecretRef, sawDriver)
	}
}
