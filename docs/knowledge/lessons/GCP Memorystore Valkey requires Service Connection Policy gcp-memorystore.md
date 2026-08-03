---
type: lesson
title: GCP Memorystore Valkey requires Service Connection Policy gcp-memorystore
description: Memorystore for Valkey DesiredAutoCreatedEndpoints fails without a regional networkconnectivity.ServiceConnectionPolicy
  on the consumer VPC with serviceClass=gcp-memorystore and PscConfig.subnetwor...
tags:
- gcp
- memorystore
- psa
- destroy
status: stable
generated:
  by: cursor/wsl
  at: '1784489258143584'
---

Memorystore for Valkey DesiredAutoCreatedEndpoints fails without a regional networkconnectivity.ServiceConnectionPolicy on the consumer VPC with serviceClass=gcp-memorystore and PscConfig.subnetworks set to private subnets. Enable networkconnectivity.googleapis.com and serviceconsumermanagement.googleapis.com. On destroy, Cloud SQL soft-delete can block servicenetworking Connection delete (FLOW_SN_DC_RESOURCE_PREVENTING_DELETE_CONNECTION); use gcloud services vpc-peerings delete --force after SQL is gone, then delete GlobalAddress and VPC. Acceptance scripts must assert_clean and force-clean orphans.
