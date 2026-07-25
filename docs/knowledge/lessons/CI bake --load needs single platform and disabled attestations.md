---
type: lesson
title: CI bake --load needs single platform and disabled attestations
description: Supported bake targets set platforms amd64+arm64 plus SBOM/provenance attestations.
tags:
- ci
- docker
- bake
- github-actions
generated:
  at: '2026-07-24'
---

Supported bake targets set platforms amd64+arm64 plus SBOM/provenance attestations. docker buildx bake --load fails with "docker exporter does not currently support exporting manifest lists" unless CI overrides --set *.platform=linux/amd64 and passes --provenance=false --sbom=false. Singular platform overrides HCL platforms; empty attest= does not clear.
