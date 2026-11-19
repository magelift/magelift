# Local development and cloud environments

Run `magelift dev` in the Magento shop to start Compose. It selects the effective
environment’s runtime and service versions, installs Magento into an empty database
and checks the GraphQL store configuration. On subsequent starts it reuses the
database. Use `magelift local logs` and `magelift local exec --service app -- bin/magento list`
to inspect the installation.

| Concern | Local development | Cloud environment |
| --- | --- | --- |
| Application | Shop source mounted into a PHP runtime | Release from the shop’s registry, promoted by digest |
| Services | Upstream container tags or project-selected images for implemented families | Provider-managed, self-hosted or attached dependencies selected in the project |
| HTTP | nginx and PHP-FPM by default, with an optional web cache | Target load balancer, optionally with edge, TLS and DNS |
| Mail | Mailpit or configured relay | Configured provider or relay, with separate preview settings |
| Credentials | Private `.magelift/local.env` | Secret references in the shop’s cloud account |
| Storage and availability | A single Compose project with persistent volumes | Environment-specific data, storage and availability settings |
| Application maintenance | Magento CLI in the app container | Candidate task or Kubernetes job and a deployment lock |

Start with a shared project-level service definition and change capacity per
environment. Use `magelift env diff staging production` to see overrides.
An upstream image tag can move. Set the full local image reference including
its digest when a reproducible local fixture is required. A cloud provider can
expose a major/minor release while its upstream image uses a patch tag. For
example, the AWS OpenSearch selection `3.1` uses the local image tag `3.1.0`.
[Service bindings](service-bindings.md)

`magelift local down` keeps volumes. The smoke test runs a clean, dependency-installed
Magento fixture in its own directory and volumes, checks the storefront and GraphQL,
then tears it down. Cloud IAM, public DNS, managed services, HA failover, and
billing need their own checks. [Test guide](https://github.com/magelift/magelift/tree/main/tests)
