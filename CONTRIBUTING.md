# Contributing

For a first change, fix a broken link, document a confusing setup step, or add a
regression to an existing test. For a change that spans providers or alters
deployment behavior, open an issue with an example shop and the result you
expect.

## Set up the checkout

Use the Go version in [go.mod](go.mod), PHP 8.3 or later, and Composer. Install
the build package's dependencies once.

```sh
composer install --working-dir=build
make verify-community
```

The CLI, SDK, and each provider have their own Go module. Run `make test-fast`
to test all of them. Install Docker to run Floci and the local shop workflows.
Pinned versions of the documentation and Go tools are selected by the Makefile.

## Verify a change

| Change | Check |
| --- | --- |
| Go, PHP, or scripts | `make verify-community` |
| Provider or state operations | `make local-gates` |
| Generated shop workflow | `make ci-act-project` |
| Schema, CLI help, or bundled skills | `make generate`, then review the diff |
| Documentation | `make docs` |
| Landing site | `npm --prefix website run build:site` |

Report the command you ran, the configuration it used, and what passed or
failed. CI uses Floci for cloud APIs. A live run needs explicit authorization
for a dedicated account, an owner for cleanup, and teardown on exit. Keep
credentials, Pulumi state, customer data, and vendor source out of the checkout.
[Clean-room rules](docs/provenance.md)

## Find the code

- `cmd/magelift` starts the CLI. `internal/cli` implements its commands.
- `sdk/` defines shared types and the plugin protocol.
- `providers/aws/`, `providers/gcp/`, and `providers/azure/` each build a provider plugin.
- `build/` contains the PHP build package. `images/` contains the bundled nginx
  recipe and Varnish configuration.
- `docs/` contains the guides. `website/` contains the landing site.

See the [architecture guide](docs/architecture.md) for package boundaries.

## Agent skills

Contributor skills live in [`.agents/skills/`](.agents/skills/) so agents can
discover them from the checkout. The CLI bundles the [shop skills](agents/README.md)
from `agents/skills/`. Update the relevant skill when you change a command it
teaches.

Open a pull request with the problem, the change, and the checks you ran.
Use a Conventional Commit subject when creating commits, such as
`fix(config): explain an invalid service setting`.
