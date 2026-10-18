---
type: lesson
title: EKS Valkey transport must match the Magento image
description: A managed cache can be healthy while Magento still fails if the selected image cannot speak the cache service's required TLS or authentication protocol.
tags: [aws, eks, valkey, redis, magento, acceptance]
status: stable
generated:
  by: codex
  at: 2026-08-07
---

# Observation

The official Magento 2.4.9 acceptance image uses the plain Redis adapter and
does not accept the TLS and authentication settings required by a secured AWS
Valkey group. The private Valkey endpoint was reachable, but Magento cache and
session operations hung when the group required encrypted authenticated
transport.

# Rule

Certify the application protocol and the managed-service security mode
together. Do not mark Valkey support green from a provider ping alone. Either
use an image and adapter configuration that supports TLS and authentication,
or make the insecure plain-transport mode an explicit disposable acceptance
escape hatch. Production defaults must remain encrypted and authenticated.

# MageLift

The EKS acceptance stack used a private, non-authenticated, non-TLS Valkey
group only for the official image compatibility cell. The result is not a
production security recommendation. The next cache implementation step is to
make the image capability and cache transport explicit in the configuration
contract before certifying secured Valkey.
