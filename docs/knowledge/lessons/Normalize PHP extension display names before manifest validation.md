---
type: lesson
title: Normalize PHP extension display names before manifest validation
description: The fourth build E2E reached manifest finalization and failed because get_loaded_extensions
  returns the display name Zend OPcache.
tags:
- php
- manifest
- normalization
- e2e
generated:
  at: '2026-07-24'
---

The fourth build E2E reached manifest finalization and failed because get_loaded_extensions returns the display name Zend OPcache. Lowercasing alone produced zend opcache, which violates the stable manifest ID grammar. Normalize extension display names to lowercase IDs and replace non [a-z0-9_-] runs with underscores; test Zend OPcache explicitly.
