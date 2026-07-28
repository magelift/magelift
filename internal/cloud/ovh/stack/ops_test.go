package stack

import (
	"context"
	"errors"
	"io"
	"reflect"
	"testing"

	"github.com/acourtiol/magelift/internal/config"
	"github.com/acourtiol/magelift/internal/platform"
)

// Fifteen unsupported day-2 methods on the experimental OVH shell (criterion 5).
// Explicit table — not reflection — so the surface is documented and countable.
func TestUnsupportedMethodsReturnSentinelAndZeroValues(t *testing.T) {
	ctx := context.Background()
	u := unsupported{}
	ops := Ops{}

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
			name: "TailLogs",
			call: func() caseResult {
				got, err := u.TailLogs(ctx, nil, platform.LogQuery{})
				return caseResult{err: err, zero: got == nil}
			},
		},
		{
			name: "CheckRuntime",
			call: func() caseResult {
				got, err := u.CheckRuntime(ctx, nil, nil)
				return caseResult{err: err, zero: got == nil}
			},
		},
		{
			name: "PrepareExec",
			call: func() caseResult {
				got, err := u.PrepareExec(ctx, nil, nil, platform.ExecQuery{})
				return caseResult{err: err, zero: reflect.ValueOf(got).IsZero()}
			},
		},
		{
			name: "Estimate",
			call: func() caseResult {
				got, err := u.Estimate(ctx, nil, config.Config{}, platform.CostOptions{})
				return caseResult{err: err, zero: reflect.ValueOf(got).IsZero()}
			},
		},
		{
			name: "NewDeploySteps",
			call: func() caseResult {
				got, err := ops.NewDeploySteps(ctx, nil, nil, io.Discard)
				return caseResult{err: err, zero: got == nil}
			},
		},
	}

	if len(cases) != 15 {
		t.Fatalf("criterion 5 requires exactly 15 unsupported methods; got %d", len(cases))
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
		t.Fatal("State must return the unsupported shell, not nil")
	}
	if m.Secrets() == nil {
		t.Fatal("Secrets must return the unsupported shell, not nil")
	}
	if m.RuntimeObserve() == nil {
		t.Fatal("RuntimeObserve must return the unsupported shell, not nil")
	}
	if m.CostEstimator() == nil {
		t.Fatal("CostEstimator must return the unsupported shell, not nil")
	}
}
