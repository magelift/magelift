package platform

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	deployflow "github.com/magelift/magelift/internal/deploy"
)

// ErrNotSupported means the selected target does not implement an optional Ops
// capability (for example Magento candidate deploy on an experimental adapter).
var ErrNotSupported = errors.New("operation not supported for this target")

// Ops is the optional Magento-facing operations surface for a StackModule.
// Adapters that only manage the infrastructure graph omit HasOps.
type Ops interface {
	// AcquireLock returns a release function. Experimental targets may still
	// return a no-op release when they have no DIY lock, but they must warn on
	// stderr that no lock was taken (see WarnNoDIYLock).
	AcquireLock(ctx context.Context, planned PlannedStack) (release func(context.Context) error, err error)
	// NewDeploySteps returns Magento candidate deploy steps, or ErrNotSupported
	// when the adapter is infrastructure-only. backend is the CLI infrastructure
	// backend (preview/update/destroy/outputs); adapters type-assert as needed.
	NewDeploySteps(ctx context.Context, backend any, planned PlannedStack, diagnostics io.Writer) (deployflow.Steps, error)
}

// WarnNoDIYLock writes a clear warning that no DIY deployment lock was taken.
// w defaults to os.Stderr when nil. Message shape is stable for adapter tests.
func WarnNoDIYLock(w io.Writer, planned PlannedStack) {
	if w == nil {
		w = os.Stderr
	}
	target := "unknown target"
	if planned != nil {
		if id := planned.TargetDescriptor().ID; id != "" {
			target = string(id)
		} else {
			target = string(planned.Provider()) + "/" + string(planned.Runtime())
		}
	}
	fmt.Fprintf(w, "warning: DIY deployment lock was not taken for %s\n", target)
}

// HasOps is implemented by StackModules that expose Magento deploy Ops.
type HasOps interface {
	Ops() Ops
}

// HasRecordRelease is implemented by Ops adapters that accept a post-deploy
// journal hook. The CLI uses this instead of importing concrete cloud Ops types.
type HasRecordRelease interface {
	WithRecordRelease(fn func(context.Context, deployflow.Request, deployflow.Result) error) Ops
}

// ModuleOps returns Ops when the module implements HasOps.
func ModuleOps(module StackModule) Ops {
	if module == nil {
		return nil
	}
	if provider, ok := module.(HasOps); ok {
		return provider.Ops()
	}
	return nil
}
