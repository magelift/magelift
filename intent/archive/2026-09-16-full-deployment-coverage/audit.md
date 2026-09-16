> HISTORICAL INVENTORY 2026-09-16: `intent/audit.md` F13 keeps this file as
> useful inventory but corrects some conclusions — secret references do not
> imply manual creation, domain ownership does not make every DNS operation
> manual, managed KMS is not inherently anti-automation; billing, domain
> ownership, licenses, and some email approvals stay honestly human. The
> three-vendor email scope is superseded by the rewritten draft (one validated
> SMTP path for alpha). Retained; do not treat vendor conclusions as approved.

# Deployment-path audit (order 22, box 1.1 — 2026-09-16) (HISTORICAL INVENTORY)

Every manual item on the AWS deployment path plus the email-relevant EU
surface, each with file:line evidence and exactly one disposition:
deliberate-BYO/manual (with deciding doc + operator procedure) or gap-to-close
(covered in this intent, or a follow-up stub). Zero untriaged items.

Conventions: `internal/config/model.go` line numbers are pre-change (config
edits in box 2.1 shift them). All greps re-runnable from the repo root.

## A. Email (this intent's scope)

### A1. SES identity, DKIM, SMTP credentials — GAP-TO-CLOSE HERE (box 3.1)

- `internal/config/model.go:106-112`: `EmailConfig` carries mode/host/port/
  username/from plus a credential *reference*; no identity, domain, or
  provisioning fields.
- `internal/config/config.go:942-944`: `ses` validation requires
  operator-supplied values only.
- `grep sesv2 internal/cloud/`: zero SES resources anywhere under
  `internal/cloud/` (only prose/test substring noise).
- `docs/capability-matrix.md` SES row: "configuration-only until
  Magento-origin delivery is evidenced."
- Pulumi coverage exists (aws/sdk v7.46.0 `sesv2/emailIdentity.go` + IAM for
  SMTP creds), so this gap closes here, not via stub.

### A2. Scaleway TEM domain, validation, credentials — GAP-TO-CLOSE HERE (box 3.2)

- No `tem` mode exists (`EmailConfig` enum at `model.go:107` lists only
  `disabled|smtp|ses`); no `internal/cloud/scaleway/tem/` package.
- Provider `pulumiverse/pulumi-scaleway/sdk v1.55.1` (pinned in `go.mod`)
  ships `tem/domain.go` (`Domain`), `tem/domainValidation.go`
  (`DomainValidation`), and `iamApiKey.go` (TEM SMTP uses API key pairs) —
  verified in the module cache. Coverage confirmed, closes here.

### A3. OVH mailbox account — GAP-TO-CLOSE HERE (box 3.3)

- No OVH email mode or `internal/cloud/ovh/email/` package.
- Provider `ovh/pulumi-ovh/sdk/v2 v2.19.1` (pinned) ships
  `EmailDomainAccount` (+ getters) and NO domain-service resource (tree grep)
  — accounts-only confirmed, closes here on that shape.

### A4. Cloudflare Email Sending — DELIBERATE-MANUAL

- Pulumi provider v6.20.0 tree contains zero `*sending*` paths (only
  `email_routing_*` inbound-forwarding and `email_security_*` resources —
  verified via the `pulumi/pulumi-cloudflare` repo tree at tag v6.20.0).
  The newer TF-side `email_sending_subdomain` service postdates what v6.20.0
  wraps. API-only hand-rolling rejected per spec (no backend bypasses).
- Procedure: `smtp` mode + the Cloudflare relay pointers in
  `docs/operations.md`. Revisit when a stable Pulumi sending resource ships.

### A5. SendGrid — DELIBERATE-MANUAL

- No maintained Pulumi package (single-maintainer alpha disqualifies); the
  `sendgrid` mode was already removed for this reason (`grep -ri sendgrid
  go.mod internal/config/model.go` returns empty).
- Procedure: `smtp` mode pointed at `smtp.sendgrid.net:587` with username
  `apikey` (capability-matrix SendGrid row).

### A6. Generic `smtp` mode — DELIBERATE-BYO

- No single service exists to manage; the mode IS the bring-your-relay seam.
- Deciding doc: `EmailConfig` model comment ("Magento SMTP on a cloud
  environment", refs-only) + operations relay table.
- Procedure: `docs/operations.md` provider relay endpoints (OVH port 465/587,
  TEM `smtp.tem.scaleway.com:587`, SendGrid) + `magelift secret set
  --value-stdin` for the credential (`docs/operations.md:92-95`).

### A7. GCP email — DELIBERATE-N/A

- No native email service exists on GCP (out of scope per intent). Covered by
  the `smtp` recipes like every non-email-native target.

### A8. SES sandbox exit — DELIBERATE-MANUAL

- AWS Support human step; no adapter can close it. Surfaced pre-deploy in
  this intent's docs (box 4.1), never discovered at first send.

### A9. OVH MX Plan service purchase — DELIBERATE-BYO

- The service (domain hosting + quota) is purchased/out-of-band; only
  accounts on it are API-managed (A3). Operator supplies the domain.

### A10. TEM tier upgrades — DELIBERATE-MANUAL

- Essential (pay-as-you-go) is the managed default; Scale upgrades are a
  billing human step. Limits recorded in validation errors + matrix (box 4.1).

## B. Secrets and credentials — DELIBERATE-BYO (core design)

### B1. All credential references

- Composer credentials (`model.go:151`), email credential (`model.go:112`),
  Fastly token (`model.go:170`), encryption-key secrets (`model.go:315,355,
  390`), cache/session/queue secret ARNs (`model.go:442-444`), Magento
  encryption-key secret ARN (`model.go:445`), brownfield master secret ARN
  (`model.go:481`).
- Deciding docs: `AGENTS.md` Never rule (secret values never in
  YAML/logs/evidence) + `EmailConfig` model comment ("secret references,
  never plaintext YAML").
- Procedure: `magelift secret set <name> --value-stdin` (never prints),
  `magelift secret list` (names + ARNs), `magelift secret remove --yes`
  (`docs/operations.md:92-95`); acceptance runs pass exact ARNs per
  `docs/aws-acceptance.md:156-166`.

## C. AWS attachment points

### C1. Route53 hosted zone — DELIBERATE-BYO

- `HostedZoneID` input (`model.go:437`); adopted via
  `sdk.ExistingResourceRef`, never created
  (`internal/cloud/aws/stack/config.go:95`). Domain ownership is inherently
  operator-side; adapters (including SES DKIM in this intent) only add
  records into it.
- Procedure: acceptance prerequisites + domain docs; records managed per
  adapter.

### C2. ACM certificates (ALB + CloudFront) — GAP-TO-CLOSE via STUB

- `ALBCertificateARN` / `CloudFrontCertificateARN` inputs (`model.go:438-439`),
  adopted via `ExistingResourceRef` (`stack/config.go:96-97`); no `acm.NewCertificate`
  anywhere (`grep acm.NewCertificate internal/cloud/aws/` empty).
- Automatable in principle (Pulumi Certificate + DNS validation records into
  the supplied zone) but needs validation-flow + cross-account design this
  intent does not own.
- Stub: `acm-managed-certificates` — provision ALB/CloudFront ACM certs with
  DNS validation into the supplied hosted zone.

### C3. Customer KMS key — DELIBERATE-BYO

- `KMSKeyARN` input (`model.go:436`); referenced for bucket encryption
  (`internal/cloud/aws/observability/component.go:327`), never created.
  Customer-managed keys are compliance artifacts — operator-owned by nature.
- Procedure: acceptance prerequisites (`target.aws.encryptionKeySecretArn`
  pattern in `docs/aws-acceptance.md:146-147`).

### C4. SNS notification topic — GAP-TO-CLOSE via STUB

- `SNSTopicARN` input (`model.go:440`), threaded to
  `NotificationTopicARN` (`internal/cloud/aws/stack/component.go:264`); no
  `sns.NewTopic` anywhere. No BYO justification (a notification topic has no
  compliance or ownership reason to pre-exist); needs ownership design.
- Stub: `managed-notify-topic` — provision the SNS notification topic instead
  of requiring its ARN.

### C5. S3 access-log bucket (acceptance) — DELIBERATE-BYO

- Acceptance-only prerequisite ("an existing S3 access-log bucket in the
  target region", `docs/aws-acceptance.md:145`). Product buckets ARE managed
  (`s3.NewBucket` in `internal/cloud/aws/storage/component.go:63` and
  `observability/component.go:311`).
- Procedure: acceptance prerequisites doc.

### C6. Brownfield RDS adopt — DELIBERATE-BYO

- `ExternalID` + `SecretARN` (`model.go:480-481`); adopted resources are never
  mutated into managed ones (spec gotcha).
- Procedure: `docs/brownfield-attach.md`.

### C7. Production DNS cutover — DELIBERATE-MANUAL

- Operator DNS change by nature (registrar + zone ownership).
- Procedure: `scripts/acceptance/lib-cloudflare-dns.sh` +
  `scripts/cutover-dns-cloudflare.sh` + acceptance DNS docs.

### C8. Image digest + Cosign identity (acceptance inputs) — DELIBERATE-BYO

- Operator build artifacts (`docs/aws-acceptance.md:148-150`).
- Procedure: `magelift build --push` + `magelift sign` (`docs/builds.md`).

## D. Non-email EU gaps — CHECKED, none additional

### D1. OVH/Scaleway beyond email — no further deployment-path BYO found

- Target structs carry only account selectors (`ServiceName`, `ProjectID`),
  network inputs, secret refs, and image digests (`model.go:304-358`).
- Adapter data reads are runtime API clients (OVH observability
  subscriptions, K8s service reads), not deployment inputs — verified by
  `Lookup|Get*` grep over `internal/cloud/ovh/` + `internal/cloud/scaleway/`.
- No stub. Email (A2/A3) is the only EU deployment-path gap this audit found.

## E. Already managed (checked, not manual — no triage needed)

- Product S3 buckets, VPC/network/database/cache/search/queue/runtime stacks,
  CloudFront distribution + WAF (from supplied certs/zone), GCP WIF + CI
  service account via `magelift bootstrap` (`docs/operations.md:98-100`),
  AWS bootstrap IAM roles (`internal/cloud/aws/bootstrap/policy.go`).
- Preview email stays `disabled` (deliberate default: per-preview SES
  identities on ephemeral domains would churn verification + sandbox
  against zero benefit; production-class only per spec).

## Tally

21 items: 3 covered-here (A1/A2/A3), 2 follow-up stubs
(`acm-managed-certificates`, `managed-notify-topic`), 16 deliberate
(BYO/manual/N/A/managed-default). Zero untriaged. Zero orphans.
