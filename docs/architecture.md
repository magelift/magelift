# MageLift architecture

MageLift deploys the Magento application and its infrastructure into the shop’s cloud account. One trusted PHP file describes the application, service versions and environments. The same shop can run a native storefront or expose Magento’s GraphQL API to a storefront hosted elsewhere. [Configure a project](configuration.md) · [Connect a storefront](storefront-recipes.md)

## From a shop checkout to a running environment

1. The CLI evaluates `magelift.php`, resolves the selected environment and reports version differences from Adobe’s published requirements. The shop can select its own versions. A provider can still reject a resource, protocol or regional SKU it cannot implement. [Version advice](compatibility.md) · [Service bindings](service-bindings.md)
2. The build produces an OCI image and records its digest in a separate manifest. A team promotes the same signed image across environments. Composer credentials and runtime secret values stay out of the image. [Builds](builds.md) · [Artifact manifest](adr/0005-external-artifact-manifest.md)
3. The CLI selects the provider and runtime, verifies the plugin against its lock and asks it to plan. The plugin owns the Pulumi program. The shop’s backend holds stack state. Before a change, the CLI checks protection, acquires the deployment lock and requests a preview. [Install providers](install.md#providers) · [Bootstrap](bootstrap.md)
4. During deployment the provider registers the candidate image, runs Magento migrations and reports rollout and health results. A failed migration stops the traffic change. Rollback redeploys an older image. Database recovery uses its own backup and restore procedure. [Operations](operations.md)

A staging or UAT environment has a stable name. Protection, capacity and redundancy come from its settings. A pull-request preview gets its own stack name, hostname and expiry. Previews can reuse bootstrap resources while keeping separate state keys and deployment locks. An operator can compare resolved settings before provisioning. [Environment configuration](configuration.md)

## Providers

AWS, Google Cloud and Azure each ship a `magelift-provider-<name>` executable from a separate Go module under `providers/`. The CLI uses the same steps for each one. It selects an exact version, verifies its digest and Cosign signature, then checks the provider identity, protocol version and declared operations. Help and local project validation work before a cloud plugin is installed. Each provider process has access to the operator’s cloud credentials. [Distribution trust](adr/0014-distribution-trust.md) · [Provider contract](adr/0013-provider-plugin-contract.md)

| Provider | Runtimes | Examples of provider-owned work |
| --- | --- | --- |
| AWS | ECS Fargate, experimental EKS | VPC, RDS or Aurora, ElastiCache, S3, IAM, deployment tasks and CloudWatch |
| Google Cloud | GKE Autopilot, experimental GKE Standard | GKE, Cloud SQL, private connectivity, Cloud Storage, workload identity and Cloud Monitoring |
| Azure | Experimental AKS | Dedicated or shared AKS, MySQL Flexible Server, Azure Files, Blob Storage and Key Vault |

Set `MAGELIFT_EXPERIMENTAL_PROVIDERS=1` to enable AWS EKS, OVHcloud MKS or Scaleway Kapsule. The OVHcloud and Scaleway adapters are compiled into the CLI and maintained under `internal/cloud/`. New first-party providers follow the plugin layout. Resource support varies by runtime. The capability guide distinguishes mock resource graphs, emulator requests and dated runs in a real account. [Capability guide](capability-matrix.md) · [Add a provider](adding-a-provider.md)

### AWS Magento product matrix

ECS Fargate can provision or attach a network, use RDS MySQL or MariaDB, and select Aurora where the requested engine is available. Cache and session settings can differ. Search can use a managed OpenSearch service or an external endpoint. EKS can run a custom OpenSearch image with the plugins a Magento extension needs. Database queues, RabbitMQ and other queue options depend on the target. Edge and observability settings also belong to the chosen runtime. Check the generated plan for the selected services, sizes, retention and network exposure. [Service options](service-bindings.md) · [EKS](aws-eks-experimental.md) · [Existing resources](brownfield-attach.md)

## Repository layout

| Path | What belongs here |
| --- | --- |
| `cmd/`, `cli/`, `internal/cli/` | CLI entry point, generators, public Go embedding API and command implementations |
| `sdk/` | Public Magento service types, extension interfaces, protocol operations and version negotiation |
| `providers/<name>/` | One first-party plugin, its executable under `cmd/`, cloud resources, operations, Go module and Floci tests |
| `internal/registry/`, `internal/providerhost/` | Runtime registration, verified plugin discovery, installation, process startup and typed calls |
| `internal/config/`, `internal/projectphp/`, `internal/paasimport/` | Configuration, trusted PHP evaluation and import from a shop’s PaaS configuration |
| `internal/platform/`, `internal/deploy/`, `internal/automation/` | Stack and operation contracts, Magento deployment ordering and Pulumi Automation helpers |
| `internal/cloud/kube/`, `internal/s3state/`, `internal/mediasync/` | Kubernetes workload operations, S3-compatible state storage and Magento media transfer used by multiple adapters |
| `internal/external/`, `internal/shared/` | External edge and telemetry adapters, shared recovery and state archive logic |
| `build/`, `images/`, `internal/build/` | Magento’s Composer build package, optional image recipes, and the Go code that runs builds |
| `agents/`, `contrib/` | Skills installed into shop projects and contributor-only runbooks, respectively |
| `tests/`, `providers/<name>/tests/`, `testdata/` | Cross-provider checks, provider-specific acceptance helpers where needed, and public synthetic fixtures |
| `docs/`, `website/` | MkDocs guides and the Astro website. MkDocs writes its generated output to `site/`. |

The root module and each nested module are listed in `go.work`. A root `go test ./...` covers one module. `make test-fast` runs them all. `make core-leanness` checks the CLI dependency graph for accidentally linked cloud plugins. [Testing](https://github.com/magelift/magelift/tree/main/tests) · [Contributing](https://github.com/magelift/magelift/blob/main/CONTRIBUTING.md)

## Runtime storage and access

The image contains Magento code, compiled dependency injection output and static assets. Magento uses writable runtime directories for temporary files, generated configuration and logs. Durable media lives in object storage or a mounted volume, according to the target. The database, session store and Magento encryption key must survive a rollout. Each provider maps secret references into the workload. A shop migrating from Upsun or Adobe Commerce Cloud must preserve its existing encryption key. [Backups and operations](operations.md) · [Migration procedure](weekend-migrate.md)

Native edge and telemetry resources belong to the runtime. An external edge or observability service can use a separate adapter with its own credentials and lifecycle. Check the outputs and the signals each adapter supports before relying on a dashboard or alert. [Operations guide](operations.md) · [Provider coverage](capability-matrix.md)

## Local development and verification

`magelift dev` runs the shop locally with Docker Compose. Pulumi mocks check resource registrations and dependencies. Floci exercises selected SDK requests against AWS, GCP and Azure emulators. The repository’s cloud CI uses these emulators. Generated shop workflows deploy into the shop’s own account. `make ci-act-project` tests generated workflow behavior against a local registry. Each test record describes the path it exercised. [Local and cloud environments](local-vs-cloud.md) · [Historical run records](evidence/README.md)
