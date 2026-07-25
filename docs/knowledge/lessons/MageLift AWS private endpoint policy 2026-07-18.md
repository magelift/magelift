---
type: lesson
title: MageLift AWS private endpoint policy 2026-07-18
description: Standard and high-availability AWS networks use private interface VPC endpoints for ECR API/DKR,
  CloudWatch Logs, Secrets Manager, SSM, SSM Messages, and EC2 Messages, plus the S3 gateway endpoint.
tags:
- aws
- network
- security
- cost
generated:
  at: '2026-07-24'
---

Standard and high-availability AWS networks use private interface VPC endpoints for ECR API/DKR, CloudWatch Logs, Secrets Manager, SSM, SSM Messages, and EC2 Messages, plus the S3 gateway endpoint. Preview keeps only S3 to avoid fixed endpoint-hour costs. Endpoint security groups allow HTTPS from the VPC CIDR and private DNS is enabled.
