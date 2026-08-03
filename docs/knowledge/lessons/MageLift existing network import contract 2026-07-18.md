---
type: lesson
title: MageLift existing network import contract 2026-07-18
description: The AWS target now accepts target.aws.existing.network with an AWS network reference plus
  one public, private, and data subnet ID per configured availability zone.
tags:
- aws
- network
- imports
- pulumi
status: stable
generated:
  at: '2026-07-24'
---

The AWS target now accepts target.aws.existing.network with an AWS network reference plus one public, private, and data subnet ID per configured availability zone. Plan validation rejects incomplete or unsafe IDs. The network component registers imported VPC and subnet outputs without creating VPC, subnet, route, NAT, or endpoint resources. Imported networks retain routing and private AWS service access ownership.
