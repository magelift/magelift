package stack

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"reflect"
	"strings"
	"testing"

	"github.com/acourtiol/magelift/internal/automation"
	"github.com/acourtiol/magelift/internal/cloud/kube"
	"github.com/acourtiol/magelift/internal/config"
	"github.com/acourtiol/magelift/internal/platform"
)

// Eleven unsupported day-2 methods on the experimental Scaleway shell after shared
// Observe + Steps moved to kube. Parallel to the OVH table — duplicated deliberately
// to avoid a cross-adapter helper.
func TestUnsupportedMethodsReturnSentinelAndZeroValues(t *testing.T) {
	ctx := context.Background()
	u := unsupported{}

	type caseResult struct {
		err  error
		zero bool
	}

	cases := []struct {
		name string
		call func() caseResult
	}{
		{
			name: "VerifyAccount",
			call: func() caseResult {
				return caseResult{err: u.VerifyAccount(ctx, nil), zero: true}
			},
		},
		{
			name: "Ensure",
			call: func() caseResult {
				got, err := u.Ensure(ctx, nil, platform.BootstrapRequest{})
				return caseResult{err: err, zero: reflect.ValueOf(got).IsZero()}
			},
		},
		{
			name: "Status",
			call: func() caseResult {
				locked, info, backend, err := u.Status(ctx, nil)
				return caseResult{err: err, zero: !locked && info == nil && backend == ""}
			},
		},
		{
			name: "Lock",
			call: func() caseResult {
				release, err := u.Lock(ctx, nil, "owner")
				return caseResult{err: err, zero: release == nil}
			},
		},
		{
			name: "Unlock",
			call: func() caseResult {
				info, err := u.Unlock(ctx, nil)
				return caseResult{err: err, zero: info == nil}
			},
		},
		{
			name: "Backup",
			call: func() caseResult {
				got, err := u.Backup(ctx, nil)
				return caseResult{err: err, zero: got == (platform.BackupResult{})}
			},
		},
		{
			name: "Restore",
			call: func() caseResult {
				got, err := u.Restore(ctx, nil, "loc")
				return caseResult{err: err, zero: got == (platform.RestoreResult{})}
			},
		},
		{
			name: "List",
			call: func() caseResult {
				got, err := u.List(ctx, nil)
				return caseResult{err: err, zero: got == nil}
			},
		},
		{
			name: "Set",
			call: func() caseResult {
				return caseResult{err: u.Set(ctx, nil, "k", nil), zero: true}
			},
		},
		{
			name: "Remove",
			call: func() caseResult {
				return caseResult{err: u.Remove(ctx, nil, "k"), zero: true}
			},
		},
		{
			name: "Estimate",
			call: func() caseResult {
				got, err := u.Estimate(ctx, nil, config.Config{}, platform.CostOptions{})
				return caseResult{err: err, zero: reflect.ValueOf(got).IsZero()}
			},
		},
	}

	if len(cases) != 11 {
		t.Fatalf("unsupported shell requires exactly 11 methods after Observe+Steps moved to kube; got %d", len(cases))
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := tc.call()
			if !errors.Is(result.err, platform.ErrNotSupported) {
				t.Fatalf("err = %v, want ErrNotSupported", result.err)
			}
			if !result.zero {
				t.Fatalf("%s returned a non-zero value alongside ErrNotSupported", tc.name)
			}
		})
	}
}

func TestModuleAccessorsReturnNonNilUnsupportedShells(t *testing.T) {
	m := Module{}
	if m.Bootstrap() == nil {
		t.Fatal("Bootstrap must return the unsupported shell, not nil")
	}
	if m.State() == nil {
		t.Fatal("State must return a non-nil adapter, not nil")
	}
	if m.Secrets() == nil {
		t.Fatal("Secrets must return the unsupported shell, not nil")
	}
	if m.RuntimeObserve() == nil {
		t.Fatal("RuntimeObserve must return shared kube.Observe, not nil")
	}
	if _, ok := m.RuntimeObserve().(*kube.Observe); !ok {
		t.Fatalf("RuntimeObserve type identity: want *kube.Observe, got %T", m.RuntimeObserve())
	}
	if m.CostEstimator() == nil {
		t.Fatal("CostEstimator must return the unsupported shell, not nil")
	}
}

// TestUnsupportedSourceGuardSentinelReturns parses ops.go and requires every
// method on unsupported to return platform.ErrNotSupported in the error
// position. Ops.AcquireLock is covered by TestAcquireLockWarnsNoDIYLockTaken
// (warn-then-noop), not by this ErrNotSupported walk.
func TestUnsupportedSourceGuardSentinelReturns(t *testing.T) {
	guardUnsupportedSentinelReturns(t, "ops.go")
}

func guardUnsupportedSentinelReturns(t *testing.T, sourceFile string) {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, sourceFile, nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v (guard must fail closed on parse errors)", sourceFile, err)
	}

	var failures []string
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Recv == nil || len(fn.Recv.List) != 1 || fn.Body == nil {
			continue
		}
		recvType := receiverTypeName(fn.Recv.List[0].Type)
		if recvType != "unsupported" {
			continue
		}
		if !methodReturnsNotSupported(fn) {
			failures = append(failures, fmt.Sprintf(
				"%s.%s no longer returns platform.ErrNotSupported in the error position; restore the sentinel or remove this method from the unsupported sentinel guard deliberately",
				recvType, fn.Name.Name,
			))
		}
	}
	if len(failures) > 0 {
		t.Fatalf("sentinel guard failed:\n  - %s", strings.Join(failures, "\n  - "))
	}
}

func receiverTypeName(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		if id, ok := t.X.(*ast.Ident); ok {
			return id.Name
		}
	}
	return ""
}

func methodReturnsNotSupported(fn *ast.FuncDecl) bool {
	sawReturn := false
	allGood := true
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		ret, ok := n.(*ast.ReturnStmt)
		if !ok || len(ret.Results) == 0 {
			return true
		}
		sawReturn = true
		if !isPlatformErrNotSupported(ret.Results[len(ret.Results)-1]) {
			allGood = false
			return false
		}
		return true
	})
	return sawReturn && allGood
}

func isPlatformErrNotSupported(expr ast.Expr) bool {
	sel, ok := expr.(*ast.SelectorExpr)
	if !ok || sel.Sel == nil || sel.Sel.Name != "ErrNotSupported" {
		return false
	}
	pkg, ok := sel.X.(*ast.Ident)
	return ok && pkg.Name == "platform"
}

func TestAcquireLockWarnsNoDIYLockTaken(t *testing.T) {
	var buf bytes.Buffer
	prev := diyLockWarnOut
	diyLockWarnOut = &buf
	t.Cleanup(func() { diyLockWarnOut = prev })

	release, err := Ops{}.AcquireLock(context.Background(), Planned{})
	if err != nil {
		t.Fatalf("AcquireLock error: %v", err)
	}
	if release == nil {
		t.Fatal("AcquireLock must return a noop release")
	}
	if err := release(context.Background()); err != nil {
		t.Fatalf("noop release: %v", err)
	}
	msg := buf.String()
	lower := strings.ToLower(msg)
	if !strings.Contains(lower, "lock") || !strings.Contains(msg, "DIY") || !strings.Contains(lower, "not taken") {
		t.Fatalf("expected warning that no DIY lock was taken, got %q", msg)
	}
	if !strings.Contains(msg, "scaleway") && !strings.Contains(msg, "kapsule") {
		t.Fatalf("expected provider/runtime context in warning, got %q", msg)
	}
}

func TestNewDeployStepsTypeIdentity(t *testing.T) {
	ops := Ops{
		NewCandidate: func(context.Context, kube.Backend) (kube.CandidateRunner, error) {
			return stubCandidate{}, nil
		},
		NewRuntime: func(context.Context, kube.Backend) (kube.RuntimeChecker, error) {
			return stubRuntime{}, nil
		},
	}
	steps, err := ops.NewDeploySteps(context.Background(), &stubBackend{}, Planned{Spec: scwDeploySpec()}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := steps.(*kube.Steps); !ok {
		t.Fatalf("want *kube.Steps, got %T", steps)
	}
	if _, err := (Ops{}).NewDeploySteps(context.Background(), struct{}{}, Planned{Spec: scwDeploySpec()}, io.Discard); err == nil || !strings.Contains(err.Error(), "backend with outputs") {
		t.Fatalf("expected wrong-backend error, got %v", err)
	}
}

type stubCandidate struct{}

func (stubCandidate) RegisterCandidate(context.Context, kube.CandidateRequest) (kube.Candidate, error) {
	return kube.Candidate{}, nil
}
func (stubCandidate) RunMigrations(context.Context, kube.Candidate) error { return nil }
func (stubCandidate) Cleanup(context.Context, kube.Candidate) error       { return nil }

type stubRuntime struct{}

func (stubRuntime) Check(context.Context, string, string) (kube.ServiceHealth, error) {
	return kube.ServiceHealth{DesiredReplicas: 1, ReadyReplicas: 1, Available: true}, nil
}

type stubBackend struct{}

func (*stubBackend) Outputs(context.Context) (map[string]any, error) { return map[string]any{}, nil }
func (*stubBackend) Preview(context.Context, automation.Request, io.Writer) (map[string]int, error) {
	return map[string]int{}, nil
}
func (*stubBackend) Update(context.Context, automation.Request, io.Writer) (map[string]int, error) {
	return map[string]int{}, nil
}
func (*stubBackend) Destroy(context.Context, automation.Request, io.Writer) (map[string]int, error) {
	return map[string]int{}, nil
}

func scwDeploySpec() Spec {
	return Spec{
		Identity: Identity{
			Project: "shop", ScalewayProject: "11111111-1111-1111-1111-111111111111", Environment: "preview",
			Region: "fr-par", Zone: "fr-par-1", EnvironmentClass: "preview", Preset: "preview",
			Labels: map[string]string{"magelift-managed-by": "magelift"},
		},
		Application: Application{Edition: "open-source", Version: "2.4.8", Mode: "integrated", WebRuntime: "nginx-fpm"},
		Artifact:    Artifact{ImageDigest: "ghcr.io/acourtiol/magento@sha256:abcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcd"},
		Policy:      NetworkPolicy{NetworkCIDR: "172.16.0.0/22", Zones: []string{"fr-par-1"}},
		Catalog: CatalogSelection{
			DatabaseNodeType: "DB-DEV-S", RedisNodeType: "RED1-MICRO", CacheMode: "redis",
			KapsuleVersion: "1.29.1", NodeType: "DEV1-M", NodeCount: 2,
			CPURequest: "500m", MemoryRequest: "1Gi", DesiredWebReplicas: 1,
		},
		Dependencies: Dependencies{DatabaseName: "magento", MasterUsername: "magento"},
	}
}
