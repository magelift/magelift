## 1. Plugin-first core and nginx WebRuntime

- [x] 1.1 Add public `WebRuntime` descriptor/registry (ID, independent version, Adobe status per Magento release, MageLift tier, Compose fragment, cloud image/health) without stack `CoreOutputKeys`. Verify in-process nginx-fpm tests and unknown-ID reject before mutate.
- [x] 1.2 Keep `cmd/magelift` and `internal/cli` free of Pulumi and of `internal/cloud/<provider>/` client constructors; commands go through `sdk/v1` ports. Verify cleanup/leftover tests use a fake module, not a live GCP client in Cobra.
- [x] 1.3 Wire `application.webRuntime` to registered IDs, default `nginx-fpm`. Verify `make generate` and omitted YAML still plans nginx-fpm.
- [x] 1.4 Fold current nginx PHP-FPM Compose and cloud HTTP through the nginx-fpm plugin with no default-project behavior change. Verify `GOMAXPROCS=1 GOFLAGS=-p=1` localdev and AWS runtime tests.

## 2. FrankenPHP classic and Apache (Adobe hatch)

- [x] 2.1 Register `frankenphp-classic`: Adobe-unsupported on 2.4.8-p3+ / 2.4.9 without `compatibility.allowUnsupported`; hatch-warn when set; classic image; Compose; cloud task/sidecar. Docs state FrankenPHP has no Adobe row. Verify fail-closed and hatch-warn tests.
- [x] 2.2 Keep `frankenphp-worker` unregistered. Verify fail-closed names the missing plugin.
- [x] 2.3 Register `php-apache` with the same Adobe hatch contract as FrankenPHP. Verify fail-closed and hatch-warn tests.
- [x] 2.4 `magelift extensions` lists ID, version, Adobe flag, Magento range for nginx-fpm, frankenphp-classic, php-apache.
- [x] 2.5 Community signed go-plugin path without `plugin.Open`. Verify unsigned artifact refused.

## 3. Catalog honesty and Magento overlays

- [x] 3.1 Unset AWS standard/HA `queueMode` → `ecs-rabbitmq`; explicit `amazon-mq` experimental-warn. Verify `magelift config explain`.
- [x] 3.2 Matrix columns Adobe vs MageLift vs certified for every implemented cell. Verify docs match admission tests.
- [x] 3.3 ACC/Upsun import writes `application.magento` overlays (`frontName`, cookies, CORS, consumers). Verify import tests.
- [x] 3.4 ADR 0002, local vs cloud, operations, FAQ, examples. Humanizer then remove-ai-marks. Verify no page calls FrankenPHP or Apache Adobe-certified.

## 4. Architecture-review release blockers

- [x] 4.1 `CertificationTier` from evidence tuple (provider, runtime, compute mode, Magento release, catalog), not static certified on all ECS or all Autopilot. Verify Managed Instances and Autopilot HA do not inherit preview certified.
- [x] 4.2 GCP (and matching AWS) deploy lock: client errors fail closed; `Release` uses caller context, owner, generation. Verify tests that GCS construct failure does not return a no-op unlock.
- [x] 4.3 Orchestrator/CLI `errors.Join` primary error with lock-release error. Verify both appear.
- [x] 4.4 `Plan` uses caller context; returned region/class/protection/digest must match the request. Provider subprocess must be executable, not a kind string only. Verify mismatch fails before mutate.
- [x] 4.5 `make lint` green (`gofmt` target.go + observe_test.go; staticcheck S1011 component.go).

## 5. Observability X-Ray plugin

- [x] 5.1 Register an X-Ray `ObservabilityAdapter` (or typed unavailable if not ready). Verify YAML X-Ray without plugin is typed unavailable, not a silent IAM-only claim.
- [x] 5.2 After the plugin exists, one Magento-origin X-Ray cell on AWS Fargate KEEP. Verify traces in evidence or typed unsupported.

## 6. Packed certification campaign

- [x] 6.1 Complete Bedrock playground + Lambda web-app promo for AWS credits. Verify **no** Magento evidence file. Lambda Function URL COMPLETED 2026-08-23; Bedrock playground COMPLETED 2026-08-23T14:52Z (tutorial UI stuck after Run; billing activity still flipped). No Magento evidence file.
- [x] 6.2 GCP Autopilot Magento 2.4.8-p5 / 2.4.9 KEEP; cell-update; destroy + orphan on close. **Owned by** [`certify-gcp`](../certify-gcp/tasks.md) (task 3.2). Do not duplicate KEEP procedure here.
- [x] 6.3 AWS KEEP packed matrix, one artifact digest: Fargate Magento origin; warm `ecs-rabbitmq`, `amazon-mq`, OpenSearch, `aurora-mysql`, HA, CloudWatch; Magento-origin CloudFront if cheap; cold Managed Instances; cold EKS Magento + EKS RabbitMQ. Verify each promoted row has Magento (or declared infra-only) evidence for that tuple. **Owned by** [`certify-aws`](../certify-aws/tasks.md) (task 3.2). Do not rewrite `cells-aws-campaign-fargate-keep.txt` from this tracker.
- [x] 6.4 OVH one MKS preview (~$200 credits). Scaleway one Kapsule from **$50 own-money cap** shared with Cloudflare/SendGrid/Fastly/New Relic. Attach vendors to GCP origin only. **Owned by** [`certify-ovh`](../certify-ovh/tasks.md) (task 3.1) and [`certify-scaleway`](../certify-scaleway/tasks.md) (task 3.1).
- [x] 6.5 Worktrees, unique prefixes, `GOMAXPROCS=1 GOFLAGS=-p=1` per tree. Isolated Pulumi state.

## 7. Honesty docs

- [x] 7.1 Matrix/FAQ: CloudWatch vs X-Ray plugin, Armor withheld, Fastly experimental, Cloudflare DNS-only, SES/SendGrid config vs delivery, FrankenPHP/Apache hatch.
- [x] 7.2 `magelift audit` is not a customer SOC 2 / ISO 27001 / GDPR certificate.
- [x] 7.3 Named gaps remain until closed: no `websites[]`, Cloud SQL attach missing, SQS/Pub/Sub Magento modules, split cache AWS-only.

## 8. Verification

- [x] 8.1 `make generate` and `GOMAXPROCS=1 GOFLAGS=-p=1 go test ./internal/config/ ./internal/cli/ ./internal/localdev/ ./sdk/v1/ -count=1` pass.
- [x] 8.2 `openspec validate --change audit-catalog-cert-campaign --strict` passes.
