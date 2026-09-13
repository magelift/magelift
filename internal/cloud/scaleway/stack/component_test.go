package stack

import (
	"strings"
	"sync"
	"testing"

	"github.com/magelift/magelift/internal/config"
	"github.com/magelift/magelift/internal/platform"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

func TestPlanFromConfigRejectsAWSTarget(t *testing.T) {
	_, err := PlanFromConfig(config.Config{
		Target: config.Target{Provider: "aws", Runtime: "ecs-fargate"},
	}, "staging")
	if err == nil || !strings.Contains(err.Error(), "Scaleway stack requires") {
		t.Fatalf("expected Scaleway target rejection, got %v", err)
	}
}

func TestPlanFromConfigRejectsUnderprovisionedScalewayPreset(t *testing.T) {
	cfg := config.Config{
		SchemaVersion: 1,
		Project:       config.Project{Name: "shop"},
		Application:   config.Application{Edition: "open-source", Version: "2.4.8", Mode: "integrated"},
		Target: config.Target{Provider: "scaleway", Runtime: "kapsule", Scaleway: &config.ScalewayTarget{
			ProjectID: "11111111-1111-1111-1111-111111111111", Region: "fr-par", Zone: "fr-par-1", Zones: []string{"fr-par-1"},
			ImageDigest: "ghcr.io/magelift/magento@sha256:abcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcd", EncryptionKeySecret: "magento-key",
		}},
		Defaults: config.Defaults{Region: "fr-par", Preset: "standard"}, Class: "staging", Preset: "standard",
	}
	if _, err := PlanFromConfig(cfg, "staging"); err == nil || !strings.Contains(err.Error(), "requires at least 2 availability zones") {
		t.Fatalf("underprovisioned Scaleway standard preset was accepted: %v", err)
	}
}

func TestPlanFromConfigMapsAdvancedScalewaySettings(t *testing.T) {
	backupEnabled := true
	backupSameRegion := true
	encryptionAtRest := true
	backupFrequency := 12
	backupRetention := 14
	cfg := config.Config{
		SchemaVersion: 1,
		Project:       config.Project{Name: "shop"},
		Application:   config.Application{Edition: "open-source", Version: "2.4.8", Mode: "integrated", WebRuntime: "nginx-fpm"},
		Target: config.Target{Provider: "scaleway", Runtime: "kapsule", Scaleway: &config.ScalewayTarget{
			ProjectID: "11111111-1111-1111-1111-111111111111", Region: "fr-par", Zone: "fr-par-2",
			Zones: []string{"fr-par-1", "fr-par-2"}, NetworkCIDR: "10.90.0.0/16", ImageDigest: "ghcr.io/magelift/magento@sha256:abcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcd",
			DatabaseName: "commerce", MasterUsername: "dbadmin", EncryptionKeySecret: "magento-key", DatabaseNodeType: "DB-GP-M", DatabaseHighAvailability: true,
			DatabaseBackupEnabled: &backupEnabled, DatabaseBackupFrequency: &backupFrequency, DatabaseBackupRetention: &backupRetention, DatabaseBackupSameRegion: &backupSameRegion, DatabaseEncryptionAtRest: &encryptionAtRest,
			RedisNodeType: "RED1-S", RedisVersion: "8.6.3", RedisClusterSize: 2, CacheMode: "redis", KapsuleVersion: "1.36.1", NodeType: "GP1-M", NodeCount: 4,
			CPURequest: "1", MemoryRequest: "2Gi", DesiredWebReplicas: 5, QueueConsumerCount: 3,
		}},
		Defaults: config.Defaults{Region: "fr-par", Preset: "standard"}, Class: "staging", Preset: "standard",
	}
	spec, err := PlanFromConfig(cfg, "staging")
	if err != nil {
		t.Fatal(err)
	}
	if spec.Policy.NetworkCIDR != "10.90.0.0/16" || len(spec.Policy.Zones) != 2 || spec.Identity.Zone != "fr-par-2" {
		t.Fatalf("network settings were not mapped: %#v", spec)
	}
	if spec.Catalog.DatabaseNodeType != "DB-GP-M" || !spec.Catalog.DatabaseHighAvailability || spec.Catalog.RedisNodeType != "RED1-S" || spec.Catalog.RedisClusterSize != 2 || spec.Catalog.NodeType != "GP1-M" || spec.Catalog.NodeCount != 4 || spec.Catalog.DesiredWebReplicas != 5 || spec.Catalog.QueueConsumerCount != 3 {
		t.Fatalf("Scaleway catalog settings were not mapped: %#v", spec.Catalog)
	}
	if spec.Catalog.DatabaseBackupEnabled == nil || !*spec.Catalog.DatabaseBackupEnabled || spec.Catalog.DatabaseBackupFrequency != 12 || spec.Catalog.DatabaseBackupRetention != 14 || spec.Catalog.DatabaseBackupSameRegion == nil || !*spec.Catalog.DatabaseBackupSameRegion || spec.Catalog.DatabaseEncryptionAtRest == nil || !*spec.Catalog.DatabaseEncryptionAtRest {
		t.Fatalf("Scaleway database protection settings were not mapped: %#v", spec.Catalog)
	}
	if spec.Catalog.CPURequest != "1" || spec.Catalog.MemoryRequest != "2Gi" || spec.Dependencies.DatabaseName != "commerce" || spec.Dependencies.MasterUsername != "dbadmin" {
		t.Fatalf("Scaleway workload/dependency settings were not mapped: %#v", spec)
	}
}

func TestPlanFromYAMLMapsAdvancedScalewaySettings(t *testing.T) {
	input := `schemaVersion: 1
project: {name: shop}
application: {edition: open-source, version: 2.4.8, mode: integrated}
build: {php: "8.3"}
target:
  provider: scaleway
  runtime: kapsule
  scaleway:
    projectId: 11111111-1111-1111-1111-111111111111
    region: fr-par
    zone: fr-par-2
    zones: [fr-par-1, fr-par-2]
    networkCidr: 10.90.0.0/16
    imageDigest: ghcr.io/magelift/magento@sha256:abcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcd
    databaseName: commerce
    masterUsername: dbadmin
    encryptionKeySecret: magento-key
    databaseNodeType: DB-GP-M
    databaseHighAvailability: true
    databaseBackupEnabled: true
    databaseBackupFrequencyHours: 12
    databaseBackupRetentionDays: 14
    databaseBackupSameRegion: true
    databaseEncryptionAtRest: true
    redisNodeType: RED1-S
    redisVersion: 8.6.3
    redisClusterSize: 2
    cacheMode: redis
    kapsuleVersion: 1.36.1
    nodeType: GP1-M
    nodeCount: 4
    cpuRequest: "1"
    memoryRequest: 2Gi
    desiredWebReplicas: 5
    queueConsumerCount: 3
defaults: {region: fr-par, preset: standard}
environments: {staging: {class: staging}}
`
	file, err := config.Load([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	effective, err := file.Resolve("staging", config.ResolveOptions{})
	if err != nil {
		t.Fatal(err)
	}
	spec, err := PlanFromConfig(effective.Config, "staging")
	if err != nil {
		t.Fatal(err)
	}
	if spec.Policy.NetworkCIDR != "10.90.0.0/16" || len(spec.Policy.Zones) != 2 || spec.Identity.Zone != "fr-par-2" {
		t.Fatalf("YAML network settings were not mapped: %#v", spec)
	}
	if spec.Catalog.DatabaseNodeType != "DB-GP-M" || !spec.Catalog.DatabaseHighAvailability || spec.Catalog.RedisClusterSize != 2 || spec.Catalog.NodeType != "GP1-M" || spec.Catalog.NodeCount != 4 || spec.Catalog.DesiredWebReplicas != 5 || spec.Catalog.QueueConsumerCount != 3 {
		t.Fatalf("YAML catalog settings were not mapped: %#v", spec.Catalog)
	}
	if spec.Catalog.DatabaseBackupEnabled == nil || !*spec.Catalog.DatabaseBackupEnabled || spec.Catalog.DatabaseBackupFrequency != 12 || spec.Catalog.DatabaseBackupRetention != 14 || spec.Catalog.DatabaseEncryptionAtRest == nil || !*spec.Catalog.DatabaseEncryptionAtRest {
		t.Fatalf("YAML database protection settings were not mapped: %#v", spec.Catalog)
	}
	if effective.Provenance["target.scaleway.databaseBackupRetentionDays"].Source != "project" {
		t.Fatalf("advanced YAML provenance was not retained: %#v", effective.Provenance["target.scaleway.databaseBackupRetentionDays"])
	}
	if !strings.HasPrefix(effective.Fingerprint, "sha256:") {
		t.Fatalf("resolved YAML fingerprint = %q", effective.Fingerprint)
	}
	changed, err := config.Load([]byte(strings.Replace(input, "desiredWebReplicas: 5", "desiredWebReplicas: 6", 1)))
	if err != nil {
		t.Fatal(err)
	}
	changedEffective, err := changed.Resolve("staging", config.ResolveOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if effective.Fingerprint == changedEffective.Fingerprint {
		t.Fatal("advanced YAML change did not change the resolved fingerprint")
	}
}

func TestModuleRegistersRequiredOutputs(t *testing.T) {
	registry := platform.NewModuleRegistry()
	if err := registry.RegisterModule(Module{}); err != nil {
		t.Fatal(err)
	}
	module, found := registry.Module("scaleway", "kapsule")
	if !found || module.CertificationTier() != platform.TierExperimental {
		t.Fatal("expected experimental Scaleway module")
	}
}

func TestProgramBuildsMockGraph(t *testing.T) {
	backupEnabled := true
	backupSameRegion := true
	encryptionAtRest := true
	spec := Spec{
		Identity: Identity{
			Project: "shop", ScalewayProject: "11111111-1111-1111-1111-111111111111", Environment: "preview",
			Region: "fr-par", Zone: "fr-par-1", EnvironmentClass: "preview", Preset: "preview",
			Labels: map[string]string{"magelift-managed-by": "magelift"},
		},
		Application: Application{Edition: "open-source", Version: "2.4.8", Mode: "integrated", WebRuntime: "nginx-fpm"},
		Artifact:    Artifact{ImageDigest: "ghcr.io/magelift/magento@sha256:abcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcd"},
		Policy:      NetworkPolicy{NetworkCIDR: "172.16.0.0/22", Zones: []string{"fr-par-1", "fr-par-2"}},
		Catalog: CatalogSelection{
			DatabaseNodeType: "DB-DEV-S", DatabaseHighAvailability: true, DatabaseBackupEnabled: &backupEnabled, DatabaseBackupFrequency: 12, DatabaseBackupRetention: 14, DatabaseBackupSameRegion: &backupSameRegion, DatabaseEncryptionAtRest: &encryptionAtRest,
			RedisNodeType: "RED1-MICRO", RedisClusterSize: 2, CacheMode: "redis",
			KapsuleVersion: "1.36.1", NodeType: "DEV1-M", NodeCount: 2,
			CPURequest: "500m", MemoryRequest: "1Gi", DesiredWebReplicas: 2,
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

	wantTokens := []string{
		"scaleway:network/vpc:Vpc",
		"scaleway:network/privateNetwork:PrivateNetwork",
		"scaleway:databases/instance:Instance",
		"scaleway:redis/cluster:Cluster",
		"scaleway:kubernetes/cluster:Cluster",
		"scaleway:kubernetes/pool:Pool",
		"kubernetes:apps/v1:Deployment",
		"kubernetes:core/v1:Service",
		"random:index/randomPassword:RandomPassword",
	}
	seen := make(map[string]bool, len(mocks.resources))
	poolCount := 0
	poolZones := make(map[string]bool)
	webSpread := false
	for _, registered := range mocks.resources {
		seen[registered.TypeToken] = true
		if strings.HasPrefix(registered.TypeToken, "aws:") || strings.HasPrefix(registered.TypeToken, "gcp:") {
			t.Fatalf("Scaleway program registered a non-Scaleway cloud resource: %s", registered.TypeToken)
		}
		if registered.TypeToken == "scaleway:databases/instance:Instance" {
			if !registered.Inputs["isHaCluster"].BoolValue() {
				t.Fatal("Scaleway advanced RDB graph must propagate high availability")
			}
			if registered.Inputs["disableBackup"].BoolValue() || registered.Inputs["backupScheduleFrequency"].NumberValue() != 12 || registered.Inputs["backupScheduleRetention"].NumberValue() != 14 || !registered.Inputs["backupSameRegion"].BoolValue() || !registered.Inputs["encryptionAtRest"].BoolValue() {
				t.Fatalf("Scaleway database protection settings were not propagated: %#v", registered.Inputs)
			}
			privateNetwork := registered.Inputs[resource.PropertyKey("privateNetwork")].ObjectValue()
			if !privateNetwork[resource.PropertyKey("enableIpam")].BoolValue() {
				t.Fatal("Scaleway database private network must enable IPAM")
			}
		}
		if registered.TypeToken == "scaleway:redis/cluster:Cluster" && registered.Inputs["clusterSize"].NumberValue() != 2 {
			t.Fatalf("Scaleway Redis HA cluster size was not propagated: %v", registered.Inputs)
		}
		if registered.TypeToken == "scaleway:kubernetes/pool:Pool" {
			poolCount++
			poolZones[registered.Inputs["zone"].StringValue()] = true
			if registered.Inputs["size"].NumberValue() != 1 {
				t.Fatalf("multi-zone Kapsule pool must receive one node in this fixture: %v", registered.Inputs)
			}
		}
		if registered.TypeToken == "kubernetes:apps/v1:Deployment" && strings.HasSuffix(registered.Name, "-web") {
			podSpec := registered.Inputs["spec"].ObjectValue()[resource.PropertyKey("template")].ObjectValue()[resource.PropertyKey("spec")].ObjectValue()
			spreads := podSpec[resource.PropertyKey("topologySpreadConstraints")].ArrayValue()
			webSpread = len(spreads) == 1 && spreads[0].ObjectValue()[resource.PropertyKey("topologyKey")].StringValue() == "topology.kubernetes.io/zone" && spreads[0].ObjectValue()[resource.PropertyKey("whenUnsatisfiable")].StringValue() == "DoNotSchedule"
		}
		if registered.TypeToken == "random:index/randomPassword:RandomPassword" && !registered.Inputs[resource.PropertyKey("special")].BoolValue() {
			t.Fatal("Scaleway managed-service passwords must include a special character")
		}
	}
	for _, token := range wantTokens {
		if !seen[token] {
			t.Fatalf("expected resource token %q in graph, got %v", token, mocks.tokens())
		}
	}
	if poolCount != 2 || len(poolZones) != 2 || !poolZones["fr-par-1"] || !poolZones["fr-par-2"] {
		t.Fatalf("configured zones were not translated to one Kapsule pool per zone: count=%d zones=%v", poolCount, poolZones)
	}
	if !webSpread {
		t.Fatal("multi-zone web Deployment must use strict zone spreading")
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
	case "scaleway:network/vpc:Vpc":
		state["name"] = resource.NewStringProperty(args.Name)
	case "scaleway:network/privateNetwork:PrivateNetwork":
		state["name"] = resource.NewStringProperty(args.Name)
	case "scaleway:databases/instance:Instance":
		state["name"] = resource.NewStringProperty(args.Name)
		state["endpointIp"] = resource.NewStringProperty("172.16.0.5")
		state["loadBalancer"] = resource.NewObjectProperty(resource.PropertyMap{
			"ip": resource.NewStringProperty("172.16.0.6"),
		})
	case "scaleway:redis/cluster:Cluster":
		state["name"] = resource.NewStringProperty(args.Name)
		state["connectionString"] = resource.NewStringProperty("redis://magelift:secret@172.16.0.7:6379/0")
	case "scaleway:kubernetes/cluster:Cluster":
		state["name"] = resource.NewStringProperty(args.Name)
		state["version"] = resource.NewStringProperty("1.36.1")
		state["apiserverUrl"] = resource.NewStringProperty("https://cluster.pub.k8s.fr-par.scw.cloud:6443")
		state["kubeconfigs"] = resource.NewArrayProperty([]resource.PropertyValue{
			resource.NewObjectProperty(resource.PropertyMap{
				"host":                 resource.NewStringProperty("https://cluster.pub.k8s.fr-par.scw.cloud:6443"),
				"token":                resource.NewStringProperty("mock-scw-token"),
				"clusterCaCertificate": resource.NewStringProperty("Y2E="),
			}),
		})
	case "scaleway:kubernetes/pool:Pool":
		state["name"] = resource.NewStringProperty(args.Name)
	case "kubernetes:core/v1:Service":
		state["status"] = resource.NewObjectProperty(resource.PropertyMap{
			"loadBalancer": resource.NewObjectProperty(resource.PropertyMap{
				"ingress": resource.NewArrayProperty([]resource.PropertyValue{
					resource.NewObjectProperty(resource.PropertyMap{"ip": resource.NewStringProperty("51.1.2.3")}),
				}),
			}),
		})
		state["metadata"] = resource.NewObjectProperty(resource.PropertyMap{"name": resource.NewStringProperty(args.Name)})
	case "random:index/randomPassword:RandomPassword":
		state["result"] = resource.NewStringProperty("generated-password-value-32chars!!")
	}
	return args.Name + "-id", state, nil
}

func (m *stackMocks) Call(args pulumi.MockCallArgs) (resource.PropertyMap, error) {
	return resource.PropertyMap{}, nil
}

func (m *stackMocks) tokens() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	tokens := make([]string, len(m.resources))
	for index, resource := range m.resources {
		tokens[index] = resource.TypeToken
	}
	return tokens
}
