---
type: lesson
title: RDS capability checks must match the VPC filter
description: AWS RDS orderable-instance responses change with the VPC filter, so diagnostics must reproduce the SDK request before changing fail-closed admission.
tags: [aws, rds, admission, availability-zones, diagnostics]
status: stable
generated:
  by: codex
  at: 2026-08-17
sources:
  - id: rds-orderable-option
    resource: https://docs.aws.amazon.com/AmazonRDS/latest/APIReference/API_OrderableDBInstanceOption.html
    title: Amazon RDS OrderableDBInstanceOption
  - id: rds-orderable-api
    resource: https://docs.aws.amazon.com/AmazonRDS/latest/APIReference/API_DescribeOrderableDBInstanceOptions.html
    title: Amazon RDS DescribeOrderableDBInstanceOptions
---

# RDS capability checks must match the VPC filter

`DescribeOrderableDBInstanceOptions` is sensitive to the `Vpc` request
parameter. The first `awsba` diagnosis queried MySQL `8.4.10`/`db.t4g.micro`
without `--vpc` and saw an orderable record with no visible AZ list. The
adapter sends `Vpc=true`; the matching response explicitly listed
`eu-west-3b` and `eu-west-3c`, so rejecting a plan that selected
`eu-west-3a` was correct.

When diagnosing a provider admission failure, reproduce the exact SDK filters
before weakening a fail-closed guard. The `AvailabilityZones` member is
optional in the AWS schema, but an explicit list returned for the matching
request remains authoritative.
