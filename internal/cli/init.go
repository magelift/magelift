package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/magelift/magelift/internal/config"
	"github.com/magelift/magelift/internal/paasimport"
	"github.com/magelift/magelift/internal/projectphp"
	"github.com/spf13/cobra"
)

func initCommand(o *options) *cobra.Command {
	var fromACC, fromUpsun, newShop bool
	var provider, configOut, app, storefrontOrigin string
	var edition, release string
	var location starterLocation
	cmd := &cobra.Command{
		Use: "init", Short: "Configure a Magento shop with PHP", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if (fromACC && fromUpsun) || (newShop && (fromACC || fromUpsun)) {
				return invalid(errors.New("--new, --from-acc and --from-upsun are mutually exclusive"))
			}
			if err := validateStarterProvider(provider); err != nil {
				return invalid(err)
			}
			if err := location.validate(provider); err != nil {
				return invalid(err)
			}
			if !newShop && (cmd.Flags().Changed("edition") || cmd.Flags().Changed("magento-version")) {
				return invalid(errors.New("--edition and --magento-version select the Composer project for --new"))
			}
			if edition != "open-source" && edition != "commerce" {
				return invalid(errors.New("--edition must be open-source or commerce"))
			}
			if release != "" {
				pinned, err := normalizeMagentoVersion(release)
				if err != nil || !strings.HasPrefix(pinned, "2.") {
					return invalid(errors.New("--magento-version requires a pinned Magento 2 release"))
				}
				release = pinned
			}
			if app != "" && !fromUpsun {
				return invalid(errors.New("--app selects a Magento application from --from-upsun"))
			}
			if storefrontOrigin != "" {
				if config.ValidateStorefrontOrigin(storefrontOrigin) != nil {
					return invalid(errors.New("--storefront-origin requires an HTTP or HTTPS origin without a path, query, credentials or fragment"))
				}
				storefrontOrigin = strings.TrimSuffix(storefrontOrigin, "/")
			}
			path := o.configPath
			if configOut != "" {
				path = configOut
			}
			path = filepath.Clean(path)
			if filepath.Ext(path) != ".php" {
				return invalid(errors.New("the MageLift project file must end in .php"))
			}

			root := shopSourceRoot(o.configPath)
			if newShop {
				if err := createComposerShop(cmd.Context(), root, edition, release, o); err != nil {
					return err
				}
			}
			var source *paasimport.Source
			if fromUpsun || fromACC {
				kind := paasimport.SourceACC
				if fromUpsun {
					kind = paasimport.SourceUpsun
				}
				var err error
				source, err = paasimport.Read(root, kind, app)
				if err != nil {
					return invalid(err)
				}
			}
			shopRoot := root
			if source != nil && source.AppRoot != "" {
				shopRoot = filepath.Join(root, source.AppRoot)
				// The selected app and configuration must stay in the import repository.
				resolvedRoot, e := filepath.EvalSymlinks(root)
				if e != nil {
					return invalid(e)
				}
				resolvedShop, e := filepath.EvalSymlinks(shopRoot)
				if e != nil {
					return invalid(e)
				}
				rel, e := filepath.Rel(resolvedRoot, resolvedShop)
				if e != nil || !filepath.IsLocal(rel) {
					return invalid(errors.New("application source.root resolves outside the import repository"))
				}
				if configOut == "" && !cmd.Flags().Changed("config") {
					path = filepath.Join(shopRoot, "magelift.php")
				}
			}
			if info, err := os.Lstat(path); err == nil {
				if !o.yes || !info.Mode().IsRegular() {
					return invalid(fmt.Errorf("%s already exists", path))
				}
			} else if !errors.Is(err, os.ErrNotExist) {
				return err
			}
			shop, err := inspectShopSource(shopRoot)
			if err != nil {
				return invalid(err)
			}
			doc, err := starterDocument(provider, shop)
			if err != nil {
				return invalid(err)
			}
			var residuals []paasimport.UnmappedKey
			if source != nil {
				residuals, err = source.Apply(doc)
				if err != nil {
					return invalid(err)
				}
			}
			if storefrontOrigin != "" {
				application := doc["application"].(map[string]any)
				application["mode"] = "headless"
				magento := application["magento"].(map[string]any)
				magento["storefrontOrigin"] = storefrontOrigin
				magento["corsOrigins"] = []string{storefrontOrigin}
			}
			location.apply(doc, provider)
			normalized, err := json.Marshal(doc)
			if err != nil {
				return err
			}
			f, err := config.Load(normalized)
			if err != nil {
				return err
			}
			if _, err := f.ResolveBuild(); err != nil {
				return invalid(err)
			}
			data, err := projectphp.Render(doc)
			if err != nil {
				return err
			}
			if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
				return err
			}
			if err := projectphp.InstallSDK(path); err != nil {
				return err
			}
			report := paasimport.SidecarPath(path)
			if len(residuals) > 0 {
				if info, e := os.Lstat(report); e == nil && (!o.yes || !info.Mode().IsRegular()) {
					return invalid(fmt.Errorf("%s already exists", report))
				} else if e != nil && !errors.Is(e, os.ErrNotExist) {
					return e
				}
			}
			if err := writeInitFile(path, data, o.yes); err != nil {
				return err
			}
			if len(residuals) != 0 {
				if err := writeInitFile(report, []byte(paasimport.RenderUnmappedReport(residuals)), o.yes); err != nil {
					return err
				}
				return invalid(fmt.Errorf("import produced %d unmapped key(s); review %s", len(residuals), report))
			}
			return o.write(initWriteResult{Path: path, Provider: provider, Environment: "staging", Edition: shop.Edition, Release: shop.Version, Next: "run magelift dev to open the local storefront"})
		},
	}
	cmd.Flags().BoolVar(&fromACC, "from-acc", false, "import Adobe Commerce Cloud configuration")
	cmd.Flags().BoolVar(&fromUpsun, "from-upsun", false, "import Upsun or Platform.sh configuration")
	cmd.Flags().BoolVar(&newShop, "new", false, "create a Magento project in an empty directory with Composer")
	cmd.Flags().StringVar(&edition, "edition", "open-source", "edition to create with --new (open-source or commerce)")
	cmd.Flags().StringVar(&release, "magento-version", "", "exact Magento 2 release to create with --new (defaults to Composer's selection)")

	cmd.Flags().StringVar(&app, "app", "", "Magento application to import from a multi-app Upsun project")
	cmd.Flags().StringVar(&configOut, "config-out", "", "output PHP project file for review")
	cmd.Flags().StringVar(&storefrontOrigin, "storefront-origin", "", "connect an externally deployed headless storefront at this origin")
	cmd.Flags().StringVar(&provider, "provider", "gcp", "destination cloud (aws, gcp or azure)")
	cmd.Flags().StringVar(&location.account, "cloud-account", "", "AWS account ID, Google Cloud project ID, or Azure subscription ID")
	cmd.Flags().StringVar(&location.region, "region", "", "default cloud region for the shop")
	cmd.Flags().StringVar(&location.domain, "domain", "", "production host, also used as the parent of staging, UAT and preview hosts")
	return cmd
}

func writeInitFile(path string, data []byte, replace bool) error {
	if replace {
		return replaceFile(path, data, 0644)
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		return err
	}
	_, writeErr := file.Write(data)
	err = errors.Join(writeErr, file.Close())
	if err != nil {
		_ = os.Remove(path)
	}
	return err
}

func createComposerShop(ctx context.Context, root, edition, release string, o *options) error {
	entries, err := os.ReadDir(root)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.Name() != ".git" {
			return invalid(errors.New("magelift init --new requires an empty directory; use magelift init for an existing shop"))
		}
	}
	// Composer rejects a destination containing .git. Stage the installation
	// beside the destination so a failed download leaves the shop untouched.
	staging, err := os.MkdirTemp(filepath.Join(root, ".."), ".magelift-new-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(staging)
	pkg := "magento/project-community-edition"
	if edition == "commerce" {
		pkg = "magento/project-enterprise-edition"
	}
	args := []string{"create-project", "--no-interaction", "--repository-url=https://repo.magento.com/", pkg, staging}
	if release != "" {
		args = append(args, release)
	}
	cmd := exec.CommandContext(ctx, "composer", args...)
	cmd.Stdout, cmd.Stderr = o.stdout, o.stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("create %s with Composer (configure Adobe Marketplace credentials for repo.magento.com): %w", pkg, err)
	}
	shop, err := inspectShopSource(staging)
	if err != nil {
		return err
	}
	if shop.Edition != edition || (release != "" && shop.Version != release) {
		return fmt.Errorf("Composer created %s %s, requested %s %s", shop.Edition, shop.Version, edition, release)
	}
	files, err := os.ReadDir(staging)
	if err != nil {
		return err
	}
	// Check every destination before moving anything. Roll back only the
	// entries moved by this invocation if a later rename fails.
	names := make([]string, 0, len(files))
	for _, entry := range files {
		if entry.Name() == ".git" {
			continue
		}
		target := filepath.Join(root, entry.Name())
		if _, err := os.Lstat(target); !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("destination %s must be empty", target)
		}
		names = append(names, entry.Name())
	}
	for idx, name := range names {
		if err := os.Rename(filepath.Join(staging, name), filepath.Join(root, name)); err != nil {
			for i := idx - 1; i >= 0; i-- {
				err = errors.Join(err, os.Rename(filepath.Join(root, names[i]), filepath.Join(staging, names[i])))
			}
			return err
		}
	}
	return nil
}
