---
type: lesson
title: MageLift AWS infra choice escape hatches 2026-07-19
description: 'Explicit target.aws choices (not silent certified-shape swaps): natMode=nat-gateway|fck-nat
  (default nat-gateway), catalog.databaseEngine=aurora-mysql|rds-mysql (default aurora-mysql; rds-mysql
  pre...'
tags:
- aws
- free-tier
- rds
- fck-nat
status: stable
generated:
  at: '2026-07-24'
---

Explicit target.aws choices (not silent certified-shape swaps): natMode=nat-gateway|fck-nat (default nat-gateway), catalog.databaseEngine=aurora-mysql|rds-mysql (default aurora-mysql; rds-mysql preview-only), catalog.searchMode=serverless|provisioned|disabled (defaults by preset). Free-tier acceptance uses fck-nat + rds-mysql db.t4g.micro + searchMode disabled. fck-nat AMI owner 568608671756 name fck-nat-al2023-*-arm64-ebs, instance t4g.micro, SourceDestCheck=false, private routes via ENI. RDS uses rds.Instance ManageMasterUserPassword same secret contract as Aurora.
