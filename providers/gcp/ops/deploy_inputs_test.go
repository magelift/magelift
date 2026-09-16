package ops

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/magelift/magelift/internal/cloud/kube"
	gcpstack "github.com/magelift/magelift/providers/gcp/stack"
	"github.com/magelift/magelift/sdk"
)

func TestBuildDeployInputsMapsSpec(t *testing.T) {
	t.Parallel()
	data, err := BuildDeployInputs(gcpTestSpec())
	if err != nil {
		t.Fatal(err)
	}
	var decoded kube.DeploySpec
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	spec := gcpTestSpec()
	if decoded.ImageDigest != spec.Artifact.ImageDigest || decoded.DatabaseName != "magento" {
		t.Fatalf("identity inputs were not mapped: %#v", decoded)
	}
	if decoded.ApplicationMode != "integrated" || decoded.ApplicationVersion != "2.4.8" || decoded.WebRuntime != "nginx-fpm" {
		t.Fatalf("application inputs were not mapped: %#v", decoded)
	}
	if decoded.CPURequest != "500m" || decoded.MemoryRequest != "1Gi" {
		t.Fatalf("workload inputs were not mapped: %#v", decoded)
	}
	if decoded.CloudProject != "example-gcp-project" || decoded.Region != "europe-west1" {
		t.Fatalf("project inputs were not mapped: %#v", decoded)
	}
}

func gcpTestSpec() gcpstack.Spec {
	return gcpstack.Spec{
		Identity: gcpstack.Identity{
			Project: "shop", GCPProject: "example-gcp-project", Environment: "preview",
			Region: "europe-west1", EnvironmentClass: "preview", Preset: sdk.PresetPreview,
		},
		Application:  gcpstack.Application{Edition: "open-source", Version: "2.4.8", Mode: "integrated", WebRuntime: "nginx-fpm"},
		Artifact:     gcpstack.Artifact{ImageDigest: "ghcr.io/magelift/magento@sha256:" + strings.Repeat("a", 64)},
		Policy:       gcpstack.NetworkPolicy{NetworkCIDR: "10.20.0.0/16", Zones: []string{"europe-west1-b"}},
		Catalog:      gcpstack.CatalogSelection{DesiredWebReplicas: 1, AutopilotCPURequest: "500m", AutopilotMemoryRequest: "1Gi"},
		Dependencies: gcpstack.Dependencies{DatabaseName: "magento", MasterUsername: "magento", EncryptionKeySecret: "magento-crypt-key"},
	}
}
