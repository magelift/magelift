---
status: draft
slug: psalm-7-stable
---

# Intent: Psalm 7 after it is a stable release

## Problem

`build/` is on Psalm 6.17.1. Psalm 7 exists only as a beta (7.0.0-beta19 at the 2026-09-12 bump). Magelift does not take betas for the static-analysis gate. Contributors stay on 6.x until 7 is production-suitable and still fits PHP 8.2.

## Evidence

Packagist latest `vimeo/psalm` was 7.0.0-beta19; latest 6.x was 6.17.1. `make php-test` passes on 6.17.1. Psalm 7 PHP floor and rule churn against `build/`: not checked.

## Proposed outcome

Psalm 7 stable is the `make php-test` analyser, with findings fixed in source (no generated baseline). PHP 8.2 remains the platform in `composer.json` unless a separate floor change is accepted.

## Affected users and systems

`build/` contributors, `composer psalm`, PHP CI.

## Constraints

No beta in CI. PHP 8.2 floor until a separate intent. Dynamic JSON and process-boundary checks stay in source, not a baseline dump.

## Out of scope

PHPUnit 12. PHPStan 3 if it appears. Rewriting Magento analysis.

## Open questions

Does Psalm 7 stable require PHP above 8.2? Which 6.x checks become errors that Magelift must fix vs disable with evidence?
