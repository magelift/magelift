package kube_test

import (
	"context"
	"io"
	"net/netip"
	"strings"
	"testing"

	"github.com/magelift/magelift/internal/automation"
	"github.com/magelift/magelift/internal/cloud/aws/eksops"
	gcpops "github.com/magelift/magelift/internal/cloud/gcp/ops"
	gcpstack "github.com/magelift/magelift/internal/cloud/gcp/stack"
	"github.com/magelift/magelift/internal/cloud/kube"
	ovhstack "github.com/magelift/magelift/internal/cloud/ovh/stack"
	scwstack "github.com/magelift/magelift/internal/cloud/scaleway/stack"
	"github.com/magelift/magelift/internal/platform"
	sdk "github.com/magelift/magelift/sdk/v1"
)

// TestFourModuleTypeIdentity is the Phase 6 SC1–SC2 offline gate: gcp ops,
// eksops, ovh stack, and scaleway stack all return the same concrete
// *kube.Observe and *kube.Steps types (D-01, D-03).
func TestFourModuleTypeIdentity(t *testing.T) {
	digest := "ghcr.io/magelift/magento@sha256:" + strings.Repeat("a", 64)
	backend := &identityBackend{}

	modules := []struct {
		name    string
		observe platform.RuntimeObserve
		steps   func(t *testing.T) any
	}{
		{
			name:    "gcp",
			observe: platform.ModuleRuntimeObserve(gcpops.Module{}),
			steps: func(t *testing.T) any {
				t.Helper()
				ops := gcpops.Ops{
					NewCandidate: identityCandidateFactory,
					NewRuntime:   identityRuntimeFactory,
				}
				planned := gcpstack.Planned{Spec: gcpIdentitySpec(digest)}
				s, err := ops.NewDeploySteps(context.Background(), backend, planned, io.Discard)
				if err != nil {
					t.Fatalf("gcp NewDeploySteps: %v", err)
				}
				return s
			},
		},
		{
			name:    "eksops",
			observe: platform.ModuleRuntimeObserve(eksops.Module{}),
			steps: func(t *testing.T) any {
				t.Helper()
				ops := eksops.Ops{
					NewCandidate: identityCandidateFactory,
					NewRuntime:   identityRuntimeFactory,
				}
				planned := eksops.Planned{Spec: eksIdentitySpec(digest)}
				s, err := ops.NewDeploySteps(context.Background(), backend, planned, io.Discard)
				if err != nil {
					t.Fatalf("eksops NewDeploySteps: %v", err)
				}
				return s
			},
		},
		{
			name:    "ovh",
			observe: platform.ModuleRuntimeObserve(ovhstack.Module{}),
			steps: func(t *testing.T) any {
				t.Helper()
				ops := ovhstack.Ops{
					NewCandidate: identityCandidateFactory,
					NewRuntime:   identityRuntimeFactory,
				}
				planned := ovhstack.Planned{Spec: ovhIdentitySpec(digest)}
				s, err := ops.NewDeploySteps(context.Background(), backend, planned, io.Discard)
				if err != nil {
					t.Fatalf("ovh NewDeploySteps: %v", err)
				}
				return s
			},
		},
		{
			name:    "scaleway",
			observe: platform.ModuleRuntimeObserve(scwstack.Module{}),
			steps: func(t *testing.T) any {
				t.Helper()
				ops := scwstack.Ops{
					NewCandidate: identityCandidateFactory,
					NewRuntime:   identityRuntimeFactory,
				}
				planned := scwstack.Planned{Spec: scwIdentitySpec(digest)}
				s, err := ops.NewDeploySteps(context.Background(), backend, planned, io.Discard)
				if err != nil {
					t.Fatalf("scaleway NewDeploySteps: %v", err)
				}
				return s
			},
		},
	}

	for _, m := range modules {
		t.Run(m.name, func(t *testing.T) {
			if m.observe == nil {
				t.Fatal("RuntimeObserve returned nil")
			}
			if _, ok := m.observe.(*kube.Observe); !ok {
				t.Fatalf("RuntimeObserve type identity: want *kube.Observe, got %T", m.observe)
			}
			steps := m.steps(t)
			if _, ok := steps.(*kube.Steps); !ok {
				t.Fatalf("NewDeploySteps type identity: want *kube.Steps, got %T", steps)
			}
		})
	}
}

type identityBackend struct{}

func (*identityBackend) Outputs(context.Context) (map[string]any, error) {
	return map[string]any{}, nil
}
func (*identityBackend) Preview(context.Context, automation.Request, io.Writer) (map[string]int, error) {
	return map[string]int{}, nil
}
func (*identityBackend) Update(context.Context, automation.Request, io.Writer) (map[string]int, error) {
	return map[string]int{}, nil
}
func (*identityBackend) Destroy(context.Context, automation.Request, io.Writer) (map[string]int, error) {
	return map[string]int{}, nil
}

type identityCandidate struct{}

func (identityCandidate) RegisterCandidate(context.Context, kube.CandidateRequest) (kube.Candidate, error) {
	return kube.Candidate{}, nil
}
func (identityCandidate) RunMigrations(context.Context, kube.Candidate) error { return nil }
func (identityCandidate) Cleanup(context.Context, kube.Candidate) error       { return nil }

type identityRuntime struct{}

func (identityRuntime) Check(context.Context, string, string) (kube.ServiceHealth, error) {
	return kube.ServiceHealth{DesiredReplicas: 1, ReadyReplicas: 1, Available: true}, nil
}

func identityCandidateFactory(context.Context, kube.Backend) (kube.CandidateRunner, error) {
	return identityCandidate{}, nil
}

func identityRuntimeFactory(context.Context, kube.Backend) (kube.RuntimeChecker, error) {
	return identityRuntime{}, nil
}

func gcpIdentitySpec(digest string) gcpstack.Spec {
	return gcpstack.Spec{
		Identity: gcpstack.Identity{
			Project: "shop", GCPProject: "example-gcp-project", Environment: "preview",
			Region: "europe-west1", EnvironmentClass: "preview", Preset: sdk.PresetPreview,
		},
		Application:  gcpstack.Application{Edition: "open-source", Version: "2.4.8", Mode: "integrated", WebRuntime: "nginx-fpm"},
		Artifact:     gcpstack.Artifact{ImageDigest: digest},
		Policy:       gcpstack.NetworkPolicy{NetworkCIDR: "10.20.0.0/16", Zones: []string{"europe-west1-b"}},
		Catalog:      gcpstack.CatalogSelection{DesiredWebReplicas: 1, AutopilotCPURequest: "500m", AutopilotMemoryRequest: "1Gi"},
		Dependencies: gcpstack.Dependencies{DatabaseName: "magento", MasterUsername: "magento"},
	}
}

func eksIdentitySpec(digest string) eksops.Spec {
	return eksops.Spec{
		Identity: eksops.Identity{
			Project: "shop", Environment: "preview", AccountID: "123456789012", Region: "eu-west-3",
			EnvironmentClass: "preview", Preset: sdk.PresetPreview,
			Tags: map[string]string{"magelift:managed-by": "magelift"},
		},
		Application: eksops.Application{Edition: "open-source", Version: "2.4.8", Mode: "integrated", WebRuntime: "nginx-fpm"},
		Artifact:    eksops.Artifact{ImageDigest: digest},
		Policy: eksops.NetworkPolicy{
			VPCCIDR: netip.MustParsePrefix("10.42.0.0/16"), AvailabilityZones: []string{"eu-west-3a", "eu-west-3b"}, NatMode: eksops.NatModeFckNat,
		},
		Catalog: eksops.CatalogSelection{
			DatabaseEngine: eksops.DatabaseEngineRDSMySQL, InstanceClass: "db.t4g.micro", InstanceCount: 1,
			ValkeyNodeType: "cache.t4g.micro", ValkeyReplicaCount: 0,
			CPURequest: "500m", MemoryRequest: "1Gi", DesiredWebReplicas: 1,
			BackupDays: 1, MySQLVersion: "8.4.10", ValkeyVersion: "8.1",
		},
		Dependencies: eksops.Dependencies{
			KMSKeyARN:        "arn:aws:kms:eu-west-3:123456789012:key/00000000-0000-0000-0000-000000000000",
			CacheSecretARN:   "arn:aws:secretsmanager:eu-west-3:123456789012:secret:cache",
			EncryptionKeyARN: "arn:aws:secretsmanager:eu-west-3:123456789012:secret:crypt",
			DatabaseName:     "magento", MasterUsername: "magento",
		},
	}
}

func ovhIdentitySpec(digest string) ovhstack.Spec {
	return ovhstack.Spec{
		Identity: ovhstack.Identity{
			Project: "shop", ServiceName: "xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx", Environment: "preview",
			Region: "GRA9", EnvironmentClass: "preview", Preset: "preview",
			Labels: map[string]string{"magelift-managed-by": "magelift"},
		},
		Application:  ovhstack.Application{Edition: "open-source", Version: "2.4.8", Mode: "integrated", WebRuntime: "nginx-fpm"},
		Artifact:     ovhstack.Artifact{ImageDigest: digest},
		Policy:       ovhstack.NetworkPolicy{NetworkCIDR: "10.30.0.0/16", Zones: []string{"GRA9"}},
		Catalog:      ovhstack.CatalogSelection{DatabaseFlavor: "db1-4", DatabasePlan: "essential", ValkeyFlavor: "db1-4", ValkeyPlan: "essential", NodeFlavor: "b3-8", NodeCount: 1, CPURequest: "500m", MemoryRequest: "1Gi", DesiredWebReplicas: 1},
		Dependencies: ovhstack.Dependencies{DatabaseName: "magento", MasterUsername: "magento"},
	}
}

func scwIdentitySpec(digest string) scwstack.Spec {
	return scwstack.Spec{
		Identity: scwstack.Identity{
			Project: "shop", ScalewayProject: "11111111-1111-1111-1111-111111111111", Environment: "preview",
			Region: "fr-par", Zone: "fr-par-1", EnvironmentClass: "preview", Preset: "preview",
			Labels: map[string]string{"magelift-managed-by": "magelift"},
		},
		Application: scwstack.Application{Edition: "open-source", Version: "2.4.8", Mode: "integrated", WebRuntime: "nginx-fpm"},
		Artifact:    scwstack.Artifact{ImageDigest: digest},
		Policy:      scwstack.NetworkPolicy{NetworkCIDR: "172.16.0.0/22", Zones: []string{"fr-par-1"}},
		Catalog: scwstack.CatalogSelection{
			DatabaseNodeType: "DB-DEV-S", RedisNodeType: "RED1-MICRO", CacheMode: "redis",
			KapsuleVersion: "1.29.1", NodeType: "DEV1-M", NodeCount: 2,
			CPURequest: "500m", MemoryRequest: "1Gi", DesiredWebReplicas: 1,
		},
		Dependencies: scwstack.Dependencies{DatabaseName: "magento", MasterUsername: "magento"},
	}
}
