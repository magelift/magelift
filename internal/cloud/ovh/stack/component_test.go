package stack

import (
	"strings"
	"sync"
	"testing"

	"github.com/magelift/magelift/internal/config"
	"github.com/magelift/magelift/internal/platform"
	sdk "github.com/magelift/magelift/sdk/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

func TestPlanFromConfigRejectsAWSTarget(t *testing.T) {
	_, err := PlanFromConfig(config.Config{
		Target: config.Target{Provider: "aws", Runtime: "ecs-fargate"},
	}, "staging")
	if err == nil || !strings.Contains(err.Error(), "OVH stack requires") {
		t.Fatalf("expected OVH target rejection, got %v", err)
	}
}

func TestSpecRejectsImpossibleMKSWorkerTopology(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name       string
		plan       string
		zones      []string
		nodeCount  int
		wantErrMsg string
	}{
		{name: "free multi-zone", plan: "free", zones: []string{"eu-west-par-a", "eu-west-par-b"}, nodeCount: 2, wantErrMsg: "free plan supports one availability zone"},
		{name: "under-sized multi-zone", plan: "standard", zones: []string{"eu-west-par-a", "eu-west-par-b", "eu-west-par-c"}, nodeCount: 2, wantErrMsg: "node count 2 is too small"},
		{name: "empty zone", plan: "standard", zones: []string{""}, nodeCount: 1, wantErrMsg: "availability zone 0 is empty"},
		{name: "duplicate zone", plan: "standard", zones: []string{"eu-west-par-a", "EU-WEST-PAR-A"}, nodeCount: 2, wantErrMsg: "availability zone \"EU-WEST-PAR-A\" is duplicated"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			spec := Spec{
				Identity: Identity{Project: "shop", ServiceName: "xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx", Environment: "staging", Region: "EU-WEST-PAR", EnvironmentClass: "staging", Preset: sdk.PresetStandard},
				Artifact: Artifact{ImageDigest: "ghcr.io/magelift/magento@sha256:abcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcd"},
				Policy:   NetworkPolicy{NetworkCIDR: "10.30.0.0/16", Zones: testCase.zones},
				Catalog:  CatalogSelection{MKSPlan: testCase.plan, NodeCount: testCase.nodeCount, DesiredWebReplicas: 1},
				Dependencies: Dependencies{
					DatabaseName: "magento", MasterUsername: "magento", EncryptionKeySecret: "magento-crypt-key",
				},
			}
			err := spec.Validate()
			if err == nil || !strings.Contains(err.Error(), testCase.wantErrMsg) {
				t.Fatalf("Validate() error = %v, want substring %q", err, testCase.wantErrMsg)
			}
		})
	}
}

func TestSpecRejectsUnsupportedOVHManagedServiceShapes(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name       string
		mutate     func(*CatalogSelection)
		wantErrMsg string
	}{
		{name: "MySQL business node limit", mutate: func(catalog *CatalogSelection) {
			catalog.DatabasePlan = "business"
			catalog.DatabaseNodeCount = 3
		}, wantErrMsg: "MySQL plan \"business\" supports at most 2"},
		{name: "Valkey business node limit", mutate: func(catalog *CatalogSelection) {
			catalog.ValkeyPlan = "business"
			catalog.ValkeyNodeCount = 3
		}, wantErrMsg: "Valkey plan \"business\" supports at most 2"},
		{name: "MySQL production minimum", mutate: func(catalog *CatalogSelection) {
			catalog.DatabasePlan = "production"
			catalog.DatabaseNodeCount = 1
		}, wantErrMsg: "MySQL plan \"production\" requires at least 2"},
		{name: "Valkey discovery is one node", mutate: func(catalog *CatalogSelection) {
			catalog.ValkeyPlan = "discovery"
			catalog.ValkeyNodeCount = 2
		}, wantErrMsg: "Valkey plan \"discovery\" supports at most 1"},
		{name: "unsupported MySQL version", mutate: func(catalog *CatalogSelection) {
			catalog.DatabaseVersion = "9.0"
		}, wantErrMsg: "unsupported OVH MySQL version \"9.0\""},
		{name: "unsupported Valkey version", mutate: func(catalog *CatalogSelection) {
			catalog.ValkeyVersion = "10.0"
		}, wantErrMsg: "unsupported OVH Valkey version \"10.0\""},
		{name: "unsupported Valkey enterprise plan", mutate: func(catalog *CatalogSelection) {
			catalog.ValkeyPlan = "enterprise"
		}, wantErrMsg: "unsupported OVH Valkey plan \"enterprise\""},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			spec := Spec{
				Identity: Identity{Project: "shop", ServiceName: "xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx", Environment: "staging", Region: "EU-WEST-PAR", EnvironmentClass: "staging", Preset: sdk.PresetStandard},
				Artifact: Artifact{ImageDigest: "ghcr.io/magelift/magento@sha256:abcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcd"},
				Policy:   NetworkPolicy{NetworkCIDR: "10.30.0.0/16", Zones: []string{"eu-west-par-a"}},
				Catalog:  CatalogSelection{MKSPlan: "standard", NodeCount: 1, DesiredWebReplicas: 1},
				Dependencies: Dependencies{
					DatabaseName: "magento", MasterUsername: "magento", EncryptionKeySecret: "magento-crypt-key",
				},
			}
			testCase.mutate(&spec.Catalog)
			err := spec.Validate()
			if err == nil || !strings.Contains(err.Error(), testCase.wantErrMsg) {
				t.Fatalf("Validate() error = %v, want substring %q", err, testCase.wantErrMsg)
			}
		})
	}
}

func TestSpecAcceptsCurrentOVHValkeyVersions(t *testing.T) {
	for _, version := range []string{"7.2", "8.0", "8.1", "9.0", "9.1"} {
		t.Run(version, func(t *testing.T) {
			spec := Spec{
				Identity: Identity{Project: "shop", ServiceName: "xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx", Region: "EU-WEST-PAR", Environment: "staging", EnvironmentClass: "staging", Preset: sdk.PresetStandard},
				Artifact: Artifact{ImageDigest: "ghcr.io/magelift/magento@sha256:abcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcd"},
				Policy:   NetworkPolicy{NetworkCIDR: "10.30.0.0/16", Zones: []string{"eu-west-par-a"}},
				Catalog:  CatalogSelection{ValkeyVersion: version, MKSPlan: "standard", NodeCount: 1, DesiredWebReplicas: 1},
				Dependencies: Dependencies{
					DatabaseName: "magento", MasterUsername: "magento", EncryptionKeySecret: "magento-crypt-key",
				},
			}
			if err := spec.Validate(); err != nil {
				t.Fatalf("Validate() rejected Valkey %s: %v", version, err)
			}
		})
	}
}

func TestPlanFromConfigMapsAdvancedOVHSettings(t *testing.T) {
	databaseDeletionProtection := true
	valkeyDeletionProtection := true
	cfg := config.Config{
		SchemaVersion: 1,
		Project:       config.Project{Name: "shop"},
		Application:   config.Application{Edition: "open-source", Version: "2.4.8", Mode: "integrated", WebRuntime: "nginx-fpm"},
		Target: config.Target{Provider: "ovh", Runtime: "mks", OVH: &config.OVHTarget{
			ServiceName: "xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx", Region: "EU-WEST-PAR", Zones: []string{"eu-west-par-a", "eu-west-par-b", "eu-west-par-c"}, NetworkCIDR: "10.91.0.0/16",
			ImageDigest: "ghcr.io/magelift/magento@sha256:abcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcd", DatabaseName: "commerce", MasterUsername: "dbadmin", EncryptionKeySecret: "magento-key",
			DatabaseFlavor: "db1-7", DatabasePlan: "business", DatabaseVersion: "8.4", DatabaseNodeCount: 2, DatabaseBackupTime: "03:30", DatabaseBackupRegions: []string{"GRA9", "GRA11"}, DatabaseDeletionProtection: &databaseDeletionProtection,
			ValkeyFlavor: "db1-7", ValkeyPlan: "business", ValkeyVersion: "8.1", ValkeyNodeCount: 2, ValkeyBackupTime: "04:00", ValkeyBackupRegions: []string{"GRA9"}, ValkeyDeletionProtection: &valkeyDeletionProtection,
			MKSPlan: "standard", NodeFlavor: "b3-16", NodeCount: 3, CPURequest: "1", MemoryRequest: "2Gi", DesiredWebReplicas: 4, QueueConsumerCount: 3,
			AttachFloatingIPs: true, PrivateNetworkRoutingAsDefault: true,
		}},
		Defaults: config.Defaults{Region: "EU-WEST-PAR", Preset: "standard"}, Class: "staging", Preset: "standard",
	}
	spec, err := PlanFromConfig(cfg, "staging")
	if err != nil {
		t.Fatal(err)
	}
	if spec.Policy.NetworkCIDR != "10.91.0.0/16" || len(spec.Policy.Zones) != 3 || spec.Catalog.MKSPlan != "standard" || !spec.Catalog.AttachFloatingIPs || !spec.Catalog.PrivateNetworkRoutingAsDefault {
		t.Fatalf("OVH network and plan settings were not mapped: %#v", spec)
	}
	if spec.Catalog.DatabaseFlavor != "db1-7" || spec.Catalog.DatabasePlan != "business" || spec.Catalog.DatabaseVersion != "8.4" || spec.Catalog.DatabaseNodeCount != 2 || spec.Catalog.ValkeyFlavor != "db1-7" || spec.Catalog.ValkeyPlan != "business" || spec.Catalog.ValkeyVersion != "8.1" || spec.Catalog.ValkeyNodeCount != 2 || spec.Catalog.NodeFlavor != "b3-16" || spec.Catalog.NodeCount != 3 || spec.Catalog.DesiredWebReplicas != 4 || spec.Catalog.QueueConsumerCount != 3 {
		t.Fatalf("OVH catalog settings were not mapped: %#v", spec.Catalog)
	}
	if spec.Catalog.DatabaseBackupTime != "03:30" || len(spec.Catalog.DatabaseBackupRegions) != 2 || spec.Catalog.DatabaseDeletionProtection == nil || !*spec.Catalog.DatabaseDeletionProtection || spec.Catalog.ValkeyBackupTime != "04:00" || len(spec.Catalog.ValkeyBackupRegions) != 1 || spec.Catalog.ValkeyDeletionProtection == nil || !*spec.Catalog.ValkeyDeletionProtection {
		t.Fatalf("OVH database protection settings were not mapped: %#v", spec.Catalog)
	}
	if spec.Catalog.CPURequest != "1" || spec.Catalog.MemoryRequest != "2Gi" || spec.Dependencies.DatabaseName != "commerce" || spec.Dependencies.MasterUsername != "dbadmin" {
		t.Fatalf("OVH workload/dependency settings were not mapped: %#v", spec)
	}
}

func TestPlanFromYAMLMapsAdvancedOVHSettings(t *testing.T) {
	input := `schemaVersion: 1
project: {name: shop}
application: {edition: open-source, version: 2.4.8, mode: integrated}
build: {php: "8.3"}
target:
  provider: ovh
  runtime: mks
  ovh:
    serviceName: xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
    region: EU-WEST-PAR
    zones: [eu-west-par-a, eu-west-par-b, eu-west-par-c]
    networkCidr: 10.91.0.0/16
    imageDigest: ghcr.io/magelift/magento@sha256:abcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcd
    databaseName: commerce
    masterUsername: dbadmin
    encryptionKeySecret: magento-key
    databaseFlavor: db1-7
    databasePlan: business
    databaseVersion: 8.4
    databaseNodeCount: 2
    databaseBackupTime: "03:30"
    databaseBackupRegions: [GRA9, GRA11]
    databaseDeletionProtection: true
    valkeyFlavor: db1-7
    valkeyPlan: business
    valkeyVersion: 8.1
    valkeyNodeCount: 2
    valkeyBackupTime: "04:00"
    valkeyBackupRegions: [GRA9]
    valkeyDeletionProtection: true
    mksPlan: standard
    nodeFlavor: b3-16
    nodeCount: 3
    cpuRequest: "1"
    memoryRequest: 2Gi
    desiredWebReplicas: 4
    queueConsumerCount: 3
    attachFloatingIps: true
    privateNetworkRoutingAsDefault: true
defaults: {region: EU-WEST-PAR, preset: standard}
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
	if spec.Policy.NetworkCIDR != "10.91.0.0/16" || len(spec.Policy.Zones) != 3 || spec.Catalog.MKSPlan != "standard" || !spec.Catalog.AttachFloatingIPs || !spec.Catalog.PrivateNetworkRoutingAsDefault {
		t.Fatalf("YAML network and plan settings were not mapped: %#v", spec)
	}
	if spec.Catalog.DatabaseFlavor != "db1-7" || spec.Catalog.DatabasePlan != "business" || spec.Catalog.DatabaseNodeCount != 2 || spec.Catalog.ValkeyFlavor != "db1-7" || spec.Catalog.ValkeyPlan != "business" || spec.Catalog.ValkeyNodeCount != 2 || spec.Catalog.NodeFlavor != "b3-16" || spec.Catalog.NodeCount != 3 || spec.Catalog.DesiredWebReplicas != 4 || spec.Catalog.QueueConsumerCount != 3 {
		t.Fatalf("YAML catalog settings were not mapped: %#v", spec.Catalog)
	}
	if spec.Catalog.DatabaseBackupTime != "03:30" || len(spec.Catalog.DatabaseBackupRegions) != 2 || spec.Catalog.DatabaseDeletionProtection == nil || !*spec.Catalog.DatabaseDeletionProtection || spec.Catalog.ValkeyBackupTime != "04:00" || len(spec.Catalog.ValkeyBackupRegions) != 1 || spec.Catalog.ValkeyDeletionProtection == nil || !*spec.Catalog.ValkeyDeletionProtection {
		t.Fatalf("YAML database protection settings were not mapped: %#v", spec.Catalog)
	}
	if effective.Provenance["target.ovh.databaseBackupRegions"].Source != "project" {
		t.Fatalf("advanced YAML provenance was not retained: %#v", effective.Provenance["target.ovh.databaseBackupRegions"])
	}
	if !strings.HasPrefix(effective.Fingerprint, "sha256:") {
		t.Fatalf("resolved YAML fingerprint = %q", effective.Fingerprint)
	}
	changed, err := config.Load([]byte(strings.Replace(input, "desiredWebReplicas: 4", "desiredWebReplicas: 5", 1)))
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
	module, found := registry.Module("ovh", "mks")
	if !found || module.CertificationTier() != platform.TierExperimental {
		t.Fatal("expected experimental OVH module")
	}
}

func TestProgramBuildsMockGraph(t *testing.T) {
	databaseDeletionProtection := true
	valkeyDeletionProtection := true
	spec := Spec{
		Identity: Identity{
			Project: "shop", ServiceName: "xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx", Environment: "preview",
			Region: "GRA9", EnvironmentClass: "preview", Preset: "preview",
			Labels: map[string]string{"magelift-managed-by": "magelift"},
		},
		Application:  Application{Edition: "open-source", Version: "2.4.8", Mode: "integrated", WebRuntime: "nginx-fpm"},
		Artifact:     Artifact{ImageDigest: "ghcr.io/magelift/magento@sha256:abcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcd"},
		Policy:       NetworkPolicy{NetworkCIDR: "10.30.0.0/16", Zones: []string{"GRA9"}},
		Catalog:      CatalogSelection{DatabaseFlavor: "db1-4", DatabasePlan: "business", DatabaseVersion: "8.4", DatabaseNodeCount: 2, DatabaseBackupTime: "03:30", DatabaseBackupRegions: []string{"GRA9"}, DatabaseDeletionProtection: &databaseDeletionProtection, ValkeyFlavor: "db1-4", ValkeyPlan: "business", ValkeyVersion: "8.1", ValkeyNodeCount: 2, ValkeyBackupTime: "04:00", ValkeyBackupRegions: []string{"GRA9"}, ValkeyDeletionProtection: &valkeyDeletionProtection, MKSPlan: "standard", NodeFlavor: "b3-8", NodeCount: 1, CPURequest: "500m", MemoryRequest: "1Gi", DesiredWebReplicas: 1},
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
		"ovh:CloudProject/networkPrivate:NetworkPrivate",
		"ovh:CloudProject/networkPrivateSubnetV2:NetworkPrivateSubnetV2",
		"ovh:CloudProject/database:Database",
		"ovh:CloudProject/kube:Kube",
		"ovh:CloudProject/kubeNodePool:KubeNodePool",
		"kubernetes:apps/v1:Deployment",
		"kubernetes:core/v1:Service",
		"random:index/randomPassword:RandomPassword",
	}
	seen := map[string]bool{}
	for _, r := range mocks.resources {
		seen[r.TypeToken] = true
		if strings.HasPrefix(r.TypeToken, "aws:") || strings.HasPrefix(r.TypeToken, "gcp:") {
			t.Fatalf("unexpected foreign provider token %s", r.TypeToken)
		}
	}
	for _, token := range wantTokens {
		if !seen[token] {
			t.Fatalf("missing expected token %s in %#v", token, seen)
		}
	}
	if mocks.databaseCount < 2 {
		t.Fatalf("expected MySQL + Valkey database resources, got %d", mocks.databaseCount)
	}
	for _, r := range mocks.resources {
		if r.TypeToken == "ovh:CloudProject/networkPrivateSubnetV2:NetworkPrivateSubnetV2" && r.Inputs["networkId"].StringValue() != "region-network-uuid" {
			t.Fatalf("OVH subnet must use regional OpenStack network UUID, got %q", r.Inputs["networkId"].StringValue())
		}
		if r.TypeToken == "ovh:CloudProject/database:Database" && r.Name == "shop-preview-sql" && r.Inputs["version"].StringValue() != "8.4" {
			t.Fatalf("OVH MySQL version must be configurable, got %q", r.Inputs["version"].StringValue())
		}
		if r.TypeToken == "ovh:CloudProject/database:Database" && r.Name == "shop-preview-valkey" && r.Inputs["version"].StringValue() != "8.1" {
			t.Fatalf("OVH Valkey version must be configurable, got %q", r.Inputs["version"].StringValue())
		}
		if r.TypeToken == "ovh:CloudProject/database:Database" && (r.Name == "shop-preview-sql" || r.Name == "shop-preview-valkey") && len(r.Inputs["nodes"].ArrayValue()) != 2 {
			t.Fatalf("OVH managed database node count was not propagated for %s: %v", r.Name, r.Inputs["nodes"])
		}
		if r.TypeToken == "ovh:CloudProject/database:Database" && r.Name == "shop-preview-sql" && (r.Inputs["backupTime"].StringValue() != "03:30" || len(r.Inputs["backupRegions"].ArrayValue()) != 1 || !r.Inputs["deletionProtection"].BoolValue()) {
			t.Fatalf("OVH MySQL protection settings were not propagated: %#v", r.Inputs)
		}
		if r.TypeToken == "ovh:CloudProject/database:Database" && r.Name == "shop-preview-valkey" && (r.Inputs["backupTime"].StringValue() != "04:00" || len(r.Inputs["backupRegions"].ArrayValue()) != 1 || !r.Inputs["deletionProtection"].BoolValue()) {
			t.Fatalf("OVH Valkey protection settings were not propagated: %#v", r.Inputs)
		}
		if r.TypeToken == "ovh:CloudProject/kube:Kube" && r.Inputs["plan"].StringValue() != "standard" {
			t.Fatalf("OVH MKS plan must be configurable, got %q", r.Inputs["plan"].StringValue())
		}
	}
}

type stackMocks struct {
	mu            sync.Mutex
	resources     []pulumi.MockResourceArgs
	databaseCount int
}

func (m *stackMocks) NewResource(args pulumi.MockResourceArgs) (string, resource.PropertyMap, error) {
	m.mu.Lock()
	m.resources = append(m.resources, args)
	if args.TypeToken == "ovh:CloudProject/database:Database" {
		m.databaseCount++
	}
	m.mu.Unlock()
	state := args.Inputs.Copy()
	switch args.TypeToken {
	case "ovh:CloudProject/networkPrivate:NetworkPrivate":
		state["name"] = resource.NewStringProperty(args.Name)
		state["status"] = resource.NewStringProperty("ACTIVE")
		state["regionsOpenstackIds"] = resource.NewObjectProperty(resource.PropertyMap{
			"GRA9": resource.NewStringProperty("region-network-uuid"),
		})
	case "ovh:CloudProject/networkPrivateSubnetV2:NetworkPrivateSubnetV2":
		state["name"] = resource.NewStringProperty(args.Name)
		state["cidr"] = resource.NewStringProperty("10.30.0.0/24")
		state["gatewayIp"] = resource.NewStringProperty("10.30.0.1")
	case "ovh:CloudProject/database:Database":
		state["endpoints"] = resource.NewArrayProperty([]resource.PropertyValue{
			resource.NewObjectProperty(resource.PropertyMap{
				"domain": resource.NewStringProperty("db.example.ovh.net"),
				"port":   resource.NewNumberProperty(3306),
			}),
		})
		state["status"] = resource.NewStringProperty("READY")
	case "ovh:CloudProject/kube:Kube":
		state["name"] = resource.NewStringProperty(args.Name)
		state["kubeconfig"] = resource.NewStringProperty("apiVersion: v1\nkind: Config\n")
		state["url"] = resource.NewStringProperty("https://k8s.example.ovh.net")
		state["status"] = resource.NewStringProperty("READY")
		state["nodesSubnetId"] = resource.NewStringProperty("subnet-id")
	case "ovh:CloudProject/kubeNodePool:KubeNodePool":
		state["name"] = resource.NewStringProperty(args.Name)
		state["status"] = resource.NewStringProperty("READY")
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
