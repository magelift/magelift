# ADR 0020. Report the shop URL and ingress address separately

Accepted on 2026-10-18.

GKE Ingress reports the load balancer's IP address or hostname. The
certificate and routing rules use the shop's domain. Developers need both
values when they set up DNS and check HTTPS.

Once the native ingress has an address, `applicationURL` uses the shop
hostname and TLS scheme. `nativeEdgeIngressAddress` holds the raw load
balancer address for the DNS record. Both native values stay empty while the
ingress is waiting for an address. The raw address output is empty when
native edge is disabled.

Local tests cover absent addresses, raw IPs and hostnames, and the URL's
scheme and hostname. After creating the DNS record, check the certificate
and storefront in the deployed environment. [Cloud setup](../bootstrap.md)
