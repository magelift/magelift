# MageLift

MageLift deploys Magento Open Source and Adobe Commerce from a project YAML file
and a native CLI into cloud accounts the operator already owns. Defaults lean
toward production; escape hatches stay explicit.

> **Project status:** pre-alpha. Configuration, CLI, and infrastructure contracts
> are still changing. Do not treat them as stable. MageLift is a working name
> pending trademark and package-name clearance.

## Scope

**Certified:** AWS ECS Fargate. **Experimental:** GCP GKE Autopilot
([docs](docs/gcp-experimental.md)). The project does not call itself multi-cloud
until two first-party targets are certified
([ADR 0007](docs/adr/0007-multi-provider-community-targets.md)).

What it covers today:

- typed configuration with compatibility checks before mutate
- build once, promote by digest
- preview and deploy with production-oriented defaults
- day-2 commands without editing Pulumi or Go on the supported path

MageLift is not a hosting service. Each cloud lives under
`internal/cloud/<provider>/` behind `platform.StackModule`
([adding a provider](docs/adding-a-provider.md)).

## Development

```sh
make help
make verify
make floci-test
```

`make floci-test` runs the Floci AWS emulator suite (bootstrap, locks, secrets,
logs, media restore, ECS candidates). Docker is required; an AWS account is not.

Local Magento without cloud credentials:

```sh
magelift dev init
magelift dev up
magelift dev status
```

Defaults are MySQL and Valkey. `magelift dev up --service app` also starts
OpenSearch and RabbitMQ with digest-pinned images. Override images with
`MAGELIFT_LOCAL_*_IMAGE` if needed. HTTP is `http://localhost:8080/`; local HTTPS
is `https://localhost:8443/` via Caddy’s internal CA.

Seed a repo that already has Composer deps installed:

```sh
MAGELIFT_LOCAL_ADMIN_PASSWORD='UseLocalPassword1234' magelift dev seed
```

The password lands only in ignored `.magelift/local.env` (mode 0600).

After bootstrap:

```sh
magelift config validate --env staging
magelift preview --env staging
magelift deploy --env staging
magelift outputs --env staging
```

`preview` plans without mutating. Deploy and destroy take the provider lock when
Ops exist (AWS DIY S3 lock today). Production and protected destroys need
`--yes`. Set `PULUMI_BACKEND_URL` for DIY/local Pulumi state. Set
`MAGELIFT_AWS_ENDPOINT_URL` only for loopback emulators such as
`http://127.0.0.1:4566`; public or credential-bearing URLs are rejected.

Read [architecture](docs/architecture.md), [CONTRIBUTING](CONTRIBUTING.md), and
[provenance](docs/provenance.md) before sending a PR.

## Trademarks

MageLift is independent of Adobe Inc. Magento and Adobe Commerce are Adobe
trademarks, used here only to describe compatibility. Clear the MageLift name and
public package identifiers before the first public release.

## License

Original work is [Apache License 2.0](LICENSE). Third-party materials keep their
own licenses and notices.
