# ADR 0001. Keep shop configuration in PHP

Magento developers already maintain PHP in the shop repository. A single
`magelift.php` lets them reuse values and helpers across environments with
the tools they already use.

The CLI evaluates the trusted PHP file and resolves the resulting settings
in Go. A CLI edit to an environment can write to an adjacent PHP file while
leaving the shop's handwritten project file in place. The Adobe Commerce
Cloud and Upsun importers read the source platform's YAML and produce PHP
for the shop owner to review. [Project settings](../configuration.md) · [Migration guide](../migrating-from-paas.md)
