---
type: lesson
title: Floci exposed S3 lock body checksum failure
description: AWS SDK for Go v2 S3 PutObject checksum middleware rejects an unseekable io.NopCloser(bytes.Reader)
  body when talking to Floci over HTTP.
tags:
- floci
- aws-sdk
- s3
- state-lock
- failure
generated:
  at: '2026-07-24'
---

AWS SDK for Go v2 S3 PutObject checksum middleware rejects an unseekable io.NopCloser(bytes.Reader) body when talking to Floci over HTTP. State lock writes must pass a seekable bytes.Reader so request checksum computation works in emulators and real AWS.
