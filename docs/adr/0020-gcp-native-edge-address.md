# ADR 0020: GCP native edge URL and ingress address

- Status: Accepted
- Date: 2026-10-18

## Context

GKE Ingress publishes a load balancer IP while its managed certificate and
routing use the configured shop hostname. Returning an HTTPS IP as the
application URL breaks certificate and hostname checks.

## Decision

For GCP native edge, `applicationURL` uses the configured hostname and TLS
scheme once Ingress publishes a load balancer address. The existing stack
outputs gain `nativeEdgeIngressAddress`, containing the raw IP or hostname
needed for customer-managed DNS. Both values stay empty before an address is
available. The address output stays empty when native edge is disabled.

This changes outputs only. It creates no DNS records or additional resources,
and adds no YAML key, manifest or RPC method. Ingress readiness alone does not
prove DNS delegation, certificate activation or application health.

## Verification

Local regressions cover hostname URLs, TLS schemes, missing addresses, IP and
hostname load balancer addresses, and unchanged resource counts. Live HTTPS
and application health remain separate acceptance checks.
