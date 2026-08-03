---
type: lesson
title: MageLift emulator endpoint safety contract 2026-07-18
description: MAGELIFT_AWS_ENDPOINT_URL is now parsed centrally under internal/cloud/aws/endpoint.
tags:
- security
- floci
- aws-endpoint
- failure-correction
status: stable
generated:
  at: '2026-07-24'
---

MAGELIFT_AWS_ENDPOINT_URL is now parsed centrally under internal/cloud/aws/endpoint. Only empty values or HTTP(S) loopback hosts (localhost, 127.0.0.1, ::1) are accepted, with no credentials, query, fragment, or path other than a trailing slash. Every AWS SDK constructor that honors the override uses this parser before client creation, while production leaves the variable unset. The first focused test rejected a trailing slash because url.Parse reports it as path '/', then the parser was corrected to normalize that harmless form; the full race suite and Floci tests pass.
