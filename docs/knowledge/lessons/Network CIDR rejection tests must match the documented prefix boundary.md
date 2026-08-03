---
type: lesson
title: Network CIDR rejection tests must match the documented prefix boundary
description: The network component accepts canonical IPv4 VPC prefixes through /24 because splitting them
  into /28 subnets still supports the nine-subnet maximum.
tags:
- network
- testing
- cidr
- failed-attempt
generated:
  at: '2026-07-24'
---

The network component accepts canonical IPv4 VPC prefixes through /24 because splitting them into /28 subnets still supports the nine-subnet maximum. A test incorrectly expected a canonical /24 to fail. Boundary rejection tests must use /25 or narrower when validation explicitly allows /24.
