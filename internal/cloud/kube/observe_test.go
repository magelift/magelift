package kube

import (
	"context"
	"strings"
	"testing"

	"github.com/acourtiol/magelift/internal/platform"
	sdk "github.com/acourtiol/magelift/sdk/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

type observePlanned struct {
	project, env string
}

func (p observePlanned) StackName() string                          { return p.project + "-" + p.env }
func (p observePlanned) Provider() sdk.ProviderID                   { return "gcp" }
func (p observePlanned) Runtime() sdk.RuntimeID                     { return "gke-autopilot" }
func (p observePlanned) Project() string                            { return p.project }
func (p observePlanned) Environment() string                        { return p.env }
func (p observePlanned) Region() string                             { return "europe-west9" }
func (p observePlanned) CertificationTier() platform.CertificationTier {
	return platform.TierCertified
}
func (p observePlanned) EnvironmentClass() string                   { return "preview" }
func (p observePlanned) Protected() bool                            { return false }
func (p observePlanned) ImageDigest() string                        { return "" }
func (p observePlanned) WithImageDigest(string) (platform.PlannedStack, error) {
	return p, nil
}
func (p observePlanned) TargetDescriptor() sdk.TargetDescriptor {
	return sdk.TargetDescriptor{Provider: "gcp", Runtime: "gke-autopilot"}
}

func TestObserveTailLogsFakeClientset(t *testing.T) {
	deployment := "shop-preview-app-web"
	cs := fake.NewClientset(&corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "web-0",
			Namespace: "default",
			Labels:    map[string]string{"app": deployment},
		},
	})
	obs := NewObserve(cs)
	events, err := obs.TailLogs(context.Background(), observePlanned{project: "shop", env: "preview"}, platform.LogQuery{
		Limit: 10,
	})
	if err != nil {
		t.Fatalf("TailLogs: %v", err)
	}
	if len(events) == 0 {
		t.Fatal("TailLogs returned no events")
	}
	if !strings.Contains(events[0].Message, "fake logs") {
		t.Fatalf("message = %q, want fake logs", events[0].Message)
	}
}

func TestObserveTailLogsEmptyPods(t *testing.T) {
	obs := NewObserve(fake.NewClientset())
	_, err := obs.TailLogs(context.Background(), observePlanned{project: "shop", env: "preview"}, platform.LogQuery{
		Workload: "missing",
		Limit:    5,
	})
	if err == nil || !strings.Contains(err.Error(), "no pods found") {
		t.Fatalf("expected no pods found error, got %v", err)
	}
}

func TestGCPRuntimeObserveTypeIdentity(t *testing.T) {
	// Imported via blank? Avoid cycle: assert NewObserveWithFactory shape used by GCP collapse.
	obs := NewObserveWithFactory(ClientFromOutputs)
	if _, ok := any(obs).(platform.RuntimeObserve); !ok {
		t.Fatal("Observe must implement platform.RuntimeObserve")
	}
	if _, ok := any(obs).(*Observe); !ok {
		t.Fatalf("want *Observe, got %T", obs)
	}
}
