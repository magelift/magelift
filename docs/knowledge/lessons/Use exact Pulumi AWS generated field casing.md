---
type: lesson
title: Use exact Pulumi AWS generated field casing
description: On 2026-07-18 the first full verification of the AWS edge component failed because the Pulumi
  AWS v7 field is HttpPort, not HTTPPort.
tags:
- pulumi
- aws
- edge
- verification
- failure
generated:
  at: '2026-07-24'
---

On 2026-07-18 the first full verification of the AWS edge component failed because the Pulumi AWS v7 field is HttpPort, not HTTPPort. The component was corrected and its mock tests plus full make verify now pass.
