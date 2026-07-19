package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"regexp"

	awssecrets "github.com/acourtiol/magelift/internal/cloud/aws/secrets"
	"github.com/spf13/cobra"
)

const maxSecretValueBytes = 64 * 1024

var secretName = regexp.MustCompile(`^[A-Za-z0-9/_+=.@-]{1,512}$`)

type secretStore interface {
	List(context.Context) ([]awssecrets.Secret, error)
	Set(context.Context, string, []byte) error
	Remove(context.Context, string) error
}

func secretStoreFor(o *options, ctx context.Context) (secretStore, error) {
	effective, _, err := o.resolveWithEnvironment()
	if err != nil {
		return nil, err
	}
	if o.newSecrets == nil {
		return nil, errors.New("secret store factory is required")
	}
	store, err := o.newSecrets(ctx, effective.Config.Defaults.Region)
	if err != nil {
		return nil, fmt.Errorf("initialize secret store: %w", err)
	}
	return store, nil
}

func secretSetCommand(o *options) *cobra.Command {
	var valueStdin bool
	command := &cobra.Command{
		Use:   "set <name>",
		Short: "Create or update an AWS Secrets Manager secret",
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
			store, err := secretStoreFor(o, cmd.Context())
			if err != nil {
				return invalid(err)
			}
			if err := store.Set(cmd.Context(), name, value); err != nil {
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
		Short: "List AWS Secrets Manager secret names",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			store, err := secretStoreFor(o, cmd.Context())
			if err != nil {
				return invalid(err)
			}
			secrets, err := store.List(cmd.Context())
			if err != nil {
				return fmt.Errorf("list secrets: %w", err)
			}
			return o.write(secrets)
		},
	}
}

func secretRemoveCommand(o *options) *cobra.Command {
	return &cobra.Command{
		Use:   "remove <name>",
		Short: "Schedule an AWS Secrets Manager secret for deletion",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			if !secretName.MatchString(name) {
				return invalid(errors.New("secret name must contain only letters, numbers, /, _, +, =, ., @, or -"))
			}
			if !o.yes {
				return invalid(errors.New("removing a secret requires --yes"))
			}
			store, err := secretStoreFor(o, cmd.Context())
			if err != nil {
				return invalid(err)
			}
			if err := store.Remove(cmd.Context(), name); err != nil {
				return fmt.Errorf("remove secret: %w", err)
			}
			return o.write(map[string]any{"name": name, "scheduled": true, "recoveryWindowDays": 30})
		},
	}
}
