# 08-06 free-tier AWS VPC+RDS adopt confirm (D-04 paid cell)

**Recorded:** 2026-07-30T12:24:50Z  
**Result:** HUMAN_GATE — blocked on ADC / AWS session (no live PASS invented)

## Identity check

```bash
aws sts get-caller-identity
```

**Exit code:** `255`

**Stderr (verbatim):**

```text
aws: [ERROR]: Your session has expired. Please reauthenticate using 'aws login'.
aws: [ERROR]: Your session has expired. Please reauthenticate using 'aws login'.
```

## Paid confirm status

| Step | Status |
| --- | --- |
| `aws sts get-caller-identity` | FAIL — session expired |
| Adopt free-tier VPC + RDS via `existing.network` + `existing.database` | NOT RUN |
| Preview ADOPT lines | NOT RUN |
| Apply + destroy MageLift stack | NOT RUN |
| `aws ec2 describe-vpcs` / `aws rds describe-db-instances` after destroy | NOT RUN |

## Honesty (D-04 / T-08-17)

Offline adopt evidence is recorded in `scratch/08-06-offline-evidence.md` (mocks PASS). This file does **not** claim live AWS adopt or describe-after-destroy. Spend map pass 3/3 remains open until ADC works.

## Resume when ADC available

1. `aws login` / `aws sso login` (or refresh ADC) until `aws sts get-caller-identity` succeeds
2. Adopt pre-existing free-tier VPC + RDS MySQL; preview → apply → destroy MageLift stack only
3. Confirm adopted VPC/RDS still exist via describe APIs
4. Replace this HUMAN_GATE note with commands + outputs (PASS) — serial only; abort if swap climbs (AGENTS.md)
