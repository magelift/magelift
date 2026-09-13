---
type: lesson
title: EKS Auto Mode NLB defaults to internal
description: An Auto Mode Service of type LoadBalancer provisions an internal NLB unless aws-load-balancer-scheme is internet-facing, so Magento HTTP health from outside the VPC times out.
tags: [aws, eks, nlb, magento, health]
status: stable
generated:
  by: cursor-grok
  at: 2026-08-24
---

# EKS Auto Mode NLB defaults to internal

EKS Auto Mode treats `type: LoadBalancer` as an NLB (`loadBalancerClass: eks.amazonaws.com/nlb` is the cluster default). The scheme defaults to **internal**. The Service hostname then resolves to a VPC address (for MageLift, the shop CIDR), so `CheckRuntime` GET from the operator laptop records `Magento HTTP probe did not complete`.

Port-forward to the web pod can still return Magento HTTP (often 302). That is not public LoadBalancer health.

Set `service.beta.kubernetes.io/aws-load-balancer-scheme: internet-facing` on the web Service. Tag public subnets `kubernetes.io/role/elb=1` and private subnets `kubernetes.io/role/internal-elb=1` so Auto Mode can place the public NLB. Changing scheme in place recreates the NLB; wait until DNS is not RFC1918 before Magento `config:set` of `base_url`.

`pulumi.com/skipAwait` freezes stack `applicationURL` on the previous hostname. `CheckRuntime` must GET the live Service LoadBalancer, not that output, or Magento HTTP keeps timing out against the old internal NLB.

See [GCP Magento candidate health needs LoadBalancer base_url](GCP%20Magento%20candidate%20health%20needs%20LoadBalancer%20base_url.md) for the localhost 302 after the NLB is reachable.
