package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/acourtiol/magelift/internal/automation"
	awsbootstrap "github.com/acourtiol/magelift/internal/cloud/aws/bootstrap"
	awsstack "github.com/acourtiol/magelift/internal/cloud/aws/stack"
	awsstate "github.com/acourtiol/magelift/internal/cloud/aws/state"
	"github.com/acourtiol/magelift/internal/cosign"
	deployflow "github.com/acourtiol/magelift/internal/deploy"
	sdk "github.com/acourtiol/magelift/sdk/v1"
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
		infrastructureCommand(o, "preview", "Preview AWS infrastructure changes", func(ctx context.Context, backend infrastructureBackend, request automation.Request) (infrastructureResult, error) {
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
	command := &cobra.Command{Use: name, Short: short, Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		result, err := o.executeInfrastructure(cmd.Context(), name, operation, digest)
		if err != nil {
			return err
		}
		return o.write(result)
	}}
	if name == "deploy" {
		command.Flags().StringVar(&digest, "digest", "", "override the configured immutable image digest")
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
	environment, spec, err := o.infrastructureSpecFor(name == "destroy")
	if err != nil {
		return infrastructureResult{}, invalid(err)
	}
	if digestOverride != "" {
		spec.Artifact.ImageDigest = digestOverride
		if err := spec.Validate(); err != nil {
			return infrastructureResult{}, invalid(fmt.Errorf("deployment digest is invalid: %w", err))
		}
	}
	if (name == "deploy" || name == "destroy") && spec.Identity.EnvironmentClass == "production" && !o.yes {
		return infrastructureResult{}, invalid(errors.New("production changes require explicit --yes approval"))
	}
	if name == "destroy" && spec.Lifecycle.Protection {
		return infrastructureResult{}, invalid(errors.New("protected environments must be unprotected before destroy"))
	}
	if name == "deploy" {
		return o.runDeployment(ctx, environment, spec, spec.Artifact.ImageDigest)
	}
	if o.newBackend == nil {
		return infrastructureResult{}, errors.New("infrastructure backend factory is required")
	}
	backendURL := strings.TrimSpace(o.getenv("PULUMI_BACKEND_URL"))
	backend, err := o.newBackend(ctx, stackName(spec), spec, backendURL)
	if err != nil {
		return infrastructureResult{}, fmt.Errorf("create infrastructure backend: %w", err)
	}
	requestTarget := sdk.TargetDescriptor{ID: sdk.TargetID("aws.ecs-fargate"), Provider: "aws", Runtime: "ecs-fargate"}
	var release func(context.Context) error
	if name == "destroy" {
		if o.newLock == nil {
			return infrastructureResult{}, errors.New("deployment lock factory is required")
		}
		release, err = o.newLock(ctx, spec)
		if err != nil {
			return infrastructureResult{}, fmt.Errorf("acquire deployment lock: %w", err)
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
	result.Stack = stackName(spec)
	return result, nil
}

type deploymentOptions struct {
	rollback                 bool
	acknowledgeForwardOnlyDB bool
}

func (o *options) runDeployment(ctx context.Context, environment string, spec awsstack.Spec, digest string) (infrastructureResult, error) {
	return o.runDeploymentWithOptions(ctx, environment, spec, digest, deploymentOptions{})
}

func (o *options) runDeploymentWithOptions(ctx context.Context, environment string, spec awsstack.Spec, digest string, deployOptions deploymentOptions) (infrastructureResult, error) {
	if o.newBackend == nil {
		return infrastructureResult{}, errors.New("infrastructure backend factory is required")
	}
	if o.newDeploySteps != nil && spec.Identity.EnvironmentClass == "production" {
		if err := o.requireSignedRelease(ctx, environment, digest); err != nil {
			return infrastructureResult{}, err
		}
	}
	backendURL := strings.TrimSpace(o.getenv("PULUMI_BACKEND_URL"))
	backend, err := o.newBackend(ctx, stackName(spec), spec, backendURL)
	if err != nil {
		return infrastructureResult{}, fmt.Errorf("create infrastructure backend: %w", err)
	}
	requestTarget := sdk.TargetDescriptor{ID: sdk.TargetID("aws.ecs-fargate"), Provider: "aws", Runtime: "ecs-fargate"}
	if o.newDeploySteps != nil {
		if o.newLock == nil {
			return infrastructureResult{}, errors.New("deployment lock factory is required")
		}
		steps, stepsErr := o.newDeploySteps(ctx, backend, spec, o.stderr)
		if stepsErr != nil {
			return infrastructureResult{}, fmt.Errorf("create deployment workflow: %w", stepsErr)
		}
		result, runErr := deployflow.New(cliDeploymentLock{factory: o.newLock, spec: spec}, steps).Run(ctx, deployflow.Request{
			Target: requestTarget, ImageDigest: digest, Production: spec.Identity.EnvironmentClass == "production", Approved: o.yes,
			Rollback: deployOptions.rollback, AcknowledgeForwardOnlyDB: deployOptions.acknowledgeForwardOnlyDB,
		})
		if runErr != nil {
			return infrastructureResult{}, runErr
		}
		return infrastructureResult{Environment: environment, Stack: stackName(spec), Preview: result.Preview, Update: result.Update}, nil
	}
	if o.newLock == nil {
		return infrastructureResult{}, errors.New("deployment lock factory is required")
	}
	release, err := o.newLock(ctx, spec)
	if err != nil {
		return infrastructureResult{}, fmt.Errorf("acquire deployment lock: %w", err)
	}
	result, err := func() (infrastructureResult, error) {
		preview, previewErr := automation.NewRunner(backend, o.stderr).Preview(ctx, automation.Request{Target: requestTarget})
		if previewErr != nil {
			return infrastructureResult{}, previewErr
		}
		update, updateErr := automation.NewRunner(backend, o.stderr).Update(ctx, automation.Request{Target: requestTarget})
		return infrastructureResult{Environment: environment, Stack: stackName(spec), Preview: preview, Update: update}, updateErr
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
	factory func(context.Context, awsstack.Spec) (func(context.Context) error, error)
	spec    awsstack.Spec
}

func (l cliDeploymentLock) Acquire(ctx context.Context, _ deployflow.Request) (func(context.Context) error, error) {
	return l.factory(ctx, l.spec)
}

func outputsCommand(o *options) *cobra.Command {
	return &cobra.Command{Use: "outputs", Short: "Read outputs from the selected environment", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		environment, spec, err := o.infrastructureSpec()
		if err != nil {
			return invalid(err)
		}
		if o.newBackend == nil {
			return errors.New("infrastructure backend factory is required")
		}
		backendURL := strings.TrimSpace(o.getenv("PULUMI_BACKEND_URL"))
		backend, err := o.newBackend(cmd.Context(), stackName(spec), spec, backendURL)
		if err != nil {
			return fmt.Errorf("create infrastructure backend: %w", err)
		}
		outputs, err := backend.Outputs(cmd.Context())
		if err != nil {
			return fmt.Errorf("read infrastructure outputs: %w", err)
		}
		return o.write(map[string]any{"environment": environment, "stack": stackName(spec), "outputs": outputs})
	}}
}

func (o *options) infrastructureSpec() (string, awsstack.Spec, error) {
	return o.infrastructureSpecFor(false)
}

func (o *options) infrastructureSpecFor(allowExpiredPreview bool) (string, awsstack.Spec, error) {
	effective, environment, err := o.resolveWithEnvironment()
	if err != nil {
		return "", awsstack.Spec{}, err
	}
	spec, err := awsstack.PlanFromConfigWithOptions(effective.Config, environment, awsstack.PlanOptions{AllowExpiredPreview: allowExpiredPreview})
	if err != nil {
		return "", awsstack.Spec{}, err
	}
	if err := spec.Validate(); err != nil {
		return "", awsstack.Spec{}, fmt.Errorf("deployment configuration is invalid: %w", err)
	}
	return environment, spec, nil
}

func stackName(spec awsstack.Spec) string {
	return spec.Identity.Project + "-" + spec.Identity.Environment
}

func newAWSDeploymentLock(ctx context.Context, spec awsstack.Spec) (func(context.Context) error, error) {
	plan, err := awsbootstrap.BuildPlan(awsbootstrap.Spec{
		Project: spec.Identity.Project, Environment: spec.Identity.Environment,
		AccountID: spec.Identity.AccountID, Region: spec.Identity.Region,
		AccessLogBucket: "magelift-access-logs",
	})
	if err != nil {
		return nil, err
	}
	manager, err := awsstate.NewAWS(ctx, spec.Identity.Region, plan.StateBucket, spec.Identity.Project, spec.Identity.Environment, spec.Dependencies.KMSKeyARN)
	if err != nil {
		return nil, err
	}
	host, _ := os.Hostname()
	owner := fmt.Sprintf("magelift-cli-%s-%d", host, os.Getpid())
	handle, err := manager.Acquire(ctx, spec.Identity.Project, spec.Identity.Environment, owner)
	if err != nil {
		return nil, err
	}
	return handle.Release, nil
}
