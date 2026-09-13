# AWS Aurora application-fixture recovery acceptance — 2026-08-17

This is a bounded AWS Aurora MySQL application-fixture recovery cell. It
extends the RDS control-plane path with a cluster snapshot, isolated Aurora
cluster restore, restored-writer creation, a deterministic known-content
fixture, restore-identity verification, application reads, permission and
health probes, manifest verification, and independent post-run cleanup
checks. It does not certify every AWS durable class or the full MageLift
resilience matrix.

## Scope

| Field | Value |
| --- | --- |
| Provider/account | Authenticated AWS account (identifier intentionally omitted) |
| Region | `eu-west-3` |
| Engine | `aurora-mysql` (requested version `8.0.mysql_aurora.3.08.2`) |
| Source class | `db.t3.medium` Aurora writer |
| Restore class | `db.t3.medium` Aurora writer |
| Ownership marker | `magelift/aws/aurora-recovery/codex-aws-aurora-20260817` |
| Fixture label | `fixture-rds-control-plane-codex-aws-aurora-20260817` |
| Source cluster | `magelift-aurora-codex-aws-aurora-20260817` |
| Source writer | `magelift-aurora-codex-aws-aurora-20260817-instance` |
| Manual cluster snapshot | `magelift-cluster-snapshot-9b5df85da1f04983751268bb` |
| Restore cluster | `magelift-cluster-restore-1790647961eb244669ea23c3` |
| Restore writer | `magelift-cluster-restore-1790647961eb244669ea23c3-instance` |
| DB subnet group | `magelift-rds-subnet-codex-aws-aurora-20260817` |
| Temporary security group | `magelift-rds-sg-codex-aws-aurora-20260817` (`sg-056f8cf38c401c9df`) |
| Restore duration | 539 seconds |
| Retention | 1 day |

The disposable source and restore were public only to make the local runner's
current public IPv4 address reachable. Both Aurora identities used the
temporary security group, which admitted TCP 3306 only from that `/32`. The
generated root credential was passed to the MySQL client through `MYSQL_PWD`;
it was not placed in command-line arguments or committed evidence.

## Result

PASS.

1. The source Aurora cluster became `available`, encrypted, and
   deletion-protected. Its generated writer became `available` and publicly
   reachable through the exact temporary security group.
2. The fixture store created the deterministic `magelift_recovery` database
   and one known row in `magelift_fixture_f6245c3ac69df25f`. The row contains
   the ownership marker, fixture label, and fixed payload
   `magelift-aws-rds-recovery-v1`; the recovery manifest records that table
   and one expected record.
3. The recovery cell created an encrypted manual Aurora cluster snapshot and
   verified its ownership and available state before restore.
4. The snapshot restored to an isolated Aurora cluster. The adapter waited for
   the cluster to become available, then created the restored writer with
   explicit public reachability and the exact temporary security group. The
   measured provider restore duration was 539 seconds. This follows the AWS
   restore model in which a cluster snapshot restore creates the cluster and a
   DB instance is created afterward; see the [RestoreDBClusterFromSnapshot
   API](https://docs.aws.amazon.com/AmazonRDS/latest/APIReference/API_RestoreDBClusterFromSnapshot.html),
   [Aurora restore guide](https://docs.aws.amazon.com/AmazonRDS/latest/AuroraUserGuide/aurora-restore-snapshot.html),
   and [AWS CLI restore tutorial](https://docs.aws.amazon.com/AmazonRDS/latest/AuroraUserGuide/tut-restore-cluster.CLI.html).
5. The verifier resolved the restored Aurora cluster identity before querying
   it. It confirmed the known row and payload, executed a temporary-table
   create/insert/drop permission probe with a primary key, and ran `SELECT 1`
   as the health probe. Manifest, application-read, permission, and health
   evidence all passed.
6. Recovery inventory contained the source cluster/writer, cluster snapshot,
   and isolated restore cluster/writer before teardown. The recovery cell
   deleted the restored writer before the restored cluster and then deleted
   the cluster snapshot, preserving the source cluster and its writer until
   wrapper cleanup.
7. The wrapper disabled deletion protection only after exact ownership checks,
   deleted the source writer and cluster, removed the subnet group, and
   removed the temporary security group. It exited successfully after exact
   marker cleanup.
8. Independent post-run queries returned provider not-found responses for the
   source cluster/writer, restore cluster/writer, cluster snapshot, DB subnet
   group, and temporary security group.

Observed command result, with no credential or account identifier:

```text
AWS Aurora database recovery acceptance PASS profile=default region=eu-west-3 source=aws-rds://cluster/magelift-aurora-codex-aws-aurora-20260817 backup=aws-rds://cluster-snapshot/magelift-cluster-snapshot-9b5df85da1f04983751268bb restore=aws-rds://cluster/magelift-cluster-restore-1790647961eb244669ea23c3 restoreDurationSeconds=539 retentionDays=1 cleanup=verified applicationFixture=verified manifest=verified reads=verified permissions=verified health=verified
AWS database recovery acceptance source, restore, snapshot, and subnet group were cleaned through exact ownership marker=magelift/aws/aurora-recovery/codex-aws-aurora-20260817 sourceKind=cluster
```

Independent cleanup evidence:

```text
DBClusterNotFoundFault: magelift-aurora-codex-aws-aurora-20260817
DBInstanceNotFound: magelift-aurora-codex-aws-aurora-20260817-instance
DBClusterNotFoundFault: magelift-cluster-restore-1790647961eb244669ea23c3
DBInstanceNotFound: magelift-cluster-restore-1790647961eb244669ea23c3-instance
DBClusterSnapshotNotFoundFault: magelift-cluster-snapshot-9b5df85da1f04983751268bb
DBSubnetGroupNotFoundFault: magelift-rds-subnet-codex-aws-aurora-20260817
EC2 security-group inventory: []
```

## Failed-run record and design corrections

The final PASS followed bounded failed cells. Each mutating attempt was
cleaned through its exact ownership marker; the failures are retained because
they exposed provider-specific behavior or an incorrect cluster assumption:

- The first Aurora preflight generated a password longer than Aurora's
  41-character limit. AWS rejected it before cluster creation; the wrapper now
  truncates the run identity while retaining a disposable unique credential.
- The next attempt passed deletion protection to `CreateDBInstance`. AWS
  rejected instance-level deletion protection for a cluster member and
  required protection at the cluster level. Member-level deletion protection
  was removed; the source and network resources were independently cleaned.
- The following run reached cleanup, where native cleanup tried
  `ModifyDBInstance --no-deletion-protection` on an Aurora member. AWS rejected
  that operation because deletion protection is cluster-scoped. Native cleanup
  now describes cluster membership, skips the invalid instance modification,
  deletes members before clusters, and preserves the source cluster's tagged
  members. A fake Aurora regression test covers this ordering and preservation.
- The next run completed the restore and application verification but failed
  its final assertion because a cluster source correctly appeared in
  inventory as both `aws-rds://cluster/<source>` and its writer
  `aws-rds://instance/<member>`. Cleanup verification now derives the allowed
  source-member identities from the preserved cluster instead of requiring one
  source identity. That correction produced the PASS above.

## Implementation boundary and non-claims

`cmd/aws-database-recovery-acceptance` now accepts `--source-kind cluster`,
resolves the Aurora cluster endpoint, drives the shared
`internal/certification.DatabaseRecoveryCell`, and verifies cluster-aware
cleanup. `internal/cloud/aws/resilience` translates Aurora cluster snapshots
and writer creation through the AWS SDK boundary, applies restore network
settings, and performs member-before-cluster ownership cleanup. The local
wrapper creates and owns the temporary network boundary and source lifecycle.

This proves one known-content Aurora MySQL fixture in `eu-west-3`. It does not
prove a Magento dump, Magento HTTP health, restore-in-place, cross-region or
regional DR, HA failure, fencing/failback, corruption recovery,
credential-loss recovery, interrupted-teardown recovery, production retention
variants, or an application RPO/RTO. The 539-second value is the provider
restore duration for this run, not an application RTO or RPO measurement.
AWS RDS instance, Aurora, S3, Secrets Manager, SQS, observability, edge, and
other durable or runtime classes remain separate evidence boundaries.

AWS's [Aurora cluster creation guide](https://docs.aws.amazon.com/AmazonRDS/latest/AuroraUserGuide/Aurora.CreateInstance.html)
and [Aurora encryption documentation](https://docs.aws.amazon.com/AmazonRDS/latest/AuroraUserGuide/Overview.Encryption.html)
describe the cluster/member and encryption boundaries exercised here. The
requested version was checked against AWS's [Aurora MySQL version
documentation](https://docs.aws.amazon.com/AmazonRDS/latest/AuroraUserGuide/AuroraMySQL.Updates.Versions.html)
before mutation.

Tasks 3.3, 3.5, and 8.5 remain open for broader durable-class, restore-in-place,
HA, DR, retention, and failure-scenario requirements.
