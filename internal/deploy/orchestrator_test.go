package deploy

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/magelift/magelift/internal/automation"
	"github.com/magelift/magelift/sdk"
)

type fakeLock struct {
	order           *[]string
	owned           bool
	releaseCanceled bool
}

func (l *fakeLock) Acquire(context.Context, Request) (func(context.Context) error, error) {
	*l.order = append(*l.order, "lock.acquire")
	l.owned = true
	return func(ctx context.Context) error {
		*l.order = append(*l.order, "lock.release")
		l.releaseCanceled = ctx.Err() != nil
		l.owned = false
		return nil
	}, nil
}

type fakeSteps struct {
	order *[]string
	fail  string
}

type fakeHook struct {
	descriptor sdk.LifecycleHookDescriptor
	order      *[]string
	err        error
}

func (h fakeHook) Descriptor() sdk.LifecycleHookDescriptor                { return h.descriptor }
func (fakeHook) Validate(context.Context, sdk.LifecycleHookRequest) error { return nil }
func (h fakeHook) Run(context.Context, sdk.LifecycleHookRequest) error {
	*h.order = append(*h.order, "hook."+string(h.descriptor.ID))
	return h.err
}

func (s *fakeSteps) step(name string) error {
	*s.order = append(*s.order, name)
	if s.fail == name {
		return errors.New(name + " failed")
	}
	return nil
}
func (s *fakeSteps) Validate(context.Context, Request) error { return s.step("validate") }
func (s *fakeSteps) Preview(context.Context, Request) (automation.ChangeSummary, error) {
	if err := s.step("preview"); err != nil {
		return automation.ChangeSummary{}, err
	}
	return automation.ChangeSummary{Total: 1}, nil
}
func (s *fakeSteps) RegisterCandidate(context.Context, Request) error { return s.step("candidate") }
func (s *fakeSteps) RunMigrations(context.Context, Request) error     { return s.step("migrate") }
func (s *fakeSteps) CleanupCandidate(context.Context, Request) error  { return s.step("cleanup") }
func (s *fakeSteps) UpdateServices(context.Context, Request) (automation.ChangeSummary, error) {
	if err := s.step("update"); err != nil {
		return automation.ChangeSummary{}, err
	}
	return automation.ChangeSummary{Total: 2}, nil
}
func (s *fakeSteps) Stabilize(context.Context, Request) error      { return s.step("stabilize") }
func (s *fakeSteps) Health(context.Context, Request) error         { return s.step("health") }
func (s *fakeSteps) Record(context.Context, Request, Result) error { return s.step("record") }

func TestRunEnforcesSafeOrderAndReleasesLock(t *testing.T) {
	order := []string{}
	lock := &fakeLock{order: &order}
	steps := &fakeSteps{order: &order}
	result, err := New(lock, steps).Run(context.Background(), validRequest())
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"validate", "lock.acquire", "preview", "candidate", "migrate", "update", "stabilize", "health", "record", "cleanup", "lock.release"}
	if !reflect.DeepEqual(order, want) {
		t.Fatalf("order = %v, want %v", order, want)
	}
	if result.Preview.Total != 1 || result.Update.Total != 2 || lock.owned {
		t.Fatalf("result or lock state is incorrect: %#v", result)
	}
}

func TestFailureStillReleasesLock(t *testing.T) {
	order := []string{}
	_, err := New(&fakeLock{order: &order}, &fakeSteps{order: &order, fail: "stabilize"}).Run(context.Background(), validRequest())
	if err == nil || order[len(order)-2] != "cleanup" || order[len(order)-1] != "lock.release" {
		t.Fatalf("error or release order is incorrect: %v %v", err, order)
	}
}

func TestRunJoinsPrimaryErrorWithLockReleaseFailure(t *testing.T) {
	order := []string{}
	releaseErr := errors.New("unlock failed")
	lock := &fakeLock{order: &order}
	lockFail := &failingReleaseLock{fakeLock: lock, releaseErr: releaseErr}
	_, err := New(lockFail, &fakeSteps{order: &order, fail: "stabilize"}).Run(context.Background(), validRequest())
	if err == nil {
		t.Fatal("expected joined error")
	}
	if !strings.Contains(err.Error(), "stabilize failed") || !strings.Contains(err.Error(), "unlock failed") {
		t.Fatalf("joined error missing causes: %v", err)
	}
	if !errors.Is(err, ErrLockRelease) {
		t.Fatalf("expected ErrLockRelease in chain, got %v", err)
	}
}

type failingReleaseLock struct {
	*fakeLock
	releaseErr error
}

func (l *failingReleaseLock) Acquire(ctx context.Context, request Request) (func(context.Context) error, error) {
	release, err := l.fakeLock.Acquire(ctx, request)
	if err != nil {
		return nil, err
	}
	return func(ctx context.Context) error {
		_ = release(ctx)
		return l.releaseErr
	}, nil
}

type cancelingSteps struct {
	*fakeSteps
	cancel context.CancelFunc
}

func (s *cancelingSteps) Preview(ctx context.Context, request Request) (automation.ChangeSummary, error) {
	s.cancel()
	return s.fakeSteps.Preview(ctx, request)
}

func TestLockReleaseSurvivesContextCancellation(t *testing.T) {
	order := []string{}
	ctx, cancel := context.WithCancel(context.Background())
	lock := &fakeLock{order: &order}
	steps := &cancelingSteps{fakeSteps: &fakeSteps{order: &order}, cancel: cancel}
	if _, err := New(lock, steps).Run(ctx, validRequest()); err != nil {
		t.Fatal(err)
	}
	if lock.releaseCanceled {
		t.Fatal("deployment lock release inherited the canceled deployment context")
	}
}

func TestCandidateCleanupRunsWhenHookFailsAfterRegistration(t *testing.T) {
	order := []string{}
	orchestrator, err := NewWithHooks(&fakeLock{order: &order}, &fakeSteps{order: &order}, []sdk.LifecycleHook{
		fakeHook{
			descriptor: sdk.LifecycleHookDescriptor{ID: "vendor.after-candidate", Phase: sdk.PhaseDeploy, Relationship: sdk.HookAfter, RelativeTo: HookTargetCandidate, Timeout: 1, MaxAttempts: 1},
			order:      &order,
			err:        errors.New("hook failed"),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := orchestrator.Run(context.Background(), validRequest()); err == nil || !strings.Contains(err.Error(), "hook failed") {
		t.Fatalf("run error = %v", err)
	}
	want := []string{"validate", "lock.acquire", "preview", "candidate", "hook.vendor.after-candidate", "cleanup", "lock.release"}
	if !reflect.DeepEqual(order, want) {
		t.Fatalf("order = %v, want %v", order, want)
	}
}

func TestProductionAndRollbackGuards(t *testing.T) {
	request := validRequest()
	request.Production = true
	if _, err := New(&fakeLock{}, &fakeSteps{}).Run(context.Background(), request); !errors.Is(err, ErrApprovalRequired) {
		t.Fatalf("approval error = %v", err)
	}
	request = validRequest()
	request.Rollback = true
	if _, err := New(&fakeLock{}, &fakeSteps{}).Run(context.Background(), request); !errors.Is(err, ErrForwardOnlyRollbackAck) {
		t.Fatalf("rollback error = %v", err)
	}
	request = validRequest()
	request.Production = true
	request.Approved = true
	if _, err := New(&fakeLock{}, &fakeSteps{}).Run(context.Background(), request); !errors.Is(err, ErrMaintenanceDrainAck) {
		t.Fatalf("maintenance-drain error = %v", err)
	}
	request.AcknowledgeMaintenanceDrain = true
	order := []string{}
	if _, err := New(&fakeLock{order: &order}, &fakeSteps{order: &order}).Run(context.Background(), request); err != nil {
		t.Fatalf("production run with attestation: %v", err)
	}
}

func TestRejectsUnpinnedDigest(t *testing.T) {
	request := validRequest()
	request.ImageDigest = "ghcr.io/example/shop:latest"
	if _, err := New(&fakeLock{}, &fakeSteps{}).Run(context.Background(), request); !errors.Is(err, ErrDigestRequired) {
		t.Fatalf("digest error = %v", err)
	}
}

func TestLifecycleHooksRunAroundStableDeploymentOperation(t *testing.T) {
	order := []string{}
	lock := &fakeLock{order: &order}
	steps := &fakeSteps{order: &order}
	hooks := []sdk.LifecycleHook{
		fakeHook{order: &order, descriptor: sdk.LifecycleHookDescriptor{ID: "vendor.before-update", Phase: sdk.PhaseDeploy, Relationship: sdk.HookBefore, RelativeTo: HookTargetUpdate, Timeout: 1, MaxAttempts: 1}},
		fakeHook{order: &order, descriptor: sdk.LifecycleHookDescriptor{ID: "vendor.after-update", Phase: sdk.PhaseDeploy, Relationship: sdk.HookAfter, RelativeTo: HookTargetUpdate, Timeout: 1, MaxAttempts: 1}},
	}
	orchestrator, err := NewWithHooks(lock, steps, hooks)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := orchestrator.Run(context.Background(), validRequest()); err != nil {
		t.Fatal(err)
	}
	want := []string{"validate", "lock.acquire", "preview", "candidate", "migrate", "hook.vendor.before-update", "update", "hook.vendor.after-update", "stabilize", "health", "record", "cleanup", "lock.release"}
	if !reflect.DeepEqual(order, want) {
		t.Fatalf("order = %v, want %v", order, want)
	}
}

func TestLifecycleHooksRejectPhaseTargetMismatchBeforeRun(t *testing.T) {
	_, err := NewWithHooks(&fakeLock{}, &fakeSteps{}, []sdk.LifecycleHook{
		fakeHook{descriptor: sdk.LifecycleHookDescriptor{ID: "vendor.invalid", Phase: sdk.PhasePostDeploy, Relationship: sdk.HookBefore, RelativeTo: HookTargetUpdate, Timeout: 1, MaxAttempts: 1}},
	})
	if err == nil || !strings.Contains(err.Error(), "expected") {
		t.Fatalf("error = %v", err)
	}
}

func validRequest() Request {
	return Request{Target: sdk.TargetDescriptor{ID: "aws.ecs-fargate", Provider: "aws", Runtime: "ecs-fargate"}, ImageDigest: "ghcr.io/example/shop@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}
}
