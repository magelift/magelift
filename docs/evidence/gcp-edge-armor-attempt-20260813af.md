# GCP Magento-safe Cloud Armor attempt — 2026-08-13 (`20260813af`)

Status: **FAIL** before attach. Fingerprint import succeeded. Export was an
object with `jsonParsing=STANDARD_WITH_GRAPHQL`, `body=64KB`, `scanner=1`, and
`sqliBodyExcl=0`. GA import dropped Magento body exclusions. Cleanup emptied
NEG/backend/policy inventories.

Do not reuse `20260813af`. Next live cell needs beta `requestBodiesToExclude`
on the WAF rules, not another GA import.

See [Cloud Armor GA import drops Magento body exclusions](README.md).
