package cli

import (
	"errors"
	"fmt"
	"io"
	"regexp"

	"github.com/spf13/cobra"
)

const maxSecretValueBytes = 64 * 1024

var secretName = regexp.MustCompile(`^[A-Za-z0-9/_+=.@-]{1,512}$`)

func secretSetCommand(o *options) *cobra.Command {
	var valueStdin bool
	command := &cobra.Command{
		Use:   "set <name>",
		Short: "Create or update an application secret",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			if !secretName.MatchString(name) {
				return invalid(errors.New("secret name must contain only letters, numbers, /, _, +, =, ., @, or -"))
			}
			if !cmd.Flags().Changed("value-stdin") || !valueStdin {
				return invalid(errors.New("--value-stdin must be explicitly enabled; secret values are never accepted as arguments"))
			}
			value, err := io.ReadAll(io.LimitReader(cmd.InOrStdin(), maxSecretValueBytes+1))
			if err != nil {
				return fmt.Errorf("read secret value: %w", err)
			}
			if len(value) > maxSecretValueBytes {
				return invalid(fmt.Errorf("secret value exceeds %d-byte limit", maxSecretValueBytes))
			}
			_, planned, err := o.planStack(false)
			if err != nil {
				return invalid(err)
			}
			store, err := o.secretsPort()
			if err != nil {
				return err
			}
			if err := store.Set(cmd.Context(), planned, name, value); err != nil {
				if mapped := notSupported(err, planned, "secrets"); mapped != err {
					return mapped
				}
				return fmt.Errorf("set secret: %w", err)
			}
			return o.write(map[string]any{"name": name, "updated": true})
		},
	}
	command.Flags().BoolVar(&valueStdin, "value-stdin", false, "read the secret value from stdin")
	return command
}

func secretListCommand(o *options) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List application secret names",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, planned, err := o.planStack(false)
			if err != nil {
				return invalid(err)
			}
			store, err := o.secretsPort()
			if err != nil {
				return err
			}
			secrets, err := store.List(cmd.Context(), planned)
			if err != nil {
				if mapped := notSupported(err, planned, "secrets"); mapped != err {
					return mapped
				}
				return fmt.Errorf("list secrets: %w", err)
			}
			return o.write(secrets)
		},
	}
}

func secretRemoveCommand(o *options) *cobra.Command {
	return &cobra.Command{
		Use:   "remove <name>",
		Short: "Schedule an application secret for deletion",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			if !secretName.MatchString(name) {
				return invalid(errors.New("secret name must contain only letters, numbers, /, _, +, =, ., @, or -"))
			}
			if !o.yes {
				return invalid(errors.New("removing a secret requires --yes"))
			}
			_, planned, err := o.planStack(false)
			if err != nil {
				return invalid(err)
			}
			store, err := o.secretsPort()
			if err != nil {
				return err
			}
			if err := store.Remove(cmd.Context(), planned, name); err != nil {
				if mapped := notSupported(err, planned, "secrets"); mapped != err {
					return mapped
				}
				return fmt.Errorf("remove secret: %w", err)
			}
			return o.write(map[string]any{"name": name, "scheduled": true})
		},
	}
}
