# Free-tier AWS VPC+RDS adopt confirm

**Recorded:** 2026-08-02T14:31:25Z  
**Result:** **PASS** (spend map pass 3/3)

Account and resource IDs below are **placeholders**. Real maintainer account
numbers, VPC/NAT/RDS identifiers, and hostnames are retained offline only.

## Identity

```bash
aws sts get-caller-identity
# Account: <redacted free-tier AWS account> / region eu-north-1
```

## Pre-existing resources (created outside MageLift)

| Resource | ID (placeholder) |
| --- | --- |
| VPC | `vpc-0EXAMPLE` (`10.55.0.0/16`) |
| Public / private / data subnets | 2 AZ × 3 tiers |
| NAT Gateway | `nat-0EXAMPLE` (operator-owned egress) |
| RDS MySQL `db.t4g.micro` | operator-owned instance + ManageMasterUserPassword secret |

## MageLift adopt

Config: local `existing.network` + `existing.database` (gitignored).

### Preview — ADOPT lines

```json
"adopted": [
  "ADOPT network vpc-0EXAMPLE",
  "ADOPT database <operator-rds>"
]
```

### Apply

```bash
GOMAXPROCS=1 magelift --config <adopt-config> --env preview deploy --infra-only
```

- Duration ~10m48s; **+74 created**
- Outputs: adopted VPC id + RDS writer endpoint (reference-without-own)
- Pulumi URNs: **no** `aws:ec2/vpc:Vpc`, `aws:rds/instance`, or NAT Gateway

### Destroy MageLift stack only

```bash
GOMAXPROCS=1 magelift --config <adopt-config> --env preview destroy --yes
```

- **-74 deleted** (~9m15s); ADOPT lines still reported

### Describe-after-destroy (adopted intact)

VPC and RDS remained `available` after MageLift destroy (IDs redacted).

## Code fix required for live RDS secrets

AWS ManageMasterUserPassword secrets use names like `rds!db-…`. Validators in
`internal/cloud/aws/stack/spec.go` and `internal/cloud/aws/database/database.go`
now allow `!` in the secret name. Test:
`TestSpecValidateAcceptsRDSManagedMasterUserSecretARN`.

## Cleanup (mandatory)

After evidence: deleted RDS, NAT+EIP, subnets, route tables, IGW, VPC, and
Pulumi stacks used for the confirm. Post-check: VPC/RDS NotFound.
