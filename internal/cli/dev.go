package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	osExec "os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/magelift/magelift/internal/localdev"
	"github.com/spf13/cobra"
)

func devCommand(o *options) *cobra.Command {
	command := &cobra.Command{Use: "dev", Short: "Run the local Magento development environment"}
	command.AddCommand(devInitCommand(o), devSeedCommand(o), devComposeCommand(o, "up"), devComposeCommand(o, "down"), devComposeCommand(o, "reset"), devComposeCommand(o, "status"), devComposeCommand(o, "logs"), devExecCommand(o))
	return command
}

func devInitCommand(o *options) *cobra.Command {
	return &cobra.Command{Use: "init", Short: "Create the local Docker Compose environment", Args: cobra.NoArgs, RunE: func(*cobra.Command, []string) error {
		file, err := o.load()
		if err != nil {
			return invalid(err)
		}
		build, err := file.ResolveBuild()
		if err != nil {
			return invalid(err)
		}
		path := filepath.Join(filepath.Dir(filepath.Clean(o.configPath)), localdev.ComposeFile)
		if _, err := os.Stat(path); err == nil {
			return invalid(fmt.Errorf("%s already exists", path))
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(path, []byte(localdev.ComposeTemplate), 0o644); err != nil {
			return err
		}
		envPath := filepath.Join(filepath.Dir(path), localdev.LocalEnvFile)
		if _, err := os.Stat(envPath); errors.Is(err, os.ErrNotExist) {
			if err := os.WriteFile(envPath, nil, 0o600); err != nil {
				return err
			}
		} else if err != nil {
			return err
		} else if err := os.Chmod(envPath, 0o600); err != nil {
			return err
		}
		return o.write(map[string]any{"composeFile": path, "project": build.Project.Name, "status": "created"})
	}}
}

func devSeedCommand(o *options) *cobra.Command {
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
					return invalid(fmt.Errorf("local Compose file is missing; run %q first", "magelift dev init"))
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
			if password == "" {
				return invalid(errors.New("MAGELIFT_LOCAL_ADMIN_PASSWORD is required for local seeding"))
			}
			if err := validateLocalAdminPassword(password); err != nil {
				return invalid(err)
			}
			if err := writeLocalCredentials(filepath.Join(root, ".magelift", localdev.LocalEnvFile), map[string]string{
				"MAGELIFT_LOCAL_ADMIN_PASSWORD": password,
			}); err != nil {
				return err
			}
			if err := o.runLocalCompose(cmd.Context(), "up", "app", nil); err != nil {
				return err
			}
			install := []string{
				"sh", "-eu", "-c",
				`exec bin/magento setup:install --no-interaction "$@" --admin-password "$MAGELIFT_LOCAL_ADMIN_PASSWORD"`,
				"--",
				"--base-url=" + baseURL,
				"--db-host=database",
				"--db-name=magento",
				"--db-user=magento",
				"--db-password=magento",
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
		return invalid(errors.New("dev reset requires --yes because it removes local volumes"))
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
			return invalid(fmt.Errorf("local Compose file is missing; run %q first", "magelift dev init"))
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
	env := []string{"MAGELIFT_PROJECT_ROOT=" + root}
	if err := o.runCompose(ctx, root, env, args, o.stdout, o.stderr); err != nil {
		return &exitError{code: 3, err: fmt.Errorf("run local Compose %s: %w", action, err)}
	}
	return nil
}

func runDockerCompose(ctx context.Context, directory string, env []string, args []string, stdout, stderr io.Writer) error {
	command := osExec.CommandContext(ctx, "docker", args...)
	command.Dir = directory
	command.Env = append(os.Environ(), env...)
	command.Stdin = os.Stdin
	command.Stdout = stdout
	command.Stderr = stderr
	return command.Run()
}
