---
type: lesson
title: MageLift PHP and Magento support intersection audited 2026-07-18
description: Adobe system requirements snapshot dated 2026-06-01 lists Magento 2.4.9 with PHP 8.5, 2.4.8
  with 8.3 or 8.4, 2.4.7 with 8.2 or 8.3, and 2.4.6 with 8.1 or 8.2.
tags:
- php
- magento
- compatibility
- adobe
- containers
generated:
  at: '2026-07-24'
sources:
- id: https-experienceleague-adobe-com-en-docs-commerce-operations-installation-guide-system-requirements-https-www-php-net-supported-versions-php
  resource: https://experienceleague.adobe.com/en/docs/commerce-operations/installation-guide/system-requirements;
    https://www.php.net/supported-versions.php
---

Adobe system requirements snapshot dated 2026-06-01 lists Magento 2.4.9 with PHP 8.5, 2.4.8 with 8.3 or 8.4, 2.4.7 with 8.2 or 8.3, and 2.4.6 with 8.1 or 8.2. MageLift supports the PHP 8.2+ intersection, so 2.4.6 uses 8.2 and 2.4.4/2.4.5 remain unavailable because they require EOL PHP 8.1. On 2026-07-18 php.net listed 8.2 and 8.3 as security-fixes-only and 8.4 and 8.5 as active. Security support ends 2026-12-31, 2027-12-31, 2028-12-31, and 2029-12-31 respectively.
