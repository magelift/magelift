---
type: lesson
title: MageLift Composer lock must resolve on PHP floor 2026-07-18
description: Adding Psalm 6.16.1 on PHP 8.5 initially resolved Symfony 8.1 packages requiring PHP >=8.4.1,
  which would break the PHP 8.2 CI matrix.
tags:
- composer
- php
- ci
- dependency-policy
status: stable
generated:
  at: '2026-07-24'
---

Adding Psalm 6.16.1 on PHP 8.5 initially resolved Symfony 8.1 packages requiring PHP >=8.4.1, which would break the PHP 8.2 CI matrix. Set build/composer.json config.platform.php to 8.2.27 and regenerated the lock; Symfony 7.4 is selected and composer prohibits php 8.2.27 reports no blockers. Keep the lock resolvable on the lowest supported runtime.
