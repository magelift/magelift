# ADR 0015. Keep the GCP state passphrase in Secret Manager

Accepted on 2026-10-18.

A shop needs the same state and encryption key on a workstation and a fresh
CI runner. GCP bootstrap prepares the required APIs, a versioned GCS bucket,
a Secret Manager passphrase and an Artifact Registry repository in the shop's
project. Bootstrap owns those resources outside the Magento environment.
[Cloud setup](../bootstrap.md)

Bootstrap acquires the environment lock and checks live and noncurrent state
and backup objects before creating version 1 of the passphrase. It can resume
an interrupted initialization with an empty secret if there is no state.
Existing state requires the original key. A disabled version 1, or a missing
version 1 alongside other versions, needs operator review. Bootstrap checks
ownership and location before reusing a bucket or registry.

The provider reads version 1 into the Pulumi workspace without returning
the value in configuration, RPC or command output. For an existing stack or
recovery, an explicit `PULUMI_CONFIG_PASSPHRASE` or
`PULUMI_CONFIG_PASSPHRASE_FILE` takes precedence. Keep the original key as
long as encrypted state or backups are needed.

Local fixtures check creation, ownership, locking, archived state detection
and workspace configuration. A real project is needed to verify IAM and
decryption with the shop's state.
