package stack

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"sync"
	"testing"

	"github.com/magelift/magelift/internal/platform"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

func TestHeadlessOriginReachesAKSFPM(t *testing.T) {
	spec := testSpec()
	spec.ApplicationImage = "registry.example.invalid/shop@sha256:" + strings.Repeat("a", 64)
	spec.MagentoVersion = "2.4.9"
	spec.ApplicationMode = "headless"
	spec.CacheImage = "example.invalid/valkey:9"
	spec.SearchImage = "example.invalid/opensearch:3"
	spec.Magento = platform.MagentoOverlays{StorefrontOrigin: "https://shop.example"}
	mocks := &resourceMocks{resources: make(map[string]pulumi.MockResourceArgs)}
	if err := pulumi.RunErr(Program(spec), pulumi.WithMocks("magelift", "shop-test", mocks)); err != nil {
		t.Fatal(err)
	}
	web, ok := mocks.resources["kubernetes:apps/v1:Deployment/magento-web"]
	if !ok {
		t.Fatal("web workload missing")
	}
	payload, err := json.Marshal(web.Inputs.Mappable())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(payload), "CONFIG__DEFAULT__WEB__GRAPHQL__CORS_ALLOWED_ORIGINS") || !strings.Contains(string(payload), "https://shop.example") {
		t.Fatalf("web workload lacks CORS environment: %s", payload)
	}
}

type resourceMocks struct {
	mu        sync.Mutex
	resources map[string]pulumi.MockResourceArgs
}

func (m *resourceMocks) NewResource(args pulumi.MockResourceArgs) (string, resource.PropertyMap, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.resources[args.TypeToken] = args
	m.resources[args.TypeToken+"/"+args.Name] = args
	outputs := args.Inputs.Copy()
	outputs["name"] = resource.NewStringProperty(args.Name)
	if args.TypeToken == "azure-native:dbformysql:Server" {
		outputs["fullyQualifiedDomainName"] = resource.NewStringProperty("mysql.example.invalid")
	}
	if args.TypeToken == "azure-native:containerservice:ManagedCluster" {
		outputs["identityProfile"] = resource.NewObjectProperty(resource.PropertyMap{
			"kubeletidentity": resource.NewObjectProperty(resource.PropertyMap{"objectId": resource.NewStringProperty("example-kubelet-principal")}),
		})
	}
	if args.TypeToken == "azure-native:keyvault:Vault" {
		outputs["properties"] = resource.NewObjectProperty(resource.PropertyMap{"vaultUri": resource.NewStringProperty("https://vault.example.invalid")})
	}
	if args.TypeToken == "random:index/randomPassword:RandomPassword" {
		outputs["result"] = resource.MakeSecret(resource.NewStringProperty("generated-password"))
	}
	return args.Name + "-id", outputs, nil
}

func (m *resourceMocks) Call(args pulumi.MockCallArgs) (resource.PropertyMap, error) {
	switch args.Token {
	case "azure-native:containerservice:listManagedClusterUserCredentials":
		return resource.PropertyMap{"kubeconfigs": resource.NewArrayProperty([]resource.PropertyValue{
			resource.NewObjectProperty(resource.PropertyMap{"name": resource.NewStringProperty("user"), "value": resource.NewStringProperty(base64.StdEncoding.EncodeToString([]byte("apiVersion: v1\nkind: Config\n")))}),
		})}, nil
	case "azure-native:storage:listStorageAccountKeys":
		return resource.PropertyMap{"keys": resource.NewArrayProperty([]resource.PropertyValue{
			resource.NewObjectProperty(resource.PropertyMap{"value": resource.NewStringProperty("test-storage-key")}),
		})}, nil
	}
	return args.Args, nil
}

func testSpec() Spec {
	return Spec{
		SubscriptionID: "subscription", TenantID: "tenant", ResourceGroup: "shop-rg", Region: "francecentral",
		Project: "shop", Environment: "test", ClusterName: "shop-aks", NodeSubnetID: "/subscriptions/sub/resourceGroups/rg/providers/Microsoft.Network/virtualNetworks/vnet/subnets/nodes",
		MySQLServerName: "shop-mysql", MySQLSubnetID: "/subscriptions/sub/db-subnet", MySQLDNSZoneID: "/subscriptions/sub/private-dns",
		MySQLVersion: "8.0.21", MySQLUsername: "magento", MySQLPassword: pulumi.String("secret-password"),
		StorageAccount: "shopstorage", VaultName: "shop-vault",
	}
}

func TestProgramRegistersPrivateDatabaseAndSharedMedia(t *testing.T) {
	for _, highAvailability := range []bool{false, true} {
		t.Run(map[bool]string{false: "preview", true: "high-availability"}[highAvailability], func(t *testing.T) {
			spec := testSpec()
			spec.HighAvailability = highAvailability
			mocks := &resourceMocks{resources: make(map[string]pulumi.MockResourceArgs)}
			if err := pulumi.RunErr(Program(spec), pulumi.WithMocks("magelift", "shop-test", mocks)); err != nil {
				t.Fatal(err)
			}
			for _, token := range []string{"azure-native:containerservice:ManagedCluster", "azure-native:dbformysql:Server", "azure-native:dbformysql:Database", "azure-native:storage:StorageAccount", "azure-native:storage:FileShare", "azure-native:keyvault:Vault"} {
				if _, ok := mocks.resources[token]; !ok {
					t.Errorf("missing resource %s", token)
				}
			}
			server := mocks.resources["azure-native:dbformysql:Server"].Inputs
			password := server["administratorLoginPassword"]
			if !password.IsSecret() {
				t.Error("database password must be a Pulumi secret")
			}
			network := server["network"].ObjectValue()
			if network["publicNetworkAccess"].StringValue() != "Disabled" {
				t.Error("database public access must be disabled")
			}
			mode := server["highAvailability"].ObjectValue()["mode"].StringValue()
			if highAvailability && mode != "ZoneRedundant" {
				t.Fatalf("high availability mode = %q", mode)
			}
			if !highAvailability && mode != "Disabled" {
				t.Fatalf("preview high availability mode = %q", mode)
			}
		})
	}
}

func TestSpecRequiresPrivateDatabaseInputs(t *testing.T) {
	spec := testSpec()
	spec.MySQLSubnetID = ""
	if err := spec.Validate(); err == nil || !strings.Contains(err.Error(), "MySQL subnet") {
		t.Fatalf("missing subnet error = %v", err)
	}
}

func TestProgramCreatesPrivateNetworkWhenOmitted(t *testing.T) {
	spec := testSpec()
	spec.NodeSubnetID = ""
	spec.MySQLSubnetID = ""
	spec.MySQLDNSZoneID = ""
	spec.MySQLPassword = nil
	mocks := &resourceMocks{resources: make(map[string]pulumi.MockResourceArgs)}
	if err := pulumi.RunErr(Program(spec), pulumi.WithMocks("magelift", "shop-test", mocks)); err != nil {
		t.Fatal(err)
	}
	for _, token := range []string{"azure-native:network:VirtualNetwork/network", "azure-native:network:Subnet/node-subnet", "azure-native:network:Subnet/database-subnet", "azure-native:privatedns:PrivateZone/database-dns", "azure-native:privatedns:VirtualNetworkLink/database-dns-link", "random:index/randomPassword:RandomPassword/mysql-password", "azure-native:keyvault:Secret/mysql-password-secret"} {
		if _, ok := mocks.resources[token]; !ok {
			t.Errorf("missing resource %s", token)
		}
	}
	subnet := mocks.resources["azure-native:network:Subnet/database-subnet"].Inputs
	delegations := subnet["delegations"].ArrayValue()
	if len(delegations) != 1 || delegations[0].ObjectValue()["serviceName"].StringValue() != "Microsoft.DBforMySQL/flexibleServers" {
		t.Fatalf("unexpected database subnet delegation: %v", delegations)
	}
}

func TestProgramIncludesMagentoWorkloads(t *testing.T) {
	spec := testSpec()
	spec.ApplicationImage = "registry.example.invalid/shop@sha256:" + strings.Repeat("a", 64)
	spec.MagentoVersion = "2.4.9"
	spec.ApplicationMode = "integrated"
	spec.CacheImage = "example.invalid/valkey:9"
	spec.SearchImage = "example.invalid/opensearch:3"
	spec.ImagePullSecret = "registry-credentials"
	mocks := &resourceMocks{resources: make(map[string]pulumi.MockResourceArgs)}
	if err := pulumi.RunErr(Program(spec), pulumi.WithMocks("magelift", "shop-test", mocks)); err != nil {
		t.Fatal(err)
	}
	for _, token := range []string{"kubernetes:apps/v1:Deployment/magento-web", "kubernetes:apps/v1:Deployment/magento-cron", "kubernetes:core/v1:PersistentVolume/azure-media-volume", "kubernetes:core/v1:PersistentVolumeClaim/azure-media-claim", "kubernetes:core/v1:Service/magento-web-service"} {
		if _, ok := mocks.resources[token]; !ok {
			t.Errorf("missing Magento resource %s", token)
		}
	}
	for _, name := range []string{"magento-web", "magento-cron"} {
		deployment := mocks.resources["kubernetes:apps/v1:Deployment/"+name].Inputs
		pod := deployment["spec"].ObjectValue()["template"].ObjectValue()["spec"].ObjectValue()
		pull := pod["imagePullSecrets"].ArrayValue()
		if len(pull) != 1 || pull[0].ObjectValue()["name"].StringValue() != "registry-credentials" {
			t.Errorf("%s image pull secret = %v", name, pull)
		}
		volumes := pod["volumes"].ArrayValue()
		if len(volumes) != 1 || volumes[0].ObjectValue()["persistentVolumeClaim"].ObjectValue()["claimName"].StringValue() != "magelift-media" {
			t.Errorf("%s media volume = %v", name, volumes)
		}
		foundTLS := false
		for _, container := range pod["containers"].ArrayValue() {
			environment := container.ObjectValue()["env"]
			if !environment.IsArray() {
				continue
			}
			for _, value := range environment.ArrayValue() {
				entry := value.ObjectValue()
				if entry["name"].StringValue() == "MAGELIFT_DATABASE_TLS_CA" && entry["value"].StringValue() == "/etc/ssl/certs/ca-certificates.crt" {
					foundTLS = true
				}
			}
		}
		if !foundTLS {
			t.Errorf("%s does not enable database TLS", name)
		}
	}
}

func TestProgramGrantsACRPullToKubelet(t *testing.T) {
	spec := testSpec()
	spec.ACRResourceID = "/subscriptions/registry-sub/resourceGroups/images/providers/Microsoft.ContainerRegistry/registries/store"
	mocks := &resourceMocks{resources: make(map[string]pulumi.MockResourceArgs)}
	if err := pulumi.RunErr(Program(spec), pulumi.WithMocks("magelift", "shop-test", mocks)); err != nil {
		t.Fatal(err)
	}
	role, ok := mocks.resources["azure-native:authorization:RoleAssignment/acr-pull"]
	if !ok {
		t.Fatal("ACR role assignment is missing")
	}
	if role.Inputs["scope"].StringValue() != spec.ACRResourceID || role.Inputs["principalId"].StringValue() != "example-kubelet-principal" {
		t.Fatalf("wrong ACR pull assignment: %v", role.Inputs)
	}
	if !strings.Contains(role.Inputs["roleDefinitionId"].StringValue(), "/subscriptions/registry-sub/") {
		t.Fatalf("role definition uses wrong subscription: %v", role.Inputs)
	}
}

func TestPreviewPoolReferencesClusterAndIsolatesEnvironments(t *testing.T) {
	var namespaces, volumes []string
	for _, env := range []string{"pr-1", "pr-2"} {
		spec := testSpec()
		spec.Environment = env
		spec.ClusterName = "ml-" + env
		spec.Namespace = "ml-" + env
		spec.PreviewPoolCluster = "shared-aks"
		spec.MySQLServerName = "mysql-" + env
		spec.StorageAccount = "storage" + strings.ReplaceAll(env, "-", "")
		spec.VaultName = "vault-" + env
		spec.ApplicationImage = "registry.example.invalid/shop@sha256:" + strings.Repeat("a", 64)
		spec.MagentoVersion = "2.4.9"
		spec.CacheImage = "valkey/valkey:9"
		spec.SearchImage = "opensearchproject/opensearch:3"
		mocks := &resourceMocks{resources: make(map[string]pulumi.MockResourceArgs)}
		if err := pulumi.RunErr(Program(spec), pulumi.WithMocks("magelift", env, mocks)); err != nil {
			t.Fatal(err)
		}
		if _, ok := mocks.resources["azure-native:containerservice:ManagedCluster/aks"]; ok {
			t.Fatal("pool stack must not create or own AKS")
		}
		cluster, ok := mocks.resources["azure-native:containerservice:ManagedCluster/shared-aks"]
		if !ok || cluster.ID == "" || cluster.ReadRPC == nil || !strings.HasSuffix(cluster.ID, "/managedClusters/shared-aks") {
			t.Fatalf("cluster is not read-only: %#v", cluster)
		}
		ns := mocks.resources["kubernetes:core/v1:Namespace/preview-namespace"].Inputs["metadata"].ObjectValue()["name"].StringValue()
		volume := mocks.resources["kubernetes:core/v1:PersistentVolume/azure-media-volume"].Inputs["metadata"].ObjectValue()["name"].StringValue()
		namespaces, volumes = append(namespaces, ns), append(volumes, volume)
		for _, token := range []string{"kubernetes:networking.k8s.io/v1:NetworkPolicy/namespace-ingress", "kubernetes:networking.k8s.io/v1:NetworkPolicy/web-ingress", "azure-native:dbformysql:Server/mysql", "azure-native:storage:StorageAccount/media", "azure-native:keyvault:Vault/vault"} {
			if _, ok := mocks.resources[token]; !ok {
				t.Errorf("missing owned preview resource %s", token)
			}
		}
	}
	if namespaces[0] == namespaces[1] || volumes[0] == volumes[1] {
		t.Fatalf("pool identities collide: %v %v", namespaces, volumes)
	}
}

func TestIngressTLSAndContainerInsights(t *testing.T) {
	spec := testSpec()
	spec.ApplicationImage = "registry.example.invalid/shop@sha256:" + strings.Repeat("a", 64)
	spec.MagentoVersion, spec.CacheImage, spec.SearchImage = "2.4.9", "valkey:9", "opensearch:3"
	spec.IngressClass, spec.Domain, spec.TLSSecret = "nginx", "shop.example.com", "shop-tls"
	spec.MonitorWorkspaceID = "/subscriptions/sub/resourceGroups/rg/providers/Microsoft.OperationalInsights/workspaces/logs"
	mocks := &resourceMocks{resources: make(map[string]pulumi.MockResourceArgs)}
	if err := pulumi.RunErr(Program(spec), pulumi.WithMocks("magelift", "test", mocks)); err != nil {
		t.Fatal(err)
	}
	ingress := mocks.resources["kubernetes:networking.k8s.io/v1:Ingress/magento-ingress"].Inputs["spec"].ObjectValue()
	if ingress["ingressClassName"].StringValue() != "nginx" || ingress["tls"].ArrayValue()[0].ObjectValue()["secretName"].StringValue() != "shop-tls" {
		t.Fatalf("ingress: %v", ingress)
	}
	if ingress["rules"].ArrayValue()[0].ObjectValue()["host"].StringValue() != "shop.example.com" {
		t.Fatalf("ingress host: %v", ingress)
	}
	service := mocks.resources["kubernetes:core/v1:Service/magento-web-service"].Inputs["spec"].ObjectValue()
	if service["type"].StringValue() != "ClusterIP" {
		t.Fatalf("unexpected service type: %v", service)
	}
	cluster := mocks.resources["azure-native:containerservice:ManagedCluster/aks"].Inputs
	agent := cluster["addonProfiles"].ObjectValue()["omsagent"].ObjectValue()
	if !agent["enabled"].BoolValue() || agent["config"].ObjectValue()["logAnalyticsWorkspaceResourceID"].StringValue() != spec.MonitorWorkspaceID {
		t.Fatalf("monitor addon: %v", agent)
	}
}

func TestIngressSupportsPreviewWithoutHostname(t *testing.T) {
	spec := testSpec()
	spec.ApplicationImage = "registry.example.invalid/shop@sha256:" + strings.Repeat("a", 64)
	spec.MagentoVersion, spec.CacheImage, spec.SearchImage = "2.4.9", "valkey:9", "opensearch:3"
	spec.IngressClass = "nginx"
	mocks := &resourceMocks{resources: make(map[string]pulumi.MockResourceArgs)}
	if err := pulumi.RunErr(Program(spec), pulumi.WithMocks("magelift", "test", mocks)); err != nil {
		t.Fatal(err)
	}
	ingress := mocks.resources["kubernetes:networking.k8s.io/v1:Ingress/magento-ingress"].Inputs["spec"].ObjectValue()
	rule := ingress["rules"].ArrayValue()[0].ObjectValue()
	if host, ok := rule["host"]; ok && !host.IsNull() && host.StringValue() != "" {
		t.Fatalf("unexpected host %v", host)
	}
}

func TestDestroyOnlyProgramUsesExistingStateWithoutReconstructingResources(t *testing.T) {
	spec := Spec{DestroyOnly: true, SubscriptionID: "sub", TenantID: "tenant", ResourceGroup: "rg", Region: "francecentral", Project: "shop", Environment: "preview-pr-1"}
	mocks := &resourceMocks{resources: make(map[string]pulumi.MockResourceArgs)}
	if err := pulumi.RunErr(Program(spec), pulumi.WithMocks("magelift", "preview-pr-1", mocks)); err != nil {
		t.Fatal(err)
	}
	if len(mocks.resources) != 0 {
		t.Fatalf("cleanup graph must read the existing stack state, got resources %v", mocks.resources)
	}
}
