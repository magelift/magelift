## 1. Edge contract

- [x] 1.1 Add Fastly edge intent and secret-reference validation.
- [x] 1.2 Add provider-owned configuration and stable output mapping.
- [x] 1.3 Add mock plan tests for domains, TLS, purge, and VCL references.

## 2. Migration mapping

- [x] 2.1 Inspect ACC importer fields for Fastly service, domain, TLS, and VCL intent.
- [x] 2.2 Inspect Upsun importer fields for equivalent edge intent.
- [x] 2.3 Preserve supported fields and emit stable unmapped diagnostics.
- [x] 2.4 Add plaintext secret rejection and redaction tests.

## 3. Live verification

- [x] 3.1 Add a disposable Fastly acceptance configuration with ownership markers.
- [x] 3.2 Prove preview, apply, managed TLS/ACME DNS challenge, purge, routing, destroy, and direct cleanup assertion in the current versionless Domain Management path; the run is recorded in `docs/evidence/fastly-adapter-live-2026-08-09.md` and remains experimental rather than a general Fastly failover/WAF certification.
- [x] 3.3 Keep Fastly experimental until generated evidence passes.
