package cli

import (
	"context"
	"errors"
	"fmt"
	"strings"

	mageliftupgrade "github.com/acourtiol/magelift/internal/upgrade"
	"github.com/spf13/cobra"
)

type upgradeRelease = mageliftupgrade.Release

type upgradeClient interface {
	Latest(context.Context) (upgradeRelease, error)
	Tagged(context.Context, string) (upgradeRelease, error)
	Install(context.Context, upgradeRelease, string) error
}

func upgradeCommand(o *options) *cobra.Command {
	var check bool
	var version string
	command := &cobra.Command{
		Use:   "upgrade",
		Short: "Check for or install a signed MageLift CLI release",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if o.newUpgrade == nil {
				return &exitError{code: 3, err: errors.New("upgrade is not configured")}
			}
			client := o.newUpgrade()
			if client == nil {
				return &exitError{code: 3, err: errors.New("upgrade client is not configured")}
			}
			var (
				release upgradeRelease
				err     error
			)
			if strings.TrimSpace(version) == "" {
				release, err = client.Latest(cmd.Context())
			} else {
				release, err = client.Tagged(cmd.Context(), version)
			}
			if err != nil {
				return &exitError{code: 3, err: err}
			}
			if check || strings.TrimPrefix(Version, "v") == strings.TrimPrefix(release.TagName, "v") {
				return o.write(map[string]any{"current": Version, "latest": release.TagName, "updateAvailable": strings.TrimPrefix(Version, "v") != strings.TrimPrefix(release.TagName, "v")})
			}
			if o.executable == nil {
				return &exitError{code: 3, err: errors.New("current executable path is not configured")}
			}
			executable, err := o.executable()
			if err != nil {
				return &exitError{code: 3, err: fmt.Errorf("locate current executable: %w", err)}
			}
			if err := client.Install(cmd.Context(), release, executable); err != nil {
				return &exitError{code: 3, err: err}
			}
			return o.write(map[string]any{"current": Version, "updated": release.TagName, "executable": executable})
		},
	}
	command.Flags().BoolVar(&check, "check", false, "check for an update without replacing the executable")
	command.Flags().StringVar(&version, "version", "", "install a specific release tag")
	return command
}
