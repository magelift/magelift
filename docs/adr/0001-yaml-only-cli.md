# ADR 0001: YAML-only CLI in the user's account

- Status: Accepted
- Date: 2026-08-22

## Context

Magento teams coming from Adobe Commerce Cloud or Upsun need one CLI that deploys into **their** AWS or GCP account. MageLift must not become a host, a billing proxy, or a second YAML dialect that names cloud products for portable Magento concerns.

## Decision

The supported user path is YAML-only: add `magelift.yaml` and use Magelift commands (`doctor`, `bootstrap`, `deploy`, `health`, `destroy`, and the day-2 set). The CLI provisions in the user's cloud account. It does not resell capacity or add a resource markup.

Laptop preview does not require the operator to run Pulumi, kubectl, Cosign, or GitHub federation commands. Those tools may run underneath the CLI.

Presets keep day-one YAML small. Catalog fields are the escape hatch. Headless means Magento `application.mode: headless|integrated`. Storefront frameworks stay outside the CLI.

## Consequences

Docs and CLI output talk Magento, not "open a Pulumi stack." Provider topology stays out of portable YAML (ADR 0003). Certification status is explicit (ADR 0002).

## Alternatives considered

- Hosted MageLift PaaS: rejected. The product is an in-account CLI.
- Requiring operators to drive Pulumi or kubectl: rejected for the supported path.

## Provenance

Product charter in `docs/architecture.md`. No third-party source copied.
