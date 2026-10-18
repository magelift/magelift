package platform_test

import (
	"context"
	"errors"
	"io"
	"testing"

	awsops "github.com/magelift/magelift/internal/cloud/aws/ops"
	"github.com/magelift/magelift/internal/platform"
	"github.com/magelift/magelift/internal/providerhost"
)

func TestModuleOpsAWSAndGCP(t *testing.T) {
	t.Parallel()
	awsOps := platform.ModuleOps(awsops.Module{})
	if awsOps == nil {
		t.Fatal("AWS module must expose Ops")
	}
	gcp, err := providerhost.NewShimModule("gke-autopilot", providerhost.NewLazyClient(func(context.Context) (*providerhost.Client, error) {
		return nil, errors.New("must not dial")
	}))
	if err != nil {
		t.Fatal(err)
	}
	gcpOps := platform.ModuleOps(gcp)
	if gcpOps == nil {
		t.Fatal("GCP module must expose Ops")
	}
	if platform.ModuleBootstrap(gcp) == nil {
		t.Fatal("GCP module must expose Bootstrap")
	}
	if platform.ModuleState(gcp) == nil {
		t.Fatal("GCP module must expose State")
	}
	if platform.ModuleSecrets(gcp) == nil {
		t.Fatal("GCP module must expose Secrets")
	}
	if platform.ModuleRuntimeObserve(gcp) == nil {
		t.Fatal("GCP module must expose RuntimeObserve")
	}
	_, err = gcpOps.NewDeploySteps(context.Background(), nil, nil, io.Discard)
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
