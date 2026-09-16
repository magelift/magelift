package sdk

import (
	"strings"
	"testing"
)

func TestArchitectureIntentValidatesProviderNeutralBoundaries(t *testing.T) {
	intent := ArchitectureIntent{
		ProfileID:            "aws-ecs-ha",
		Provider:             "aws",
		Runtime:              "ecs-fargate",
		AccountOrProjectRef:  "account-ref",
		Region:               "eu-north-1",
		ComputeMode:          "fargate",
		NetworkMode:          "private-subnets",
		IngressMode:          "load-balancer",
		ArtifactDigest:       "registry.example/app@sha256:" + strings.Repeat("a", 64),
		SchemaFingerprint:    strings.Repeat("b", 64),
		MigrationFingerprint: strings.Repeat("c", 64),
		OwnershipMarker:      "magelift-run=run-1",
		Boundaries: []ServiceBoundaryIntent{
			{Role: "database", Family: "mysql", Major: "8.4", Ownership: ServiceManaged, ResourceReference: "arn:aws:rds:eu-west-1:123456789012:cluster:magelift", BackupProfile: "daily-encrypted", RecoveryProfile: "same-region", CapabilityID: "database.mysql"},
			{Role: "cache", Family: "valkey", Major: "9", Ownership: ServiceManaged, BackupProfile: "rebuild", RecoveryProfile: "same-region", CapabilityID: "cache.valkey"},
		},
		Resilience: ResilienceIntent{
			ProfileID: "regional-ha", AvailabilityTarget: "99.9", RPOSeconds: 300, RTOSeconds: 900,
			RetentionDays: 14, RecoveryScope: "runtime-and-durable-data", FailoverOwner: "platform",
			FencingPolicy: "single-writer-lease", RecoveryDestinations: []RecoveryDestination{RecoverySameRegion, RecoveryAlternateRegion},
			DataClasses: []DataClassIntent{{Name: "database", SourceOfTruth: "managed-database", FailureDomains: []string{"zone"}, BackupMethod: "provider-snapshot", RestoreMethod: "isolated-instance", IntegrityMethod: "checksum-and-read", LossSemantics: "no-loss-target", RetentionDays: 14, Encrypted: true, Immutable: true, DeletionProtection: true, OwnershipMarker: "magelift-run=run-1"}},
		},
	}
	if err := intent.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestArchitectureIntentRejectsIncompleteResilience(t *testing.T) {
	intent := ArchitectureIntent{
		ProfileID: "gcp-gke", Provider: "gcp", Runtime: "gke-autopilot", AccountOrProjectRef: "project",
		Region: "europe-west1", ComputeMode: "autopilot", NetworkMode: "vpc", IngressMode: "load-balancer",
		ArtifactDigest: "digest", SchemaFingerprint: "schema", MigrationFingerprint: "migration", OwnershipMarker: "marker",
		Boundaries: []ServiceBoundaryIntent{{Role: "database", Family: "mysql", Major: "8.4", Ownership: ServiceManaged, BackupProfile: "backup", RecoveryProfile: "restore"}},
	}
	if err := intent.Validate(); err == nil || !strings.Contains(err.Error(), "RPO and RTO") {
		t.Fatalf("incomplete resilience was accepted: %v", err)
	}
}

func TestArchitectureIntentRejectsMultilineResourceReference(t *testing.T) {
	intent := ArchitectureIntent{
		ProfileID: "aws-ecs", Provider: "aws", Runtime: "ecs-fargate", AccountOrProjectRef: "account", Region: "eu-west-1",
		ComputeMode: "fargate", NetworkMode: "private", IngressMode: "load-balancer", ArtifactDigest: "digest", SchemaFingerprint: "schema", MigrationFingerprint: "migration", OwnershipMarker: "marker",
		Boundaries: []ServiceBoundaryIntent{{Role: "database", Family: "mysql", Major: "8.4", Ownership: ServiceManaged, ResourceReference: "arn:aws:rds:eu-west-1:x:db/foo\nunsafe", BackupProfile: "backup", RecoveryProfile: "restore"}},
	}
	if err := intent.Validate(); err == nil || !strings.Contains(err.Error(), "resource reference") {
		t.Fatalf("multiline resource reference was accepted: %v", err)
	}
}

func TestArchitectureIntentRejectsUnscopedResilienceOwnership(t *testing.T) {
	intent := ArchitectureIntent{
		ProfileID: "aws-ecs", Provider: "aws", Runtime: "ecs-fargate", AccountOrProjectRef: "account", Region: "eu-west-1",
		ComputeMode: "fargate", NetworkMode: "private", IngressMode: "load-balancer", ArtifactDigest: "digest", SchemaFingerprint: "schema", MigrationFingerprint: "migration", OwnershipMarker: "magelift/run-1",
		Resilience: ResilienceIntent{
			ProfileID: "same-region", AvailabilityTarget: "best-effort", RPOSeconds: 60, RTOSeconds: 300, RetentionDays: 1,
			RecoveryScope: "runtime", FailoverOwner: "operator", FencingPolicy: "manual", RecoveryDestinations: []RecoveryDestination{RecoverySameRegion},
			DataClasses: []DataClassIntent{{Name: "database", SourceOfTruth: "managed", FailureDomains: []string{"zone"}, BackupMethod: "snapshot", RestoreMethod: "isolated", IntegrityMethod: "checksum", LossSemantics: "recover", RetentionDays: 1, OwnershipMarker: "other/run-1"}},
		},
	}
	if err := intent.Validate(); err == nil || !strings.Contains(err.Error(), "ownership marker") {
		t.Fatalf("unscoped resilience ownership was accepted: %v", err)
	}
	intent.Resilience.DataClasses[0].OwnershipMarker = "magelift/run-1/database"
	if err := intent.Validate(); err != nil {
		t.Fatalf("scoped data-class ownership was rejected: %v", err)
	}
}

func TestProjectionTargetValidation(t *testing.T) {
	tests := []struct {
		name   string
		target ProjectionTarget
		want   string
	}{
		{name: "ecs service", target: ProjectionTarget{Runtime: "ecs", Cluster: "shop", Service: "web", Container: "php"}},
		{name: "ecs pinned task", target: ProjectionTarget{Runtime: "ecs", Cluster: "shop", Task: "task-123", Container: "php"}},
		{name: "kubernetes workload", target: ProjectionTarget{Runtime: "kubernetes", Namespace: "magento", Workload: "web", Container: "php"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := test.target.Validate(); err != nil {
				t.Fatalf("ProjectionTarget.Validate() error = %v", err)
			}
		})
	}
	invalid := ProjectionTarget{Runtime: "ecs", Cluster: "shop", Service: "web", Task: "task-123", Container: "php"}
	if err := invalid.Validate(); err == nil || !strings.Contains(err.Error(), "exactly one") {
		t.Fatalf("ambiguous projection target was accepted: %v", err)
	}
	invalid = ProjectionTarget{Runtime: "kubernetes", Namespace: "magento", Workload: "web\nunsafe", Container: "php"}
	if err := invalid.Validate(); err == nil || !strings.Contains(err.Error(), "single-line") {
		t.Fatalf("multiline projection target was accepted: %v", err)
	}
}

func TestValidateEdgeAndObservabilityCompositions(t *testing.T) {
	if err := ValidateEdgeIntent(EdgeIntent{
		ExternalProvider: "fastly", Lifecycle: ExternalLifecycleExtension, Certification: ExternalExperimental,
		Mode: "both", NativeProvider: "cloudfront", CredentialRefs: []string{"secret://fastly/token"},
		OriginHealthRef: "health/origin", Domains: []string{"shop.example.com"}, TLS: true, TLSMode: "managed", DNSMode: "external", OwnershipMarker: "magelift/test/edge",
		Health: EdgeHealthIntent{OriginURL: "https://origin.example.com/health", ExpectedRouteTarget: "dualstack.e.sni.global.fastly.net", RoutePath: "/health", ExpectedStatus: 200, RouteTimeoutSeconds: 90, RoutePollSeconds: 2},
	}); err != nil {
		t.Fatal(err)
	}
	invalidHealth := EdgeIntent{Mode: "external", ExternalProvider: "fastly", Lifecycle: ExternalLifecycleExtension, Certification: ExternalExperimental, OriginHealthRef: "health/origin", Domains: []string{"shop.example.com"}, OwnershipMarker: "magelift/test/edge", Health: EdgeHealthIntent{ExpectedStatus: 99}}
	if err := ValidateEdgeIntent(invalidHealth); err == nil || !strings.Contains(err.Error(), "expected status") {
		t.Fatalf("invalid edge health status was accepted: %v", err)
	}
	if err := ValidateObservabilityIntent(ObservabilityIntent{
		ExternalProvider: "newrelic", Lifecycle: ExternalLifecycleExtension, Certification: ExternalExperimental,
		OwnershipMarker: "magelift/architecture/test",
		CredentialRefs:  []string{"secret://newrelic/license"},
		Signals:         []string{"logs", "metrics", "traces"}, Logs: true, Metrics: true, Traces: true,
		SamplingRatio: 0.25, RetentionDays: 30,
	}); err != nil {
		t.Fatal(err)
	}
}

func TestValidateObservabilityTreatsNoneAsNoDestination(t *testing.T) {
	err := ValidateObservabilityIntent(ObservabilityIntent{NativeProvider: "none", OwnershipMarker: "magelift/architecture/test", Signals: []string{"metrics"}, Metrics: true})
	if err == nil || !strings.Contains(err.Error(), "destination") {
		t.Fatalf("none destination was accepted: %v", err)
	}
}

func TestValidateObservabilityRequiresOwnershipScope(t *testing.T) {
	err := ValidateObservabilityIntent(ObservabilityIntent{NativeProvider: "cloudwatch", Signals: []string{"metrics"}, Metrics: true})
	if err == nil || !strings.Contains(err.Error(), "ownership marker") {
		t.Fatalf("unscoped observability intent was accepted: %v", err)
	}
}

func TestValidateEdgeTreatsNoneAsNoProvider(t *testing.T) {
	if err := ValidateEdgeIntent(EdgeIntent{Mode: "none", NativeProvider: "none", ExternalProvider: "none"}); err != nil {
		t.Fatalf("none providers should be inert in none mode: %v", err)
	}
}

func TestValidateObservabilityAlertsAndSLOsRequireActionableMetadata(t *testing.T) {
	intent := ObservabilityIntent{
		NativeProvider: "cloudwatch", OwnershipMarker: "magelift/architecture/test", Signals: []string{"backup-age"},
		Alerts:     []AlertIntent{{ID: "backup-stale", Signal: "backup-age", Severity: "critical", Operator: "gt", Threshold: 300, WindowSeconds: 300, Owner: "oncall", RunbookURL: "https://runbooks.example/backup", DeduplicationKey: "profile/backup"}},
		Dashboards: []DashboardIntent{{ID: "resilience", Signals: []string{"backup-age"}, Owner: "oncall"}},
		SLOs:       []SLOIntent{{ID: "availability", Signal: "application-health", Target: 0.999, WindowSeconds: 2592000, Owner: "oncall", RunbookURL: "https://runbooks.example/availability", ErrorBudgetPolicy: "page-on-burn"}},
	}
	if err := ValidateObservabilityIntent(intent); err != nil {
		t.Fatal(err)
	}
	intent.Alerts[0].RunbookURL = "http://unsafe.example/backup"
	if err := ValidateObservabilityIntent(intent); err == nil || !strings.Contains(err.Error(), "HTTPS") {
		t.Fatalf("insecure alert runbook accepted: %v", err)
	}
	intent.Alerts[0].RunbookURL = "https://runbooks.example/backup"
	intent.Dashboards[0].Signals = []string{"backup-age", "backup-age"}
	if err := ValidateObservabilityIntent(intent); err == nil || !strings.Contains(err.Error(), "duplicate signal") {
		t.Fatalf("duplicate dashboard signal accepted: %v", err)
	}
}
