# Updating dependencies

Use the newest stable release that works on the oldest runtime MageLift supports. Keep the version constraint and lockfile together. A service version that a shop selects in `magelift.php` belongs to that shop.

| Component | Where to update it | What to check |
| --- | --- | --- |
| Go and Pulumi | Root, SDK and provider modules, plus `go.work` | Run `make test-fast`. Check new major module paths separately. For provider upgrades, inspect Pulumi resource graphs and any changes to resource names, defaults or replacement behavior. |
| PHP build library | `build/composer.json` and `build/composer.lock` | Resolve against the PHP 8.3 platform in the manifest. Run PHPUnit, PHPStan and Psalm with `make php-test`. The optional PHP 8.6 CI job tests the build library. Magento runtime recipes cover PHP 8.3 through 8.5. |
| CLI config decoding | Go modules and the PHP project evaluator | Check strict decoding, environment inheritance and where each resolved setting came from. The YAML decoder also reads vendor migration inputs. |
| Website and docs | `website/package.json`, its npm lockfile and `docs/requirements.txt` | Run `npm ci` and `npm run build:site` in `website/`. The build includes the strict MkDocs check. |
| Containers and emulators | Dockerfile, Bake and Compose image references | Use a published image digest for each supported architecture. Build the affected recipe and run the corresponding runtime or Floci tests. |
| GitHub Actions | Repository workflows and the shop workflow generator | Resolve the action’s release tag to its commit. For an annotated tag, pin the underlying commit. Regenerate the sample workflows and run `make workflow-check` and `make ci-act-project`. |

Dependabot checks the Go, Composer, npm, Python, Docker and Actions dependencies weekly. Review major upgrades for API changes and new runtime requirements. For PHP branch lifetimes and Magento service recommendations, consult the upstream tables. [PHP support dates](https://www.php.net/supported-versions.php) · [Adobe Commerce system requirements](https://experienceleague.adobe.com/en/docs/commerce-operations/installation-guide/system-requirements)

CI runs Go vulnerability and license checks, CodeQL, ShellCheck, actionlint and zizmor. Trivy scans the locally built PHP images. The root licenses for the OVH Pulumi and OKMS Go packages are listed in `NOTICE` because the license scanner cannot classify their nested packages. Changes to CLI help belong in the Cobra command. Run `make generate cli-docs` to update generated files and the CLI reference.
