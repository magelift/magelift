# Brownfield attach (existing VPC + RDS)

Bring an existing AWS VPC and/or RDS MySQL instance into a MageLift stack
**without** MageLift owning destroy. Greenfield (MageLift creates network + DB)
remains the default. Field reference: [configuration.md](configuration.md).

Phase 8 delivers AWS attach (ATTACH-01..04). There is no `magelift detach` CLI —
detach is a documented manual un-adopt.

## What can be attached

| Resource | Config | Required inputs |
| --- | --- | --- |
| Existing VPC | `target.aws.existing.network` | `provider: aws`, `kind: network`, `externalId` (VPC ID); plus `existing.publicSubnetIds` / `privateSubnetIds` / `dataSubnetIds` (one of each per AZ); `vpcCidr` still required for security-group rules |
| Existing RDS MySQL | `target.aws.existing.database` | `provider: aws`, `kind: database`, `externalId` (instance ID or ARN), `secretArn` (Secrets Manager master-user secret — never an inline password), `endpoint` (writer hostname) |

Preview reports `ADOPT network <id>` / `ADOPT database <id>` for each ref (not
CREATE). Apply adopts by reference: MageLift does not import the VPC/RDS into
Pulumi state for destroy ownership.

Example shape (illustrative):

```yaml
target:
  provider: aws
  runtime: ecs-fargate
  aws:
    vpcCidr: "10.0.0.0/16"
    availabilityZones: [eu-west-3a, eu-west-3b]
    encryptionKeySecretArn: aws-secrets-manager://…
    existing:
      network:
        provider: aws
        kind: network
        externalId: vpc-0abc123
      publicSubnetIds: [subnet-pub-a, subnet-pub-b]
      privateSubnetIds: [subnet-priv-a, subnet-priv-b]
      dataSubnetIds: [subnet-data-a, subnet-data-b]
      database:
        provider: aws
        kind: database
        externalId: db-magento-prod
        secretArn: arn:aws:secretsmanager:eu-west-3:123456789012:secret:magento/db
        endpoint: magento.xxxxx.eu-west-3.rds.amazonaws.com
```

## What cannot be attached (this milestone)

- **Cloud SQL** (or any GCP managed DB) — not a configuration key; GCP GKE
  Autopilot is certified for greenfield Magento deploy, but brownfield DB
  adopt remains AWS-only this milestone.
- **Non-AWS VPC / DB** — OVH, Scaleway, and multi-cloud attach are out of scope.
- **Import into Pulumi state for destroy ownership** — MageLift references
  existing IDs; it does not claim the cloud resource so that `destroy` would
  delete it. Do not expect a Pulumi `import` that transfers lifecycle ownership.

## Limits (existing network mode)

When `target.aws.existing.network` is set, MageLift **does not** create NAT
gateways, route tables, or VPC endpoints. The operator owns egress and private
AWS service access (NAT, endpoints, peering, etc.). `natMode` applies to
greenfield networks only. Subnets you list must already support the Magento /
ECS path you need.

Adopted resources are refuse-before-mutate: destroy or replace intent against an
adopted external ID fails closed naming the resource (`MageLift does not own this
resource`). MageLift-owned children (ECS services, ALB, etc.) remain destroyable.

## Detach path (manual un-adopt)

There is no detach command. To leave cloud VPC/RDS intact while removing MageLift:

1. **Remove MageLift-owned stack only** — `magelift env destroy <env> --yes` (or
   equivalent stack destroy) after protection is off. Adopted VPC/RDS were never
   in Pulumi state, so destroy targets MageLift-owned children only.
2. **Or start a new stack without `existing.*`** — drop the `existing.network` /
   `existing.database` blocks (and related subnet ID lists) from a fresh
   environment config if you no longer want attach mode.
3. **Do not** attempt to destroy the VPC/RDS through MageLift — refuse will fail
   closed if that intent is expressed against adopted refs.

### Offline proof (ATTACH-04)

Reference-without-own is asserted in
`internal/cloud/aws/stack/adopt_test.go`:

- `TestAdoptedDetachDestroyRecordsNoDeleteForAdoptedIDs` — mock destroy of
  MageLift-owned children never emits Delete for adopted VPC/RDS external IDs;
  empty intent (owned-child detach) is allowed; explicit destroy intent is
  refused.
- Related: `TestRefuseAdoptedMutation*` / `TestAdoptedRefuse*` name every
  adopted external ID on mutate.

```sh
GOMAXPROCS=1 GOFLAGS=-p=1 go test ./internal/cloud/aws/stack/ -count=1 \
  -run 'Adopt|Refuse|Adopted|Detach'
```

### Live confirm

After a live destroy of an attach stack, operators can confirm the VPC and RDS
still exist:

```sh
aws ec2 describe-vpcs --vpc-ids <vpc-id>
aws rds describe-db-instances --db-instance-identifier <db-id>
```

Maintainer free-tier confirm (2026-08-02, account `669890779205` / `eu-north-1`):
preview reported `ADOPT network` + `ADOPT database`; apply created MageLift-owned
children only; destroy removed those children; describe-after-destroy showed VPC
and RDS still `available`. Evidence:
[aws-brownfield-adopt-2026-08-02.md](evidence/aws-brownfield-adopt-2026-08-02.md).

**Note:** RDS ManageMasterUserPassword secret ARNs contain `!` (`rds!db-…`);
MageLift validators accept that form.

## Related

- [Configuration](configuration.md) — `target.aws.existing.*` fields
- [Migrating from PaaS](migrating-from-paas.md) — dump/media cutover vs attach
- [ADR 0010](adr/0010-database-dump-seed.md) — dump seed; attach supersession note
- [Post-beta roadmap](post-beta-roadmap.md) — remaining post-beta tracks
