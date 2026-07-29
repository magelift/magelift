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

	"github.com/acourtiol/magelift/internal/config"
	"github.com/acourtiol/magelift/internal/seeddump"
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
	return &cobra.Command{
		Use:   "destroy <environment>",
		Short: "Destroy an environment and remove its configuration overlay",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !o.yes {
				return invalid(errors.New("destroying an environment requires --yes"))
			}
			result, err := o.destroyEnvironment(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			return o.write(result)
		},
	}
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
	if _, err := file.Resolve(name, config.ResolveOptions{}); err != nil {
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
			effective, err := file.Resolve(name, config.ResolveOptions{})
			if err != nil {
				return invalid(err)
			}
			result := map[string]any{
				"environment": name,
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
			store, err := seeddump.New(projectRoot, name)
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
