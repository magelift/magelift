# Magento and service versions

MageLift suggests PHP and service versions from the Adobe system requirements
snapshot taken on 2026-11-02. Each shop chooses its versions in `magelift.php`.
Check the selected cloud and region before relying on a managed service.
[Adobe system requirements](https://experienceleague.adobe.com/en/docs/commerce-operations/installation-guide/system-requirements) · [Provider support](capability-matrix.md)

| Magento line | PHP in the bundled recipe | Magento patch in the snapshot |
| --- | --- | --- |
| 2.4.9 | 8.5 | 2.4.9 |
| 2.4.8 | 8.3, 8.4 | 2.4.8-p5 |
| 2.4.7 | 8.3 | 2.4.7-p10 |

The nginx/PHP-FPM recipe covers PHP 8.3 through 8.5. Adobe lists PHP 8.2 for
2.4.6 and PHP 8.1 for 2.4.4 and 2.4.5. Those releases require images with
their own PHP version. Adobe also lists PHP 8.2 for 2.4.7.

Use the CLI to check a service before editing the shop configuration.

```sh
magelift compatibility catalog 2.4.9
magelift compatibility validate --release 2.4.8-p5 \
  --component search --option elasticsearch --version 8
```

| Result | Meaning |
| --- | --- |
| `adobe-supported` | Adobe lists the choice for that release. |
| `magelift-compatible` | MageLift can use the choice, such as a database queue or Fastly integration. |
| `unsupported` | The choice falls outside Adobe's listed versions or options. |
| `unavailable` | The selected MageLift path cannot use the service or release. |

MageLift reports warnings when your versions differ from Adobe's guidance.
For a local service version outside the tested rows, the plan shows the
upstream image tag it will use. You can supply an image by digest to pin
exactly what Docker runs. [Service settings](service-bindings.md)

Adobe lists Composer 2.10 for Magento 2.4.9 and 2.4.8-p5. The 2.4.9 release
notes also list Composer 2.9. Check the shop's locked plugins before changing
the Composer binary. [Composer settings](builds.md#composer) · [Adobe 2.4.9 release notes](https://experienceleague.adobe.com/en/docs/commerce-operations/release/notes/adobe-commerce/2-4-9)
