# ADR 0016: Owned GKE node identity

- Status: Accepted
- Date: 2026-10-18
- Supersedes: the node identity and base-bootstrap IAM details in ADRs 0007 and 0015.

## Context

A project's default Compute service account can have privileges unrelated to
MageLift. Using it for GKE nodes also makes registry access depend on shared IAM.

## Decision

Base GCP bootstrap prepares a deterministic, environment-owned node service
account after checking ownership of its Artifact Registry repository. It grants
repository reader and the project-level roles/container.defaultNodeServiceAccount
role. This happens without GitHub options and requires project IAM management
permission. It does not change the default Compute account's bindings.

New Autopilot clusters and Standard node pools explicitly select the owned
account and cloud-platform OAuth scope. CI deployment receives actAs on that
account through the existing reviewed CI permission plan. CI deployment roles
still require --apply-ci-permissions. No YAML key or provider RPC is added.

Ownership mismatch is refused before grants. Policy merges preserve unrelated
members, conditions and audit settings, use etags, and retry a conflict once.

## Consequences

The node account is a bootstrap prerequisite retained after environment destroy.
Cleanup must remove only this run's grants before deleting the account. Cluster
and node-pool identity fields are creation-only: Pulumi ignores subsequent
identity changes to avoid replacing an existing cluster implicitly. Use a fresh
environment to migrate an older default-account deployment.

Local API fixtures and Pulumi mocks cover ownership, grants, ordering, explicit
identity and replacement guards. Live Autopilot creation and legacy migration
remain acceptance checks; mocks do not establish Google API acceptance.

## Alternatives

Reusing the default account would inherit shared privileges. A new configuration
key would burden the guided path with an identity choice it can derive safely.

## Provenance

Original implementation using public Google IAM, Artifact Registry and GKE
NodeConfig APIs and the existing Pulumi provider contract.
