package stack

import (
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

	"github.com/magelift/magelift/internal/automation"
	"github.com/magelift/magelift/internal/cloud/kube"
	"github.com/magelift/magelift/internal/config"
	"github.com/magelift/magelift/internal/platform"
)

// Remaining unsupported allowlist after Observe+Steps+State moved off the shell
// (D-05): Bootstrap VerifyAccount/Ensure, Secrets List/Set/Remove, Cost Estimate.
func TestUnsupportedAllowlistMethodsReturnSentinelAndZeroValues(t *testing.T) {
	ctx := context.Background()
	u := unsupported{}

	type caseResult struct {
		err  error
		zero bool // true when all non-error returns are zero values
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

	if len(cases) != 6 {
		t.Fatalf("unsupported allowlist requires exactly 6 methods (Bootstrap/Secrets/Cost); got %d", len(cases))
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

func TestSharedDay2PortsAreNotErrNotSupported(t *testing.T) {
	m := Module{}
	if _, ok := m.State().(State); !ok {
		t.Fatalf("State type = %T, want stack.State", m.State())
	}
	obs := m.RuntimeObserve()
	if obs == nil {
		t.Fatal("RuntimeObserve must return shared kube.Observe, not nil")
	}
	if _, ok := obs.(*kube.Observe); !ok {
		t.Fatalf("RuntimeObserve type identity: want *kube.Observe, got %T", obs)
	}
	if _, err := obs.TailLogs(context.Background(), nil, platform.LogQuery{}); errors.Is(err, platform.ErrNotSupported) {
		t.Fatal("RuntimeObserve must not return ErrNotSupported from TailLogs")
	}
}

func TestBootstrapSecretsNilSuccessGuards(t *testing.T) {
	m := Module{}
	boot := m.Bootstrap()
	if boot == nil {
		t.Fatal("Bootstrap must not be nil")
	}
	got, err := boot.Ensure(context.Background(), nil, platform.BootstrapRequest{})
	if !errors.Is(err, platform.ErrNotSupported) {
		t.Fatalf("Bootstrap.Ensure err = %v, want ErrNotSupported", err)
	}
	if !reflect.ValueOf(got).IsZero() {
		t.Fatal("Bootstrap.Ensure must not nil-succeed with a non-zero result")
	}
	sec := m.Secrets()
	if sec == nil {
		t.Fatal("Secrets must not be nil")
	}
	list, err := sec.List(context.Background(), nil)
	if !errors.Is(err, platform.ErrNotSupported) {
		t.Fatalf("Secrets.List err = %v, want ErrNotSupported", err)
	}
	if list != nil {
		t.Fatal("Secrets.List must not nil-succeed with a non-nil slice")
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
// position. Ops.AcquireLock delegates to State.Lock (not this shell).
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

func TestAcquireLockDelegatesToState(t *testing.T) {
	_, err := Ops{}.AcquireLock(context.Background(), Planned{Spec: Spec{
		Identity: Identity{Project: "shop", Environment: "staging", Region: "GRA9"},
	}})
	if err == nil {
		t.Fatal("AcquireLock succeeded without state bucket — must call State.Lock")
	}
	if errors.Is(err, platform.ErrNotSupported) {
		t.Fatal("AcquireLock must not return ErrNotSupported once State is wired")
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
	steps, err := ops.NewDeploySteps(context.Background(), &stubBackend{}, Planned{Spec: ovhDeploySpec()}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := steps.(*kube.Steps); !ok {
		t.Fatalf("want *kube.Steps, got %T", steps)
	}
	if errors.Is(err, platform.ErrNotSupported) {
		t.Fatal("NewDeploySteps must not return ErrNotSupported")
	}
	if _, err := (Ops{}).NewDeploySteps(context.Background(), struct{}{}, Planned{Spec: ovhDeploySpec()}, io.Discard); err == nil || !strings.Contains(err.Error(), "backend with outputs") {
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

func ovhDeploySpec() Spec {
	return Spec{
		Identity: Identity{
			Project: "shop", ServiceName: "xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx", Environment: "preview",
			Region: "GRA9", EnvironmentClass: "preview", Preset: "preview",
			Labels: map[string]string{"magelift-managed-by": "magelift"},
		},
		Application:  Application{Edition: "open-source", Version: "2.4.8", Mode: "integrated", WebRuntime: "nginx-fpm"},
		Artifact:     Artifact{ImageDigest: "ghcr.io/magelift/magento@sha256:abcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcd"},
		Policy:       NetworkPolicy{NetworkCIDR: "10.30.0.0/16", Zones: []string{"GRA9"}},
		Catalog:      CatalogSelection{DatabaseFlavor: "db1-4", DatabasePlan: "essential", ValkeyFlavor: "db1-4", ValkeyPlan: "essential", NodeFlavor: "b3-8", NodeCount: 1, CPURequest: "500m", MemoryRequest: "1Gi", DesiredWebReplicas: 1},
		Dependencies: Dependencies{DatabaseName: "magento", MasterUsername: "magento"},
	}
}
