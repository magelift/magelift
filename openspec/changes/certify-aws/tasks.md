## 1. Tracker split

- [x] 1.1 Add `certification-aws` to `openspec/README.md` capabilities. Point `audit-catalog-cert-campaign` task 6.3 at this change without rewriting the in-flight KEEP catalog file. Verify `openspec validate --change certify-aws --strict`.
- [x] 1.2 State in `docs/capability-matrix.md` that shared certification rules live in `certification-matrix` and AWS cells live here. Verify the page names the certified Fargate subset and lists experimental families.

## 2. Implemented AWS catalog

- [x] 2.1 Enumerate Fargate compute modes, EKS compute modes, `databaseEngine`, `searchMode`, `queueMode`, NAT, web runtime, edge, and observability with Adobe / MageLift / certified-or-experimental-or-unavailable columns. Verify every schema enum in `AWSCatalog` / `AWSCatalogFargate` / `AWSCatalogEKS` appears or is typed unavailable.
- [x] 2.2 Document Magento-on-AWS wiring as MageLift product contracts (writer endpoint + secret JSON; provisioned OpenSearch in-VPC HTTPS Magento can query; AOSS sidecar). Private sibling shops are examples only and MUST NOT appear as certification evidence. Verify `docs/operations.md` / `docs/architecture.md` match adapter tests.
- [x] 2.3 Keep Aurora 3.11/3.12 as the Adobe-gated pin; name Aurora 8.4 as one private-shop example, not a silent product default or certified pin. Verify admission tests still reject unaimed 8.4.

## 3. Certified subset honesty

- [x] 3.1 Confirm `CertificationTier` still keys off evidence tuples so MI, EKS, Aurora, Amazon MQ, provisioned search, and HA cannot inherit Fargate preview certified. Verify `GOMAXPROCS=1 GOFLAGS=-p=1 go test ./internal/config/ ./internal/cloud/aws/stack/ ./internal/platform/ -count=1`.
- [x] 3.2 After this change is applied, run packed AWS KEEP cells under this tracker (not Cartesian shops): remaining Fargate catalog, then cold MI, then cold EKS, then destroy + orphan assert. Verify each promoted row has Magento (or declared infra-only) evidence for that tuple. Do not mark experimental PASS as certified. Fargate packed catalog `awsba` 9/9 PASS then destroy + `assert_clean ok` 2026-08-23 ([awsba](../../../docs/evidence/aws-ecs-fargate-packed-keep-awsba-20260823.md)). Cold Managed Instances `awsmi` 2/2 PASS then destroy + `assert_clean ok` 2026-08-23 ([awsmi](../../../docs/evidence/aws-ecs-managed-instances-packed-keep-awsmi-20260823.md)). Cold EKS Auto Mode `awsek` 4/4 PASS (Magento HTTP 200 + RabbitMQ) then destroy + `assert_clean ok` 2026-08-23 ([awsek](../../../docs/evidence/aws-eks-auto-mode-packed-keep-awsek-20260823.md)).

## 4. Docs lockstep

- [x] 4.1 Update `docs/aws-acceptance.md` and evidence README links to `certification-aws`. Humanizer then remove-ai-marks on edited human pages. Verify `make docs` if those pages are in the MkDocs nav.
