---
type: lesson
title: AWS free plan blocks Aurora MySQL 2026-07-19
description: Free plan AWS accounts reject CreateDBCluster for aurora-mysql (FreeTierRestrictionError);
  only aurora-postgresql allowed.
tags:
- aws
- acceptance
- free-tier
- aurora
generated:
  at: '2026-07-24'
---

Free plan AWS accounts reject CreateDBCluster for aurora-mysql (FreeTierRestrictionError); only aurora-postgresql allowed. Magento needs MySQL so acceptance requires leaving free plan or an RDS MySQL fallback path.
