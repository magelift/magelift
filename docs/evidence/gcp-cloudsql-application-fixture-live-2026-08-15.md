# GCP Cloud SQL application fixture recovery acceptance, 2026-08-15

This is a bounded Google Cloud SQL MySQL 8.4 recovery cell. It adds a known
database row and application-level verification to the earlier Cloud SQL
control-plane cells. It does not certify every GCP durable class or the full
MageLift resilience matrix.

## Cell

- Provider: Google Cloud SQL Admin API
- Project: redacted in committed evidence
- Region: `europe-west1`
- Engine: MySQL `8.4`
- Edition and capacity: Enterprise, `db-f1-micro`, zonal, 10 GB SSD
- Source: one disposable ownership-labeled instance
- Recovery: one provider on-demand backup and one isolated restore in the
  same project and region
- Fixture: one row in the deterministic `magelift_recovery` database table
- Runner: `make gcp-cloudsql-acceptance-local`
- Restore policy: same-region isolated destination, one-day requested
  retention, protected provider `ON_DEMAND` backup

The wrapper generated the root credential for this run and authorized only the
runner's current public IPv4 address for the disposable source. The credential
was passed to the MySQL client through `MYSQL_PWD`; it was not placed in the
MySQL or Docker argument list and was not written to evidence. The public IP
and root credential are test-cell choices, not a production connectivity
recommendation.

## Result

PASS.

1. The source became `RUNNABLE`, and the ownership and fixture labels were
   read back from Cloud SQL before the fixture was seeded.
2. The fixture store created the recovery database and a deterministic table,
   then inserted one known row containing the run marker, fixture ID, and
   fixed payload. Re-running the seed operation accepts the same row and
   rejects unexpected content.
3. The recovery cell created an on-demand backup and verified its provider
   identity, successful state, type, protection, and retention evidence.
4. The backup restored into an isolated instance. The restore took 937 seconds
   to reach the provider's usable state.
5. The verifier resolved the restored resource identity before reading the
   fixture. It checked the expected row and count, ran a create/insert/drop
   permission probe, and ran `SELECT 1` as a health probe. The manifest,
   application read, permissions, and health proofs all passed.
6. The recovery inventory contained the source instance, restore instance,
   and backup before teardown.
7. The command deleted the provider backup and source fixture. The shell
   cleanup then deleted only instances carrying the exact run marker.
8. Independent post-run queries returned no matching `magelift-csql-*` or
   `magelift-recovery-*` instances and no matching `magelift-recovery:` backup
   descriptions.

Observed output, with project and resource identities redacted:

```text
gcp Cloud SQL acceptance PASS project=<redacted>
source=<redacted>
restore=gcp-cloud-sql://projects/<redacted>/instances/<redacted>
backup=gcp-cloud-sql-backup://projects/<redacted>/backups/<redacted>
restoreDurationSeconds=937 retentionDays=1 backupCleanup=verified
applicationFixture=verified manifest=verified reads=verified
permissions=verified health=verified instances=2

post-run inventory: markerOwnedInstances=0 recoveryBackups=0
```

## Implementation boundary

`cmd/gcp-cloudsql-acceptance` now drives the shared
`internal/certification.DatabaseRecoveryCell` with a Cloud SQL fixture store
and a verifier. The verifier receives the restore resource URI from the
recovery cell, so it does not accidentally read the source after an isolated
restore. The provider adapter still owns backup creation and backup deletion;
the shell wrapper owns exact instance cleanup because the Cloud SQL adapter
does not expose an instance-delete operation through the generic database
recovery contract.

The provider documents that a restore to a new MySQL instance carries the
source instance's databases and users. The test therefore uses the restored
instance's recovered root credential rather than adding a password-reset step.
See [Cloud SQL restore guidance](https://cloud.google.com/sql/docs/mysql/backup-recovery/restoring).

## Boundary and non-claims

- This proves one known-content database fixture on Cloud SQL MySQL 8.4. It
  does not prove a Magento dump, Magento HTTP health, or a full application
  deployment.
- The measured 937 seconds is provider restore duration for this run. It is
  not an application RTO or an application RPO measurement.
- Restore-in-place, cross-region recovery, alternate-project recovery,
  regional DR, HA failure, fencing, failback, split-brain prevention,
  corruption recovery, credential-loss recovery, and interrupted-teardown
  recovery remain untested.
- GCS, Secret Manager, Pub/Sub, Memorystore, GKE, search, and other GCP
  durable or runtime classes remain separate evidence boundaries.
- AWS, Scaleway, and OVHcloud recovery claims remain independent. This cell
  advances, but does not close, OpenSpec tasks 3.3, 3.5, or 8.5.
