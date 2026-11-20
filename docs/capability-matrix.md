# Provider capabilities and test evidence

Choose the Magento, PHP and service versions in `magelift.php` and test them
with your shop. Your cloud and runtime determine which resources MageLift can
configure and which operations are available.

| Target | Implementation and evidence |
| --- | --- |
| AWS ECS Fargate | Provider plugin, Pulumi resource graph, Go lifecycle tests, Floci AWS SDK tests and dated deployment records |
| GCP GKE Autopilot | Provider plugin, Pulumi resource graph, Kubernetes workload tests, Floci GCP SDK tests and dated deployment records |
| Azure AKS | Provider plugin, AKS and MySQL resource graphs, dedicated and shared cluster modes, migration and rollout tests, Floci Blob and Key Vault, and local PHP/MySQL TLS tests. Live Azure deployment remains unverified |
| AWS EKS, GCP GKE Standard | Experimental runtimes with operation-specific coverage |

Synthetic shop tests import Upsun and Compose configurations for AWS, GCP and Azure and resolve staging, UAT, production and preview. Pulumi tests cover separate session stores, search plugins and RabbitMQ resources. A disposable local Kubernetes cluster exercises SQL restore and media access. [Migration procedure](weekend-migrate.md)

Go tests check configuration and lifecycle behavior. Pulumi mocks record which
resources a plan registers, their inputs and their dependencies. The Floci
suites send cloud SDK requests to local emulators. Dated deployment records
name the provider, region, release, resources and outcome.
[Tests](https://github.com/magelift/magelift/tree/main/tests) · [Run evidence](evidence/README.md)

Version 1.0 uses the local checks for its release verification. The live
records cover earlier versions. Run a staging rehearsal in your account
before moving shop traffic.

## Choosing a runtime

| Runtime | What to check for your shop |
| --- | --- |
| AWS ECS Fargate | A starter can use managed database, cache, search and queue services. If you supply a VPC or database, review the resources marked `ADOPT` in the plan. MageLift protects them during teardown. |
| AWS EKS | Enable `MAGELIFT_EXPERIMENTAL_PROVIDERS=1`. Use EKS when you need Kubernetes workloads such as OpenSearch with custom plugins. Review ingress, persistence and the node count. |
| GCP GKE | Autopilot and Standard share the Magento rollout. Check your search image’s node requirements, Cloud SQL availability and the HA settings for each stateful service. |
| Azure AKS | Choose a dedicated cluster or attach an existing one. Review the namespace, storage and database TLS settings before restoring media and SQL. |

A shop can keep the Magento storefront or connect a separately deployed frontend. For headless shops, verify the actual storefront origin, cookies, GraphQL and checkout in staging. Preview environments can use smaller services while keeping the same database family, search adapter and session behavior as production. [Service bindings](service-bindings.md) · [Migration rehearsal](weekend-migrate.md)

## Running the checks

Run `make local-gates` for the provider modules, Pulumi graphs and all three Floci suites. To test one provider, use `make floci-aws-test`, `make floci-gcp-test` or `make floci-azure-test`. Run `make ci-act-project` to exercise a generated shop workflow with a local image registry. The shop’s staging rehearsal covers cloud identity, regional service availability, customer data, traffic cutover and recovery. [Test commands](https://github.com/magelift/magelift/blob/main/tests/README.md)

The archive includes Magento deployments, restores and service failures.
[Run records](evidence/README.md)
