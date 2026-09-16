# Log

## 2026-09-15

- Anonymized employer-linked repo slugs to "prior shop" in two notes, with history-preserving rename.
- Scrubbed the employer name from the tree (docs, knowledge note, new intent, one archive line) and renamed the Terraform OpenSearch source note to `prior-terraform-opensearch.md`; rule codified in `AGENTS.md`. Git history still contains the name.
- Removed the `sendgrid` email mode across the tree (no maintained Pulumi package): updated [Cloud email uses secret references not credentialEnv](lessons/Cloud%20email%20uses%20secret%20references%20not%20credentialEnv.md) (SES-only) and [Local-gates is the account-free emulator stack](lessons/Local-gates%20is%20the%20account-free%20emulator%20stack.md) (vendor list).
- Migrated `tasks/` into the bundle, then deleted the folder. Only `tasks/findings.md` was tracked; the rest was `/tasks/`-ignored scratch.
- Added [OVH orphan teardown needs API sweep and gateway-first delete](lessons/OVH%20orphan%20teardown%20needs%20API%20sweep%20and%20gateway-first%20delete.md): CLI has no private-network list (sweep via API), gateway-first delete with cascade, vlan-0 linger, 1-per-region gateway quota, Gen-3-only MKS flavors.
- Added [AWS live acceptance traps](lessons/AWS%20live%20acceptance%20traps.md): no bootstrap before harness, hex cache secrets, VPC OpenSearch VpcId/SLR, https:// native search hostname, amazon-mq instanceType, secrets re-verify after every run. Merged per the two-new-files cap.
- Updated CloudFront CachingDisabled (stack KMS key needs explicit CloudWatch Logs statement), EXIT traps (never interrupt live runs), AWS failed resume (manual teardown order), KEEP run ID (passphrase isolation), AMQP user key (CONFIG__DEFAULT__ search bindings), Greenfield deploy (--infra-only for live spec updates).
- Skipped as session diary or code-SSOT: CI run IDs, dialproof tags/digests, STOP-SAFE state, todo.md, search-proof procedure, pricing filters and budget page size (fixed in code), allow-expired validator selection (fixed in code), GoReleaser artifacts.json (workflow comments), Scaleway password minima (mock-graph test pins), seed recipe, day2 exec fix (ADR 0011).

## 2026-08-24

- Added [EKS Auto Mode NLB defaults to internal](lessons/EKS%20Auto%20Mode%20NLB%20defaults%20to%20internal.md): Auto Mode LoadBalancer Services are internal unless `aws-load-balancer-scheme` is `internet-facing`.

## 2026-08-22

- Relocated the bundle from `docs/knowledge/` to `.agents/knowledge/`.
- Deleted session recaps, compile-miss notes, OpenSpec process notes, and near-duplicate titles. Remaining notes are unique Magento or provider traps.
- Public evidence is now a short claim table under `docs/evidence/README.md`, not a KEEP ledger.
