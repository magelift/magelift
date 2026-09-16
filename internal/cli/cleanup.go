package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/magelift/magelift/internal/cleanup"
	gcpresilience "github.com/magelift/magelift/internal/cloud/gcp/resilience"
	ovhresilience "github.com/magelift/magelift/internal/cloud/ovh/resilience"
	"github.com/magelift/magelift/internal/cloud/scaleway/resilience"
	"github.com/magelift/magelift/sdk"
	"github.com/scaleway/scaleway-sdk-go/scw"
	"github.com/spf13/cobra"
)

// CleanupProvider inventories and deletes ledger-owned provider resources.
type CleanupProvider interface {
	Inventory(context.Context, sdk.CleanupInventoryRequest) ([]sdk.CleanupInventoryResource, error)
	Delete(context.Context, sdk.CleanupResource) error
}

type cleanupProvider = CleanupProvider

func cleanupCommand(o *options) *cobra.Command {
	command := &cobra.Command{
		Use:   "cleanup",
		Short: "Reconcile interrupted environment and acceptance cleanup",
	}
	command.AddCommand(cleanupClaimCommand(o), cleanupRecordCommand(o), cleanupPlanCommand(o), cleanupReconcileCommand(o))
	return command
}

func cleanupClaimCommand(o *options) *cobra.Command {
	var ledgerPath, runID, marker, providerName, region, project, profile, kind, role, name, identity string
	var rank int
	command := &cobra.Command{
		Use:   "claim",
		Short: "Record an owned resource before creating it",
		Args:  cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			if ledgerPath == "" {
				return invalid(errors.New("--ledger is required"))
			}
			now := time.Now().UTC().Format(time.RFC3339)
			ledger, err := loadOrInitCleanupLedger(ledgerPath, sdk.CleanupLedger{
				Version:   sdk.CleanupLedgerVersion,
				RunID:     runID,
				Marker:    marker,
				Provider:  providerName,
				Region:    region,
				Project:   project,
				Profile:   profile,
				ClaimedAt: now,
			})
			if err != nil {
				return invalid(err)
			}
			resource := sdk.CleanupResource{Kind: kind, Role: role, Name: name, Identity: identity, Rank: rank, Status: sdk.CleanupStatusIntended}
			ledger, err = cleanup.Claim(ledgerPath, ledger, resource)
			if err != nil {
				return guidedWrap(err, "could not record the cleanup claim", "fix the ledger fields and claim again before creating the cloud resource", "docs/cli-reference.md")
			}
			return o.write(ledger)
		},
	}
	command.Flags().StringVar(&ledgerPath, "ledger", "", "path to the cleanup ledger JSON file")
	command.Flags().StringVar(&runID, "run-id", "", "stable run identity")
	command.Flags().StringVar(&marker, "marker", "", "ownership marker")
	command.Flags().StringVar(&providerName, "provider", "", "provider ID, such as scaleway")
	command.Flags().StringVar(&region, "region", "", "provider region")
	command.Flags().StringVar(&project, "project", "", "provider project or account")
	command.Flags().StringVar(&profile, "profile", "", "local CLI profile name")
	command.Flags().StringVar(&kind, "kind", "", "resource kind, such as rdb-instance")
	command.Flags().StringVar(&role, "role", "", "resource role: source, restore, or snapshot")
	command.Flags().StringVar(&name, "name", "", "provider name used before the identity exists")
	command.Flags().StringVar(&identity, "identity", "", "provider identity if already known")
	command.Flags().IntVar(&rank, "rank", 0, "deletion rank; snapshots are lower than source instances")
	return command
}

func cleanupRecordCommand(o *options) *cobra.Command {
	var ledgerPath, kind, name, identity string
	command := &cobra.Command{
		Use:   "record",
		Short: "Bind a provider identity after create succeeds",
		Args:  cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			if ledgerPath == "" || kind == "" || name == "" || identity == "" {
				return invalid(errors.New("--ledger, --kind, --name, and --identity are required"))
			}
			ledger, err := cleanup.Load(ledgerPath)
			if err != nil {
				return invalid(err)
			}
			ledger, err = cleanup.RecordIdentity(ledgerPath, ledger, kind, name, identity)
			if err != nil {
				return invalid(err)
			}
			return o.write(ledger)
		},
	}
	command.Flags().StringVar(&ledgerPath, "ledger", "", "path to the cleanup ledger JSON file")
	command.Flags().StringVar(&kind, "kind", "", "resource kind, such as rdb-instance")
	command.Flags().StringVar(&name, "name", "", "claimed provider name")
	command.Flags().StringVar(&identity, "identity", "", "provider identity returned by create")
	return command
}

func cleanupPlanCommand(o *options) *cobra.Command {
	var ledgerPath, dir string
	command := &cobra.Command{
		Use:   "plan",
		Short: "Show owned resources a reconcile would delete",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return o.runCleanup(cmd.Context(), ledgerPath, dir, true)
		},
	}
	command.Flags().StringVar(&ledgerPath, "ledger", "", "path to one cleanup ledger JSON file")
	command.Flags().StringVar(&dir, "dir", "", "directory of cleanup ledger JSON files")
	return command
}

func cleanupReconcileCommand(o *options) *cobra.Command {
	var ledgerPath, dir string
	command := &cobra.Command{
		Use:   "reconcile",
		Short: "Delete claimed resources left behind by an interrupted run",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if !o.yes {
				return invalid(errors.New("cleanup reconcile requires --yes; use cleanup plan for a dry run"))
			}
			return o.runCleanup(cmd.Context(), ledgerPath, dir, false)
		},
	}
	command.Flags().StringVar(&ledgerPath, "ledger", "", "path to one cleanup ledger JSON file")
	command.Flags().StringVar(&dir, "dir", "", "directory of cleanup ledger JSON files")
	return command
}

func (o *options) runCleanup(ctx context.Context, ledgerPath, dir string, dryRun bool) error {
	paths, err := cleanupLedgerPaths(ledgerPath, dir)
	if err != nil {
		return invalid(err)
	}
	reports := make([]sdk.CleanupReport, 0, len(paths))
	for _, path := range paths {
		ledger, err := cleanup.Load(path)
		if err != nil {
			return invalid(err)
		}
		provider, err := o.cleanupProviderFor(ctx, ledger)
		if err != nil {
			return guidedWrap(err, "could not construct the cleanup provider", "check the ledger provider, region, project, and local CLI profile, then rerun cleanup plan", "docs/cli-reference.md")
		}
		report, err := sdk.ReconcileCleanup(ctx, ledger, provider, provider, sdk.CleanupReconcileOptions{
			DryRun: dryRun,
			Persist: func(current sdk.CleanupLedger) error {
				return cleanup.Save(path, current)
			},
		})
		if err != nil {
			return guidedWrap(err, "cleanup reconcile did not finish", "run magelift cleanup plan on the same ledger, then retry with --yes", "docs/cli-reference.md")
		}
		reports = append(reports, report)
		if report.Status == "failed" {
			return o.writeCleanupReports(reports, &exitError{code: 1, err: errors.New(report.Detail)})
		}
	}
	return o.writeCleanupReports(reports, nil)
}

func (o *options) writeCleanupReports(reports []sdk.CleanupReport, result error) error {
	var value any = reports
	if len(reports) == 1 {
		value = reports[0]
	}
	if len(reports) == 0 {
		value = map[string]any{"status": "complete", "reports": []sdk.CleanupReport{}}
	}
	if err := o.write(value); err != nil {
		return err
	}
	return result
}

func (o *options) cleanupProviderFor(ctx context.Context, ledger sdk.CleanupLedger) (cleanupProvider, error) {
	if o.newCleanupProvider != nil {
		return o.newCleanupProvider(ctx, ledger)
	}
	if ledger.Provider == "gcp" && o.newGCPCleanupProvider != nil {
		return o.newGCPCleanupProvider(ctx, ledger)
	}
	return defaultCleanupProvider(ctx, ledger)
}

func defaultCleanupProvider(ctx context.Context, ledger sdk.CleanupLedger) (cleanupProvider, error) {
	switch ledger.Provider {
	case "gcp":
		if strings.TrimSpace(ledger.Project) == "" {
			return nil, errors.New("GCP cleanup ledgers require a project")
		}
		sqlAPI, err := gcpresilience.NewCloudSQLAPI(ctx)
		if err != nil {
			return nil, fmt.Errorf("create GCP Cloud SQL cleanup client: %w", err)
		}
		return cleanup.GCPCloudSQLProvider{SQL: sqlAPI, Project: strings.TrimSpace(ledger.Project), Marker: ledger.Marker}, nil
	case "ovh":
		return newOVHDatabaseCleanupProvider(ctx, ledger)
	case "scaleway":
		return newScalewayCleanupProvider(ctx, ledger)
	default:
		return nil, cleanup.UnsupportedProvider(ledger.Provider)
	}
}

func newOVHDatabaseCleanupProvider(ctx context.Context, ledger sdk.CleanupLedger) (cleanupProvider, error) {
	if strings.TrimSpace(ledger.Profile) == "" || strings.TrimSpace(ledger.Region) == "" || strings.TrimSpace(ledger.Project) == "" {
		return nil, errors.New("OVHcloud cleanup ledgers require profile, region, and project")
	}
	client, err := ovhresilience.NewOVHClientFromProfile(ledger.Profile)
	if err != nil {
		return nil, err
	}
	native, err := ovhresilience.NewOVHDatabaseNativeAPI(ctx, ovhresilience.NativeAPIConfig{
		DatabaseProjectID: ledger.Project, DatabaseEngine: "mysql", DatabaseRegion: ledger.Region,
		DatabaseDeletePollInterval: time.Second,
	}, client)
	if err != nil {
		return nil, err
	}
	return cleanup.OVHDatabaseProvider{Database: native.Database(), Engine: "mysql", Marker: ledger.Marker}, nil
}

func newScalewayCleanupProvider(ctx context.Context, ledger sdk.CleanupLedger) (cleanupProvider, error) {
	if strings.TrimSpace(ledger.Profile) == "" || strings.TrimSpace(ledger.Region) == "" || strings.TrimSpace(ledger.Project) == "" {
		return nil, errors.New("Scaleway cleanup ledgers require profile, region, and project")
	}
	config, err := scw.LoadConfig()
	if err != nil {
		return nil, fmt.Errorf("load Scaleway profile configuration: %w", err)
	}
	profileConfig, err := config.GetProfile(ledger.Profile)
	if err != nil {
		return nil, fmt.Errorf("load Scaleway profile %q: %w", ledger.Profile, err)
	}
	profileClient, err := scw.NewClient(scw.WithProfile(profileConfig))
	if err != nil {
		return nil, fmt.Errorf("construct Scaleway profile client: %w", err)
	}
	accessKey, accessKeyOK := profileClient.GetAccessKey()
	secretKey, secretKeyOK := profileClient.GetSecretKey()
	if !accessKeyOK || !secretKeyOK || strings.TrimSpace(accessKey) == "" || strings.TrimSpace(secretKey) == "" {
		return nil, errors.New("selected Scaleway profile does not contain an access key and secret key")
	}
	native, err := resilience.NewScalewayDatabaseNativeAPI(ctx, resilience.NativeAPIConfig{
		Region:            ledger.Region,
		DatabaseRegion:    ledger.Region,
		DatabaseProjectID: ledger.Project,
		Credentials:       credentials.NewStaticCredentialsProvider(accessKey, secretKey, ""),
		RetentionDays:     1,
	}, scw.WithProfile(profileConfig), scw.WithDefaultProjectID(ledger.Project), scw.WithDefaultRegion(scw.Region(ledger.Region)))
	if err != nil {
		return nil, err
	}
	return cleanup.ScalewayDatabaseProvider{Database: native.Database(), Marker: ledger.Marker}, nil
}

func loadOrInitCleanupLedger(path string, seed sdk.CleanupLedger) (sdk.CleanupLedger, error) {
	_, err := os.Stat(path)
	if err == nil {
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return sdk.CleanupLedger{}, fmt.Errorf("read cleanup ledger: %w", readErr)
		}
		if len(bytes.TrimSpace(data)) > 0 {
			return cleanup.Load(path)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return sdk.CleanupLedger{}, err
	}
	if seed.RunID == "" || seed.Marker == "" || seed.Provider == "" {
		return sdk.CleanupLedger{}, errors.New("creating a cleanup ledger requires --run-id, --marker, and --provider")
	}
	return seed, nil
}

func cleanupLedgerPaths(ledgerPath, dir string) ([]string, error) {
	ledgerPath = strings.TrimSpace(ledgerPath)
	dir = strings.TrimSpace(dir)
	if ledgerPath != "" && dir != "" {
		return nil, errors.New("specify either --ledger or --dir, not both")
	}
	if ledgerPath == "" && dir == "" {
		dir = cleanup.DefaultDir
	}
	if ledgerPath != "" {
		return []string{ledgerPath}, nil
	}
	if dir == cleanup.DefaultDir {
		if cwd, err := os.Getwd(); err == nil {
			dir = filepath.Join(cwd, cleanup.DefaultDir)
		}
	}
	paths, _, err := cleanup.ListDir(dir)
	if err != nil {
		return nil, err
	}
	return paths, nil
}
