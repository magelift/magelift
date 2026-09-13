package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/magelift/magelift/internal/automation"
	"github.com/magelift/magelift/internal/config"
	"github.com/magelift/magelift/internal/cosign"
	deployflow "github.com/magelift/magelift/internal/deploy"
	"github.com/magelift/magelift/internal/dumpimport"
	fastlyedge "github.com/magelift/magelift/internal/external/fastly"
	"github.com/magelift/magelift/internal/mediasync"
	"github.com/magelift/magelift/internal/paasimport"
	"github.com/magelift/magelift/internal/platform"
	"github.com/magelift/magelift/internal/providerhost"
	"github.com/magelift/magelift/internal/releasejournal"
	"github.com/magelift/magelift/internal/toolchain"
	mageliftupgrade "github.com/magelift/magelift/internal/upgrade"
	"github.com/magelift/magelift/internal/usererr"
	sdk "github.com/magelift/magelift/sdk/v1"
	"github.com/spf13/cobra"
	"go.yaml.in/yaml/v4"
)

var Version = "dev"

// Hooks provide provider-specific clients to the Cobra command layer. The
// production entrypoint supplies these; tests and documentation tools may
// leave them nil when they do not exercise the corresponding operation.
type Hooks struct {
	NewComposerSecrets         func(context.Context, string) (ComposerSecretProvider, error)
	NewComposerGCPSecrets      func(context.Context) (ComposerSecretProvider, error)
	NewLeftoverBackupDestroyer func(context.Context, platform.PlannedStack) (LeftoverBackupDestroyer, error)
	NewGCPCleanupProvider      func(context.Context, sdk.CleanupLedger) (CleanupProvider, error)
}

type exitError struct {
	code int
	err  error
}

func (e *exitError) Error() string { return e.err.Error() }
func (e *exitError) Unwrap() error { return e.err }

func ExitCode(err error) int {
	var target *exitError
	if errors.As(err, &target) {
		return target.code
	}
	return 1
}

type options struct {
	configPath              string
	environment             string
	previewRepository       string
	previewPullRequest      int64
	previewBranch           string
	previewCommit           string
	previewDomain           string
	previewGeneration       uint64
	resolvedPreviewIdentity *config.PreviewIdentity
	previewIdentityOverride *config.PreviewIdentity
	planContext             context.Context
	output                  string
	noInteraction           bool
	yes                     bool
	verbose                 int
	stdout                  io.Writer
	stderr                  io.Writer
	getenv                  func(string) string
	lookPath                func(string) (string, error)
	currentBranch           func(string) (string, error)
	terminal                environmentTerminal
	modules                 *platform.ModuleRegistry
	newBackend              func(context.Context, platform.PlannedStack, string) (infrastructureBackend, error)
	newDeploySteps          func(context.Context, infrastructureBackend, platform.PlannedStack, io.Writer) (deployflow.Steps, error)
	importSeedDump          func(context.Context, dumpimport.Options) error
	exportDump              func(context.Context, dumpimport.Options) error
	mediaSync               func(context.Context, mediasync.Options) (mediasync.Result, error)
	newLock                 func(context.Context, platform.PlannedStack) (func(context.Context) error, error)
	loadProvider            func(context.Context, string) (providerhost.Loaded, error)
	dialProvider            func(context.Context, string) (providerhost.API, error)
	providerSessions        []*providerhost.Session
	runCommand              func(context.Context, string, []string, io.Writer, io.Writer) error
	runCompose              func(context.Context, string, []string, []string, io.Writer, io.Writer) error
	dependencyRunner        toolchain.DependencyRunner
	newReleaseStore         func(string, string) (releaseStore, error)
	verifyRelease           func(context.Context, string, cosign.VerifyOptions) error
	signRelease             func(context.Context, string, cosign.SignOptions) error
	liveSchemaEpoch         func(context.Context) (int, error)
	newComposerSecrets      func(context.Context, string) (composerSecretProvider, error)
	newComposerGCPSecrets   func(context.Context) (composerSecretProvider, error)
	newUpgrade              func() upgradeClient
	newFastly               func(fastlyedge.Request) (fastlyLifecycle, error)
	executable              func() (string, error)
	// Test doubles for day-2 ports (override Module* resolution).
	testBootstrap              platform.Bootstrap
	testState                  platform.State
	testSecrets                platform.Secrets
	testRuntimeObserve         platform.RuntimeObserve
	testRuntimeTunnel          platform.RuntimeTunnel
	testCostEstimator          platform.CostEstimator
	infraOnly                  bool
	skipProviderLock           bool
	destroyBackups             bool
	newLeftoverBackupDestroyer func(context.Context, platform.PlannedStack) (leftoverBackupDestroyer, error)
	newGCPCleanupProvider      func(context.Context, sdk.CleanupLedger) (cleanupProvider, error)
	newCleanupProvider         func(context.Context, sdk.CleanupLedger) (cleanupProvider, error)
	listPreviewRecords         func(context.Context, string, string) ([]automation.PreviewRecord, error)
}

type environmentTerminal interface {
	Interactive() bool
	SelectEnvironment([]string) (string, error)
}

type consoleTerminal struct {
	in  *os.File
	out io.Writer
}

func (t consoleTerminal) Interactive() bool {
	info, err := t.in.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

func (t consoleTerminal) SelectEnvironment(environments []string) (string, error) {
	if len(environments) == 0 {
		return "", errors.New("no environments are configured")
	}
	_, _ = fmt.Fprintln(t.out, "Select an environment:")
	for i, environment := range environments {
		_, _ = fmt.Fprintf(t.out, "  %d) %s\n", i+1, environment)
	}
	_, _ = fmt.Fprint(t.out, "> ")
	line, err := bufio.NewReader(t.in).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", fmt.Errorf("read environment selection: %w", err)
	}
	selection, err := strconv.Atoi(strings.TrimSpace(line))
	if err != nil || selection < 1 || selection > len(environments) {
		return "", errors.New("invalid environment selection")
	}
	return environments[selection-1], nil
}

func (t consoleTerminal) Confirm(prompt string) (bool, error) {
	_, _ = fmt.Fprintf(t.out, "%s [y/N] ", prompt)
	line, err := bufio.NewReader(t.in).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return false, fmt.Errorf("read confirmation: %w", err)
	}
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes":
		return true, nil
	default:
		return false, nil
	}
}

// New returns the CLI command tree with an empty module registry.
// Production binaries must call NewWithModules after registering adapters
// (see cmd/magelift). Keeping Pulumi SDKs out of this package lets tools like
// gendocs link without OOM on GitHub-hosted runners.
func New() *cobra.Command {
	return NewWithModules(platform.NewModuleRegistry())
}

// NewWithModules returns the CLI wired to the given stack module registry.
func NewWithModules(modules *platform.ModuleRegistry) *cobra.Command {
	return newCommand(os.Stdout, os.Stderr, modules)
}

// NewWithModulesAndHooks returns the CLI with provider clients supplied by its
// production entrypoint.
func NewWithModulesAndHooks(modules *platform.ModuleRegistry, hooks Hooks) *cobra.Command {
	return newCommandWithHooks(os.Stdout, os.Stderr, modules, hooks)
}

func newCommand(stdout, stderr io.Writer, modules *platform.ModuleRegistry) *cobra.Command {
	return newCommandWithHooks(stdout, stderr, modules, Hooks{})
}

func newCommandWithHooks(stdout, stderr io.Writer, modules *platform.ModuleRegistry, hooks Hooks) *cobra.Command {
	if modules == nil {
		modules = platform.NewModuleRegistry()
	}
	o := &options{
		stdout:             stdout,
		stderr:             stderr,
		getenv:             os.Getenv,
		lookPath:           exec.LookPath,
		currentBranch:      gitCurrentBranch,
		terminal:           consoleTerminal{in: os.Stdin, out: stderr},
		modules:            modules,
		newBackend:         nil, // Assigned post-literal to o.defaultNewBackend below.
		loadProvider:       nil, // Assigned post-literal to o.defaultLoadProvider below.
		dialProvider:       nil, // Assigned post-literal to o.defaultDialProvider below.
		listPreviewRecords: automation.ListPreviewRecords,
		newDeploySteps:     nil,
		newLock: func(ctx context.Context, planned platform.PlannedStack) (func(context.Context) error, error) {
			module, found := modules.Module(planned.Provider(), planned.Runtime())
			if !found {
				return nil, fmt.Errorf("no stack module for %q/%q", planned.Provider(), planned.Runtime())
			}
			ops := platform.ModuleOps(module)
			if ops == nil {
				return func(context.Context) error { return nil }, nil
			}
			return ops.AcquireLock(ctx, planned)
		},
		runCommand:       runRemoteCommand,
		runCompose:       runDockerCompose,
		dependencyRunner: toolchain.SystemDependencyRunner(),
		newReleaseStore: func(root, environment string) (releaseStore, error) {
			return releasejournal.New(root, environment)
		},
		verifyRelease:              cosign.New().Verify,
		newComposerSecrets:         hooks.NewComposerSecrets,
		newComposerGCPSecrets:      hooks.NewComposerGCPSecrets,
		newLeftoverBackupDestroyer: hooks.NewLeftoverBackupDestroyer,
		newGCPCleanupProvider:      hooks.NewGCPCleanupProvider,
		newUpgrade:                 func() upgradeClient { return mageliftupgrade.New(nil) },
		newFastly: func(request fastlyedge.Request) (fastlyLifecycle, error) {
			runner := fastlyedge.ExecRunner{}
			probe, err := fastlyHealthProbe(request)
			if err != nil {
				return nil, err
			}
			return fastlyedge.NewWithProbes(runner, probe, fastlyedge.NewCLIInventoryDeletionProbe(runner)), nil
		},
		executable: os.Executable,
	}
	o.liveSchemaEpoch = func(ctx context.Context) (int, error) {
		return o.observeLiveSchemaEpoch(ctx)
	}
	o.newBackend = o.defaultNewBackend
	o.loadProvider = o.defaultLoadProvider
	o.dialProvider = o.defaultDialProvider
	o.newDeploySteps = func(ctx context.Context, backend infrastructureBackend, planned platform.PlannedStack, diagnostics io.Writer) (deployflow.Steps, error) {
		module, found := modules.Module(planned.Provider(), planned.Runtime())
		if !found {
			return nil, fmt.Errorf("no stack module for %q/%q", planned.Provider(), planned.Runtime())
		}
		ops := platform.ModuleOps(module)
		if ops == nil {
			return nil, platform.ErrNotSupported
		}
		recordRelease := func(ctx context.Context, request deployflow.Request, _ deployflow.Result) error {
			store, err := o.releaseStore(planned.Environment())
			if err != nil {
				return err
			}
			entries, err := store.List(ctx)
			if err != nil {
				return err
			}
			identity, issuer := releasejournal.SignatureMetadataForDigest(entries, request.ImageDigest)
			epoch := o.schemaEpochOrZero(ctx)
			if epoch < 1 {
				epoch = releasejournal.SchemaEpochForDigest(entries, request.ImageDigest)
			}
			_, err = store.Append(ctx, releasejournal.Entry{
				Action: releasejournal.ActionDeploy, Environment: planned.Environment(),
				DigestReference: request.ImageDigest, ForwardOnly: true,
				SignatureIdentity: identity, SignatureIssuer: issuer,
				SchemaEpoch: epoch,
			})
			return err
		}
		if hook, ok := ops.(platform.HasRecordRelease); ok {
			ops = hook.WithRecordRelease(recordRelease)
		}
		return ops.NewDeploySteps(ctx, backend, planned, diagnostics)
	}
	return newCommandWithOptions(o)
}

func newCommandWithOptions(o *options) *cobra.Command {
	root := &cobra.Command{
		Use:           "magelift",
		Short:         "Deploy Magento applications to certified and experimental cloud targets",
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRun: func(cmd *cobra.Command, _ []string) {
			o.planContext = cmd.Context()
		},
	}
	root.SetOut(o.stdout)
	root.SetErr(o.stderr)
	configDefault := o.configPath
	if configDefault == "" {
		configDefault = "magelift.yaml"
	}
	outputDefault := o.output
	if outputDefault == "" {
		outputDefault = "table"
	}
	root.PersistentFlags().StringVar(&o.configPath, "config", configDefault, "configuration file")
	root.PersistentFlags().StringVar(&o.environment, "env", o.environment, "environment name")
	root.PersistentFlags().StringVar(&o.previewRepository, "preview-repository", o.previewRepository, "canonical repository slug for a pull-request preview")
	root.PersistentFlags().Int64Var(&o.previewPullRequest, "preview-number", o.previewPullRequest, "pull-request number for a preview identity")
	root.PersistentFlags().StringVar(&o.previewBranch, "preview-branch", o.previewBranch, "source branch metadata for a preview identity")
	root.PersistentFlags().StringVar(&o.previewCommit, "preview-commit", o.previewCommit, "commit digest metadata for a preview identity")
	root.PersistentFlags().StringVar(&o.previewDomain, "preview-domain", o.previewDomain, "domain metadata for a preview identity")
	root.PersistentFlags().Uint64Var(&o.previewGeneration, "preview-generation", o.previewGeneration, "deployment generation for a preview identity")
	root.PersistentFlags().StringVarP(&o.output, "output", "o", outputDefault, "output format: table, json, or yaml")
	root.PersistentFlags().BoolVar(&o.noInteraction, "no-interaction", o.noInteraction, "never prompt for input")
	root.PersistentFlags().BoolVarP(&o.yes, "yes", "y", o.yes, "confirm destructive actions")
	root.PersistentFlags().CountVarP(&o.verbose, "verbose", "v", "increase diagnostic verbosity")

	root.AddCommand(versionCommand(o), initCommand(o), doctorCommand(o), configCommand(o), compatibilityCommand(o), certificationCommand(o), skillsCommand(o), extensionsCommand(o), edgeCommand(o), buildCommand(o), bootstrapCommand(o), benchmarkCommand(o), cleanupCommand(o))
	root.AddCommand(statusCommand(o), costCommand(o), healthCommand(o))
	root.AddCommand(infrastructureCommands(o)...)
	root.AddCommand(commandGroups(o)...)
	root.AddCommand(completionCommand(root))
	return root
}

func versionCommand(o *options) *cobra.Command {
	return &cobra.Command{Use: "version", Short: "Print version information", Args: cobra.NoArgs, RunE: func(*cobra.Command, []string) error {
		return o.write(map[string]string{"version": Version})
	}}
}

func initCommand(o *options) *cobra.Command {
	var fromACC, fromUpsun bool
	var configOut, provider string
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Create a starter magelift.yaml for Magento's PHP storefront",
		Args:  cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			if fromACC && fromUpsun {
				return invalid(fmt.Errorf("--from-acc and --from-upsun are mutually exclusive"))
			}
			if provider == "" {
				provider = "aws"
			}
			starter, err := starterForProvider(provider)
			if err != nil {
				return invalid(err)
			}
			writePath := o.configPath
			if configOut != "" {
				writePath = configOut
			}
			writePath = filepath.Clean(writePath)
			if _, err := os.Stat(writePath); err == nil {
				if !o.yes {
					return &exitError{code: 2, err: fmt.Errorf("%s already exists", writePath)}
				}
			} else if !errors.Is(err, os.ErrNotExist) {
				return err
			}
			var (
				data     []byte
				unmapped []paasimport.UnmappedKey
			)
			switch {
			case fromACC:
				root, err := os.Getwd()
				if err != nil {
					return err
				}
				result, err := paasimport.MapACC(root)
				if err != nil {
					return invalid(err)
				}
				data, unmapped = result.YAML, result.Unmapped
			case fromUpsun:
				root, err := os.Getwd()
				if err != nil {
					return err
				}
				result, err := paasimport.MapUpsun(root)
				if err != nil {
					return invalid(err)
				}
				data, unmapped = result.YAML, result.Unmapped
			default:
				data = []byte(starter)
			}
			if err := os.MkdirAll(filepath.Dir(writePath), 0o755); err != nil {
				return err
			}
			if err := os.WriteFile(writePath, data, 0o644); err != nil {
				return err
			}
			if len(unmapped) > 0 {
				sidecar := paasimport.SidecarPath(writePath)
				report := paasimport.RenderUnmappedReport(unmapped)
				if err := os.WriteFile(sidecar, []byte(report), 0o644); err != nil {
					return err
				}
				return invalid(fmt.Errorf("import produced %d unmapped key(s); see %s", len(unmapped), sidecar))
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&fromACC, "from-acc", false, "generate magelift.yaml from Adobe Commerce Cloud config")
	cmd.Flags().BoolVar(&fromUpsun, "from-upsun", false, "generate magelift.yaml from Upsun / Platform.sh config")
	cmd.Flags().StringVar(&configOut, "config-out", "", "write generated YAML to PATH for review (default: --config path)")
	cmd.Flags().StringVar(&provider, "provider", "aws", "starter provider: aws or gcp (certified starters only)")
	return cmd
}

// starterForProvider returns the starter for a certified provider. The
// provider check runs before the overwrite check so a bad flag never
// looks like a file problem. MageLift is the authority for the
// supported set: other providers have no certified starter in v1.
func starterForProvider(provider string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "aws":
		return starterAWSConfig, nil
	case "gcp":
		return starterGCPConfig, nil
	default:
		return "", fmt.Errorf("MageLift supports starter providers \"aws\" and \"gcp\" only, got %q", provider)
	}
}

func configCommand(o *options) *cobra.Command {
	cmd := &cobra.Command{Use: "config", Short: "Inspect and migrate configuration"}
	cmd.AddCommand(&cobra.Command{Use: "validate", Short: "Validate configuration", Args: cobra.NoArgs, RunE: func(*cobra.Command, []string) error {
		file, err := o.load()
		if err != nil {
			return invalid(err)
		}
		envs := file.Environments()
		if len(envs) == 0 {
			return invalid(errors.New("at least one environment is required"))
		}
		if _, err := file.ResolveBuild(); err != nil {
			return invalid(err)
		}
		var warnings []string
		for _, env := range envs {
			effective, err := file.Resolve(env, config.ResolveOptions{})
			if err != nil {
				return invalid(fmt.Errorf("environment %s: %w", env, err))
			}
			warnings = append(warnings, effective.Compatibility.Warnings...)
		}
		for _, warning := range warnings {
			if o.stderr != nil {
				_, _ = fmt.Fprintln(o.stderr, "warning: "+warning)
			}
		}
		result := map[string]any{"valid": true, "environments": envs}
		if len(warnings) > 0 {
			result["warnings"] = warnings
		}
		return o.write(result)
	}})
	cmd.AddCommand(&cobra.Command{Use: "effective", Short: "Print effective configuration with provenance", Args: cobra.NoArgs, RunE: func(*cobra.Command, []string) error {
		effective, err := o.resolve()
		if err != nil {
			return invalid(err)
		}
		return o.write(effective.SafeEffectiveOutput())
	}})
	cmd.AddCommand(&cobra.Command{Use: "explain [path]", Short: "Explain where an effective value came from", Args: cobra.ExactArgs(1), RunE: func(_ *cobra.Command, args []string) error {
		effective, err := o.resolve()
		if err != nil {
			return invalid(err)
		}
		p, ok := effective.Provenance[args[0]]
		if !ok {
			return &exitError{code: 2, err: fmt.Errorf("no value or provenance for %q", args[0])}
		}
		return o.write(map[string]any{"path": args[0], "source": p.Source, "removed": p.Removed})
	}})
	cmd.AddCommand(&cobra.Command{Use: "migrate", Short: "Migrate configuration to the current schema", Args: cobra.NoArgs, RunE: func(*cobra.Command, []string) error {
		data, err := os.ReadFile(filepath.Clean(o.configPath))
		if err != nil {
			return invalid(err)
		}
		migration, err := config.Migrate(data)
		if err != nil {
			return invalid(err)
		}
		if migration.Changed {
			info, err := os.Stat(filepath.Clean(o.configPath))
			if err != nil {
				return err
			}
			if err := replaceFile(filepath.Clean(o.configPath), migration.Data, info.Mode().Perm()); err != nil {
				return err
			}
		}
		return o.write(migration)
	}})
	return cmd
}

func replaceFile(path string, data []byte, mode os.FileMode) (err error) {
	temporary, err := os.CreateTemp(filepath.Dir(path), ".magelift.yaml-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer func() {
		if temporary != nil {
			_ = temporary.Close()
		}
		_ = os.Remove(temporaryPath)
	}()
	if err := temporary.Chmod(mode); err != nil {
		return err
	}
	if _, err := temporary.Write(data); err != nil {
		return err
	}
	if err := temporary.Sync(); err != nil {
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	temporary = nil
	return os.Rename(temporaryPath, path)
}

func (o *options) resolve() (config.Effective, error) {
	effective, _, err := o.resolveWithEnvironment()
	return effective, err
}

func (o *options) resolveWithEnvironment() (config.Effective, string, error) {
	file, err := o.load()
	if err != nil {
		return config.Effective{}, "", err
	}
	env, err := o.selectEnvironment(file)
	if err != nil {
		return config.Effective{}, "", err
	}
	effective, resolvedEnvironment, err := o.resolveEnvironment(file, env)
	return effective, resolvedEnvironment, err
}

func (o *options) selectEnvironment(file *config.File) (string, error) {
	if o.environment != "" {
		return o.environment, nil
	}
	if env := o.getenv("MAGELIFT_ENV"); env != "" {
		return env, nil
	}
	if branch, err := o.currentBranch(filepath.Dir(o.configPath)); err == nil {
		if env, found, err := file.EnvironmentForBranch(branch); err != nil {
			return "", err
		} else if found {
			return env, nil
		}
	}
	if o.noInteraction || !o.terminal.Interactive() {
		return "", invalid(usererr.New(
			"environment is required in non-interactive mode",
			"pass --env, set MAGELIFT_ENV, or map the current Git branch under environments.*.branches",
			"docs/configuration.md",
		))
	}
	return o.terminal.SelectEnvironment(file.Environments())
}

func gitCurrentBranch(directory string) (string, error) {
	cmd := exec.Command("git", "-C", directory, "branch", "--show-current")
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

func (o *options) load() (*config.File, error) {
	data, err := os.ReadFile(filepath.Clean(o.configPath))
	if err != nil {
		return nil, err
	}
	file, err := config.Load(data)
	if err != nil {
		return nil, err
	}
	lockPath := filepath.Join(filepath.Dir(o.configPath), "composer.lock")
	if lock, err := os.ReadFile(lockPath); err == nil {
		file.AttachComposerLock(lock)
	}
	return file, nil
}

func (o *options) write(value any) error {
	switch o.output {
	case "json":
		enc := json.NewEncoder(o.stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(value)
	case "yaml":
		enc := yaml.NewEncoder(o.stdout)
		defer enc.Close()
		return enc.Encode(value)
	case "table":
		data, err := yaml.Marshal(value)
		if err != nil {
			return err
		}
		_, err = o.stdout.Write(data)
		return err
	default:
		return &exitError{code: 2, err: fmt.Errorf("unsupported output format %q", o.output)}
	}
}

func invalid(err error) error { return &exitError{code: 2, err: err} }

func guided(cause, next, doc string) error {
	return invalid(usererr.New(cause, next, doc))
}

func guidedWrap(err error, cause, next, doc string) error {
	return invalid(usererr.Wrap(err, cause, next, doc))
}

func completionCommand(root *cobra.Command) *cobra.Command {
	return &cobra.Command{Use: "completion [bash|zsh|fish|powershell]", Short: "Generate shell completion", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		switch args[0] {
		case "bash":
			return root.GenBashCompletion(cmd.OutOrStdout())
		case "zsh":
			return root.GenZshCompletion(cmd.OutOrStdout())
		case "fish":
			return root.GenFishCompletion(cmd.OutOrStdout(), true)
		case "powershell":
			return root.GenPowerShellCompletion(cmd.OutOrStdout())
		default:
			return invalid(fmt.Errorf("unsupported shell %q", args[0]))
		}
	}}
}

func commandGroups(o *options) []*cobra.Command {
	groups := map[string][]string{
		"state": {"status", "backup", "restore", "unlock"}, "env": {"list", "create", "status", "dump", "import-dump", "media-sync", "destroy", "protect", "sweep", "ui"},
		"secret": {"set", "list", "remove"},
	}
	var commands []*cobra.Command
	commands = append(commands, upgradeCommand(o), loginCommand(o), execCommand(o), sshCommand(o), tunnelCommand(o), devCommand(o))
	for _, name := range []string{"cache-flush", "reindex", "cron-run", "queue-status"} {
		commands = append(commands, magentoOperationCommand(o, name))
	}
	for _, groupName := range []string{"state", "env", "secret"} {
		group := &cobra.Command{Use: groupName, Short: "MageLift " + groupName + " commands"}
		for _, name := range groups[groupName] {
			if groupName == "env" && name == "list" {
				group.AddCommand(envListCommand(o))
			} else if groupName == "env" && name == "create" {
				group.AddCommand(envCreateCommand(o))
			} else if groupName == "env" && name == "status" {
				group.AddCommand(envStatusCommand(o))
			} else if groupName == "env" && name == "dump" {
				group.AddCommand(envDumpCommand(o))
			} else if groupName == "env" && name == "import-dump" {
				group.AddCommand(envImportDumpCommand(o))
			} else if groupName == "env" && name == "media-sync" {
				group.AddCommand(envMediaSyncCommand(o))
			} else if groupName == "env" && name == "destroy" {
				group.AddCommand(envDestroyCommand(o))
			} else if groupName == "env" && name == "protect" {
				group.AddCommand(envProtectCommand(o))
			} else if groupName == "env" && name == "sweep" {
				group.AddCommand(envSweepCommand(o))
			} else if groupName == "env" && name == "ui" {
				group.AddCommand(envUICommand(o))
			} else if groupName == "state" && name == "status" {
				group.AddCommand(stateStatusCommand(o))
			} else if groupName == "state" && name == "unlock" {
				group.AddCommand(stateUnlockCommand(o))
			} else if groupName == "state" && name == "backup" {
				group.AddCommand(stateBackupCommand(o))
			} else if groupName == "state" && name == "restore" {
				group.AddCommand(stateRestoreCommand(o))
			} else if groupName == "secret" {
				switch name {
				case "set":
					group.AddCommand(secretSetCommand(o))
				case "list":
					group.AddCommand(secretListCommand(o))
				case "remove":
					group.AddCommand(secretRemoveCommand(o))
				default:
					group.AddCommand(unavailable(name))
				}
			} else {
				group.AddCommand(unavailable(name))
			}
		}
		commands = append(commands, group)
	}
	commands = append(commands, ciCommand(o), logsCommand(o))
	commands = append(commands, signCommand(o), promoteCommand(o), rollbackCommand(o), historyCommand(o), evidenceCommand(o), auditCommand(o))
	return commands
}

func unavailable(name string) *cobra.Command {
	return &cobra.Command{Use: name, Short: "Reserved for a later delivery phase", Args: cobra.ArbitraryArgs, RunE: func(*cobra.Command, []string) error {
		return &exitError{code: 3, err: fmt.Errorf("%s is not implemented in this foundation release", name)}
	}}
}

// starterConfig is the default (AWS) starter, kept as the shared test
// fixture name.
const starterConfig = starterAWSConfig

const starterAWSConfig = `schemaVersion: 1
project:
  name: example-shop
application:
  edition: open-source
  version: 2.4.9
  mode: integrated
  webRuntime: nginx-fpm
  magento:
    frontName: admin
build:
  php: "8.5"
target:
  provider: aws
  runtime: ecs-fargate
  aws:
    catalog:
      # Certified cell: no managed search on first run. Managed search
      # proves in Phase 2; until then this avoids surprise AOSS bills.
      searchMode: disabled
defaults:
  region: eu-west-3
  preset: preview
environments:
  preview:
    account: "123456789012"
    class: preview
    preset: preview
    domain: preview.example.com
    expiresAt: "2026-12-31T23:59:59Z"
  staging:
    inherits: preview
    account: "123456789012"
    class: staging
    preset: standard
    domain: staging.example.com
  production:
    inherits: staging
    account: "210987654321"
    class: production
    preset: high-availability
    domain: example.com
    protection: true
extensions: {}
`

const starterGCPConfig = `schemaVersion: 1
project:
  name: example-shop
application:
  edition: open-source
  version: 2.4.9
  mode: integrated
  webRuntime: nginx-fpm
  magento:
    frontName: admin
build:
  php: "8.5"
target:
  provider: gcp
  runtime: gke-autopilot
  gcp:
    project: example-gcp-project
    region: europe-west1
    # Certified cell: no managed search on first run.
    openSearchMode: disabled
defaults:
  region: europe-west1
  preset: preview
environments:
  preview:
    class: preview
    preset: preview
    domain: preview.example.com
    expiresAt: "2026-12-31T23:59:59Z"
  staging:
    inherits: preview
    class: staging
    preset: standard
    domain: staging.example.com
  production:
    inherits: staging
    class: production
    preset: high-availability
    domain: example.com
    protection: true
extensions: {}
`
