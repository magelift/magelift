package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/acourtiol/magelift/internal/automation"
	"github.com/acourtiol/magelift/internal/cosign"
	deployflow "github.com/acourtiol/magelift/internal/deploy"
	"github.com/acourtiol/magelift/internal/platform"
	"github.com/spf13/cobra"
)

// infrastructureBackend is the small surface the CLI needs from Pulumi. It
// keeps command tests independent of the Automation API and AWS credentials.
type infrastructureBackend interface {
	automation.Backend
	Outputs(context.Context) (map[string]any, error)
}

type infrastructureResult struct {
	Environment string                   `json:"environment" yaml:"environment"`
	Stack       string                   `json:"stack" yaml:"stack"`
	Preview     automation.ChangeSummary `json:"preview,omitempty" yaml:"preview,omitempty"`
	Update      automation.ChangeSummary `json:"update,omitempty" yaml:"update,omitempty"`
}

func infrastructureCommands(o *options) []*cobra.Command {
	return []*cobra.Command{
		infrastructureCommand(o, "preview", "Preview infrastructure changes", func(ctx context.Context, backend infrastructureBackend, request automation.Request) (infrastructureResult, error) {
			summary, err := automation.NewRunner(backend, o.stderr).Preview(ctx, request)
			return infrastructureResult{Preview: summary}, err
		}),
		infrastructureCommand(o, "deploy", "Deploy the selected environment", func(ctx context.Context, backend infrastructureBackend, request automation.Request) (infrastructureResult, error) {
			// Every deployment gets a fresh preview. Production also requires the
			// explicit --yes approval gate below.
			preview, err := automation.NewRunner(backend, o.stderr).Preview(ctx, request)
			if err != nil {
				return infrastructureResult{}, err
			}
			update, err := automation.NewRunner(backend, o.stderr).Update(ctx, request)
			return infrastructureResult{Preview: preview, Update: update}, err
		}),
		infrastructureCommand(o, "destroy", "Destroy the selected environment", o.destroyOperation),
		outputsCommand(o),
	}
}

func infrastructureCommand(o *options, name, short string, operation func(context.Context, infrastructureBackend, automation.Request) (infrastructureResult, error)) *cobra.Command {
	var digest string
	var infraOnly bool
	command := &cobra.Command{Use: name, Short: short, Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		if name == "deploy" {
			o.infraOnly = infraOnly
		}
		result, err := o.executeInfrastructure(cmd.Context(), name, operation, digest)
		if err != nil {
			return err
		}
		return o.write(result)
	}}
	if name == "deploy" {
		command.Flags().StringVar(&digest, "digest", "", "override the configured immutable image digest")
		command.Flags().BoolVar(&infraOnly, "infra-only", false, "update the infrastructure graph only (skip Magento migrate/health)")
	}
	return command
}

func (o *options) destroyOperation(ctx context.Context, backend infrastructureBackend, request automation.Request) (infrastructureResult, error) {
	preview, err := automation.NewRunner(backend, o.stderr).Preview(ctx, request)
	if err != nil {
		return infrastructureResult{}, err
	}
	destroyed, err := automation.NewRunner(backend, o.stderr).Destroy(ctx, request)
	return infrastructureResult{Preview: preview, Update: destroyed}, err
}

func (o *options) executeInfrastructure(ctx context.Context, name string, operation func(context.Context, infrastructureBackend, automation.Request) (infrastructureResult, error), digestOverride string) (infrastructureResult, error) {
	environment, planned, err := o.planStack(name == "destroy")
	if err != nil {
		return infrastructureResult{}, invalid(err)
	}
	if digestOverride != "" {
		planned, err = planned.WithImageDigest(digestOverride)
		if err != nil {
			return infrastructureResult{}, invalid(fmt.Errorf("deployment digest is invalid: %w", err))
		}
	}
	if (name == "deploy" || name == "destroy") && planned.EnvironmentClass() == "production" && !o.yes {
		return infrastructureResult{}, invalid(errors.New("production changes require explicit --yes approval"))
	}
	if name == "destroy" && planned.Protected() {
		return infrastructureResult{}, invalid(errors.New("protected environments must be unprotected before destroy"))
	}
	if name == "deploy" {
		return o.runDeployment(ctx, environment, planned, planned.ImageDigest())
	}
	if o.newBackend == nil {
		return infrastructureResult{}, errors.New("infrastructure backend factory is required")
	}
	backendURL := strings.TrimSpace(o.getenv("PULUMI_BACKEND_URL"))
	backend, err := o.newBackend(ctx, planned, backendURL)
	if err != nil {
		return infrastructureResult{}, fmt.Errorf("create infrastructure backend: %w", err)
	}
	requestTarget := planned.TargetDescriptor()
	var release func(context.Context) error
	if name == "destroy" {
		release, err = o.acquireProviderLock(ctx, planned)
		if err != nil {
			return infrastructureResult{}, err
		}
	}
	request := automation.Request{Target: requestTarget}
	result, err := operation(ctx, backend, request)
	if release != nil {
		releaseErr := release(ctx)
		if releaseErr != nil {
			if err != nil {
				err = fmt.Errorf("%w; release deployment lock: %v", err, releaseErr)
			} else {
				err = fmt.Errorf("release deployment lock: %w", releaseErr)
			}
		}
	}
	if err != nil {
		return infrastructureResult{}, err
	}
	result.Environment = environment
	result.Stack = planned.StackName()
	return result, nil
}

type deploymentOptions struct {
	rollback                 bool
	acknowledgeForwardOnlyDB bool
	infraOnly                bool
}

func (o *options) runDeployment(ctx context.Context, environment string, planned platform.PlannedStack, digest string) (infrastructureResult, error) {
	return o.runDeploymentWithOptions(ctx, environment, planned, digest, deploymentOptions{infraOnly: o.infraOnly})
}

func (o *options) runDeploymentWithOptions(ctx context.Context, environment string, planned platform.PlannedStack, digest string, deployOptions deploymentOptions) (infrastructureResult, error) {
	if o.newBackend == nil {
		return infrastructureResult{}, errors.New("infrastructure backend factory is required")
	}
	backendURL := strings.TrimSpace(o.getenv("PULUMI_BACKEND_URL"))
	backend, err := o.newBackend(ctx, planned, backendURL)
	if err != nil {
		return infrastructureResult{}, fmt.Errorf("create infrastructure backend: %w", err)
	}
	requestTarget := planned.TargetDescriptor()
	if !deployOptions.infraOnly && o.newDeploySteps != nil {
		steps, stepsErr := o.newDeploySteps(ctx, backend, planned, o.stderr)
		if stepsErr == nil && steps != nil {
			if planned.EnvironmentClass() == "production" {
				if err := o.requireSignedRelease(ctx, environment, digest); err != nil {
					return infrastructureResult{}, err
				}
			}
			if o.newLock == nil {
				return infrastructureResult{}, errors.New("deployment lock factory is required")
			}
			result, runErr := deployflow.New(cliDeploymentLock{factory: o.newLock, planned: planned}, steps).Run(ctx, deployflow.Request{
				Target: requestTarget, ImageDigest: digest, Production: planned.EnvironmentClass() == "production", Approved: o.yes,
				Rollback: deployOptions.rollback, AcknowledgeForwardOnlyDB: deployOptions.acknowledgeForwardOnlyDB,
			})
			if runErr != nil {
				return infrastructureResult{}, runErr
			}
			return infrastructureResult{Environment: environment, Stack: planned.StackName(), Preview: result.Preview, Update: result.Update}, nil
		}
		if stepsErr != nil && !errors.Is(stepsErr, platform.ErrNotSupported) {
			return infrastructureResult{}, fmt.Errorf("create deployment workflow: %w", stepsErr)
		}
		// ErrNotSupported or nil steps: infrastructure graph update only.
	}
	if o.newLock == nil {
		return infrastructureResult{}, errors.New("deployment lock factory is required")
	}
	release, err := o.newLock(ctx, planned)
	if err != nil {
		return infrastructureResult{}, fmt.Errorf("acquire deployment lock: %w", err)
	}
	result, err := func() (infrastructureResult, error) {
		preview, previewErr := automation.NewRunner(backend, o.stderr).Preview(ctx, automation.Request{Target: requestTarget})
		if previewErr != nil {
			return infrastructureResult{}, previewErr
		}
		update, updateErr := automation.NewRunner(backend, o.stderr).Update(ctx, automation.Request{Target: requestTarget})
		if updateErr != nil {
			return infrastructureResult{Environment: environment, Stack: planned.StackName(), Preview: preview}, updateErr
		}
		if outputs, outErr := backend.Outputs(ctx); outErr == nil && len(outputs) > 0 {
			if reqErr := platform.RequireOutputs(outputs, platform.RequiredOutputKeys()); reqErr != nil {
				return infrastructureResult{Environment: environment, Stack: planned.StackName(), Preview: preview, Update: update}, reqErr
			}
		}
		return infrastructureResult{Environment: environment, Stack: planned.StackName(), Preview: preview, Update: update}, nil
	}()
	if releaseErr := release(ctx); releaseErr != nil {
		if err != nil {
			err = fmt.Errorf("%w; release deployment lock: %v", err, releaseErr)
		} else {
			err = fmt.Errorf("release deployment lock: %w", releaseErr)
		}
	}
	return result, err
}

func (o *options) requireSignedRelease(ctx context.Context, environment, digest string) error {
	store, err := o.releaseStore(environment)
	if err != nil {
		return &exitError{code: 3, err: err}
	}
	entries, err := store.List(ctx)
	if err != nil {
		return &exitError{code: 3, err: fmt.Errorf("read release journal: %w", err)}
	}
	for index := len(entries) - 1; index >= 0; index-- {
		entry := entries[index]
		if entry.DigestReference == digest && entry.SignatureIdentity != "" && entry.SignatureIssuer != "" {
			if o.verifyRelease == nil {
				return &exitError{code: 3, err: errors.New("production deployment signature verification is unavailable")}
			}
			if err := o.verifyRelease(ctx, digest, cosign.VerifyOptions{CertificateIdentity: entry.SignatureIdentity, OIDCIssuer: entry.SignatureIssuer}); err != nil {
				return &exitError{code: 3, err: errors.New("production deployment signature verification failed")}
			}
			return nil
		}
	}
	return &exitError{code: 3, err: errors.New("production deployment requires a promoted digest with verified signature metadata")}
}

type cliDeploymentLock struct {
	factory func(context.Context, platform.PlannedStack) (func(context.Context) error, error)
	planned platform.PlannedStack
}

func (l cliDeploymentLock) Acquire(ctx context.Context, _ deployflow.Request) (func(context.Context) error, error) {
	return l.factory(ctx, l.planned)
}

func outputsCommand(o *options) *cobra.Command {
	return &cobra.Command{Use: "outputs", Short: "Read outputs from the selected environment", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		environment, planned, err := o.planStack(false)
		if err != nil {
			return invalid(err)
		}
		if o.newBackend == nil {
			return errors.New("infrastructure backend factory is required")
		}
		backendURL := strings.TrimSpace(o.getenv("PULUMI_BACKEND_URL"))
		backend, err := o.newBackend(cmd.Context(), planned, backendURL)
		if err != nil {
			return fmt.Errorf("create infrastructure backend: %w", err)
		}
		outputs, err := backend.Outputs(cmd.Context())
		if err != nil {
			return fmt.Errorf("read infrastructure outputs: %w", err)
		}
		return o.write(map[string]any{"environment": environment, "stack": planned.StackName(), "outputs": outputs})
	}}
}

func (o *options) planStack(allowExpiredPreview bool) (string, platform.PlannedStack, error) {
	if o.modules == nil {
		return "", nil, errors.New("stack module registry is required")
	}
	effective, environment, err := o.resolveWithEnvironment()
	if err != nil {
		return "", nil, err
	}
	_, planned, err := o.modules.Plan(effective.Config, environment, platform.PlanOptions{AllowExpiredPreview: allowExpiredPreview})
	if err != nil {
		return "", nil, err
	}
	warnExperimentalTarget(o.stderr, planned)
	return environment, planned, nil
}

// experimentalTargetWarningFmt is the TRUST-01 stderr line. Keep factual: tier,
// provider/runtime, day-2 honesty, acceptance evidence, docs — never ARNs or backend URLs.
const experimentalTargetWarningFmt = "warning: target %s/%s is experimental: day-2 operations may be unimplemented and this target has no real-account acceptance evidence; see docs/capability-matrix.md"

func warnExperimentalTarget(stderr io.Writer, planned platform.PlannedStack) {
	if planned == nil || planned.CertificationTier() != platform.TierExperimental || stderr == nil {
		return
	}
	_, _ = fmt.Fprintf(stderr, experimentalTargetWarningFmt+"\n", planned.Provider(), planned.Runtime())
}

func (o *options) acquireProviderLock(ctx context.Context, planned platform.PlannedStack) (func(context.Context) error, error) {
	if o.newLock == nil {
		return nil, errors.New("deployment lock factory is required")
	}
	release, err := o.newLock(ctx, planned)
	if err != nil {
		return nil, fmt.Errorf("acquire deployment lock: %w", err)
	}
	return release, nil
}
