package certification

import (
	"context"
	"errors"
	"strings"
	"testing"
)

const (
	testCurrentDigest = "ghcr.io/magelift/magento@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	testFaultDigest   = "registry.example.invalid/magelift@sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
)

type fakeFailedDeploymentRuntime struct {
	applied []string
	health  map[string]bool
	applyFn func(digest string) error
}

func (f *fakeFailedDeploymentRuntime) ApplyDigest(_ context.Context, digest string) error {
	f.applied = append(f.applied, digest)
	if f.applyFn != nil {
		return f.applyFn(digest)
	}
	return nil
}

func (f *fakeFailedDeploymentRuntime) Healthy(_ context.Context) (bool, error) {
	if len(f.applied) == 0 {
		return false, errors.New("no digest applied")
	}
	digest := f.applied[len(f.applied)-1]
	healthy, ok := f.health[digest]
	if !ok {
		return false, nil
	}
	return healthy, nil
}

func TestInjectFailedDeploymentRequiresUnhealthyFault(t *testing.T) {
	runtime := &fakeFailedDeploymentRuntime{health: map[string]bool{testFaultDigest: false, testCurrentDigest: true}}
	injection, err := InjectFailedDeployment(context.Background(), runtime, FailedDeploymentRequest{
		CurrentDigest: testCurrentDigest, FaultDigest: testFaultDigest,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := injection.Validate(); err != nil {
		t.Fatal(err)
	}
	if !injection.Verified || injection.ID != "failed-deployment" {
		t.Fatalf("injection = %#v", injection)
	}
	if got := runtime.applied; len(got) != 1 || got[0] != testFaultDigest {
		t.Fatalf("applied = %#v", got)
	}
}

func TestInjectFailedDeploymentAcceptsApplyErrorWhenUnhealthy(t *testing.T) {
	runtime := &fakeFailedDeploymentRuntime{
		health:  map[string]bool{testFaultDigest: false},
		applyFn: func(string) error { return errors.New("runtime health failed") },
	}
	if _, err := InjectFailedDeployment(context.Background(), runtime, FailedDeploymentRequest{
		CurrentDigest: testCurrentDigest, FaultDigest: testFaultDigest,
	}); err != nil {
		t.Fatal(err)
	}
}

func TestInjectFailedDeploymentRejectsHealthyFault(t *testing.T) {
	runtime := &fakeFailedDeploymentRuntime{health: map[string]bool{testFaultDigest: true}}
	_, err := InjectFailedDeployment(context.Background(), runtime, FailedDeploymentRequest{
		CurrentDigest: testCurrentDigest, FaultDigest: testFaultDigest,
	})
	if err == nil || !strings.Contains(err.Error(), "became healthy") {
		t.Fatalf("error = %v", err)
	}
}

func TestRestoreFailedDeploymentRequiresHealthyCurrent(t *testing.T) {
	runtime := &fakeFailedDeploymentRuntime{health: map[string]bool{testFaultDigest: false, testCurrentDigest: true}}
	if _, err := InjectFailedDeployment(context.Background(), runtime, FailedDeploymentRequest{
		CurrentDigest: testCurrentDigest, FaultDigest: testFaultDigest,
	}); err != nil {
		t.Fatal(err)
	}
	if err := RestoreFailedDeployment(context.Background(), runtime, FailedDeploymentRequest{
		CurrentDigest: testCurrentDigest, FaultDigest: testFaultDigest,
	}); err != nil {
		t.Fatal(err)
	}
	if got := runtime.applied; len(got) != 2 || got[1] != testCurrentDigest {
		t.Fatalf("applied = %#v", got)
	}
}

func TestValidateFailedDeploymentRequestRejectsUnsafeDigests(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		current string
		fault   string
		wantErr string
	}{
		{name: "identical", current: testCurrentDigest, fault: testCurrentDigest, wantErr: "must differ"},
		{name: "mutable tag", current: testCurrentDigest, fault: "ghcr.io/magelift/magento:latest", wantErr: "immutable"},
		{name: "credentials", current: "user:pass@ghcr.io/magelift/magento@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", fault: testFaultDigest, wantErr: "credentials"},
		{name: "newline", current: testCurrentDigest + "\n", fault: testFaultDigest, wantErr: "single-line"},
		{name: "empty fault", current: testCurrentDigest, wantErr: "required"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			err := ValidateFailedDeploymentRequest(FailedDeploymentRequest{CurrentDigest: test.current, FaultDigest: test.fault})
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("error = %v, want %q", err, test.wantErr)
			}
		})
	}
}
