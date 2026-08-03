package platform_test

import (
	"context"
	"io"
	"testing"

	awsops "github.com/magelift/magelift/internal/cloud/aws/ops"
	gcpops "github.com/magelift/magelift/internal/cloud/gcp/ops"
	"github.com/magelift/magelift/internal/platform"
)

func TestModuleOpsAWSAndGCP(t *testing.T) {
	t.Parallel()
	awsOps := platform.ModuleOps(awsops.Module{})
	if awsOps == nil {
		t.Fatal("AWS module must expose Ops")
	}
	gcpOps := platform.ModuleOps(gcpops.Module{})
	if gcpOps == nil {
		t.Fatal("GCP module must expose Ops")
	}
	if platform.ModuleBootstrap(gcpops.Module{}) == nil {
		t.Fatal("GCP module must expose Bootstrap")
	}
	if platform.ModuleState(gcpops.Module{}) == nil {
		t.Fatal("GCP module must expose State")
	}
	if platform.ModuleSecrets(gcpops.Module{}) == nil {
		t.Fatal("GCP module must expose Secrets")
	}
	if platform.ModuleRuntimeObserve(gcpops.Module{}) == nil {
		t.Fatal("GCP module must expose RuntimeObserve")
	}
	_, err := gcpOps.NewDeploySteps(context.Background(), nil, nil, io.Discard)
	if err == nil {
		t.Fatal("GCP NewDeploySteps with nil planned should fail")
	}
}

func TestModuleOpsNil(t *testing.T) {
	t.Parallel()
	if platform.ModuleOps(nil) != nil {
		t.Fatal("nil module should not expose Ops")
	}
}
