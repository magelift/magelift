package cli

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	osExec "os/exec"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/magelift/magelift/internal/config"
	"github.com/magelift/magelift/internal/localdev"
	"github.com/spf13/cobra"
)

func devCommand(o *options) *cobra.Command {
	command := &cobra.Command{Use: "local", Short: "Run the local Magento environment"}
	command.AddCommand(devInitCommand(o), devSeedCommand(o), devComposeCommand(o, "up"), devComposeCommand(o, "down"), devComposeCommand(o, "reset"), devComposeCommand(o, "status"), devComposeCommand(o, "logs"), devExecCommand(o))
	return command
}

func devInitCommand(o *options) *cobra.Command {
	return &cobra.Command{Use: "init", Short: "Create the local Docker Compose environment", Args: cobra.NoArgs, RunE: func(*cobra.Command, []string) error {
		runtimePlan, err := o.prepareLocalFiles(false)
		if err != nil {
			return err
		}
		path := filepath.Join(filepath.Dir(filepath.Clean(o.configPath)), localdev.ComposeFile)
		return o.write(map[string]any{"composeFile": path, "phpIniFile": filepath.Join(filepath.Dir(path), localdev.LocalPHPIniFile), "environment": runtimePlan.Environment, "runtime": runtimePlan, "status": "created"})
	}}
}

func (o *options) prepareLocalFiles(reuse bool) (localdev.RuntimePlan, error) {
	file, err := o.load()
	if err != nil {
		return localdev.RuntimePlan{}, invalid(err)
	}
	environment, err := o.selectLocalEnvironment(file)
	if err != nil {
		return localdev.RuntimePlan{}, err
	}
	effective, _, err := o.resolveEnvironment(file, environment)
	if err != nil {
		return localdev.RuntimePlan{}, invalid(err)
	}
	build, err := file.ResolveBuild()
	if err != nil {
		return localdev.RuntimePlan{}, invalid(err)
	}
	build.Local = effective.Config.Local
	hints := localdev.HintsFromConfig(effective.Config)
	hints.Environment = environment
	runtimePlan, err := localdev.PlanFor(build, hints)
	if err != nil {
		return localdev.RuntimePlan{}, invalid(err)
	}
	phpIni, err := localdev.PHPIniFor(runtimePlan)
	if err != nil {
		return localdev.RuntimePlan{}, invalid(err)
	}
	path := filepath.Join(filepath.Dir(filepath.Clean(o.configPath)), localdev.ComposeFile)
	if _, readErr := os.Stat(path); readErr == nil && !reuse {
		return localdev.RuntimePlan{}, invalid(fmt.Errorf("%s already exists", path))
	} else if readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
		return localdev.RuntimePlan{}, readErr
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return localdev.RuntimePlan{}, err
	}
	phpIniPath := filepath.Join(filepath.Dir(path), localdev.LocalPHPIniFile)
	if err := reconcileLocalGenerated(filepath.Dir(path), map[string][]byte{
		filepath.Base(path):       []byte(localdev.ComposeTemplateFor(runtimePlan)),
		filepath.Base(phpIniPath): []byte(phpIni),
	}); err != nil {
		return localdev.RuntimePlan{}, err
	}
	envPath := filepath.Join(filepath.Dir(path), localdev.LocalEnvFile)
	if info, err := os.Stat(envPath); errors.Is(err, os.ErrNotExist) {
		file, err := os.OpenFile(envPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err != nil {
			return localdev.RuntimePlan{}, err
		}
		if err := file.Close(); err != nil {
			return localdev.RuntimePlan{}, err
		}
	} else if err != nil {
		return localdev.RuntimePlan{}, err
	} else if !info.Mode().IsRegular() {
		return localdev.RuntimePlan{}, invalid(fmt.Errorf("%s must be a regular credentials file", envPath))
	} else if err := os.Chmod(envPath, 0o600); err != nil {
		return localdev.RuntimePlan{}, err
	}
	return runtimePlan, nil
}

// The journal records both the previous and intended content before writing
// either file. A retry can therefore recognize a partially completed update.
func reconcileLocalGenerated(directory string, desired map[string][]byte) error {
	manifestPath := filepath.Join(directory, "local-generated.json")
	journal := map[string][]string{}
	if data, err := os.ReadFile(manifestPath); err == nil {
		if err := json.Unmarshal(data, &journal); err != nil || journal == nil {
			return invalid(fmt.Errorf("%s is invalid; inspect it and move it aside before running dev", manifestPath))
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	next := map[string][]string{}
	for name, content := range desired {
		path := filepath.Join(directory, name)
		current, err := os.ReadFile(path)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		newHash := localContentHash(content)
		next[name] = []string{newHash}
		if err == nil {
			oldHash := localContentHash(current)
			if oldHash != newHash && !slices.Contains(journal[name], oldHash) {
				return invalid(fmt.Errorf("%s has manual changes; review and move it aside before running magelift dev", path))
			}
			if oldHash != newHash {
				next[name] = append(next[name], oldHash)
			}
		}
	}
	data, err := json.Marshal(next)
	if err != nil {
		return err
	}
	if err := writeLocalGeneratedFile(manifestPath, data, 0o600); err != nil {
		return err
	}
	for name, content := range desired {
		path := filepath.Join(directory, name)
		current, err := os.ReadFile(path)
		if err == nil {
			if string(current) == string(content) {
				continue
			}
			if !slices.Contains(next[name], localContentHash(current)) {
				return invalid(fmt.Errorf("%s changed while reconciling; review it and rerun magelift dev", path))
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if err := writeLocalGeneratedFile(path, content, 0o644); err != nil {
			return err
		}
	}
	for name, content := range desired {
		next[name] = []string{localContentHash(content)}
	}
	data, err = json.Marshal(next)
	if err != nil {
		return err
	}
	return writeLocalGeneratedFile(manifestPath, data, 0o600)
}

func localContentHash(data []byte) string {
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}

func writeLocalGeneratedFile(path string, data []byte, mode os.FileMode) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".local-generated-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err := f.Chmod(mode); err != nil {
		_ = f.Close()
		return err
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}

func devSeedCommand(o *options) *cobra.Command { return localSeed(o, false) }

func localSeed(o *options, generatePassword bool) *cobra.Command {
	var baseURL, adminUser, adminEmail, firstName, lastName, backendFrontname string
	command := &cobra.Command{
		Use:   "seed",
		Short: "Install Magento into the local Compose services",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			file, err := o.load()
			if err != nil {
				return invalid(err)
			}
			build, err := file.ResolveBuild()
			if err != nil {
				return invalid(err)
			}
			root := filepath.Dir(filepath.Clean(o.configPath))
			composePath := filepath.Join(root, localdev.ComposeFile)
			if _, err := os.Stat(composePath); err != nil {
				if errors.Is(err, os.ErrNotExist) {
					return invalid(fmt.Errorf("local Compose file is missing; run %q first", "magelift local init"))
				}
				return err
			}
			if _, err := os.Stat(filepath.Join(root, "bin", "magento")); err != nil {
				if errors.Is(err, os.ErrNotExist) {
					return invalid(errors.New("Magento CLI is missing at bin/magento; install the project dependencies first"))
				}
				return err
			}
			if _, err := os.Stat(filepath.Join(root, "app", "etc", "env.php")); err == nil {
				return o.write(map[string]any{"status": "already-installed", "project": build.Project.Name})
			} else if !errors.Is(err, os.ErrNotExist) {
				return err
			}

			password := strings.TrimSpace(o.getenv("MAGELIFT_LOCAL_ADMIN_PASSWORD"))
			if password == "" && generatePassword {
				key := make([]byte, 20)
				if _, err := rand.Read(key); err != nil {
					return err
				}
				password = "Local" + hex.EncodeToString(key)
			}
			if password == "" {
				return invalid(errors.New("MAGELIFT_LOCAL_ADMIN_PASSWORD is required for local seeding"))
			}
			if err := validateLocalAdminPassword(password); err != nil {
				return invalid(err)
			}
			if err := requireLocalDependencies(cmd.Context(), o); err != nil {
				return err
			}
			if err := writeLocalCredentials(filepath.Join(root, ".magelift", localdev.LocalEnvFile), map[string]string{
				"MAGELIFT_LOCAL_ADMIN_PASSWORD": password,
			}); err != nil {
				return err
			}
			if err := o.runLocalCompose(cmd.Context(), "up", "app", nil); err != nil {
				return err
			}
			// Validate the same database that setup:install will use, inside the
			// same container. Leave a pre-existing or partially installed store intact.
			guard := `php -r '$db=new PDO("mysql:host=database;dbname=magento","magento","magento"); if ($db->query("SHOW TABLES")->fetchColumn() !== false) {fwrite(STDERR,"Local database already contains tables. Restore its matching app/etc/env.php before seeding.\n"); exit(1);}'`
			install := []string{
				"sh", "-eu", "-c",
				guard + "\n" + `bin/magento setup:install --no-interaction "$@" --admin-password "$MAGELIFT_LOCAL_ADMIN_PASSWORD"
bin/magento app:config:import --no-interaction
exec bin/magento cache:clean config`,
				"--",
				"--base-url=" + baseURL,
				"--db-host=database",
				"--db-name=magento",
				"--db-user=magento",
				"--db-password=magento",
				"--search-engine=opensearch", "--opensearch-host=search", "--opensearch-port=9200",
				"--admin-firstname=" + firstName,
				"--admin-lastname=" + lastName,
				"--admin-email=" + adminEmail,
				"--admin-user=" + adminUser,
				"--language=en_US",
				"--currency=USD",
				"--timezone=UTC",
				"--use-rewrites=1",
				"--backend-frontname=" + backendFrontname,
			}
			return o.runLocalCompose(cmd.Context(), "exec", "app", install)
		},
	}
	command.Flags().StringVar(&baseURL, "base-url", localURL(o), "local Magento base URL")
	command.Flags().StringVar(&adminUser, "admin-user", "admin", "Magento administrator username")
	command.Flags().StringVar(&adminEmail, "admin-email", "admin@example.test", "Magento administrator email")
	command.Flags().StringVar(&firstName, "admin-firstname", "MageLift", "Magento administrator first name")
	command.Flags().StringVar(&lastName, "admin-lastname", "Local", "Magento administrator last name")
	command.Flags().StringVar(&backendFrontname, "backend-frontname", "admin", "Magento administrator URL segment")
	return command
}

func localURL(o *options) string {
	if value := strings.TrimSpace(o.getenv("MAGELIFT_LOCAL_BASE_URL")); value != "" {
		return value
	}
	if port := strings.TrimSpace(o.getenv("MAGELIFT_LOCAL_APP_HTTP_PORT")); port != "" {
		return "http://localhost:" + port + "/"
	}
	return "http://localhost:8080/"
}

func validateLocalAdminPassword(password string) error {
	if len(password) < 16 || len(password) > 128 {
		return errors.New("local administrator password must be between 16 and 128 characters")
	}
	for _, character := range password {
		if strings.ContainsRune("ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789", character) == false {
			return errors.New("local administrator password may contain only letters and numbers")
		}
	}
	return nil
}

func writeLocalCredentials(path string, values map[string]string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	var content strings.Builder
	previous, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	for _, line := range strings.Split(strings.TrimSuffix(string(previous), "\n"), "\n") {
		if line == "" {
			continue
		}
		key, _, _ := strings.Cut(line, "=")
		if _, replace := values[strings.TrimSpace(key)]; !replace {
			content.WriteString(line)
			content.WriteByte('\n')
		}
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		value := values[key]
		if strings.ContainsAny(key+value, "\x00\r\n=") {
			return errors.New("local credential contains an invalid character")
		}
		content.WriteString(key)
		content.WriteByte('=')
		content.WriteString(value)
		content.WriteByte('\n')
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".local-env-*")
	if err != nil {
		return err
	}
	temporaryName := temporary.Name()
	defer func() {
		_ = os.Remove(temporaryName)
	}()
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return err
	}
	if _, err := temporary.WriteString(content.String()); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporaryName, path); err != nil {
		return fmt.Errorf("replace local credentials: %w", err)
	}
	return nil
}

func devComposeCommand(o *options, action string) *cobra.Command {
	var service string
	command := &cobra.Command{Use: action, Short: "Run local Compose " + action, Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		return o.runLocalCompose(cmd.Context(), action, service, nil)
	}}
	if action == "up" || action == "logs" {
		command.Flags().StringVar(&service, "service", "", "limit the operation to one Compose service")
	}
	return command
}

func devExecCommand(o *options) *cobra.Command {
	var service string
	command := &cobra.Command{Use: "exec --service app -- <command>", Short: "Run a command in a local Compose service", Args: cobra.MinimumNArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		return o.runLocalCompose(cmd.Context(), "exec", service, args)
	}}
	command.Flags().StringVar(&service, "service", "app", "Compose service")
	return command
}

func (o *options) runLocalCompose(ctx context.Context, action, service string, command []string) error {
	if action == "reset" && !o.yes {
		return invalid(errors.New("local reset requires --yes because it removes local volumes"))
	}
	file, err := o.load()
	if err != nil {
		return invalid(err)
	}
	build, err := file.ResolveBuild()
	if err != nil {
		return invalid(err)
	}
	root := filepath.Dir(filepath.Clean(o.configPath))
	composeFile := filepath.Join(root, localdev.ComposeFile)
	if _, err := os.Stat(composeFile); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return invalid(fmt.Errorf("local Compose file is missing; run %q first", "magelift local init"))
		}
		return err
	}
	project, err := localdev.ProjectName(build.Project.Name)
	if err != nil {
		return invalid(err)
	}
	args, err := localdev.ComposeArgs(filepath.Join(".", localdev.ComposeFile), project, action, service, command)
	if err != nil {
		return invalid(err)
	}
	if o.runCompose == nil {
		return &exitError{code: 3, err: errors.New("Docker Compose runner is unavailable")}
	}
	if err := requireLocalDependencies(ctx, o); err != nil {
		return err
	}
	absoluteRoot, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	env := []string{"MAGELIFT_PROJECT_ROOT=" + absoluteRoot}
	if err := o.runCompose(ctx, root, env, args, o.stdout, o.stderr); err != nil {
		return &exitError{code: 3, err: fmt.Errorf("run local Compose %s: %w", action, err)}
	}
	return nil
}

func runDockerCompose(ctx context.Context, directory string, env []string, args []string, stdout, stderr io.Writer) error {
	command := osExec.CommandContext(ctx, "docker", args...)
	command.Dir = directory
	command.Env = append(os.Environ(), env...)
	if uid, gid := os.Getuid(), os.Getgid(); uid >= 0 && gid >= 0 {
		command.Env = append(command.Env, fmt.Sprintf("MAGELIFT_LOCAL_UID=%d", uid), fmt.Sprintf("MAGELIFT_LOCAL_GID=%d", gid))
	}
	command.Stdin = os.Stdin
	command.Stdout = stdout
	command.Stderr = stderr
	return command.Run()
}

func (o *options) selectLocalEnvironment(file *config.File) (string, error) {
	if o.environment != "" {
		return o.environment, nil
	}
	if env := o.getenv("MAGELIFT_ENV"); env != "" {
		return env, nil
	}
	environments := file.Environments()
	if len(environments) == 1 {
		return environments[0], nil
	}
	if slices.Contains(environments, "staging") {
		return "staging", nil
	}
	return o.selectEnvironment(file)
}
