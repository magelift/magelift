# MageLift

MageLift is an open-source application platform for deploying Magento Open Source
and Adobe Commerce repositories to infrastructure in the user's own AWS account.
The intended experience is one project YAML file and one native CLI, backed by
opinionated, production-oriented defaults and explicit escape hatches.

> **Project status:** pre-alpha. The configuration, CLI, and infrastructure
> contracts are under active development and must not yet be treated as stable.
> MageLift is a working name pending trademark and package-name clearance.

## Scope

The first certified runtime is AWS ECS Fargate. MageLift aims to provide:

- strict, explainable configuration with compatibility validation;
- immutable build-once, promote-by-digest releases;
- safe infrastructure previews and deployments;
- production defaults for security, availability, recovery, and observability;
- operational commands that do not require users to edit infrastructure code.

MageLift is not a hosting service and does not claim multi-cloud support.

## Development

Prerequisites and available checks will evolve with the implementation. Run:

```sh
make help
make verify
make floci-test
```

`make floci-test` starts the pinned Floci AWS emulator and runs the bootstrap, state,
deployment-lock, versioned-media restore, ECS runtime, Secrets Manager, CloudWatch
Logs, and candidate-task integration tests. It requires Docker but no AWS account.

For account-free application work, initialize the local Compose project and start
its dependencies:

```sh
magelift dev init
magelift dev up
magelift dev status
```

The generated project uses MySQL and Valkey by default. Run `magelift dev up
--service app` to start the full local stack with the same Debian-based FrankenPHP
image contract plus OpenSearch and RabbitMQ. Capability images are digest-pinned;
the app image uses the local FrankenPHP build tag by default and can be replaced
through the generated
`MAGELIFT_LOCAL_*_IMAGE` variables.
The app serves HTTP on `http://localhost:8080/` and a local-only HTTPS endpoint on
`https://localhost:8443/` using Caddy's internal development CA.

For a repository with Composer dependencies installed, seed a local Magento
database without AWS credentials:

```sh
MAGELIFT_LOCAL_ADMIN_PASSWORD='UseLocalPassword1234' magelift dev seed
```

The password is stored only in the ignored `.magelift/local.env` file with mode
0600 and is not printed or passed as a process argument.

After `magelift bootstrap` has prepared an account, the deployment path is:

```sh
magelift config validate --env staging
magelift preview --env staging
magelift deploy --env staging
magelift outputs --env staging
```

`preview` validates the complete AWS plan without mutating infrastructure. Deploy
and destroy use the S3 lock created during bootstrap. Production and protected
destructive operations require `--yes`. Set `PULUMI_BACKEND_URL` for a DIY or local
Pulumi backend; set `MAGELIFT_AWS_ENDPOINT_URL` only for a loopback AWS-compatible
emulator such as `http://127.0.0.1:4566`. MageLift rejects public or credential-bearing
endpoint URLs before creating AWS clients.

See [the architecture charter](docs/architecture.md), [contribution guide](CONTRIBUTING.md),
and [provenance ledger](docs/provenance.md) before contributing.

## Trademarks

MageLift is an independent project and is not affiliated with, endorsed by, or
sponsored by Adobe Inc. Magento and Adobe Commerce are used descriptively and are
trademarks or registered trademarks of Adobe Inc. See Adobe's trademark guidelines
before publishing project materials. The MageLift name and all public package
identifiers require clearance before the first public release.

## License

Original MageLift work is licensed under the [Apache License 2.0](LICENSE).
Third-party materials retain their respective licenses and notices.
