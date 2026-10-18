# MageLift

Deploy Magento Open Source and Adobe Commerce in your own AWS or GCP account
with ACC/Upsun-shaped YAML and a CLI. You keep the cloud account. MageLift does
not host stores.

No release is published yet. `main` is the GCP community alpha, and it is not
a stable release. The first public tag target is `v1.0.0-rc.1`. Until that tag
exists, install from source. Gates are in
[docs/release-readiness.md](docs/release-readiness.md).

**Site:** [magelift.dev](https://magelift.dev/) · **Docs:** [magelift.dev/docs](https://magelift.dev/docs/)

## Scope

The alpha path for a new shop is GCP GKE Autopilot.

| Target | Status |
| --- | --- |
| GCP GKE Autopilot | Certified |
| AWS ECS Fargate | Certified |
| AWS EKS, GCP GKE Standard, OVH MKS, Scaleway Kapsule | Experimental |

See the [capability matrix](docs/capability-matrix.md) and
[ADR 0002](docs/adr/0002-certified-vs-experimental.md). Only certified
cells are production-supported.

## Install

With the Go toolchain named in [`go.mod`](go.mod):

```sh
go install github.com/magelift/magelift/cmd/magelift@main
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
