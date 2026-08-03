# Free-tier AWS VPC+RDS adopt confirm

**Recorded:** 2026-08-02T14:31:25Z  
**Result:** **PASS** (spend map pass 3/3)

## Identity

```bash
aws sts get-caller-identity
# AWS: 669890779205 arn:aws:iam::669890779205:root
```

Account `669890779205` / region `eu-north-1`.

## Pre-existing resources (created outside MageLift)

| Resource | ID |
| --- | --- |
| VPC | `vpc-087507834e37773c7` (`10.55.0.0/16`) |
| Public / private / data subnets | 2 AZ × 3 tiers (see `08-06-adopt-ids.env`) |
| NAT Gateway | `nat-077388a59b03a02dc` (operator-owned egress) |
| RDS MySQL `db.t4g.micro` | `mladopt-mysql` + ManageMasterUserPassword secret |

## MageLift adopt

Config: `.magelift/adopt-confirm.magelift.yaml` (`existing.network` + `existing.database`).

Binary: `/tmp/magelift-adopt` (serial rebuild; includes RDS managed secret ARN `!` fix).

### Preview — ADOPT lines

```json
"adopted": [
  "ADOPT network vpc-087507834e37773c7",
  "ADOPT database mladopt-mysql"
]
```

### Apply

```bash
GOMAXPROCS=1 magelift --config .magelift/adopt-confirm.magelift.yaml \
  --env preview deploy --infra-only
```

- Duration ~10m48s; **+74 created**
- Outputs: `networkVpcId=vpc-087507834e37773c7`, `databaseWriter=mladopt-mysql.…rds.amazonaws.com`
- Pulumi URNs: **no** `aws:ec2/vpc:Vpc`, `aws:rds/instance`, or NAT Gateway (reference-without-own)

### Destroy MageLift stack only

```bash
GOMAXPROCS=1 magelift --config … --env preview destroy --yes
```

- **-74 deleted** (~9m15s); ADOPT lines still reported

### Describe-after-destroy (adopted intact)

```text
vpc-087507834e37773c7	available	10.55.0.0/16
mladopt-mysql	available	mladopt-mysql.cdwockum4edo.eu-north-1.rds.amazonaws.com
```

## Code fix required for live RDS secrets

AWS ManageMasterUserPassword secrets use names like `rds!db-…`. Validators in
`internal/cloud/aws/stack/spec.go` and `internal/cloud/aws/database/database.go`
now allow `!` in the secret name. Test:
`TestSpecValidateAcceptsRDSManagedMasterUserSecretARN`.

## Cleanup (mandatory)

After evidence: deleted RDS, NAT+EIP, subnets, route tables, IGW, VPC, Pulumi
stacks `acceptance-preview-aws-ecs-fargate` and empty `adoptconfirm-…`.
Post-check: VPC/RDS NotFound.
