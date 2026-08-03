# Dependency policy

MageLift uses the newest production-suitable dependency compatible with its supported runtime floor. Updates are automated, reviewed, locked, and verified by the complete test suite.

- Go declares the latest supported stable toolchain. CI reads `go.mod` rather than duplicating the version.
- Cobra tracks the latest stable minor release.
- YAML v4 is used because upstream has frozen v3 except for security maintenance and recommends v4 for new projects. Until v4 reaches a final release, its pinned release candidate must pass all strict-decoding, merge, and provenance tests before each update.
- The PHP package keeps PHP 8.2 as its runtime floor and tests through PHP 8.5. PHPUnit stays on the latest 11.x release because newer PHPUnit majors would raise the test suite's PHP floor even though the library still supports PHP 8.2.
- Composer resolves the lock file against PHP 8.2.27, the lowest patch required by
  the current static-analysis toolchain, so a dependency cannot pass on PHP 8.5 and
  fail on the PHP 8.2 CI job. The package still declares `^8.2` and CI exercises
  every supported branch.
- PHPStan and Psalm run on every verification job. Their dynamic JSON and process
  boundary checks are kept in source rather than hidden in a generated baseline.
- Docker base images are pinned by multi-platform digest. The FrankenPHP classic adapter currently uses the Debian Trixie PHP 8.5 image from the 1.12.6 release; Dependabot tracks Docker digest updates.
- Pulumi Automation API and cloud provider SDKs that MageLift implements are direct
  Go dependencies. Their versions are pinned in `go.mod`, checked with
  `go list -m -u all`, and exercised by Pulumi mock graph tests. Additional cloud
  SDKs enter the module only when that target is implemented.
- Magento and PHP support data is date-stamped in the compatibility catalog. The AWS
  stack planner applies the service-version combinations from [Adobe's system
  requirements](https://experienceleague.adobe.com/en/docs/commerce-operations/installation-guide/system-requirements)
  before it registers infrastructure resources. PHP branch support follows the
  [PHP supported versions](https://www.php.net/supported-versions.php) page.

Dependabot opens the dependency update pull requests. Go modules, Composer packages, and GitHub Actions use separate update groups so their tests and release risks stay visible.

CI also runs Go vulnerability and license checks, CodeQL for Go, and Trivy
against every published Debian PHP runtime, builder, and FrankenPHP classic image
for each supported PHP branch. `go-licenses` ignores `github.com/ovh/pulumi-ovh`
because the Apache-2.0 LICENSE sits at the module root while nested Go packages
are not classified; that license is recorded in `NOTICE`. Shellcheck validates repository shell scripts, and
actionlint validates workflow syntax and expressions;
Zizmor audits GitHub Actions workflows for unsafe
permissions, unpinned actions, and injection paths. A finding blocks the relevant job until it is fixed,
explicitly accepted with evidence, or removed from the supported release.

The CLI reference is generated from the Cobra command tree. `make cli-docs-check`
fails when command names, flags, or descriptions drift from `docs/cli-reference.md`.
