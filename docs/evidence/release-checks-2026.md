# Earlier release checks (2026)

These checks ran on earlier repository revisions. The current release
procedure is in [Publishing](../publishing.md).

## 2026-07-21 AWS preview

A preview in `eu-north-1` applied in 631 seconds on the free-tier account.
It used RDS MySQL, fck-nat, and disabled search. The initial database queue,
ECS RabbitMQ, and ECS Artemis were each exercised on that stack. The
application health endpoint returned HTTP 404, limiting the Magento runtime
checks. The NAT gateway, OpenSearch Serverless, and Aurora Serverless v2
configurations were previewed. Pulumi and the provider reported the resource
changes recorded for those plans.

## 2026-08-07 local verification

The final pre-release check ran Go tests one process at a time. The root
suite passed 1,126 tests in 102 packages. The PHP check passed Composer
validation and audit, static analysis, and 119 PHPUnit tests with 274
assertions. Shell checks, generated-file checks, the strict docs build, and
OpenSpec validation passed.

## Packaging and license checks

| Date | Check | Result |
| --- | --- | --- |
| 2026-07-22 | GoReleaser config and Go dependency licenses | Config validated and license check passed with a manual OVH Pulumi license review. |
| 2026-07-28 | `make release-smoke`, host snapshot | Passed in about 159 seconds. |
| 2026-09-13 and 2026-09-15 | Host snapshot and Go dependency licenses | Passed. The packaging check included the CLI and Google Cloud provider. |
| 2026-09-17 | Host snapshot using the reduced release config | Passed. The script checked the CLI archive, provider binary, and checksum entry. |
