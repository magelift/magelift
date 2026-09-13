---
type: lesson
title: Magento 2.4.6 rejects MySQL 8.4
description: The 2.4.6-p15 image rejected MySQL 8.4 during installation and completed with MySQL 8.0.
tags: [magento, mysql, compatibility, aws, gcp]
status: stable
generated:
  by: codex
  at: 2026-08-06
---

The exact Magento 2.4.6-p15 image rejected a MySQL 8.4.10 server with
`Current version of RDBMS is not supported`. The same installation completed
with MySQL 8.0, followed by `setup:upgrade` and the cache commands. The
compatibility matrix and provider defaults must therefore resolve the database
engine per Magento release instead of reusing the newest MySQL minor version.

MageLift's AWS compatibility default for the 2.4.6 line now uses RDS MySQL
8.0.45. The Aurora default remains separate because Aurora exposes a different
engine version string.

This is an image-backed compatibility fact, not a promise that every provider
offers that exact minor version in every region. The provider planner must still
query or validate regional engine availability before creating RDS resources.
