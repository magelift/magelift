package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/magelift/magelift/internal/localdev"
	"github.com/magelift/magelift/internal/toolchain"
	"github.com/spf13/cobra"
)

func devQuickstartCommand(o *options) *cobra.Command {
	seed := localSeed(o, true)
	command := &cobra.Command{Use: "dev", Short: "Start Magento locally, install an empty shop and check the storefront", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		ctx := cmd.Context()
		root := filepath.Dir(o.configPath)
		if _, err := os.Stat(filepath.Join(root, "bin", "magento")); err != nil {
			return invalid(fmt.Errorf("Magento CLI at bin/magento: %w; install the shop dependencies with Composer", err))
		}
		plan, err := o.prepareLocalFiles(true)
		if err != nil {
			return err
		}
		if err := requireLocalDependencies(ctx, o); err != nil {
			return err
		}
		if err := ensureDevImage(ctx, plan, o); err != nil {
			return err
		}
		if _, err := os.Stat(filepath.Join(root, "app", "etc", "env.php")); err == nil {
			if err := o.runLocalCompose(ctx, "up", "app", nil); err != nil {
				return err
			}
		} else if errors.Is(err, os.ErrNotExist) {
			if err := seed.RunE(cmd, nil); err != nil {
				return err
			}
		} else {
			return err
		}
		address := localURL(o)
		if err := waitForMagento(ctx, address); err != nil {
			return err
		}
		return o.write(map[string]any{"status": "ready", "url": address, "environment": plan.Environment, "adminCredentialsFile": filepath.Join(root, ".magelift", localdev.LocalEnvFile)})
	}}
	command.Flags().AddFlag(seed.Flags().Lookup("search-engine"))
	return command
}

func ensureDevImage(ctx context.Context, plan localdev.RuntimePlan, o *options) error {
	if o.getenv("MAGELIFT_LOCAL_APP_IMAGE") != "" || !strings.HasPrefix(plan.AppImage, "magelift/") {
		return nil
	}
	inspect := exec.CommandContext(ctx, "docker", "image", "inspect", plan.AppImage)
	if err := inspect.Run(); err == nil {
		return nil
	}
	if plan.WebServer.Family != "nginx" {
		return fmt.Errorf("build the %s runtime or set MAGELIFT_LOCAL_APP_IMAGE to your runtime image", plan.WebServer.Family)
	}
	images, err := toolchain.BuildImagesForPHP(ctx, o.stderr, "", "linux/amd64", "", plan.PHP)
	if err != nil {
		return err
	}
	tag := exec.CommandContext(ctx, "docker", "tag", images.RuntimeID, plan.AppImage)
	tag.Stderr = o.stderr
	return tag.Run()
}

func waitForMagento(ctx context.Context, address string) error {
	base, err := url.Parse(address)
	if err != nil || base.Hostname() == "" || (base.Scheme != "http" && base.Scheme != "https") {
		return errors.New("local base URL must be an HTTP or HTTPS URL")
	}
	probe := base.JoinPath("graphql")
	query := probe.Query()
	query.Set("query", "{storeConfig{code}}")
	probe.RawQuery = query.Encode()
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	client := http.Client{Timeout: 5 * time.Second}
	for {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, probe.String(), nil)
		if err != nil {
			return err
		}
		res, err := client.Do(req)
		if err == nil {
			var data struct {
				Data struct {
					StoreConfig struct {
						Code string `json:"code"`
					} `json:"storeConfig"`
				} `json:"data"`
			}
			decodeErr := json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&data)
			res.Body.Close()
			if decodeErr == nil && res.StatusCode == http.StatusOK && data.Data.StoreConfig.Code != "" {
				return nil
			}
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("Magento did not serve the storeConfig GraphQL query on %s: %w", base, ctx.Err())
		case <-time.After(time.Second):
		}
	}
}
