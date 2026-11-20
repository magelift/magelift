# ADR 0009. Import a dump explicitly and attach existing services by reference

Accepted on 2026-08-22.

Creating infrastructure, loading data and deploying Magento are separate
steps. `magelift env create --dump PATH` records the dump path. The
operator runs the import after provisioning and before deploying. Importing
over an existing schema needs a separate confirmation. Keep private dumps
outside the repository and use sanitized data in previews.
[Data migration](../weekend-migrate.md)

An AWS environment can reference an existing VPC or RDS MySQL instance.
MageLift manages the new resources that use them and leaves the attached
resources with their owner during teardown. GKE and AKS can connect to
existing databases through endpoint and credential settings. The owner of
an attached service remains responsible for its backups.
[Existing AWS resources](../brownfield-attach.md) · [Service choices](../service-bindings.md)
