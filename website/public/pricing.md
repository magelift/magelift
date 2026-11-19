# What MageLift costs

The MageLift CLI is free under the Apache-2.0 license. Your cloud provider bills you directly for the resources in your account. You also need your own entitlement if you use licensed Adobe Commerce. [License](https://github.com/magelift/magelift/blob/main/LICENSE)

| What you run | What to budget for |
| --- | --- |
| A stable environment | Compute, database capacity, search, cache, network traffic, logs and backups |
| A pull-request preview | The resources the preview creates and any ongoing cost of shared infrastructure |
| An existing service | The service’s current charges, including any capacity or traffic the shop adds |

On AWS, run `magelift cost --env preview` to estimate the configured resources. Check unpriced items and actual usage in your provider’s billing console. Budget alerts notify you about spend. A preview’s expiry makes it eligible for the scheduled cleanup job.

[Compare hosting options](https://magelift.dev/docs/compare-paas/) · [Provider coverage](https://magelift.dev/docs/capability-matrix/) · [Community support](https://github.com/magelift/magelift/blob/main/SUPPORT.md)
