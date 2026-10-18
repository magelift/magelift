---
type: lesson
title: EKS Auto Mode warm sessions need endpoint normalization
description: EKS Auto Mode has a long managed-service cold start, and AWS returns the Kubernetes API endpoint with its scheme already included.
tags: [aws, eks, kubernetes, acceptance, cleanup, efficiency]
status: stable
generated:
  by: codex
  at: 2026-08-07
---

# Observation

The first AWS EKS preview stack created RDS MySQL 8.4.10 in about 532 seconds,
Valkey in about 478 seconds, and the EKS Auto Mode cluster in about 648 seconds.
The cluster output already contained `https://`. The generated kubeconfig added
another scheme, so the Kubernetes provider tried to resolve
`https://https/<endpoint>` and failed before creating Magento workloads.

# Rule

Treat managed-service creation and deletion as the cold boundary of an
acceptance group. Keep compatible EKS cells on the same stack and fingerprint
the stack inputs so a warm checkpoint cannot hide a changed image or topology.
Normalize provider endpoints before rendering kubeconfig or client config, and
test both scheme-qualified and bare endpoint inputs.

# Cleanup

The failed run was destroyed with Pulumi's dependency order. EKS deletion took
229 seconds, Valkey deletion took 274 seconds, and the final direct inventory
was empty. RDS-created CloudWatch log groups outlived the database and needed
an exact project-prefix log-group cleanup before `assert_clean` passed. The
resource-tagging index still reported 17 stale mappings, so direct inventories
remain authoritative for the cost guarantee.
