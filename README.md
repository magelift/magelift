# MageLift

MageLift runs Magento Open Source and licensed Adobe Commerce projects in your cloud account. Define the project in PHP, start a shop locally with `magelift dev`, and deploy a digest from your own registry. Teams can use Magento’s native storefront or connect a separately deployed headless frontend.

From an existing Magento shop with a Composer lock, choose a destination and inspect its environments.

```sh
magelift init --provider gcp
magelift dev
magelift env diff staging production
```

The same project defines staging, UAT, production and pull-request previews. Cloud adapters cover AWS ECS Fargate, GCP GKE Autopilot and Azure AKS at different levels of implementation. MageLift publishes test results for individual paths. The shop owns its service versions, credentials and production acceptance. [Provider coverage](docs/capability-matrix.md) · [Getting started](docs/getting-started.md) · [Moving from Upsun or Adobe Commerce Cloud](docs/migrating-from-paas.md)

AWS, Google Cloud and Azure each have a signed provider plugin. A release CLI loads the one your project selects. To prepare it ahead of time, run `magelift providers install` in the shop checkout. [Installing providers](docs/install.md#providers)

For an existing shop, run `magelift init --from-upsun --provider aws`, `--from-acc` for Adobe Commerce Cloud, or `--from-compose` for a Compose checkout. Each importer accepts AWS, GCP or Azure. Review `magelift.unmapped.md` alongside the generated PHP before moving your data. [SQL and media rehearsal](docs/weekend-migrate.md)

## Install

With the Go toolchain named in [`go.mod`](go.mod), install from a checkout of
this commit. `go install github.com/magelift/magelift/cmd/magelift@main` links
the CLI to the last published SDK module, which is an older candidate.

```sh
git clone https://github.com/magelift/magelift.git
cd magelift
go install ./cmd/magelift
magelift version
```

Next: [getting started](docs/getting-started.md), the
[sample shop](examples/sample-shop), and [local vs cloud](docs/local-vs-cloud.md).
Signed release archives are described in [Install](docs/install.md) for after
the first release.

## Development

```sh
make help
make verify-community
```

Contributor setup, local Compose, and preview commands are in
[Contributing](CONTRIBUTING.md) and [getting started](docs/getting-started.md).

## Trademarks

MageLift is independent of Adobe Inc. Magento and Adobe Commerce are Adobe
trademarks, used here only to describe compatibility.

## License

Original work is [Apache License 2.0](LICENSE). Third-party materials keep their
own licenses and notices.
