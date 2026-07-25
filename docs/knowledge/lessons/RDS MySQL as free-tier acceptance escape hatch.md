---
type: lesson
title: RDS MySQL as free-tier acceptance escape hatch
description: 'Yes: Magento accepts standard RDS MySQL 8.'
tags:
- aws
- rds
- aurora
- free-tier
- acceptance
status: stable
decision_status: recommended
generated:
  at: '2026-07-24'
---

Yes: Magento accepts standard RDS MySQL 8. Magelift v1 certified shape stays Aurora (Cluster/Serverless v2). For free-plan accounts that reject aurora-mysql CreateDBCluster, add an explicit preview/acceptance engineMode (rds-mysql, e.g. db.t4g.micro) that provisions rds.Instance not rds.Cluster — do not silently swap certified Aurora. Wire writer endpoint + Secrets Manager the same way consumers already read. Still need NAT/fck-nat and AOSS (or skip search) for full deploy.
