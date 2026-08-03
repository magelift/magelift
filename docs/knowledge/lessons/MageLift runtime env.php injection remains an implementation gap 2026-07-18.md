---
type: lesson
title: MageLift runtime env.php injection remains an implementation gap 2026-07-18
description: The ECS runtime now passes the Aurora managed database secret reference to application and
  deploy containers and grants the ECS execution role scoped read and KMS decrypt access.
tags:
- runtime
- magento
- secrets
- gap
status: deprecated
generated:
  at: '2026-07-24'
---

The ECS runtime now passes the Aurora managed database secret reference to application and deploy containers and grants the ECS execution role scoped read and KMS decrypt access. The image still needs a runtime configuration bootstrap or Adobe Commerce MAGENTO_DC environment override strategy, including a persistent crypt key, before real Magento deployments can work. Do not claim end-to-end deployment until this is implemented and tested.
