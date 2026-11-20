# Use an existing AWS VPC or RDS database

Point a MageLift environment at an existing AWS VPC, RDS MySQL instance or
both. The existing resources stay under their owner's control when you destroy
the environment. Without an attachment, MageLift creates the resources.
[Project settings](configuration.md)

## What can be attached

| Resource | Config | Required inputs |
| --- | --- | --- |
| Existing VPC | `target.aws.existing.network` | `provider: aws`, `kind: network`, `externalId` (VPC ID); plus `existing.publicSubnetIds` / `privateSubnetIds` / `dataSubnetIds` (one of each per AZ); `vpcCidr` still required for security-group rules |
| Existing RDS MySQL | `target.aws.existing.database` | `provider: aws`, `kind: database`, `externalId` (instance ID or ARN), `secretArn` (Secrets Manager master-user secret; never an inline password), `endpoint` (writer hostname) |

The preview reports `ADOPT network <id>` or `ADOPT database <id>` for each
attachment. MageLift applies the infrastructure changes using those resource IDs.

Use this array for the `target` entry in `magelift.php`.

```php
$target = [
    'provider' => 'aws',
    'runtime' => 'ecs-fargate',
    'aws' => [
        'vpcCidr' => '10.0.0.0/16',
        'availabilityZones' => ['eu-west-3a', 'eu-west-3b'],
        'encryptionKeySecretArn' => 'arn:aws:secretsmanager:eu-west-3:123456789012:secret:magento/crypt-key',
        'existing' => [
            'network' => [
                'provider' => 'aws',
                'kind' => 'network',
                'externalId' => 'vpc-0abc123',
            ],
            'publicSubnetIds' => ['subnet-pub-a', 'subnet-pub-b'],
            'privateSubnetIds' => ['subnet-priv-a', 'subnet-priv-b'],
            'dataSubnetIds' => ['subnet-data-a', 'subnet-data-b'],
            'database' => [
                'provider' => 'aws',
                'kind' => 'database',
                'externalId' => 'db-magento-prod',
                'secretArn' => 'arn:aws:secretsmanager:eu-west-3:123456789012:secret:magento/db',
                'endpoint' => 'magento.xxxxx.eu-west-3.rds.amazonaws.com',
            ],
        ],
    ],
];
```

## Connect other services

On GCP or Azure, use a database service binding to connect to an existing
MySQL-compatible endpoint. [Existing service connections](service-bindings.md)

## Limits (existing network mode)

When `target.aws.existing.network` is set, MageLift **does not** create NAT
gateways, route tables, or VPC endpoints. The operator owns egress and private
AWS service access (NAT, endpoints, peering, etc.). `natMode` applies to
greenfield networks only. Subnets you list must already support the Magento /
ECS path you need.

Adopted resources are refuse-before-mutate: destroy or replace intent against an
adopted external ID fails closed naming the resource (`MageLift does not own this
resource`). MageLift-owned children (ECS services, ALB, etc.) remain destroyable.

## Remove the environment

Run `magelift destroy --env NAME` when you are ready to remove the environment.
MageLift checks its protection settings, previews the teardown, then removes
the resources it owns. The attached VPC and RDS instance stay with their
owner. To provision a new VPC or database, create a new environment and omit
the corresponding `existing` settings.

### Verify ownership

Run the AWS stack tests to check which resources a destroy can remove.

```sh
go test ./providers/aws/stack/ -count=1 \
  -run 'Adopt|Refuse|Adopted|Detach'
```

After tearing down an attached environment, confirm the VPC and RDS instance
in the AWS console or with the AWS CLI.

```sh
aws ec2 describe-vpcs --vpc-ids VPC_ID
aws rds describe-db-instances --db-instance-identifier DB_ID
```

Maintainer free-tier confirm (2026-08-02, disposable AWS account / `eu-north-1`):
preview reported `ADOPT network` + `ADOPT database`; apply created MageLift-owned
children only; destroy removed those children; describe-after-destroy showed VPC
and RDS still `available`. Evidence:
[aws-brownfield-adopt-2026-08-02.md](evidence/aws-brownfield-adopt-2026-08-02.md).

**Note:** RDS ManageMasterUserPassword secret ARNs contain `!` (`rds!db-…`);
MageLift validators accept that form.

## Related

- [Configuration](configuration.md): `target.aws.existing.*` fields
- [Migrating from PaaS](migrating-from-paas.md): dump/media cutover vs attach
- [ADR 0009](adr/0009-dump-seed-and-attach.md): dump seed and AWS attach
- [Provider support](capability-matrix.md)
