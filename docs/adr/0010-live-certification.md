# ADR 0010. Run cloud checks locally

The original August 2026 decision grouped manual acceptance checks into a single deployment to control spending. The [historical sessions](../certification-sessions.md) and their dated results remain available.

MageLift’s own CI runs Go tests, Pulumi mocks and Floci on AWS, GCP and Azure. Generated shop workflows use the shop’s cloud account. Maintainers can run a separately authorized acceptance exercise in a dedicated account and record its configuration, outcome and teardown. MageLift does not certify Magento versions, cloud services or deployment combinations. [Evidence policy](0002-certified-vs-experimental.md)
