---
name: magelift-extend
description: >-
  Add a MageLift provider, architecture, capability, lifecycle hook, or edge
  integration through the public extension boundary.
version: 1.0.0
---

# Extend MageLift

Use this skill when the requested behavior does not belong in the first-party
core or when a community provider needs a stable integration point.

## Working rules

- Use the versioned SDK descriptors and typed namespaces.
- Register extensions explicitly in a custom binary. The default binary does not
  search the working directory or execute nearby programs.
- Keep Pulumi and provider-specific resource code in the extension adapter.
- Validate IDs, API version, outputs, status, and provenance before
  registration.
- Do not download or execute an unsigned remote extension.
- Test the extension through the public SDK and Pulumi mocks. Record the method and configuration for any additional tests.

## Report

Include extension name and version, source identity, API version, target IDs,
capabilities, evidence tier, and the exact build provenance.
