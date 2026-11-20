# ADR 0010. Use local cloud checks

MageLift CI uses Floci for AWS, Google Cloud, and Azure APIs. Pulumi mocks
check resource arguments and dependencies. Shops can test their exact
configuration in staging. [Provider test coverage](../capability-matrix.md)

Earlier development runs grouped checks into a common deployment to control
spend. Each dated record lists the environment and what happened during the
run. [Historical runs](../evidence/README.md)
