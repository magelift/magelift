package platform_test

import (
	"context"
	"errors"
	"io"
	"testing"

	awsops "github.com/acourtiol/magelift/internal/cloud/aws/ops"
	gcpstack "github.com/acourtiol/magelift/internal/cloud/gcp/stack"
	"github.com/acourtiol/magelift/internal/platform"
)

func TestModuleOpsAWSAndGCP(t *testing.T) {
	t.Parallel()
	awsOps := platform.ModuleOps(awsops.Module{})
	if awsOps == nil {
		t.Fatal("AWS module must expose Ops")
	}
	gcpOps := platform.ModuleOps(gcpstack.Module{})
	if gcpOps == nil {
		t.Fatal("GCP module must expose Ops")
	}
	_, err := gcpOps.NewDeploySteps(context.Background(), nil, nil, io.Discard)
	if !errors.Is(err, platform.ErrNotSupported) {
		t.Fatalf("GCP NewDeploySteps error = %v, want ErrNotSupported", err)
	}
}

func TestModuleOpsNil(t *testing.T) {
	t.Parallel()
	if platform.ModuleOps(nil) != nil {
		t.Fatal("nil module should not expose Ops")
	}
}
