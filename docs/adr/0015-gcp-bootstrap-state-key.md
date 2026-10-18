# ADR 0015: GCP bootstrap prerequisites and remote state passphrase

- Status: Accepted
- Date: 2026-10-18

## Context

The test workflow needs an image destination and an encrypted Pulumi backend
before it builds or deploys. A workstation-only passphrase cannot serve a fresh
CI runner. Replacing the backend or adding a key service would duplicate the
existing GCS state and locking contracts.

## Decision

GCP bootstrap enables missing reference APIs, ensures its owned state bucket,
initializes an owned Secret Manager passphrase at immutable version 1, then
ensures an owned Docker Artifact Registry repository. These resources belong to
the user's project and remain separate from the environment's Pulumi graph.
The registry stores the user's application images; no central MageLift image
is required. IAM grants are a separate preparation stage.

Passphrase initialization uses the existing deployment lock and GCS adapter.
It checks live and noncurrent Pulumi state and backup objects before creating
a key. An absent key with existing state is refused. An interrupted empty
secret can resume, but a disabled or missing version 1 with other versions is
refused. Bootstrap does not rotate the key or migrate an existing backend.
Release of the lock has an independent bounded context and reports failures.
Ownership labels, region and registry format must match before reuse.

The GCP provider reads version 1 into the Pulumi workspace environment. The
key never enters configuration, provider RPC payloads or bootstrap output.
An explicit PULUMI_CONFIG_PASSPHRASE or PULUMI_CONFIG_PASSPHRASE_FILE remains
available for existing stacks and recovery. Backend URL and passphrase are
passed together through the existing Automation API environment option.

## Consequences

Bootstrap output contains resource references and the application destination.
The operator needs API enablement, bucket, registry and secret creation rights.
If Service Usage itself is unavailable, preparation reports the prerequisite
rather than assuming it can enable other services. Existing unrelated resource
settings and IAM policies are preserved. The state key must be retained as long
as encrypted state or its backups are needed; environment destruction does not
authorize deleting it.

Local fixtures verify requests, archived-generation detection, locking and
workspace environment composition. Real GCP permissions, state decryption and
GitHub OIDC remain acceptance checks.

## Alternatives

A checked-in passphrase or service-account key would expose credentials. A new
KMS keyring would add a persistent resource and another recovery path. Keeping
the existing Pulumi passphrase mode with a remote immutable secret avoids both.

## Provenance

Original implementation using the public GCS, Secret Manager, Artifact Registry
and Pulumi Automation API contracts.
