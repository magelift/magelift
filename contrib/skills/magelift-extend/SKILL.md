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
- Validate IDs, API version, outputs, certification tier, and provenance before
  registration.
- Do not download or execute an unsigned remote extension.
- Add mock tests first, then a disposable real-account cell before claiming support.

## Report

Include extension name and version, source identity, API version, target IDs,
capabilities, evidence tier, and the exact build provenance.
