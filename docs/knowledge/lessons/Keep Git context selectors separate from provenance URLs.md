---
type: lesson
title: Keep Git context selectors separate from provenance URLs
description: On 2026-07-18 tightening MageLift provenance URLs broke one test that still used a ref selector.
tags:
- buildkit
- provenance
- security
- tests
generated:
  at: '2026-07-24'
---

On 2026-07-18 tightening MageLift provenance URLs broke one test that still used a ref selector. Direct BuildKit Git contexts may use a single ref, tag, or branch plus a full checksum, but a separate provenance URL for a prepared local context accepts only checksum. This prevents query credentials from entering labels or attestations.
