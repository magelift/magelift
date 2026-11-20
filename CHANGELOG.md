# Changelog

MageLift uses [Semantic Versioning](https://semver.org/). The CLI, SDK,
and first-party provider modules share a release version.

## [1.0.0] - 2026-11-20

The first stable MageLift release includes signed CLI archives and separate
plugins for AWS, Google Cloud, and Azure.

- Run Magento Open Source or licensed Adobe Commerce locally with Docker.
  Configure cloud environments in `magelift.php`.
- Build an immutable image in your registry and deploy it with
  `magelift deploy --from-build <manifest>`. The rollout summary includes
  the shop and GraphQL URLs. Export them as JSON with `--outputs-file`.
- Generate GitHub Actions workflows for pull-request previews, promotion,
  and production deployment. Import settings from Adobe Commerce Cloud,
  Upsun, or Docker Compose.
- Deploy to AWS ECS Fargate or GKE Autopilot. Azure AKS, AWS EKS, and
  GKE Standard are experimental. [Provider support](docs/capability-matrix.md)

Release verification uses Pulumi mocks, Floci for AWS, GCP, and Azure API
requests, and a synthetic shop with BuildKit and a local registry. Dated
live run records describe earlier versions. [Run records](docs/evidence/README.md)
