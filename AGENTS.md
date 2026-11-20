# Working on MageLift

MageLift helps Magento teams run their shops locally and deploy into their own
AWS, Google Cloud, or Azure accounts. Write setup guides for a Magento PHP
developer who may be using cloud infrastructure for the first time. Start
with the [architecture guide](docs/architecture.md) for package ownership and
[CONTRIBUTING.md](CONTRIBUTING.md) for setup.

## Local checks

- Install PHP build dependencies once with `composer install --working-dir=build`.
  Use the Go toolchain in `go.mod`.
- Run `make verify-community` before submitting. Use `make local-gates` for
  changes to provider or state operations and `make ci-act-project` for
  generated shop workflows.
- Run `make generate` after changing project fields, CLI help, or bundled
  skills. Review the generated diff. Run `make docs` for changes to the guides.
- Review user-facing prose with the current Humanizer skill. Keep commands,
  identifiers, and technical claims accurate. [Humanizer releases](https://github.com/blader/humanizer/releases)

## Project rules

- Report the test method, configuration, and outcome. Preserve dated
  [run records](docs/evidence/README.md). Each shop chooses its service versions.
- CI uses Floci for cloud APIs. Shop workflows deploy to the shop's own account.
  A live test needs explicit authorization for a dedicated account, a cleanup
  owner, and teardown on exit.
- Keep proprietary code, customer identifiers, credentials, and Pulumi state
  out of the repository. This includes ece-tools and vendor patch databases.
  Follow the [clean-room rules](docs/provenance.md) and run `make check-clean-room`.
- Ask before a protected or production destroy, cloud spend, a release tag,
  public artifact publication, or a history rewrite.

Find contributor runbooks in [`.agents/skills/`](.agents/skills/). The CLI bundles
the [shop skills](agents/README.md). Update the relevant skill when its commands
or configuration change.
