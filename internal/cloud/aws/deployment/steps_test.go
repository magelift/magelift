package deployment

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/acourtiol/magelift/internal/automation"
	awsoperations "github.com/acourtiol/magelift/internal/cloud/aws/operations"
)

type stepsBackend struct {
	outputs map[string]any
}

func (b stepsBackend) Outputs(context.Context) (map[string]any, error) { return b.outputs, nil }
func (stepsBackend) Preview(context.Context, automation.Request, io.Writer) (map[string]int, error) {
	return map[string]int{}, nil
}
func (stepsBackend) Update(context.Context, automation.Request, io.Writer) (map[string]int, error) {
	return map[string]int{}, nil
}
func (stepsBackend) Destroy(context.Context, automation.Request, io.Writer) (map[string]int, error) {
	return map[string]int{}, nil
}

type stepsRuntime struct{}

func (stepsRuntime) Check(context.Context, string, string) (awsoperations.ServiceHealth, error) {
	return awsoperations.ServiceHealth{DesiredCount: 1, RunningCount: 1, PrimaryRollout: "COMPLETED"}, nil
}

func TestRequiredOutputDecoding(t *testing.T) {
	outputs := map[string]any{"name": "shop", "subnets": []any{"subnet-a", "subnet-b"}}
	if got, err := requiredString(outputs, "name"); err != nil || got != "shop" {
		t.Fatalf("requiredString = %q, %v", got, err)
	}
	if got, err := requiredStrings(outputs, "subnets"); err != nil || strings.Join(got, ",") != "subnet-a,subnet-b" {
		t.Fatalf("requiredStrings = %v, %v", got, err)
	}
	if _, err := requiredString(outputs, "missing"); err == nil {
		t.Fatal("missing output was accepted")
	}
}

func TestWaitForHealthyServiceUsesRuntimeEvidence(t *testing.T) {
	steps := &Steps{backend: stepsBackend{outputs: map[string]any{"clusterName": "shop-cluster", "serviceName": "shop-web-service"}}, runtime: stepsRuntime{}, waitInterval: time.Millisecond, waitTimeout: time.Second}
	if err := steps.waitForHealthyService(context.Background()); err != nil {
		t.Fatal(err)
	}
}
