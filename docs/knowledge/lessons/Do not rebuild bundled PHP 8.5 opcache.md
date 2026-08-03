---
type: lesson
title: Do not rebuild bundled PHP 8.5 opcache
description: 'On 2026-07-17, docker-php-ext-install opcache failed with "cp: cannot stat modules/*" both
  in parallel and serial builds.'
tags:
- docker
- php
- build
- failure
- root-cause
generated:
  at: '2026-07-24'
---

On 2026-07-17, docker-php-ext-install opcache failed with "cp: cannot stat modules/*" both in parallel and serial builds. Inspection proved the official PHP 8.5 FPM image already has Zend OPcache enabled as a bundled extension. The correct fix is to test for it and omit opcache from docker-php-ext-install. Do not infer a build race from the install-modules symptom.
