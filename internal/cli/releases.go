package cli

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/magelift/magelift/internal/config"
	"github.com/magelift/magelift/internal/cosign"
	"github.com/magelift/magelift/internal/releasejournal"
	"github.com/spf13/cobra"
)

const rollbackWarning = "Rollback is a forward deployment of an older signed digest. Database migrations are not reversed; use expand/contract migrations for rollback-safe schema changes."

type releaseStore interface {
	List(context.Context) ([]releasejournal.Entry, error)
	Append(context.Context, releasejournal.Entry) (releasejournal.Entry, error)
}

func promoteCommand(o *options) *cobra.Command {
	var digest, sourceEnvironment, identity, issuer string
	command := &cobra.Command{Use: "promote", Short: "Record a signed image digest as a release", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		effective, environment, err := o.resolveWithEnvironment()
		if err != nil {
			return invalid(err)
		}
		if digest == "" {
			return invalid(errors.New("--digest is required"))
		}
		if identity == "" || issuer == "" {
			discoveredIdentity, discoveredIssuer, discoverErr := o.discoverSigningIdentity(cmd.Context())
			if discoverErr != nil {
				return invalid(discoverErr)
			}
			if identity == "" {
				identity = discoveredIdentity
			}
			if issuer == "" {
				issuer = discoveredIssuer
			}
		}
		if identity == "" || issuer == "" {
			return invalid(errors.New("cannot verify the signed image from the current login"))
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
		if err := requireCosignVerificationDependencies(cmd.Context(), o); err != nil {
			return err
		}
		if err := o.verifyRelease(cmd.Context(), digest, cosign.VerifyOptions{CertificateIdentity: identity, OIDCIssuer: issuer}); err != nil {
			return &exitError{code: 3, err: errors.New("signed release verification failed")}
		}
		store, err := o.releaseStore(environment)
		if err != nil {
			return err
		}
		entry, err := store.Append(cmd.Context(), releasejournal.Entry{Action: releasejournal.ActionPromote, Environment: environment, DigestReference: digest, SourceEnvironment: sourceEnvironment, SignatureIdentity: identity, SignatureIssuer: issuer, ForwardOnly: true, SchemaEpoch: o.schemaEpochOrZero(cmd.Context())})
		if err != nil {
			return err
		}
		return o.write(entry)
	}}
	command.Flags().StringVar(&digest, "digest", "", "signed registry digest reference")
	command.Flags().StringVar(&sourceEnvironment, "from", "", "source environment")
	command.Flags().StringVar(&identity, "certificate-identity", "", "advanced: expected signing identity; omit to use the current login")
	command.Flags().StringVar(&issuer, "certificate-oidc-issuer", "", "advanced: expected signing issuer; omit to use the current login")
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
		identity, issuer := source.SignatureIdentity, source.SignatureIssuer
		if identity == "" || issuer == "" {
			identity, issuer = releasejournal.SignatureMetadataForDigest(entries, source.DigestReference)
		}
		targetEpoch := source.SchemaEpoch
		if targetEpoch < 1 {
			targetEpoch = releasejournal.SchemaEpochForDigest(entries, source.DigestReference)
		}
		if identity == "" || issuer == "" {
			return invalid(errors.New("selected release has no verified signature metadata; promote the digest with magelift promote --digest before rollback"))
		}
		if err := cosign.ValidateReference(source.DigestReference); err != nil {
			return invalid(errors.New("selected release digest is invalid"))
		}
		if len(entries) != 0 && entries[len(entries)-1].DigestReference == source.DigestReference {
			return invalid(errors.New("selected release digest is already current"))
		}
		if o.verifyRelease == nil {
			return &exitError{code: 3, err: errors.New("signed release verification is unavailable")}
		}
		if err := requireCosignVerificationDependencies(cmd.Context(), o); err != nil {
			return err
		}
		if err := o.verifyRelease(cmd.Context(), source.DigestReference, cosign.VerifyOptions{CertificateIdentity: identity, OIDCIssuer: issuer}); err != nil {
			return &exitError{code: 3, err: errors.New("signed rollback verification failed")}
		}
		liveEpoch, err := o.currentSchemaEpoch(cmd.Context(), entries)
		if err != nil {
			return invalid(err)
		}
		if err := releasejournal.RefuseIncompatibleRollback(liveEpoch, targetEpoch); err != nil {
			return invalid(err)
		}
		if o.newDeploySteps != nil {
			_, planned, err := o.planStack(false)
			if err != nil {
				return invalid(err)
			}
			planned, err = planned.WithImageDigest(source.DigestReference)
			if err != nil {
				return invalid(err)
			}
			if _, err := o.runDeploymentWithOptions(cmd.Context(), environment, planned, source.DigestReference, deploymentOptions{rollback: true, acknowledgeForwardOnlyDB: acknowledgeForwardOnlyDB}); err != nil {
				return err
			}
		}
		entry, err := store.Append(cmd.Context(), releasejournal.Entry{Action: releasejournal.ActionRollback, Environment: environment, DigestReference: source.DigestReference, SourceEnvironment: environment, SourceSequence: source.Sequence, SignatureIdentity: identity, SignatureIssuer: issuer, ForwardOnly: true, DatabaseMigrationsReversed: false, SchemaEpoch: liveEpoch})
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

func evidenceCommand(o *options) *cobra.Command {
	return &cobra.Command{
		Use:   "evidence",
		Short: "Export reconstructable production change evidence without secret values",
		Long:  "Writes digest, actor, config provenance, and backup policy from the local release journal and effective configuration. Secret values are never included.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			result, err := o.runEvidence(cmd.Context())
			if err != nil {
				return err
			}
			return o.write(result)
		},
	}
}

func (o *options) runEvidence(ctx context.Context) (map[string]any, error) {
	effective, environment, err := o.resolveWithEnvironment()
	if err != nil {
		return nil, invalid(err)
	}
	store, err := o.releaseStore(environment)
	if err != nil {
		return nil, err
	}
	entries, err := store.List(ctx)
	if err != nil {
		return nil, err
	}
	changes := make([]map[string]any, 0, len(entries))
	for _, entry := range entries {
		changes = append(changes, map[string]any{
			"sequence":    entry.Sequence,
			"recordedAt":  entry.RecordedAt,
			"action":      entry.Action,
			"digest":      entry.DigestReference,
			"actor":       entry.SignatureIdentity,
			"schemaEpoch": entry.SchemaEpoch,
		})
	}
	provenance := make(map[string]string, len(effective.Provenance))
	for path, item := range effective.Provenance {
		provenance[path] = item.Source
	}
	return map[string]any{
		"environment":      environment,
		"configDigest":     effective.Fingerprint,
		"configProvenance": provenance,
		"backupPolicy": map[string]any{
			"retentionDays":     effective.Config.Resilience.RetentionDays,
			"retainedOnDestroy": config.RetainedBackupsOnDestroy(effective.Config),
		},
		"changes": changes,
	}, nil
}

func (o *options) releaseStore(environment string) (releaseStore, error) {
	if o.newReleaseStore == nil {
		return nil, errors.New("release journal is unavailable")
	}
	return o.newReleaseStore(filepath.Dir(filepath.Clean(o.configPath)), environment)
}

func (o *options) schemaEpochOrZero(ctx context.Context) int {
	if o.liveSchemaEpoch == nil {
		return 0
	}
	epoch, err := o.liveSchemaEpoch(ctx)
	if err != nil {
		return 0
	}
	return epoch
}

func (o *options) currentSchemaEpoch(ctx context.Context, entries []releasejournal.Entry) (int, error) {
	if o.liveSchemaEpoch != nil {
		epoch, err := o.liveSchemaEpoch(ctx)
		if err != nil {
			return 0, err
		}
		return epoch, nil
	}
	if len(entries) == 0 {
		return 0, nil
	}
	return entries[len(entries)-1].SchemaEpoch, nil
}
