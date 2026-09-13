---
name: magelift-configure
description: >-
  Configure MageLift YAML and choose an Adobe-compatible architecture. Use when
  selecting PHP, database, search, queue, cache, Varnish, edge, or provider options.
version: 1.0.0
---

# Configure MageLift

Use this skill before preview or deploy when the service shape is changing.

## Working rules

- Start with `magelift compatibility catalog` for the exact Adobe patch line.
- Use the latest patch record for 2.4.6 through 2.4.9. Do not use a version range.
- Separate three facts: Adobe lists the service, MageLift implements the service,
  and the selected provider exposes the required product.
- Keep `compatibility.allowUnsupported` off unless the exception is deliberate,
  documented, and visible in the planned artifact.
- Prefer database-backed queues, no managed search, and small replicas for
  disposable previews.
- AWS Magento env must match MageLift's product contract for the selected
  YAML: writer host + secret JSON for RDS/Aurora; provisioned OpenSearch is
  in-VPC HTTPS Magento can query (no SigV4 sidecar); AOSS still needs a SigV4
  sidecar. Private sibling shops may illustrate one of those shapes. They are
  not the only target and they do not certify a cell.

## Check the result

```sh
magelift compatibility catalog 2.4.8-p5
magelift compatibility validate --release 2.4.8-p5 \
  --component search --option elasticsearch --version 8
magelift --config magelift.yaml --env staging config validate
magelift --config magelift.yaml --env staging preview
```

If a choice is Adobe-supported but the target reports unavailable, do not mark
the cell certified. Choose another target or add a provider extension.
