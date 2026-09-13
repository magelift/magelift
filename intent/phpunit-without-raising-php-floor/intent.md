---
status: draft
slug: phpunit-without-raising-php-floor
---

# Intent: PHPUnit 12 or 13 without raising the PHP 8.2 floor

## Problem

The `build/` package still supports PHP 8.2. PHPUnit 12 and 13 are out, but they raise the *test suite* PHP floor even though the library is `^8.2`. Magelift stayed on PHPUnit 11.5.x (11.5.56). Contributors cannot use PHPUnit 12/13 APIs or run the suite on a 12.x runner without dropping 8.2 CI.

## Evidence

`docs/dependency-policy.md`: PHPUnit stays on the latest 11.x because newer majors would raise the test suite's PHP floor. Packagist latest is 13.x; latest 11.x is 11.5.56. PHPUnit 12's PHP requirement vs Magelift's `^8.2` library: not checked beyond that policy sentence.

## Proposed outcome

Either PHPUnit 12+ runs on every supported PHP branch including 8.2, or Magelift *decides* to raise the test floor and records that in the dependency policy and CI matrix. The Composer package's declared `php: ^8.2` does not silently change.

## Affected users and systems

`build/` tests, `make php-test`, PHP CI jobs. Not Magento runtime images.

## Constraints

PHP 8.2 remains the library runtime floor until a separate, accepted change. CI still exercises every supported PHP branch. No generated Psalm/PHPStan baseline to hide breakage.

## Out of scope

Psalm 7. Changing Magento's PHP catalog. Shipping PHPUnit as a runtime dependency of shops.

## Open questions

Does PHPUnit 12 still run on 8.2 at all? If not, is raising the *test* floor (keep library 8.2, test on 8.3+) acceptable, or do we wait until Magelift raises the library floor?
