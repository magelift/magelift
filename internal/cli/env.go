package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/magelift/magelift/internal/automation"
	awsendpoint "github.com/magelift/magelift/internal/cloud/aws/endpoint"
	"github.com/magelift/magelift/internal/config"
	"github.com/magelift/magelift/internal/dumpimport"
	"github.com/magelift/magelift/internal/localdev"
	"github.com/magelift/magelift/internal/mediasync"
	"github.com/magelift/magelift/internal/platform"
	"github.com/magelift/magelift/internal/seeddump"
	"github.com/magelift/magelift/internal/toolchain"
	"github.com/spf13/cobra"
	"go.yaml.in/yaml/v4"
)

var environmentName = regexp.MustCompile(`^[a-z][a-z0-9-]{0,30}$`)

func envListCommand(o *options) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List configured environments",
		Args:  cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			file, err := o.load()
			if err != nil {
				return invalid(err)
			}
			names := file.Environments()
			if len(names) == 0 {
				return invalid(fmt.Errorf("%s does not define any environments", o.configPath))
			}
			rows := make([]map[string]string, len(names))
			for index, name := range names {
				rows[index] = map[string]string{"name": name}
				if name == o.environment {
					rows[index]["selected"] = "true"
				}
			}
			return o.write(rows)
		},
	}
}

func envCreateCommand(o *options) *cobra.Command {
	var account, class, preset, domain, expiresAt, dumpPath string
	var monthlyBudgetCents int64
	var protection bool
	var branches []string
	command := &cobra.Command{
		Use:   "create <environment>",
		Short: "Add an environment overlay to magelift.yaml",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			if !environmentName.MatchString(name) {
				return invalid(errors.New("environment name must be a lowercase stable name"))
			}
			if cmd.Flags().Changed("monthly-budget-cents") && monthlyBudgetCents <= 0 {
				return invalid(errors.New("monthly budget must be greater than zero"))
			}
			if dumpPath != "" {
				if _, err := os.Stat(dumpPath); err != nil {
					return guidedWrap(err,
						fmt.Sprintf("database dump %q is not readable", dumpPath),
						"pass an existing .sql or .sql.gz path; import runs after the environment first deploy (ADR 0010)",
						"docs/adr/0010-database-dump-seed.md",
					)
				}
			}
			file, err := o.load()
			if err != nil {
				return invalid(err)
			}
			for _, existing := range file.Environments() {
				if existing == name {
					return invalid(fmt.Errorf("environment %q already exists", name))
				}
			}
			data, err := os.ReadFile(filepath.Clean(o.configPath))
			if err != nil {
				return invalid(err)
			}
			var document map[string]any
			if err := yaml.Unmarshal(data, &document); err != nil {
				return invalid(fmt.Errorf("decode configuration for environment creation: %w", err))
			}
			environments, ok := document["environments"].(map[string]any)
			if !ok {
				return invalid(errors.New("environments must be a map"))
			}
			overlay := map[string]any{}
			if cmd.Flags().Changed("account") {
				overlay["account"] = account
			}
			if cmd.Flags().Changed("class") {
				overlay["class"] = class
			}
			if cmd.Flags().Changed("preset") {
				overlay["preset"] = preset
			}
			if cmd.Flags().Changed("domain") {
				overlay["domain"] = domain
			}
			if cmd.Flags().Changed("expires-at") {
				overlay["expiresAt"] = expiresAt
			}
			if cmd.Flags().Changed("monthly-budget-cents") {
				overlay["monthlyBudgetCents"] = monthlyBudgetCents
			}
			if cmd.Flags().Changed("protection") {
				overlay["protection"] = protection
			}
			if cmd.Flags().Changed("branches") {
				overlay["branches"] = branches
			}
			if dumpPath != "" {
				overlay["seedDump"] = dumpPath
			}
			environments[name] = overlay
			document["environments"] = environments
			updated, err := yaml.Marshal(document)
			if err != nil {
				return fmt.Errorf("encode environment configuration: %w", err)
			}
			prospective, err := config.Load(updated)
			if err != nil {
				return invalid(fmt.Errorf("created environment is invalid: %w", err))
			}
			if _, err := prospective.Resolve(name, config.ResolveOptions{}); err != nil {
				return invalid(fmt.Errorf("created environment is invalid: %w", err))
			}
			info, err := os.Stat(filepath.Clean(o.configPath))
			if err != nil {
				return err
			}
			if err := replaceFile(filepath.Clean(o.configPath), updated, info.Mode().Perm()); err != nil {
				return err
			}
			result := map[string]any{"environment": name, "created": true}
			if dumpPath != "" {
				projectRoot := filepath.Dir(filepath.Clean(o.configPath))
				record, err := seeddump.InitRecorded(cmd.Context(), projectRoot, name, dumpPath)
				if err != nil {
					return fmt.Errorf("initialize seed dump journal: %w", err)
				}
				result["seedDump"] = dumpPath
				result["seedDumpStatus"] = record.Status
			}
			return o.write(result)
		},
	}
	command.Flags().StringVar(&account, "account", "", "AWS account ID")
	command.Flags().StringVar(&class, "class", "", "environment class")
	command.Flags().StringVar(&preset, "preset", "", "infrastructure preset")
	command.Flags().StringVar(&domain, "domain", "", "environment domain")
	command.Flags().StringVar(&expiresAt, "expires-at", "", "preview expiration as RFC3339")
	command.Flags().Int64Var(&monthlyBudgetCents, "monthly-budget-cents", 0, "maximum monthly AWS budget in cents")
	command.Flags().BoolVar(&protection, "protection", false, "protect destructive operations")
	command.Flags().StringSliceVar(&branches, "branches", nil, "Git branches mapped to this environment")
	command.Flags().StringVar(&dumpPath, "dump", "", "path to a MySQL dump to seed after first deploy (ADR 0010)")
	return command
}

func envDestroyCommand(o *options) *cobra.Command {
	var destroyBackups bool
	command := &cobra.Command{
		Use:   "destroy <environment>",
		Short: "Destroy an environment and remove its configuration overlay",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !o.yes {
				return invalid(errors.New("destroying an environment requires --yes"))
			}
			o.destroyBackups = destroyBackups
			result, err := o.destroyEnvironment(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			return o.write(result)
		},
	}
	command.Flags().BoolVar(&destroyBackups, "destroy-backups", false, "also destroy leftover provider backups after the environment is gone; GCP Cloud SQL leftovers are deleted even when the backup policy is disposable, AWS snapshots are still refused")
	return command
}

type environmentSweepEntry struct {
	Environment string    `json:"environment" yaml:"environment"`
	ExpiresAt   time.Time `json:"expiresAt" yaml:"expiresAt"`
	Action      string    `json:"action" yaml:"action"`
	Reason      string    `json:"reason,omitempty" yaml:"reason,omitempty"`
	Result      any       `json:"result,omitempty" yaml:"result,omitempty"`
}

func envSweepCommand(o *options) *cobra.Command {
	var before string
	var dryRun bool
	command := &cobra.Command{
		Use:   "sweep",
		Short: "Destroy expired preview environments",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if !dryRun && !o.yes {
				return invalid(errors.New("environment sweep requires --yes unless --dry-run is used"))
			}
			cutoff := time.Now().UTC()
			if strings.TrimSpace(before) != "" {
				parsed, err := time.Parse(time.RFC3339, before)
				if err != nil {
					return invalid(fmt.Errorf("--before must be RFC3339: %w", err))
				}
				cutoff = parsed.UTC()
			}
			file, err := o.load()
			if err != nil {
				return invalid(err)
			}
			if o.previewIdentityRequested() {
				result, err := o.runPreviewSweep(cmd.Context(), file, cutoff, dryRun)
				if err != nil {
					return err
				}
				return o.write(result)
			}
			entries := make([]environmentSweepEntry, 0)
			for _, name := range file.Environments() {
				effective, err := file.Resolve(name, config.ResolveOptions{})
				if err != nil {
					return invalid(fmt.Errorf("resolve environment %q: %w", name, err))
				}
				expiresAt, err := parseSweepExpiration(effective.Config.ExpiresAt)
				if err != nil {
					return invalid(fmt.Errorf("environment %q: %w", name, err))
				}
				if expiresAt.IsZero() || expiresAt.After(cutoff) {
					continue
				}
				entry := environmentSweepEntry{Environment: name, ExpiresAt: expiresAt, Action: "skipped"}
				if effective.Config.Class != "preview" {
					entry.Reason = "only preview environments are eligible for TTL cleanup"
				} else if effective.Config.Protection {
					entry.Reason = "environment is protected"
				} else if dryRun {
					entry.Action = "would-destroy"
				} else {
					result, err := o.destroyEnvironment(cmd.Context(), name)
					if err != nil {
						return fmt.Errorf("destroy expired environment %q: %w", name, err)
					}
					entry.Action = "destroyed"
					entry.Result = result
				}
				entries = append(entries, entry)
				if !dryRun {
					file, err = o.load()
					if err != nil {
						return err
					}
				}
			}
			return o.write(map[string]any{"before": cutoff, "environments": entries})
		},
	}
	command.Flags().StringVar(&before, "before", "", "treat environments expiring at or before this RFC3339 time as expired")
	command.Flags().BoolVar(&dryRun, "dry-run", false, "report expired environments without changing infrastructure or configuration")
	return command
}

func (o *options) runPreviewSweep(ctx context.Context, file *config.File, cutoff time.Time, dryRun bool) (map[string]any, error) {
	repository, err := config.CanonicalPreviewRepository(o.previewRepository)
	if err != nil {
		return nil, invalid(fmt.Errorf("preview sweep requires a valid --preview-repository: %w", err))
	}
	if o.previewPullRequest < 0 {
		return nil, invalid(errors.New("--preview-number cannot be negative"))
	}
	baseEnvironment, err := o.selectEnvironment(file)
	if err != nil {
		return nil, invalid(err)
	}
	baseEffective, err := file.Resolve(baseEnvironment, config.ResolveOptions{})
	if err != nil {
		return nil, invalid(fmt.Errorf("resolve preview environment %q: %w", baseEnvironment, err))
	}
	if baseEffective.Config.Class != "preview" {
		return nil, invalid(errors.New("preview sweep requires a selected environment with class preview"))
	}
	if o.modules == nil {
		return nil, errors.New("stack module registry is required")
	}
	_, planned, err := o.modules.Plan(baseEffective.Config, baseEnvironment, platform.PlanOptions{AllowExpiredPreview: true, Context: ctx})
	if err != nil {
		return nil, invalid(err)
	}
	if err := requireTargetDependencies(ctx, o, planned); err != nil {
		return nil, err
	}
	listRecords := o.listPreviewRecords
	if listRecords == nil {
		listRecords = automation.ListPreviewRecords
	}
	records, err := listRecords(ctx, o.infrastructureBackendURL(planned), baseEffective.Config.Project.Name)
	if err != nil {
		return nil, fmt.Errorf("list preview ownership records: %w", err)
	}
	entries := make([]environmentSweepEntry, 0, len(records))
	for _, record := range records {
		if record.Metadata.Repository != repository {
			continue
		}
		if o.previewPullRequest > 0 && record.Metadata.PullRequest != o.previewPullRequest {
			continue
		}
		expiresAt, err := parseSweepExpiration(record.Metadata.ExpiresAt)
		if err != nil {
			return nil, invalid(fmt.Errorf("preview stack %q: %w", record.StackName, err))
		}
		if expiresAt.IsZero() || expiresAt.After(cutoff) {
			continue
		}
		entry := environmentSweepEntry{Environment: record.Metadata.Environment, ExpiresAt: expiresAt, Action: "skipped"}
		if baseEffective.Config.Protection {
			entry.Reason = "environment is protected"
		} else if dryRun {
			entry.Action = "would-destroy"
			entry.Result = map[string]any{"stack": record.StackName, "generation": record.Metadata.Generation}
		} else {
			result, err := o.destroyPreviewRecord(ctx, baseEnvironment, record)
			if err != nil {
				return nil, fmt.Errorf("destroy expired preview stack %q: %w", record.StackName, err)
			}
			entry.Action = "destroyed"
			entry.Result = result
		}
		entries = append(entries, entry)
	}
	return map[string]any{"before": cutoff, "environments": entries}, nil
}

func (o *options) destroyPreviewRecord(ctx context.Context, baseEnvironment string, record automation.PreviewRecord) (map[string]any, error) {
	identity, err := previewIdentityFromRecord(record.Metadata)
	if err != nil {
		return nil, err
	}
	previousEnvironment := o.environment
	previousRepository := o.previewRepository
	previousPullRequest := o.previewPullRequest
	previousBranch := o.previewBranch
	previousCommit := o.previewCommit
	previousDomain := o.previewDomain
	previousGeneration := o.previewGeneration
	previousIdentity := o.previewIdentityOverride
	previousResolvedIdentity := o.resolvedPreviewIdentity
	o.environment = baseEnvironment
	o.previewRepository = identity.Repository
	o.previewPullRequest = identity.PullRequest
	o.previewBranch = identity.Branch
	o.previewCommit = identity.CommitDigest
	o.previewDomain = identity.Domain
	o.previewGeneration = identity.Generation
	o.previewIdentityOverride = &identity
	defer func() {
		o.environment = previousEnvironment
		o.previewRepository = previousRepository
		o.previewPullRequest = previousPullRequest
		o.previewBranch = previousBranch
		o.previewCommit = previousCommit
		o.previewDomain = previousDomain
		o.previewGeneration = previousGeneration
		o.previewIdentityOverride = previousIdentity
		o.resolvedPreviewIdentity = previousResolvedIdentity
	}()
	_, planned, err := o.planStack(true)
	if err != nil {
		return nil, err
	}
	if planned.StackName() != record.StackName {
		return nil, fmt.Errorf("preview ownership record points to stack %q, expected %q", record.StackName, planned.StackName())
	}
	result, err := o.executeInfrastructure(ctx, "destroy", o.destroyOperation, "")
	if err != nil {
		return nil, err
	}
	return map[string]any{"stack": result.Stack, "generation": identity.Generation, "destroyed": result}, nil
}

func previewIdentityFromRecord(metadata automation.PreviewMetadata) (config.PreviewIdentity, error) {
	identity, err := config.BuildPreviewIdentity(config.PreviewIdentityInput{
		Project:     metadata.Project,
		Repository:  metadata.Repository,
		PullRequest: metadata.PullRequest,
		Branch:      metadata.Branch,
		Commit:      metadata.CommitDigest,
		Generation:  metadata.Generation,
		Domain:      metadata.Domain,
		ExpiresAt:   metadata.ExpiresAt,
	})
	if err != nil {
		return config.PreviewIdentity{}, fmt.Errorf("rebuild preview identity: %w", err)
	}
	if identity.Environment != metadata.Environment || identity.StackKey != metadata.StackKey || identity.OwnershipMarker != metadata.Owner {
		return config.PreviewIdentity{}, errors.New("persisted preview ownership record does not match its derived identity")
	}
	return identity, nil
}

func parseSweepExpiration(value string) (time.Time, error) {
	if value == "" {
		return time.Time{}, nil
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}, fmt.Errorf("expiresAt must be RFC3339: %w", err)
	}
	return parsed.UTC(), nil
}

func (o *options) destroyEnvironment(ctx context.Context, name string) (map[string]any, error) {
	if !environmentName.MatchString(name) {
		return nil, invalid(errors.New("environment name must be a lowercase stable name"))
	}
	file, err := o.load()
	if err != nil {
		return nil, invalid(err)
	}
	found := false
	for _, existing := range file.Environments() {
		if existing == name {
			found = true
			break
		}
	}
	if !found {
		return nil, invalid(fmt.Errorf("environment %q does not exist", name))
	}
	if _, _, err := o.resolveEnvironment(file, name); err != nil {
		return nil, invalid(err)
	}
	previous := o.environment
	o.environment = name
	defer func() { o.environment = previous }()
	result, err := o.executeInfrastructure(ctx, "destroy", o.destroyOperation, "")
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(filepath.Clean(o.configPath))
	if err != nil {
		return nil, err
	}
	var document map[string]any
	if err := yaml.Unmarshal(data, &document); err != nil {
		return nil, fmt.Errorf("decode configuration after environment destroy: %w", err)
	}
	environments, ok := document["environments"].(map[string]any)
	if !ok {
		return nil, errors.New("environments must be a map")
	}
	delete(environments, name)
	document["environments"] = environments
	updated, err := yaml.Marshal(document)
	if err != nil {
		return nil, fmt.Errorf("encode configuration after environment destroy: %w", err)
	}
	info, err := os.Stat(filepath.Clean(o.configPath))
	if err != nil {
		return nil, err
	}
	if err := replaceFile(filepath.Clean(o.configPath), updated, info.Mode().Perm()); err != nil {
		return nil, fmt.Errorf("remove destroyed environment from configuration: %w", err)
	}
	return map[string]any{"environment": name, "destroyed": result, "removed": true}, nil
}

func envProtectCommand(o *options) *cobra.Command {
	command := &cobra.Command{
		Use:   "protect <environment>",
		Short: "Enable or disable destructive-operation protection",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			protect := cmd.Flags().Changed("on")
			unprotect := cmd.Flags().Changed("off")
			if protect == unprotect {
				return invalid(errors.New("choose exactly one of --on or --off"))
			}
			name := args[0]
			if !environmentName.MatchString(name) {
				return invalid(errors.New("environment name must be a lowercase stable name"))
			}
			file, err := o.load()
			if err != nil {
				return invalid(err)
			}
			effective, err := file.Resolve(name, config.ResolveOptions{})
			if err != nil {
				return invalid(err)
			}
			if unprotect && effective.Config.Class == "production" && !o.yes {
				return invalid(errors.New("disabling protection for production requires --yes"))
			}
			data, err := os.ReadFile(filepath.Clean(o.configPath))
			if err != nil {
				return invalid(err)
			}
			var document map[string]any
			if err := yaml.Unmarshal(data, &document); err != nil {
				return invalid(fmt.Errorf("decode config for protection update: %w", err))
			}
			environments, ok := document["environments"].(map[string]any)
			if !ok {
				return invalid(errors.New("environments must be a map"))
			}
			overlay, ok := environments[name].(map[string]any)
			if !ok {
				return invalid(fmt.Errorf("environment %q must be a map", name))
			}
			overlay["protection"] = protect
			environments[name] = overlay
			document["environments"] = environments
			updated, err := yaml.Marshal(document)
			if err != nil {
				return fmt.Errorf("encode protected configuration: %w", err)
			}
			info, err := os.Stat(filepath.Clean(o.configPath))
			if err != nil {
				return err
			}
			if err := replaceFile(filepath.Clean(o.configPath), updated, info.Mode().Perm()); err != nil {
				return err
			}
			return o.write(map[string]any{"environment": name, "protection": protect})
		},
	}
	command.Flags().Bool("on", false, "protect destructive operations")
	command.Flags().Bool("off", false, "allow destructive operations")
	return command
}

func envDumpCommand(o *options) *cobra.Command {
	var dest string
	var sanitize bool
	command := &cobra.Command{
		Use:   "dump <environment>",
		Short: "Create a Magento database dump from a live environment and write it locally",
		Long: strings.TrimSpace(`
Runs mysqldump against the selected environment over the dumpimport transport (host mysql, Docker Compose, or kube) and writes the SQL only to --to.

The dump is unsanitized Magento data unless --sanitize is set. --sanitize hashes mailbox addresses only and is not certified anonymization. Database passwords travel through MAGELIFT_DUMPIMPORT_* / MYSQL_PWD and are not printed. Overwriting --to requires --yes.
`),
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			if !environmentName.MatchString(name) {
				return invalid(errors.New("environment name must be a lowercase stable name"))
			}
			if strings.TrimSpace(dest) == "" || strings.TrimSpace(dest) == "-" {
				return invalid(errors.New("--to must be a local file path"))
			}
			result, err := o.runEnvDump(cmd.Context(), name, dest, sanitize)
			if err != nil {
				return err
			}
			return o.write(result)
		},
	}
	command.Flags().StringVar(&dest, "to", "", "local .sql or .sql.gz path to write (required)")
	command.Flags().BoolVar(&sanitize, "sanitize", false, "hash mailbox addresses in the dump (not certified anonymous)")
	_ = command.MarkFlagRequired("to")
	return command
}

func envUICommand(o *options) *cobra.Command {
	var target string
	command := &cobra.Command{
		Use:   "ui <environment>",
		Short: "Print a time-limited management UI tunnel without starting it",
		Long: strings.TrimSpace(`
Resolves a provider management UI (database, queue, or search) through the same capability path as tunnel --session-only.

A missing UI is typed unavailable (exit 3) and does not block dump, logs, or other tunnels the target supports.
`),
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			if !environmentName.MatchString(name) {
				return invalid(errors.New("environment name must be a lowercase stable name"))
			}
			o.environment = name
			uiTarget, err := normalizeManagementUITarget(target)
			if err != nil {
				return invalid(err)
			}
			result, _, err := o.prepareTunnel(cmd.Context(), uiTarget, 0, 0)
			if err != nil {
				return err
			}
			return o.write(map[string]any{
				"environment": result.Environment,
				"target":      result.Target,
				"localPort":   result.LocalPort,
				"remotePort":  result.RemotePort,
				"launcher":    result.Launcher,
				"args":        result.Args,
				"sessionOnly": true,
			})
		},
	}
	command.Flags().StringVar(&target, "target", platform.TunnelTargetDatabaseUI, "management UI: db-ui, queue-ui, or search-ui")
	return command
}

func normalizeManagementUITarget(target string) (string, error) {
	normalized, err := platform.NormalizeTunnelTarget(target)
	if err != nil {
		return "", err
	}
	switch normalized {
	case platform.TunnelTargetDatabaseUI, platform.TunnelTargetQueueUI, platform.TunnelTargetSearchUI:
		return normalized, nil
	default:
		return "", fmt.Errorf("management UI target must be db-ui, queue-ui, or search-ui")
	}
}

func envImportDumpCommand(o *options) *cobra.Command {
	return &cobra.Command{
		Use:   "import-dump <environment>",
		Short: "Import the environment seedDump into the target database",
		Long:  "Runs the dump importer with journal transitions (recorded|failed → importing → imported|failed). Retries into a non-empty database require persistent --yes (D-04). Named import-dump to avoid colliding with magelift local seed.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			if !environmentName.MatchString(name) {
				return invalid(errors.New("environment name must be a lowercase stable name"))
			}
			result, err := o.runSeedDumpImport(cmd.Context(), name, seedDumpImportExplicit)
			if err != nil {
				return err
			}
			return o.write(result)
		},
	}
}

func envMediaSyncCommand(o *options) *cobra.Command {
	var source string
	command := &cobra.Command{
		Use:   "media-sync <environment>",
		Short: "Upload a local media tree into the environment media bucket (merge)",
		Long: strings.TrimSpace(`
Upload files from --source into the environment's media object-storage bucket (stack output mediaBucket).

Key mapping: if a path contains pub/media/, object keys are relative to that prefix; otherwise --source is treated as the media root and keys are relative to it.

Default mode is merge: each local file is PutObject (overwrite on conflict). Remote keys absent locally are not deleted. After upload, listing is checked so every source key is present (empty missing-key diff).

Does not auto-run after deploy; seedMedia auto-seed is a follow-on.
`),
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			if !environmentName.MatchString(name) {
				return invalid(errors.New("environment name must be a lowercase stable name"))
			}
			if strings.TrimSpace(source) == "" {
				return invalid(errors.New("--source is required"))
			}
			result, err := o.runMediaSync(cmd.Context(), name, source)
			if err != nil {
				return err
			}
			return o.write(result)
		},
	}
	command.Flags().StringVar(&source, "source", "", "local media directory to upload (required)")
	_ = command.MarkFlagRequired("source")
	return command
}

func (o *options) runMediaSync(ctx context.Context, environment, source string) (map[string]any, error) {
	syncFn := o.mediaSync
	bucket := ""
	var client mediasync.API
	if syncFn == nil {
		environmentName, planned, err := o.planStackForEnvironment(environment)
		if err != nil {
			return nil, err
		}
		_ = environmentName
		if err := requireTargetDependencies(ctx, o, planned); err != nil {
			return nil, err
		}
		if o.newBackend == nil {
			return nil, errors.New("infrastructure backend factory is required")
		}
		backendURL := o.infrastructureBackendURL(planned)
		backend, err := o.newBackend(ctx, planned, backendURL)
		if err != nil {
			return nil, fmt.Errorf("create infrastructure backend: %w", err)
		}
		outputs, err := backend.Outputs(ctx)
		if err != nil {
			return nil, fmt.Errorf("read infrastructure outputs: %w", err)
		}
		bucket, err = platform.RequireStringOutput(outputs, platform.OutputMediaBucket)
		if err != nil {
			return nil, invalid(err)
		}
		client, err = newMediaS3Client(ctx, planned.Region())
		if err != nil {
			return nil, err
		}
		syncFn = mediasync.Sync
	}
	syncResult, err := syncFn(ctx, mediasync.Options{Source: source, Bucket: bucket, Client: client})
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"environment": environment,
		"bucket":      bucket,
		"uploaded":    syncResult.Uploaded,
		"keys":        syncResult.Keys,
		"listingDiff": map[string]any{
			"missing": syncResult.Diff.Missing,
			"extra":   syncResult.Diff.Extra,
			"empty":   syncResult.Diff.Empty(),
		},
	}, nil
}

// planStackForEnvironment plans without mutating o.environment from flags when
// the env name comes from the media-sync positional argument.
func (o *options) planStackForEnvironment(environment string) (string, platform.PlannedStack, error) {
	if o.modules == nil {
		return "", nil, errors.New("stack module registry is required")
	}
	file, err := o.load()
	if err != nil {
		return "", nil, invalid(err)
	}
	effective, err := file.Resolve(environment, config.ResolveOptions{})
	if err != nil {
		return "", nil, invalid(err)
	}
	_, planned, err := o.modules.Plan(effective.Config, environment, platform.PlanOptions{Context: o.planContext})
	if err != nil {
		return "", nil, err
	}
	warnExperimentalTarget(o.stderr, planned)
	return environment, planned, nil
}

func newMediaS3Client(ctx context.Context, region string) (*s3.Client, error) {
	endpoint, err := awsendpoint.FromEnv()
	if err != nil {
		return nil, invalid(err)
	}
	cfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(region))
	if err != nil {
		return nil, fmt.Errorf("load AWS config for media sync: %w", err)
	}
	return s3.NewFromConfig(cfg, func(options *s3.Options) {
		if endpoint != "" {
			options.BaseEndpoint = awssdk.String(endpoint)
			options.UsePathStyle = true
		}
	}), nil
}

func envStatusCommand(o *options) *cobra.Command {
	return &cobra.Command{
		Use:   "status <environment>",
		Short: "Show environment overlay status including seed dump journal",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			if !environmentName.MatchString(name) {
				return invalid(errors.New("environment name must be a lowercase stable name"))
			}
			file, err := o.load()
			if err != nil {
				return invalid(err)
			}
			effective, resolvedName, err := o.resolveEnvironment(file, name)
			if err != nil {
				return invalid(err)
			}
			result := map[string]any{
				"environment": resolvedName,
				"class":       effective.Config.Class,
				"domain":      effective.Config.Domain,
				"protected":   effective.Config.Protection,
			}
			seedDump := effective.Config.SeedDump
			if seedDump == "" {
				return o.write(result)
			}
			result["seedDump"] = seedDump
			projectRoot := filepath.Dir(filepath.Clean(o.configPath))
			store, err := seeddump.New(projectRoot, resolvedName)
			if err != nil {
				return invalid(err)
			}
			record, err := store.Read(cmd.Context())
			if err != nil {
				return fmt.Errorf("read seed dump journal: %w", err)
			}
			if record == nil {
				result["seedDumpStatus"] = "unavailable"
				return o.write(result)
			}
			result["seedDumpStatus"] = record.Status
			if record.Status == seeddump.StatusFailed && record.Reason != "" {
				result["seedDumpReason"] = record.Reason
			}
			return o.write(result)
		},
	}
}

type seedDumpImportKind int

const (
	seedDumpImportExplicit seedDumpImportKind = iota
	seedDumpImportAuto
)

// runSeedDumpImport marks journal importing, runs dumpimport, then imported|failed.
// First recorded path forces Yes=true (Pattern 3 / D-04); retries from failed require o.yes when nonempty.
func (o *options) runSeedDumpImport(ctx context.Context, environment string, kind seedDumpImportKind) (map[string]any, error) {
	file, err := o.load()
	if err != nil {
		return nil, invalid(err)
	}
	effective, resolvedEnvironment, err := o.resolveEnvironment(file, environment)
	if err != nil {
		return nil, invalid(err)
	}
	dumpPath := strings.TrimSpace(effective.Config.SeedDump)
	if dumpPath == "" {
		if kind == seedDumpImportAuto {
			return nil, nil
		}
		return nil, invalid(errors.New("environment has no seedDump configured"))
	}

	projectRoot := filepath.Dir(filepath.Clean(o.configPath))
	store, err := seeddump.New(projectRoot, resolvedEnvironment)
	if err != nil {
		return nil, invalid(err)
	}
	record, err := store.Read(ctx)
	if err != nil {
		return nil, fmt.Errorf("read seed dump journal: %w", err)
	}
	if record == nil {
		if kind == seedDumpImportAuto {
			return nil, nil
		}
		return nil, invalid(errors.New("seed dump journal is missing; run env create --dump first"))
	}
	if kind == seedDumpImportAuto && record.Status != seeddump.StatusRecorded {
		return nil, nil
	}

	yes := o.yes
	if record.Status == seeddump.StatusRecorded {
		// First recorded path: operator intent already recorded via --dump (D-01/D-04 Pattern 3).
		yes = true
	}

	opts := dumpimport.Options{
		DumpPath: dumpPath,
		Yes:      yes,
		WorkDir:  projectRoot,
	}
	if name := strings.TrimSpace(effective.Config.Project.Name); name != "" {
		if project, err := localdev.ProjectName(name); err == nil {
			opts.ComposeProject = project
		}
	}
	o.applyDumpimportTransportEnv(&opts)

	if err := requireDependencies(ctx, o, toolchain.SpecsForDumpImport(
		dependencyRunner(o), opts.Runner, strings.HasSuffix(strings.ToLower(dumpPath), ".gz"),
	), "database dump import"); err != nil {
		return nil, err
	}
	if _, err := store.MarkImporting(ctx); err != nil {
		return nil, invalid(err)
	}

	importFn := o.importSeedDump
	if importFn == nil {
		importFn = dumpimport.Import
	}

	if err := importFn(ctx, opts); err != nil {
		reason := strings.TrimSpace(err.Error())
		if reason == "" {
			reason = "dump import failed"
		}
		if _, markErr := store.MarkFailed(ctx, reason); markErr != nil {
			return nil, fmt.Errorf("%w; mark seed dump failed: %v", err, markErr)
		}
		if errors.Is(err, dumpimport.ErrNonEmptyRequiresYes) {
			return nil, invalid(err)
		}
		return nil, err
	}

	imported, err := store.MarkImported(ctx)
	if err != nil {
		return nil, fmt.Errorf("mark seed dump imported: %w", err)
	}
	return map[string]any{
		"environment":    environment,
		"seedDump":       dumpPath,
		"seedDumpStatus": imported.Status,
	}, nil
}

// maybeAutoImportSeedDump runs once after successful deploy when status is recorded (D-01/D-02).
func (o *options) maybeAutoImportSeedDump(ctx context.Context, environment string) error {
	_, err := o.runSeedDumpImport(ctx, environment, seedDumpImportAuto)
	return err
}

func (o *options) applyDumpimportTransportEnv(opts *dumpimport.Options) {
	if o.getenv == nil {
		return
	}
	if runner := strings.TrimSpace(o.getenv("MAGELIFT_DUMPIMPORT_RUNNER")); runner != "" {
		opts.Runner = runner
	}
	if host := strings.TrimSpace(o.getenv("MAGELIFT_DUMPIMPORT_HOST")); host != "" {
		opts.Host = host
	}
	if user := strings.TrimSpace(o.getenv("MAGELIFT_DUMPIMPORT_USER")); user != "" {
		opts.User = user
	}
	if pass := o.getenv("MAGELIFT_DUMPIMPORT_PASSWORD"); pass != "" {
		opts.Password = pass
	}
	if ns := strings.TrimSpace(o.getenv("MAGELIFT_DUMPIMPORT_NAMESPACE")); ns != "" {
		opts.Namespace = ns
	}
	if pod := strings.TrimSpace(o.getenv("MAGELIFT_DUMPIMPORT_POD")); pod != "" {
		opts.Pod = pod
	}
	if sel := strings.TrimSpace(o.getenv("MAGELIFT_DUMPIMPORT_POD_SELECTOR")); sel != "" {
		opts.PodSelector = sel
	}
	if kc := strings.TrimSpace(o.getenv("MAGELIFT_DUMPIMPORT_KUBECONFIG")); kc != "" {
		opts.Kubeconfig = kc
	}
	if db := strings.TrimSpace(o.getenv("MAGELIFT_DUMPIMPORT_DATABASE")); db != "" {
		opts.Database = db
	}
	if container := strings.TrimSpace(o.getenv("MAGELIFT_DUMPIMPORT_CONTAINER")); container != "" {
		opts.Container = container
	}
}

func (o *options) runEnvDump(ctx context.Context, environment, dest string, sanitize bool) (map[string]any, error) {
	file, err := o.load()
	if err != nil {
		return nil, invalid(err)
	}
	effective, _, err := o.resolveEnvironment(file, environment)
	if err != nil {
		return nil, invalid(err)
	}
	projectRoot := filepath.Dir(filepath.Clean(o.configPath))
	absDest, err := filepath.Abs(dest)
	if err != nil {
		return nil, invalid(err)
	}
	opts := dumpimport.Options{
		OutputPath: absDest,
		Yes:        o.yes,
		Sanitize:   sanitize,
		WorkDir:    projectRoot,
	}
	if name := strings.TrimSpace(effective.Config.Project.Name); name != "" {
		if project, err := localdev.ProjectName(name); err == nil {
			opts.ComposeProject = project
		}
	}
	o.applyDumpimportTransportEnv(&opts)
	if err := requireDependencies(ctx, o, toolchain.SpecsForDumpExport(
		dependencyRunner(o), opts.Runner,
	), "database dump retrieve"); err != nil {
		return nil, err
	}
	exportFn := o.exportDump
	if exportFn == nil {
		exportFn = dumpimport.Export
	}
	if err := exportFn(ctx, opts); err != nil {
		if errors.Is(err, dumpimport.ErrOutputExists) {
			return nil, invalid(err)
		}
		return nil, err
	}
	label := dumpimport.UnsanitizedLabel
	if sanitize {
		label = dumpimport.SanitizedLabel
	}
	return map[string]any{
		"environment": environment,
		"path":        absDest,
		"sanitized":   sanitize,
		"label":       label,
	}, nil
}
