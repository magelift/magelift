package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/magelift/magelift/internal/automation"
	"github.com/magelift/magelift/internal/config"
	"github.com/magelift/magelift/internal/cosign"
	deployflow "github.com/magelift/magelift/internal/deploy"
	"github.com/magelift/magelift/internal/platform"
	"github.com/magelift/magelift/internal/providerhost"
	"github.com/magelift/magelift/internal/usererr"
	"github.com/magelift/magelift/sdk"
	"github.com/spf13/cobra"
)

// concurrentUpdateExit is the dedicated code for preview/deploy/destroy
// when another Pulumi update holds the stack lock. Generated CI matches
// on it to distinguish a collision (wait and retry) from a graph bug.
const concurrentUpdateExit = 5

// infrastructureBackend is the small surface the CLI needs from Pulumi. It
// keeps command tests independent of the Automation API and AWS credentials.
type infrastructureBackend interface {
	automation.Backend
	Outputs(context.Context) (map[string]any, error)
}

// mapConcurrentUpdateError converts a classified stack collision into the
// dedicated exit code. Errors that already carry a code pass through, and
// everything else is untouched.
func mapConcurrentUpdateError(err error) error {
	var exit *exitError
	if errors.As(err, &exit) {
		return err
	}
	var concurrentErr *automation.ConcurrentUpdateError
	if errors.As(err, &concurrentErr) {
		return &exitError{code: concurrentUpdateExit, err: err}
	}
	return err
}

// mapPluginError converts typed provider failures into stable exit codes
// with next-step guidance. Errors that already carry a code pass through,
// non-plugin errors are untouched, and unmapped plugin codes keep their
// redacted provider text under the default exit.
func mapPluginError(err error) error {
	var exit *exitError
	if errors.As(err, &exit) {
		return err
	}
	var pluginErr *providerhost.PluginError
	if !errors.As(err, &pluginErr) {
		return err
	}
	switch pluginErr.Code {
	case sdk.ErrCodeInvalid:
		return invalid(usererr.Wrap(err, "provider rejected the request", "fix the flagged input and retry", "docs/onboarding.md"))
	case sdk.ErrCodeCredential:
		return &exitError{code: 3, err: usererr.Wrap(err, "provider credentials expired or were revoked", "re-authenticate (gcloud auth login or workload identity), then retry", "docs/onboarding.md#prerequisites")}
	case sdk.ErrCodeCompatibility:
		return &exitError{code: 3, err: usererr.Wrap(err, "provider plugin is incompatible", "reinstall the provider artifacts beside the CLI", "docs/onboarding.md#prerequisites")}
	case sdk.ErrCodeConflict, sdk.ErrCodeNotFound, sdk.ErrCodeIntegrity:
		return &exitError{code: 3, err: err}
	default:
		return err
	}
}

func bindLiveQueueReplicas(ctx context.Context, backend infrastructureBackend, planned platform.PlannedStack) (platform.PlannedStack, error) {
	if _, ok := planned.(platform.LiveQueueReplicaBinder); !ok {
		return planned, nil
	}
	outputs, err := backend.Outputs(ctx)
	if err != nil {
		return planned, nil
	}
	return platform.BindLiveQueueReplicas(planned, outputs)
}

type infrastructureResult struct {
	Environment          string                   `json:"environment" yaml:"environment"`
	Stack                string                   `json:"stack" yaml:"stack"`
	Preview              automation.ChangeSummary `json:"preview,omitempty" yaml:"preview,omitempty"`
	Update               automation.ChangeSummary `json:"update,omitempty" yaml:"update,omitempty"`
	Adopted              []string                 `json:"adopted,omitempty" yaml:"adopted,omitempty"`
	DestroyBackups       bool                     `json:"destroyBackups,omitempty" yaml:"destroyBackups,omitempty"`
	DestroyedBackupNames []string                 `json:"destroyedBackupNames,omitempty" yaml:"destroyedBackupNames,omitempty"`
	RetainedBackups      []config.RetainedBackup  `json:"retainedBackups,omitempty" yaml:"retainedBackups,omitempty"`
}

func infrastructureCommands(o *options) []*cobra.Command {
	return []*cobra.Command{
		infrastructureCommand(o, "preview", "Preview Magento environment changes without applying", func(ctx context.Context, backend infrastructureBackend, request automation.Request) (infrastructureResult, error) {
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
	var ackMaintenanceDrain bool
	var skipProviderLock bool
	var destroyBackups bool
	command := &cobra.Command{Use: name, Short: short, Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		if name == "deploy" {
			o.infraOnly = infraOnly
			o.ackMaintenanceDrain = ackMaintenanceDrain
		}
		if name == "destroy" {
			o.skipProviderLock = skipProviderLock
			o.destroyBackups = destroyBackups
		}
		result, err := o.executeInfrastructure(cmd.Context(), name, operation, digest)
		if err != nil {
			return mapPluginError(mapConcurrentUpdateError(err))
		}
		return o.write(result)
	}}
	if name == "deploy" {
		command.Flags().StringVar(&digest, "digest", "", "override the configured immutable image digest")
		command.Flags().BoolVar(&infraOnly, "infra-only", false, "update the infrastructure graph only (skip Magento migrate/health)")
		command.Flags().BoolVar(&ackMaintenanceDrain, "ack-maintenance-drain", false, "attest backup, maintenance mode, and drained writers for production schema risk (see docs/operations.md)")
	}
	if name == "destroy" {
		command.Flags().BoolVar(&skipProviderLock, "skip-lock", false, "skip the provider distributed state lock (acceptance cleanup only)")
		command.Flags().BoolVar(&destroyBackups, "destroy-backups", false, "also destroy leftover provider backups after the environment is gone; GCP Cloud SQL leftovers are deleted even when the backup policy is disposable, AWS snapshots are still refused")
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

func (o *options) refuseUnimplementedDestroyBackups() error {
	if !o.destroyBackups {
		return nil
	}
	effective, _, err := o.resolveWithEnvironment()
	if err != nil {
		return err
	}
	retained := config.RetainedBackupsOnDestroy(effective.Config)
	for _, item := range retained {
		if destroyBackupsImplemented(item.Mechanism) {
			continue
		}
		return invalid(fmt.Errorf("--destroy-backups is not implemented while %s backups remain inside retention; omit the flag so destroy keeps them", item.Mechanism))
	}
	return nil
}

func (o *options) applyDestroyBackupRetention(ctx context.Context, planned platform.PlannedStack, result *infrastructureResult) error {
	effective, _, err := o.resolveWithEnvironment()
	if err != nil {
		return err
	}
	retained := config.RetainedBackupsOnDestroy(effective.Config)
	if !o.destroyBackups {
		result.RetainedBackups = retained
		return nil
	}
	result.DestroyBackups = true
	names, err := o.destroyLeftoverProviderBackups(ctx, planned, retained)
	if err != nil {
		return err
	}
	result.DestroyedBackupNames = names
	return nil
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
	if err := requireTargetDependencies(ctx, o, planned); err != nil {
		return infrastructureResult{}, err
	}
	if name != "destroy" {
		planned, err = o.admitPlanned(ctx, planned)
		if err != nil {
			return infrastructureResult{}, invalid(fmt.Errorf("provider plan admission: %w", err))
		}
	}
	if (name == "deploy" || name == "destroy") && planned.EnvironmentClass() == "production" && !o.yes {
		return infrastructureResult{}, invalid(errors.New("production changes require explicit --yes approval"))
	}
	if name == "destroy" && planned.Protected() {
		return infrastructureResult{}, invalid(errors.New("protected environments must be unprotected before destroy"))
	}
	if name == "destroy" {
		if err := o.refuseUnimplementedDestroyBackups(); err != nil {
			return infrastructureResult{}, err
		}
	}
	if name == "deploy" {
		return o.runDeployment(ctx, environment, planned, planned.ImageDigest())
	}
	if o.newBackend == nil {
		return infrastructureResult{}, errors.New("infrastructure backend factory is required")
	}
	backendURL := o.infrastructureBackendURL(planned)
	backend, err := o.newBackend(ctx, planned, backendURL)
	if err != nil {
		return infrastructureResult{}, fmt.Errorf("create infrastructure backend: %w", err)
	}
	defer o.closeProviderSessions()
	if name != "destroy" {
		planned, err = bindLiveQueueReplicas(ctx, backend, planned)
		if err != nil {
			return infrastructureResult{}, invalid(err)
		}
	}
	requestTarget := planned.TargetDescriptor()
	request := o.automationRequest(requestTarget, name == "destroy")
	if err := automation.ValidateRequest(ctx, backend, request); err != nil {
		return infrastructureResult{}, err
	}
	var release func(context.Context) error
	if name == "destroy" && !o.skipProviderLock {
		release, err = o.acquireProviderLock(ctx, planned)
		if err != nil {
			return infrastructureResult{}, err
		}
	} else if name == "destroy" && o.skipProviderLock {
		announceSkipProviderLock(o.stderr)
	}
	if err := refuseAdoptedMutationForOperation(planned, name); err != nil {
		return infrastructureResult{}, err
	}
	adopted := announceAdoptedResources(o.stderr, planned)
	result, err := operation(ctx, backend, request)
	if err == nil && name == "destroy" {
		err = o.destroyFastlyEdge(ctx, environment)
	}
	if release != nil {
		err = joinReleaseError(err, release(ctx))
	}
	if err != nil {
		return infrastructureResult{}, err
	}
	result.Environment = environment
	result.Stack = planned.StackName()
	result.Adopted = adopted
	if name == "destroy" {
		if err := o.applyDestroyBackupRetention(ctx, planned, &result); err != nil {
			return infrastructureResult{}, err
		}
	}
	return result, nil
}

type deploymentOptions struct {
	rollback                    bool
	acknowledgeForwardOnlyDB    bool
	acknowledgeMaintenanceDrain bool
	infraOnly                   bool
	dependenciesChecked         bool
}

func (o *options) runDeployment(ctx context.Context, environment string, planned platform.PlannedStack, digest string) (infrastructureResult, error) {
	return o.runDeploymentWithOptions(ctx, environment, planned, digest, deploymentOptions{infraOnly: o.infraOnly, acknowledgeMaintenanceDrain: o.ackMaintenanceDrain, dependenciesChecked: true})
}

func (o *options) runDeploymentWithOptions(ctx context.Context, environment string, planned platform.PlannedStack, digest string, deployOptions deploymentOptions) (infrastructureResult, error) {
	if !deployOptions.dependenciesChecked {
		if err := requireTargetDependencies(ctx, o, planned); err != nil {
			return infrastructureResult{}, err
		}
	}
	if o.newBackend == nil {
		return infrastructureResult{}, errors.New("infrastructure backend factory is required")
	}
	backendURL := o.infrastructureBackendURL(planned)
	backend, err := o.newBackend(ctx, planned, backendURL)
	if err != nil {
		return infrastructureResult{}, fmt.Errorf("create infrastructure backend: %w", err)
	}
	defer o.closeProviderSessions()
	planned, err = bindLiveQueueReplicas(ctx, backend, planned)
	if err != nil {
		return infrastructureResult{}, invalid(err)
	}
	requestTarget := planned.TargetDescriptor()
	request := o.automationRequest(requestTarget, false)
	if err := automation.ValidateRequest(ctx, backend, request); err != nil {
		return infrastructureResult{}, err
	}
	if deployOptions.infraOnly {
		announceInfraOnlyDeploy(o.stderr)
		preview, previewErr := automation.NewRunner(backend, o.stderr).Preview(ctx, request)
		if previewErr != nil {
			return infrastructureResult{}, previewErr
		}
		update, updateErr := automation.NewRunner(backend, o.stderr).Update(ctx, request)
		if updateErr != nil {
			return infrastructureResult{Environment: environment, Stack: planned.StackName(), Preview: preview}, updateErr
		}
		adopted := announceAdoptedResources(o.stderr, planned)
		if outputs, outErr := backend.Outputs(ctx); outErr == nil && len(outputs) > 0 {
			if reqErr := platform.RequireOutputs(outputs, platform.RequiredOutputKeys()); reqErr != nil {
				return infrastructureResult{Environment: environment, Stack: planned.StackName(), Preview: preview, Update: update, Adopted: adopted}, reqErr
			}
		}
		return infrastructureResult{Environment: environment, Stack: planned.StackName(), Preview: preview, Update: update, Adopted: adopted}, nil
	}
	if o.newDeploySteps != nil {
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
			if refuseErr := refuseAdoptedMutationForOperation(planned, "deploy"); refuseErr != nil {
				return infrastructureResult{}, refuseErr
			}
			adopted := announceAdoptedResources(o.stderr, planned)
			result, runErr := deployflow.New(cliDeploymentLock{factory: o.newLock, planned: planned}, steps).Run(ctx, deployflow.Request{
				Target: requestTarget, ImageDigest: digest, Preview: request.Preview, Production: planned.EnvironmentClass() == "production", Approved: o.yes,
				Rollback: deployOptions.rollback, AcknowledgeForwardOnlyDB: deployOptions.acknowledgeForwardOnlyDB,
				AcknowledgeMaintenanceDrain: deployOptions.acknowledgeMaintenanceDrain,
			})
			if runErr != nil {
				return infrastructureResult{}, runErr
			}
			out := infrastructureResult{Environment: environment, Stack: planned.StackName(), Preview: result.Preview, Update: result.Update, Adopted: adopted}
			// Auto-import after lock release (deployflow.Run completed); D-01/D-02 once-from-recorded.
			if err := o.maybeAutoImportSeedDump(ctx, environment); err != nil {
				return out, err
			}
			originURL := ""
			if outputs, outputErr := backend.Outputs(ctx); outputErr == nil {
				if applicationURL, ok := outputs[platform.OutputApplicationURL].(string); ok {
					originURL = applicationURL
				}
			}
			if err := o.applyFastlyEdge(ctx, environment, originURL); err != nil {
				return out, err
			}
			return out, nil
		}
		if stepsErr != nil && !errors.Is(stepsErr, platform.ErrNotSupported) {
			return infrastructureResult{}, fmt.Errorf("create deployment workflow: %w", stepsErr)
		}
		// ErrNotSupported or nil steps: refuse unless the operator asked for infra-only.
		return infrastructureResult{}, refuseInfraOnlyDeploy(planned)
	}
	if o.newLock == nil {
		return infrastructureResult{}, errors.New("deployment lock factory is required")
	}
	release, err := o.newLock(ctx, planned)
	if err != nil {
		return infrastructureResult{}, fmt.Errorf("acquire deployment lock: %w", err)
	}
	result, err := func() (infrastructureResult, error) {
		if refuseErr := refuseAdoptedMutationForOperation(planned, "deploy"); refuseErr != nil {
			return infrastructureResult{}, refuseErr
		}
		adopted := announceAdoptedResources(o.stderr, planned)
		preview, previewErr := automation.NewRunner(backend, o.stderr).Preview(ctx, request)
		if previewErr != nil {
			return infrastructureResult{}, previewErr
		}
		update, updateErr := automation.NewRunner(backend, o.stderr).Update(ctx, request)
		if updateErr != nil {
			return infrastructureResult{Environment: environment, Stack: planned.StackName(), Preview: preview, Adopted: adopted}, updateErr
		}
		if outputs, outErr := backend.Outputs(ctx); outErr == nil && len(outputs) > 0 {
			if reqErr := platform.RequireOutputs(outputs, platform.RequiredOutputKeys()); reqErr != nil {
				return infrastructureResult{Environment: environment, Stack: planned.StackName(), Preview: preview, Update: update, Adopted: adopted}, reqErr
			}
		}
		return infrastructureResult{Environment: environment, Stack: planned.StackName(), Preview: preview, Update: update, Adopted: adopted}, nil
	}()
	err = joinReleaseError(err, releaseDeploymentLock(ctx, release))
	return result, err
}

func (o *options) requireSignedRelease(ctx context.Context, environment, digest string) error {
	if err := requireCosignVerificationDependencies(ctx, o); err != nil {
		return err
	}
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

const deploymentLockReleaseTimeout = time.Minute

func releaseDeploymentLock(ctx context.Context, release func(context.Context) error) error {
	if release == nil {
		return errors.New("deployment lock release function is required")
	}
	releaseContext, cancel := context.WithTimeout(context.WithoutCancel(ctx), deploymentLockReleaseTimeout)
	defer cancel()
	return release(releaseContext)
}

func joinReleaseError(err, releaseErr error) error {
	if releaseErr == nil {
		return err
	}
	return errors.Join(err, fmt.Errorf("release deployment lock: %w", releaseErr))
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
		if err := requireTargetDependencies(cmd.Context(), o, planned); err != nil {
			return err
		}
		backendURL := o.infrastructureBackendURL(planned)
		backend, err := o.newBackend(cmd.Context(), planned, backendURL)
		if err != nil {
			return fmt.Errorf("create infrastructure backend: %w", err)
		}
		defer o.closeProviderSessions()
		var outputs map[string]any
		// Prefer redacted secrets for user-facing JSON (kubeconfig, DB passwords).
		// Fail closed: never fall through to decrypted Outputs on redaction error.
		if redactor, ok := backend.(interface {
			RedactedOutputs(context.Context) (map[string]any, error)
		}); ok {
			outputs, err = redactor.RedactedOutputs(cmd.Context())
			if err != nil {
				return fmt.Errorf("read redacted infrastructure outputs: %w", err)
			}
		} else {
			outputs, err = backend.Outputs(cmd.Context())
			if err != nil {
				return fmt.Errorf("read infrastructure outputs: %w", err)
			}
		}
		return o.write(map[string]any{"environment": environment, "stack": planned.StackName(), "outputs": outputs})
	}}
}

func (o *options) planStack(allowExpiredPreview bool) (string, platform.PlannedStack, error) {
	return o.planStackWith(platform.PlanOptions{AllowExpiredPreview: allowExpiredPreview})
}

func (o *options) planStackWith(opts platform.PlanOptions) (string, platform.PlannedStack, error) {
	o.resolvedPreviewIdentity = nil
	if o.modules == nil {
		return "", nil, errors.New("stack module registry is required")
	}
	effective, environment, err := o.resolveWithEnvironment()
	if err != nil {
		return "", nil, err
	}
	o.resolvedPreviewIdentity = effective.Config.PreviewIdentity
	if opts.Context == nil {
		opts.Context = o.planContext
	}
	_, planned, err := o.modules.Plan(effective.Config, environment, opts)
	if err != nil {
		return "", nil, err
	}
	warnExperimentalTarget(o.stderr, planned)
	return environment, planned, nil
}

// experimentalTargetWarningFmt is the TRUST-01 stderr line. Keep factual: tier,
// provider/runtime, coverage honesty, docs; never ARNs or backend URLs.
const experimentalTargetWarningFmt = "warning: MageLift: target %s/%s is experimental: broader release, architecture, and day-2 acceptance coverage is incomplete; see docs/capability-matrix.md"

func warnExperimentalTarget(stderr io.Writer, planned platform.PlannedStack) {
	if planned == nil || planned.CertificationTier() != platform.TierExperimental || stderr == nil {
		return
	}
	_, _ = fmt.Fprintf(stderr, experimentalTargetWarningFmt+"\n", planned.Provider(), planned.Runtime())
}

// infraOnlyDeployNoticeFmt is the TRUST-02 stderr line when --infra-only proceeds.
const infraOnlyDeployNoticeFmt = "notice: --infra-only: Magento migrate, cutover, and health were skipped\n"

const skipProviderLockNoticeFmt = "notice: --skip-lock: provider distributed state lock was skipped; use only when no concurrent operation is running\n"

func announceInfraOnlyDeploy(stderr io.Writer) {
	if stderr == nil {
		return
	}
	_, _ = io.WriteString(stderr, infraOnlyDeployNoticeFmt)
}

func announceSkipProviderLock(stderr io.Writer) {
	if stderr == nil {
		return
	}
	_, _ = io.WriteString(stderr, skipProviderLockNoticeFmt)
}

func refuseInfraOnlyDeploy(planned platform.PlannedStack) error {
	cause := fmt.Sprintf(
		"deploy on target %s/%s (%s) cannot run Magento migrate, cutover, and health",
		planned.Provider(), planned.Runtime(), planned.CertificationTier(),
	)
	return invalid(usererr.New(
		cause,
		"Re-run with --infra-only to update the infrastructure graph only.",
		"docs/capability-matrix.md",
	))
}

func announceAdoptedResources(stderr io.Writer, planned platform.PlannedStack) []string {
	attach, ok := planned.(platform.BrownfieldAttach)
	if !ok {
		return nil
	}
	lines := attach.AdoptedResourceLines()
	if len(lines) == 0 {
		return nil
	}
	if stderr != nil {
		for _, line := range lines {
			_, _ = fmt.Fprintln(stderr, line)
		}
	}
	return append([]string(nil), lines...)
}

// refuseAdoptedMutationForOperation consults the brownfield refuse gate on
// Update (deploy) and Destroy paths. Stack-scoped ops pass an empty intent so
// Magento deploy/destroy remain allowed while AdoptReport still surfaces ADOPT
// lines; callers that pass destroy/replace intent against an adopted network or
// database fail closed with a named ownership error (D-02).
func refuseAdoptedMutationForOperation(planned platform.PlannedStack, operation string) error {
	if operation != "deploy" && operation != "destroy" {
		return nil
	}
	attach, ok := planned.(platform.BrownfieldAttach)
	if !ok {
		return nil
	}
	if err := attach.RefuseAdoptedMutation(platform.AdoptMutationIntent("")); err != nil {
		return err
	}
	return nil
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
