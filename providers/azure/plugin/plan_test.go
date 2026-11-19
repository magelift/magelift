package plugin

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/magelift/magelift/sdk"
)

func testPlanRequest() *sdk.PlanRequest {
	return &sdk.PlanRequest{
		ProtocolVersion: sdk.ProtocolV1,
		Envelope:        sdk.Envelope{Project: "shop", Environment: "preview", EnvironmentClass: "preview", Region: "francecentral", StackName: "shop-preview-azure-aks", Preset: "preview"},
		Application:     sdk.Application{Edition: "open-source", Version: "2.4.9", Mode: "integrated", WebRuntime: "nginx-fpm"},
		Runtime:         "aks",
		TargetBlock:     []byte("subscriptionId: sub\nresourceGroup: group\ntenantId: tenant\nimageDigest: registry.example/shop@sha256:" + strings.Repeat("a", 64) + "\nstateBackendUrl: azblob://states\n"),
		Services: map[string]sdk.ServiceBinding{
			"database": {Family: "mysql", Version: "8.0.21", Ownership: "managed"},
			"cache":    {Family: "valkey", Ownership: "self-hosted", Image: "valkey/valkey:9"},
			"search":   {Family: "opensearch", Ownership: "self-hosted", Image: "opensearchproject/opensearch:3"},
			"queue":    {Family: "database"},
		},
	}
}

func TestPlanRetainsChoicesWithoutSecrets(t *testing.T) {
	request := testPlanRequest()
	var result sdk.PlanResult
	if err := (&Server{}).Plan(request, &result); err != nil || result.Error != nil {
		t.Fatalf("plan: %v, %v", err, result.Error)
	}
	var planned storedPlan
	if err := json.Unmarshal(result.Plan.Opaque, &planned); err != nil {
		t.Fatal(err)
	}
	if planned.Spec.MySQLVersion != "8.0.21" || planned.Spec.CacheImage != "valkey/valkey:9" || planned.Spec.SearchImage != "opensearchproject/opensearch:3" {
		t.Fatalf("plan lost user selections: %#v", planned.Spec)
	}
	if planned.Spec.SubscriptionID != "sub" || planned.Spec.ResourceGroup != "group" || result.Plan.StateBackendURL != "azblob://states" {
		t.Fatalf("plan lost Azure identity: %#v", result.Plan)
	}
	if strings.Contains(string(result.Plan.Opaque), "password") {
		t.Fatal("plan must not contain password material")
	}
	var again sdk.PlanResult
	if err := (&Server{}).Plan(request, &again); err != nil || again.Error != nil || string(result.Plan.Opaque) != string(again.Plan.Opaque) {
		t.Fatalf("plan is not deterministic: %v, %v", err, again.Error)
	}
}

func TestManagedMySQLARMVersion(t *testing.T) {
	for _, tc := range []struct{ selected, arm string }{{"8.0", "8.0.21"}, {"8.0.42", "8.0.21"}, {"8.4.7", "8.4"}} {
		t.Run(tc.selected, func(t *testing.T) {
			request := testPlanRequest()
			request.Services["database"] = sdk.ServiceBinding{Family: "mysql", Version: tc.selected, Ownership: "managed"}
			var result sdk.PlanResult
			if err := (&Server{}).Plan(request, &result); err != nil || result.Error != nil {
				t.Fatalf("plan: %v, %v", err, result.Error)
			}
			var plan storedPlan
			if err := json.Unmarshal(result.Plan.Opaque, &plan); err != nil {
				t.Fatal(err)
			}
			if plan.Spec.MySQLVersion != tc.arm || request.Services["database"].Version != tc.selected {
				t.Fatalf("ARM version %q, requested %q", plan.Spec.MySQLVersion, request.Services["database"].Version)
			}
		})
	}
}

func TestPlanRejectsUnsupportedBindings(t *testing.T) {
	for _, test := range []struct {
		name   string
		change func(*sdk.PlanRequest)
		want   string
	}{
		{"unknown service", func(r *sdk.PlanRequest) { r.Services["media"] = sdk.ServiceBinding{Family: "blob"} }, "services.media"},
		{"attached database", func(r *sdk.PlanRequest) {
			r.Services["database"] = sdk.ServiceBinding{Family: "mysql", Ownership: "external", Endpoint: "db.example"}
		}, "external database"},
		{"cache credential", func(r *sdk.PlanRequest) {
			r.Services["cache"] = sdk.ServiceBinding{Family: "redis", Ownership: "external", Endpoint: "cache.example", Secret: "azure-keyvault://vault/key"}
		}, "cache authentication"},
		{"domain", func(r *sdk.PlanRequest) { r.Envelope.Domain = "shop.example" }, "domain"},
		{"backend credentials", func(r *sdk.PlanRequest) {
			r.TargetBlock = []byte(strings.Replace(string(r.TargetBlock), "azblob://states", "azblob://states?token=secret", 1))
		}, "stateBackendUrl"},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := testPlanRequest()
			test.change(request)
			var result sdk.PlanResult
			if err := (&Server{}).Plan(request, &result); err != nil {
				t.Fatal(err)
			}
			if result.Error == nil || !strings.Contains(result.Error.Message, test.want) {
				t.Fatalf("error = %#v, want %q", result.Error, test.want)
			}
		})
	}
}

func TestPlanBuildsImagesFromOperatorVersions(t *testing.T) {
	request := testPlanRequest()
	request.Services["cache"] = sdk.ServiceBinding{Family: "valkey", Ownership: "self-hosted", Version: "8.1.2"}
	request.Services["search"] = sdk.ServiceBinding{Family: "opensearch", Ownership: "self-hosted", Version: "2.19"}
	var result sdk.PlanResult
	if err := (&Server{}).Plan(request, &result); err != nil || result.Error != nil {
		t.Fatalf("plan: %v, %v", err, result.Error)
	}
	var planned storedPlan
	if err := json.Unmarshal(result.Plan.Opaque, &planned); err != nil {
		t.Fatal(err)
	}
	if planned.Spec.CacheImage != "valkey/valkey:8.1.2" || planned.Spec.SearchImage != "opensearchproject/opensearch:2.19.0" {
		t.Fatalf("version choices lost: %#v", planned.Spec)
	}
}

func TestPreviewPoolRequiresPreviewAndExistingReferences(t *testing.T) {
	request := testPlanRequest()
	request.Envelope.EnvironmentClass = "preview"
	request.TargetBlock = append(request.TargetBlock, []byte("previewPool:\n  clusterName: shared-aks\n  nodeSubnetId: /subscriptions/sub/resourceGroups/group/providers/Microsoft.Network/virtualNetworks/shared/subnets/nodes\n  mysqlSubnetId: /subscriptions/sub/resourceGroups/group/providers/Microsoft.Network/virtualNetworks/shared/subnets/mysql\n  mysqlDnsZoneId: /subscriptions/sub/resourceGroups/group/providers/Microsoft.Network/privateDnsZones/shared.mysql.database.azure.com\n")...)
	var result sdk.PlanResult
	if err := (&Server{}).Plan(request, &result); err != nil || result.Error != nil {
		t.Fatalf("plan: %v, %v", err, result.Error)
	}
	var plan storedPlan
	if err := json.Unmarshal(result.Plan.Opaque, &plan); err != nil {
		t.Fatal(err)
	}
	if plan.Spec.PreviewPoolCluster != "shared-aks" || plan.Spec.Namespace == "" {
		t.Fatalf("pool not retained: %#v", plan.Spec)
	}
	request.Envelope.EnvironmentClass = "production"
	result = sdk.PlanResult{}
	if err := (&Server{}).Plan(request, &result); err != nil || result.Error == nil {
		t.Fatalf("non-preview must be rejected: %v %v", err, result.Error)
	}
}

func TestAzureStarterPlanWithIngressAndMonitor(t *testing.T) {
	request := testPlanRequest()
	request.Envelope.EnvironmentClass = "stable"
	request.Envelope.Domain = "shop.example.com"
	request.Services["cache"] = sdk.ServiceBinding{Family: "valkey", Ownership: "self-hosted", Version: "9"}
	request.Services["search"] = sdk.ServiceBinding{Family: "opensearch", Ownership: "self-hosted", Version: "3"}
	request.Edge = sdk.EdgeIntent{Mode: "native", NativeProvider: "azure", TLS: true, TLSMode: "external", DNSMode: "external", OriginHealthRef: "health:origin", OwnershipMarker: "magelift/test/edge"}
	request.Observability = sdk.ObservabilityIntent{NativeProvider: "azure-monitor", Logs: true, Signals: []string{"logs"}, OwnershipMarker: "magelift/test/observability"}
	if err := sdk.ValidateEdgeIntent(request.Edge); err != nil {
		t.Fatal(err)
	}
	if err := sdk.ValidateObservabilityIntent(request.Observability); err != nil {
		t.Fatal(err)
	}
	request.TargetBlock = append(request.TargetBlock, []byte("ingressClass: nginx\ntlsSecret: shop-tls\nmonitorWorkspaceId: /subscriptions/sub/resourceGroups/group/providers/Microsoft.OperationalInsights/workspaces/logs\n")...)
	var result sdk.PlanResult
	if err := (&Server{}).Plan(request, &result); err != nil || result.Error != nil {
		t.Fatalf("normal Azure plan: %v, %v", err, result.Error)
	}
	var plan storedPlan
	if err := json.Unmarshal(result.Plan.Opaque, &plan); err != nil {
		t.Fatal(err)
	}
	if plan.Spec.Domain != "shop.example.com" || plan.Spec.TLSSecret != "shop-tls" || plan.Spec.MonitorWorkspaceID == "" || plan.Spec.CacheImage != "valkey/valkey:9" {
		t.Fatalf("plan omitted choices: %#v", plan.Spec)
	}
}

func TestPreviewStateUsesBootstrappedContainerAndUniqueStacks(t *testing.T) {
	server := &Server{}
	base := testPlanRequest()
	base.TargetBlock = []byte("subscriptionId: sub\nresourceGroup: group\ntenantId: tenant\n")
	base.AccountOnly = true
	var bootstrap sdk.PlanResult
	if err := server.Plan(base, &bootstrap); err != nil || bootstrap.Error != nil {
		t.Fatalf("bootstrap plan: %v %v", err, bootstrap.Error)
	}
	var stacks, resources []string
	for _, environment := range []string{"preview-pr-1", "preview-pr-2"} {
		request := testPlanRequest()
		request.TargetBlock = []byte("subscriptionId: sub\nresourceGroup: group\ntenantId: tenant\nimageDigest: registry.example/shop@sha256:" + strings.Repeat("a", 64) + "\n")
		request.StateEnvironment = "preview"
		request.Envelope.Environment, request.Envelope.StackName = environment, "shop-"+environment+"-azure-aks"
		request.Envelope.ExpiresAt = time.Now().Add(time.Hour).UTC().Format(time.RFC3339)
		var planned sdk.PlanResult
		if err := server.Plan(request, &planned); err != nil || planned.Error != nil {
			t.Fatalf("preview plan: %v %v", err, planned.Error)
		}
		if planned.Plan.StateBackendURL != bootstrap.Plan.StateBackendURL {
			t.Fatalf("preview backend %q != bootstrap %q", planned.Plan.StateBackendURL, bootstrap.Plan.StateBackendURL)
		}
		var opaque storedPlan
		if err := json.Unmarshal(planned.Plan.Opaque, &opaque); err != nil {
			t.Fatal(err)
		}
		stacks = append(stacks, planned.Plan.StackName)
		resources = append(resources, opaque.Spec.ClusterName)
	}
	if stacks[0] == stacks[1] || resources[0] == resources[1] {
		t.Fatalf("preview identities collide: %v %v", stacks, resources)
	}
}

func TestPreviewTeardownDoesNotRequireCurrentApplicationOrTLS(t *testing.T) {
	request := testPlanRequest()
	request.StateEnvironment = "preview"
	request.AllowExpiredPreview = true
	request.Envelope.Environment = "preview-pr-1"
	request.Envelope.StackName = "shop-preview-pr-1-azure-aks"
	request.Envelope.ExpiresAt = "2020-01-01T00:00:00Z"
	request.Services = nil
	request.TargetBlock = []byte("subscriptionId: sub\nresourceGroup: group\ntenantId: tenant\n")
	request.Edge = sdk.EdgeIntent{TLS: true}
	var planned sdk.PlanResult
	if err := (&Server{}).Plan(request, &planned); err != nil || planned.Error != nil {
		t.Fatalf("expired cleanup plan: %v %v", err, planned.Error)
	}
	var opaque storedPlan
	if err := json.Unmarshal(planned.Plan.Opaque, &opaque); err != nil || !opaque.Spec.DestroyOnly || planned.Plan.ImageDigest != "" {
		t.Fatalf("teardown = %#v, %v", opaque, err)
	}
}

func TestStableTeardownDoesNotRequireCurrentApplication(t *testing.T) {
	request := testPlanRequest()
	request.AllowExpiredPreview = true
	request.Envelope.EnvironmentClass, request.Envelope.Environment = "staging", "staging"
	request.TargetBlock = []byte("subscriptionId: sub\nresourceGroup: group\ntenantId: tenant\n")
	request.Services = nil
	var result sdk.PlanResult
	if err := (&Server{}).Plan(request, &result); err != nil || result.Error != nil {
		t.Fatalf("stable destroy plan: %v %#v", err, result.Error)
	}
}

func TestAzureMaterializedPreviewExpiration(t *testing.T) {
	server := &Server{}
	request := testPlanRequest()
	request.Envelope.EnvironmentClass = "staging"
	var result sdk.PlanResult
	if err := server.Plan(request, &result); err != nil || result.Error != nil {
		t.Fatalf("stable preview-sized staging: %v %v", err, result.Error)
	}
	request = testPlanRequest()
	request.StateEnvironment = "preview"
	request.Envelope.ExpiresAt = ""
	result = sdk.PlanResult{}
	if err := server.Plan(request, &result); err != nil || result.Error == nil {
		t.Fatalf("unbounded PR preview must fail: %v %#v", err, result.Error)
	}
}

func TestObservationPlanNeedsNoImageServicesOrTLS(t *testing.T) {
	request := testPlanRequest()
	request.RuntimeObserve = true
	request.TargetBlock = []byte("subscriptionId: sub\nresourceGroup: group\ntenantId: tenant\n")
	request.Services = nil
	request.Edge = sdk.EdgeIntent{TLS: true}
	request.Envelope.ExpiresAt = "2020-01-01T00:00:00Z"
	server := &Server{}
	var result sdk.PlanResult
	if err := server.Plan(request, &result); err != nil || result.Error != nil {
		t.Fatalf("observation plan: %v %#v", err, result.Error)
	}
	var stored storedPlan
	if err := json.Unmarshal(result.Plan.Opaque, &stored); err != nil || !stored.Spec.ObserveOnly {
		t.Fatalf("observation mode: %#v %v", stored, err)
	}
	server.outputs = func(context.Context, storedPlan) (map[string]any, []string, error) {
		return map[string]any{"clusterName": "existing"}, nil, nil
	}
	call := &sdk.StackCall{ProtocolVersion: sdk.ProtocolV1, Envelope: request.Envelope, Plan: result.Plan}
	var outputs sdk.OutputsResult
	if err := server.Outputs(call, &outputs); err != nil || outputs.Error != nil || !strings.Contains(string(outputs.ValuesJSON), "existing") {
		t.Fatalf("outputs: %v %#v", err, outputs)
	}
	var preview sdk.LifecycleResult
	if err := server.Preview(call, &preview); err != nil || preview.Error == nil {
		t.Fatalf("observation plan cannot preview: %v %#v", err, preview)
	}
	var applied sdk.LifecycleResult
	if err := server.Apply(call, &applied); err != nil || applied.Error == nil {
		t.Fatalf("observation plan cannot apply: %v %#v", err, applied)
	}
}

func TestStarterRequiresActionableAzureDependencies(t *testing.T) {
	server := &Server{}
	request := testPlanRequest()
	request.Envelope.Domain = "shop.example.com"
	var result sdk.PlanResult
	if err := server.Plan(request, &result); err != nil || result.Error == nil || !strings.Contains(result.Error.Message, "install an ingress controller") || !strings.Contains(result.Error.Message, "target.azure.ingressClass") {
		t.Fatalf("domain guidance: %v %#v", err, result.Error)
	}
	request = testPlanRequest()
	request.Envelope.Preset = "high-availability"
	request.Envelope.EnvironmentClass = "production"
	result = sdk.PlanResult{}
	if err := server.Plan(request, &result); err != nil || result.Error != nil {
		t.Fatalf("HA web with explicit singleton cache: %v %#v", err, result.Error)
	}
	var planned storedPlan
	if err := json.Unmarshal(result.Plan.Opaque, &planned); err != nil {
		t.Fatal(err)
	}
	if !planned.Spec.HighAvailability || planned.Spec.CacheImage == "" {
		t.Fatalf("HA workload / selected singleton lost: %#v", planned.Spec)
	}
}

func TestPlanKeepsSelfHostedShopServicesAndKeyReference(t *testing.T) {
	r := testPlanRequest()
	r.Services["database"] = sdk.ServiceBinding{Family: "mariadb", Ownership: "self-hosted", Version: "11.4", StorageGiB: 50}
	r.Services["cache"] = sdk.ServiceBinding{Family: "valkey", Ownership: "self-hosted", Version: "8.1", EvictionPolicy: "allkeys-lru", MemoryMiB: 512, StorageGiB: 12}
	r.Services["session"] = sdk.ServiceBinding{Family: "valkey", Ownership: "self-hosted", Version: "8.1", EvictionPolicy: "noeviction", MemoryMiB: 128, StorageGiB: 8}
	r.Services["queue"] = sdk.ServiceBinding{Family: "rabbitmq", Ownership: "self-hosted", Version: "4.1", StorageGiB: 20}
	r.Services["search"] = sdk.ServiceBinding{Family: "opensearch", Ownership: "self-hosted", Version: "3.1", Plugins: []string{"analysis-icu", "analysis-phonetic"}, StorageGiB: 32}
	r.TargetBlock = append(r.TargetBlock, []byte("encryptionKeySecretName: existing-magento-key\n")...)
	var result sdk.PlanResult
	if err := (&Server{}).Plan(r, &result); err != nil || result.Error != nil {
		t.Fatalf("plan %v / %#v", err, result.Error)
	}
	var plan storedPlan
	if err := json.Unmarshal(result.Plan.Opaque, &plan); err != nil {
		t.Fatal(err)
	}
	if plan.Spec.DatabaseImage != "mariadb:11.4" || plan.Spec.DatabaseStorageGiB != 50 || plan.Spec.CacheMemoryMiB != 512 || plan.Spec.CacheStorageGiB != 12 || plan.Spec.SessionMemoryMiB != 128 || plan.Spec.SessionStorageGiB != 8 || plan.Spec.QueueStorageGiB != 20 || plan.Spec.SearchStorageGiB != 32 || plan.Spec.CacheEvictionPolicy != "allkeys-lru" || plan.Spec.SessionEvictionPolicy != "noeviction" || plan.Spec.SessionImage != "valkey/valkey:8.1" || plan.Spec.QueueImage != "rabbitmq:4.1-management-alpine" || len(plan.Spec.SearchPlugins) != 2 || plan.Spec.EncryptionKeySecretName != "existing-magento-key" {
		t.Fatalf("choices not preserved: %#v", plan.Spec)
	}
}

func TestAzureEncryptionKeySecretIDValidation(t *testing.T) {
	const valid = "https://example-vault.vault.azure.net/secrets/magento-key/0123456789abcdef0123456789abcdef"
	host, name, version, err := parseEncryptionSecretID(valid)
	if err != nil || host != "https://example-vault.vault.azure.net" || name != "magento-key" || version != "0123456789abcdef0123456789abcdef" {
		t.Fatalf("parsed %s %s %s: %v", host, name, version, err)
	}
	for _, bad := range []string{"http://example-vault.vault.azure.net/secrets/k/0123456789abcdef0123456789abcdef", "https://evil.example/secrets/k/0123456789abcdef0123456789abcdef", "https://example-vault.vault.azure.net/secrets/k", "https://example-vault.vault.azure.net/secrets/k/0123456789abcdef0123456789abcdef?query=secret"} {
		if _, _, _, err := parseEncryptionSecretID(bad); err == nil {
			t.Errorf("accepted %q", bad)
		}
	}
}

func TestPlanAttachesTLSDatabaseAndRabbitMQ(t *testing.T) {
	r := testPlanRequest()
	r.Services["database"] = sdk.ServiceBinding{Family: "mariadb", Ownership: "external", Endpoint: "db.example.com", Username: "shop", Secret: "kubernetes://shop-db"}
	r.Services["queue"] = sdk.ServiceBinding{Family: "rabbitmq", Ownership: "existing", Endpoint: "amqps://mq.example.com:5671", Username: "shopmq", Secret: "kubernetes://shop-mq"}
	var result sdk.PlanResult
	if err := (&Server{}).Plan(r, &result); err != nil || result.Error != nil {
		t.Fatalf("plan %v %#v", err, result.Error)
	}
	var plan storedPlan
	if err := json.Unmarshal(result.Plan.Opaque, &plan); err != nil {
		t.Fatal(err)
	}
	if plan.Spec.DatabaseEndpoint != "db.example.com" || plan.Spec.DatabaseSecretName != "shop-db" || plan.Spec.MySQLUsername != "shop" || plan.Spec.QueueEndpoint != "mq.example.com" || plan.Spec.QueuePort != "5671" || plan.Spec.QueueUsername != "shopmq" || plan.Spec.QueueSecretName != "shop-mq" || plan.Spec.QueueImage != "" {
		t.Fatalf("external connection lost: %#v", plan.Spec)
	}
	for _, bad := range []string{"amqp://mq.example.com", "amqps://user:pass@mq.example.com", "amqps://mq.example.com/other"} {
		r.Services["queue"] = sdk.ServiceBinding{Family: "rabbitmq", Ownership: "external", Endpoint: bad, Username: "shopmq", Secret: "kubernetes://shop-mq"}
		result = sdk.PlanResult{}
		if err := (&Server{}).Plan(r, &result); err != nil || result.Error == nil {
			t.Fatalf("accepted %q: %v %#v", bad, err, result.Error)
		}
	}
}

func TestPlanPreservesHASingletonDataChoices(t *testing.T) {
	r := testPlanRequest()
	r.Envelope.Preset = "high-availability"
	r.Envelope.EnvironmentClass = "production"
	r.Services["database"] = sdk.ServiceBinding{Family: "mariadb", Ownership: "self-hosted", Version: "11.4", Parameters: map[string]string{"character_set_server": "utf8mb4"}}
	r.Services["session"] = sdk.ServiceBinding{Family: "valkey", Ownership: "self-hosted", Version: "8.1"}
	var result sdk.PlanResult
	if err := (&Server{}).Plan(r, &result); err != nil || result.Error != nil {
		t.Fatalf("plan: %v %#v", err, result.Error)
	}
	var plan storedPlan
	_ = json.Unmarshal(result.Plan.Opaque, &plan)
	if !plan.Spec.HighAvailability || plan.Spec.DatabaseImage != "mariadb:11.4" || plan.Spec.SessionImage != "valkey/valkey:8.1" || plan.Spec.DatabaseParameters["character_set_server"] != "utf8mb4" {
		t.Fatalf("plan choices: %#v", plan.Spec)
	}
}
