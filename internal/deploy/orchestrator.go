package deploy

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"time"

	"github.com/acourtiol/magelift/internal/automation"
	sdk "github.com/acourtiol/magelift/sdk/v1"
)

var (
	ErrApprovalRequired       = errors.New("production deployment approval is required")
	ErrDigestRequired         = errors.New("deployment requires an immutable image digest")
	ErrForwardOnlyRollbackAck = errors.New("rollback requires acknowledgement that database migrations are forward-only")
	ErrLockRelease            = errors.New("deployment lock release failed")
)

const candidateCleanupTimeout = 2 * time.Minute

var digestPattern = regexp.MustCompile(`^[^\s@]+@sha256:[a-f0-9]{64}$`)

type Request struct {
	Target                   sdk.TargetDescriptor
	ImageDigest              string
	Application              sdk.Application
	Artifact                 sdk.BuildArtifact
	Production               bool
	Approved                 bool
	Rollback                 bool
	AcknowledgeForwardOnlyDB bool
}

type Lock interface {
	Acquire(context.Context, Request) (func(context.Context) error, error)
}

type Steps interface {
	Validate(context.Context, Request) error
	Preview(context.Context, Request) (automation.ChangeSummary, error)
	RegisterCandidate(context.Context, Request) error
	RunMigrations(context.Context, Request) error
	CleanupCandidate(context.Context, Request) error
	UpdateServices(context.Context, Request) (automation.ChangeSummary, error)
	Stabilize(context.Context, Request) error
	Health(context.Context, Request) error
	Record(context.Context, Request, Result) error
}

type Result struct {
	Preview automation.ChangeSummary `json:"preview" yaml:"preview"`
	Update  automation.ChangeSummary `json:"update" yaml:"update"`
}

type Orchestrator struct {
	lock  Lock
	steps Steps
	hooks *hookPlan
}

func New(lock Lock, steps Steps) *Orchestrator {
	return &Orchestrator{lock: lock, steps: steps}
}

func NewWithHooks(lock Lock, steps Steps, hooks []sdk.LifecycleHook) (*Orchestrator, error) {
	plan, err := newHookPlan(hooks)
	if err != nil {
		return nil, err
	}
	return &Orchestrator{lock: lock, steps: steps, hooks: plan}, nil
}

func (o *Orchestrator) Run(ctx context.Context, request Request) (result Result, err error) {
	if o == nil || o.lock == nil || o.steps == nil {
		return Result{}, errors.New("deployment lock and steps are required")
	}
	if !digestPattern.MatchString(request.ImageDigest) {
		return Result{}, ErrDigestRequired
	}
	if request.Production && !request.Approved {
		return Result{}, ErrApprovalRequired
	}
	if request.Rollback && !request.AcknowledgeForwardOnlyDB {
		return Result{}, ErrForwardOnlyRollbackAck
	}
	if err := sdk.ValidateTargetDescriptor(request.Target); err != nil {
		return Result{}, err
	}
	if err := o.steps.Validate(ctx, request); err != nil {
		return Result{}, err
	}
	hookRequest := sdk.LifecycleHookRequest{Application: request.Application, Artifact: request.Artifact}
	if hookRequest.Artifact.ImageDigest == "" {
		hookRequest.Artifact.ImageDigest = request.ImageDigest
	}
	if o.hooks != nil {
		for _, hook := range o.hooks.ordered {
			if err := hook.Validate(ctx, hookRequest); err != nil {
				return Result{}, fmt.Errorf("validate lifecycle hook %q: %w", hook.Descriptor().ID, err)
			}
		}
	}
	release, err := o.lock.Acquire(ctx, request)
	if err != nil {
		return Result{}, err
	}
	if release == nil {
		return Result{}, errors.New("deployment lock returned no release function")
	}
	candidateRegistered := false
	defer func() {
		if releaseErr := release(ctx); releaseErr != nil && err == nil {
			err = fmt.Errorf("%w: %v", ErrLockRelease, releaseErr)
		}
	}()
	defer func() {
		if !candidateRegistered {
			return
		}
		cleanupContext, cancel := context.WithTimeout(context.WithoutCancel(ctx), candidateCleanupTimeout)
		cleanupErr := o.steps.CleanupCandidate(cleanupContext, request)
		cancel()
		if cleanupErr != nil {
			err = errors.Join(err, fmt.Errorf("clean up deployment candidate: %w", cleanupErr))
		}
	}()
	if skip, hookErr := o.runHooks(ctx, hookRequest, HookTargetPreview, sdk.HookBefore); hookErr != nil {
		return Result{}, hookErr
	} else if !skip {
		result.Preview, err = o.steps.Preview(ctx, request)
		if err != nil {
			return Result{}, err
		}
	}
	if _, hookErr := o.runHooks(ctx, hookRequest, HookTargetPreview, sdk.HookAfter); hookErr != nil {
		return Result{}, hookErr
	}
	if skip, hookErr := o.runHooks(ctx, hookRequest, HookTargetCandidate, sdk.HookBefore); hookErr != nil {
		return Result{}, hookErr
	} else if !skip {
		if err = o.steps.RegisterCandidate(ctx, request); err != nil {
			return Result{}, err
		}
		candidateRegistered = true
	}
	if _, hookErr := o.runHooks(ctx, hookRequest, HookTargetCandidate, sdk.HookAfter); hookErr != nil {
		return Result{}, hookErr
	}
	if skip, hookErr := o.runHooks(ctx, hookRequest, HookTargetMigrations, sdk.HookBefore); hookErr != nil {
		return Result{}, hookErr
	} else if !skip {
		if err = o.steps.RunMigrations(ctx, request); err != nil {
			return Result{}, err
		}
	}
	if _, hookErr := o.runHooks(ctx, hookRequest, HookTargetMigrations, sdk.HookAfter); hookErr != nil {
		return Result{}, hookErr
	}
	if skip, hookErr := o.runHooks(ctx, hookRequest, HookTargetUpdate, sdk.HookBefore); hookErr != nil {
		return Result{}, hookErr
	} else if !skip {
		result.Update, err = o.steps.UpdateServices(ctx, request)
		if err != nil {
			return Result{}, err
		}
	}
	if _, hookErr := o.runHooks(ctx, hookRequest, HookTargetUpdate, sdk.HookAfter); hookErr != nil {
		return Result{}, hookErr
	}
	if skip, hookErr := o.runHooks(ctx, hookRequest, HookTargetStabilize, sdk.HookBefore); hookErr != nil {
		return Result{}, hookErr
	} else if !skip {
		if err = o.steps.Stabilize(ctx, request); err != nil {
			return Result{}, err
		}
	}
	if _, hookErr := o.runHooks(ctx, hookRequest, HookTargetStabilize, sdk.HookAfter); hookErr != nil {
		return Result{}, hookErr
	}
	if skip, hookErr := o.runHooks(ctx, hookRequest, HookTargetHealth, sdk.HookBefore); hookErr != nil {
		return Result{}, hookErr
	} else if !skip {
		if err = o.steps.Health(ctx, request); err != nil {
			return Result{}, err
		}
	}
	if _, hookErr := o.runHooks(ctx, hookRequest, HookTargetHealth, sdk.HookAfter); hookErr != nil {
		return Result{}, hookErr
	}
	if skip, hookErr := o.runHooks(ctx, hookRequest, HookTargetRecord, sdk.HookBefore); hookErr != nil {
		return Result{}, hookErr
	} else if !skip {
		if err = o.steps.Record(ctx, request, result); err != nil {
			return Result{}, err
		}
	}
	if _, hookErr := o.runHooks(ctx, hookRequest, HookTargetRecord, sdk.HookAfter); hookErr != nil {
		return Result{}, hookErr
	}
	return result, nil
}

func (o *Orchestrator) runHooks(ctx context.Context, request sdk.LifecycleHookRequest, target sdk.HookID, relationship sdk.HookRelationship) (bool, error) {
	if o.hooks == nil {
		return false, nil
	}
	return o.hooks.run(ctx, request, target, relationship)
}
