package cli

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"

	awsstack "github.com/acourtiol/magelift/internal/cloud/aws/stack"
	"github.com/acourtiol/magelift/internal/cosign"
	"github.com/acourtiol/magelift/internal/releasejournal"
	"github.com/spf13/cobra"
)

const rollbackWarning = "Rollback is a forward deployment of an older signed digest. Database migrations are not reversed; use expand/contract migrations for rollback-safe schema changes."

type releaseStore interface {
	List(context.Context) ([]releasejournal.Entry, error)
	Append(context.Context, releasejournal.Entry) (releasejournal.Entry, error)
}

func promoteCommand(o *options) *cobra.Command {
	var digest, sourceEnvironment, identity, issuer string
	command := &cobra.Command{Use: "promote", Short: "Record promotion of a signed immutable digest", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		effective, environment, err := o.resolveWithEnvironment()
		if err != nil {
			return invalid(err)
		}
		if digest == "" || identity == "" || issuer == "" {
			return invalid(errors.New("--digest, --certificate-identity, and --certificate-oidc-issuer are required"))
		}
		if err := cosign.ValidateReference(digest); err != nil {
			return invalid(err)
		}
		if effective.Config.Class == "production" && !o.yes {
			return invalid(errors.New("production promotion requires --yes"))
		}
		if o.verifyRelease == nil {
			return &exitError{code: 3, err: errors.New("signed release verification is unavailable")}
		}
		if err := o.verifyRelease(cmd.Context(), digest, cosign.VerifyOptions{CertificateIdentity: identity, OIDCIssuer: issuer}); err != nil {
			return &exitError{code: 3, err: errors.New("signed release verification failed")}
		}
		store, err := o.releaseStore(environment)
		if err != nil {
			return err
		}
		entry, err := store.Append(cmd.Context(), releasejournal.Entry{Action: releasejournal.ActionPromote, Environment: environment, DigestReference: digest, SourceEnvironment: sourceEnvironment, SignatureIdentity: identity, SignatureIssuer: issuer, ForwardOnly: true})
		if err != nil {
			return err
		}
		return o.write(entry)
	}}
	command.Flags().StringVar(&digest, "digest", "", "signed registry digest reference")
	command.Flags().StringVar(&sourceEnvironment, "from", "", "source environment")
	command.Flags().StringVar(&identity, "certificate-identity", "", "expected signing certificate identity")
	command.Flags().StringVar(&issuer, "certificate-oidc-issuer", "", "expected signing certificate OIDC issuer")
	return command
}

func rollbackCommand(o *options) *cobra.Command {
	var sequence int
	var acknowledgeForwardOnlyDB bool
	command := &cobra.Command{Use: "rollback", Short: "Deploy a previous signed digest as a new release", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		effective, environment, err := o.resolveWithEnvironment()
		if err != nil {
			return invalid(err)
		}
		if !acknowledgeForwardOnlyDB {
			return invalid(errors.New("rollback requires --ack-forward-only because database migrations are never reversed; use expand/contract migrations"))
		}
		if sequence < 1 {
			return invalid(errors.New("--to-sequence must select a previous release"))
		}
		if effective.Config.Class == "production" && !o.yes {
			return invalid(errors.New("production rollback requires --yes"))
		}
		store, err := o.releaseStore(environment)
		if err != nil {
			return err
		}
		entries, err := store.List(cmd.Context())
		if err != nil {
			return err
		}
		if sequence > len(entries) {
			return invalid(fmt.Errorf("release sequence %d does not exist", sequence))
		}
		source := entries[sequence-1]
		if source.SignatureIdentity == "" || source.SignatureIssuer == "" {
			return invalid(errors.New("selected release has no verified signature metadata"))
		}
		if err := cosign.ValidateReference(source.DigestReference); err != nil {
			return invalid(errors.New("selected release digest is invalid"))
		}
		if len(entries) != 0 && entries[len(entries)-1].DigestReference == source.DigestReference {
			return invalid(errors.New("selected release digest is already current"))
		}
		if o.newDeploySteps != nil {
			if o.verifyRelease == nil {
				return &exitError{code: 3, err: errors.New("signed release verification is unavailable")}
			}
			if err := o.verifyRelease(cmd.Context(), source.DigestReference, cosign.VerifyOptions{CertificateIdentity: source.SignatureIdentity, OIDCIssuer: source.SignatureIssuer}); err != nil {
				return &exitError{code: 3, err: errors.New("signed rollback verification failed")}
			}
			_, spec, err := o.infrastructureSpec()
			if err != nil {
				return invalid(err)
			}
			planned := awsstack.Planned{Spec: spec}
			planned.Spec.Artifact.ImageDigest = source.DigestReference
			if _, err := o.runDeploymentWithOptions(cmd.Context(), environment, planned, source.DigestReference, deploymentOptions{rollback: true, acknowledgeForwardOnlyDB: acknowledgeForwardOnlyDB}); err != nil {
				return err
			}
		}
		entry, err := store.Append(cmd.Context(), releasejournal.Entry{Action: releasejournal.ActionRollback, Environment: environment, DigestReference: source.DigestReference, SourceEnvironment: environment, SourceSequence: source.Sequence, SignatureIdentity: source.SignatureIdentity, SignatureIssuer: source.SignatureIssuer, ForwardOnly: true, DatabaseMigrationsReversed: false})
		if err != nil {
			return err
		}
		_, _ = fmt.Fprintln(o.stderr, rollbackWarning)
		return o.write(map[string]any{"release": entry, "warning": rollbackWarning})
	}}
	command.Flags().IntVar(&sequence, "to-sequence", 0, "previous release sequence")
	command.Flags().BoolVar(&acknowledgeForwardOnlyDB, "ack-forward-only", false, "acknowledge that rollback never reverses database migrations")
	return command
}

func historyCommand(o *options) *cobra.Command {
	return &cobra.Command{Use: "history", Short: "List the local release journal", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		_, environment, err := o.resolveWithEnvironment()
		if err != nil {
			return invalid(err)
		}
		store, err := o.releaseStore(environment)
		if err != nil {
			return err
		}
		entries, err := store.List(cmd.Context())
		if err != nil {
			return err
		}
		return o.write(map[string]any{"environment": environment, "releases": entries})
	}}
}

func (o *options) releaseStore(environment string) (releaseStore, error) {
	if o.newReleaseStore == nil {
		return nil, errors.New("release journal is unavailable")
	}
	return o.newReleaseStore(filepath.Dir(filepath.Clean(o.configPath)), environment)
}
