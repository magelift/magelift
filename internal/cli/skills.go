package cli

import (
	"errors"
	"fmt"
	"os"

	skillbundle "github.com/magelift/magelift/internal/skills"
	"github.com/magelift/magelift/internal/toolchain"
	"github.com/spf13/cobra"
)

func skillsCommand(o *options) *cobra.Command {
	command := &cobra.Command{
		Use:   "skills",
		Short: "List, install, and verify MageLift agent skills",
	}
	command.AddCommand(skillsListCommand(o), skillsInstallCommand(o), skillsVerifyCommand(o))
	return command
}

func skillsListCommand(o *options) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List bundled first-party skills",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			manifest, err := skillbundle.BundledManifest()
			if err != nil {
				return err
			}
			return o.write(manifest)
		},
	}
}

func skillsInstallCommand(o *options) *cobra.Command {
	var (
		agent   string
		scope   string
		mode    string
		backend string
		replace bool
		names   []string
	)
	command := &cobra.Command{
		Use:   "install",
		Short: "Install bundled skills for an agent",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			projectRoot, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("find project directory: %w", err)
			}
			switch backend {
			case "direct":
				installed, err := skillbundle.Install(skillbundle.InstallOptions{
					ProjectRoot: projectRoot,
					Agent:       agent,
					Scope:       scope,
					Mode:        mode,
					Replace:     replace,
					Skills:      names,
				})
				if err != nil {
					return invalid(err)
				}
				return o.write(map[string]any{"backend": backend, "installed": installed})
			case "skills-cli":
				if err := requireDependencies(cmd.Context(), o, toolchain.SpecsForSkillsCLI(), "skills CLI backend"); err != nil {
					return err
				}
				sourceRoot := skillbundle.FindSourceRoot(projectRoot)
				if sourceRoot == "" {
					return invalid(errors.New("the skills CLI backend needs a MageLift source checkout; use the direct backend from a release binary"))
				}
				if err := skillbundle.RunSkillsCLI(cmd.Context(), sourceRoot, agent, scope, mode, names, o.stdout, o.stderr); err != nil {
					return invalid(err)
				}
				return o.write(map[string]any{"backend": backend, "sourceRoot": sourceRoot, "agent": agent, "scope": scope})
			default:
				return invalid(fmt.Errorf("unsupported skills backend %q; use direct or skills-cli", backend))
			}
		},
	}
	command.Flags().StringVar(&agent, "agent", "generic", "agent path: codex, claude, cursor, or generic")
	command.Flags().StringVar(&scope, "scope", "project", "installation scope: project or global")
	command.Flags().StringVar(&mode, "mode", "auto", "installation mode: auto, copy, or symlink")
	command.Flags().StringVar(&backend, "backend", "direct", "installer backend: direct or skills-cli")
	command.Flags().BoolVar(&replace, "replace", false, "replace an unrelated skill already at the destination")
	command.Flags().StringSliceVarP(&names, "skill", "s", nil, "install only the named skill; repeat the flag for more than one")
	return command
}

func skillsVerifyCommand(o *options) *cobra.Command {
	var (
		agent string
		scope string
		names []string
	)
	command := &cobra.Command{
		Use:   "verify",
		Short: "Verify installed first-party skills without executing them",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			report, err := skillbundle.Verify(skillbundle.InstallOptions{
				Agent:  agent,
				Scope:  scope,
				Skills: names,
			})
			if err != nil {
				return invalid(err)
			}
			if err := o.write(report); err != nil {
				return err
			}
			if !report.Clean {
				return invalid(errors.New("skill verification failed"))
			}
			return nil
		},
	}
	command.Flags().StringVar(&agent, "agent", "generic", "agent path: codex, claude, cursor, or generic")
	command.Flags().StringVar(&scope, "scope", "project", "installation scope: project or global")
	command.Flags().StringSliceVarP(&names, "skill", "s", nil, "verify only the named skill; repeat the flag for more than one")
	return command
}
