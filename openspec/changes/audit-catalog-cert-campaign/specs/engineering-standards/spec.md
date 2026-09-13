## ADDED Requirements

### Requirement: Plugin and certification docs stay honest

When web-runtime plugins or certification tiers change, the capability matrix, ADR 0002, local vs cloud, operations, and FAQ MUST update in the same change. Prose MUST go through humanizer then remove-ai-marks. Pages MUST say Adobe-supported vs MageLift-plugin vs certified evidence, not "we support every web server equally."

#### Scenario: FrankenPHP is documented as a plugin

- **WHEN** FrankenPHP classic ships as a plugin
- **THEN** the matrix lists it Adobe-unsupported (hatch required) and nginx-fpm remains the certified Adobe-aligned default

### Requirement: Addon and compliance pages name current evidence bounds

Touched human pages MUST state CloudWatch/GCP ops graphs vs Magento three-signal cert, X-Ray as an observability plugin (typed unavailable until registered), CloudFront Magento-origin vs alias-HTTPS docs host, Cloud Armor `requestBodiesToExclude` withheld, Fastly experimental, Cloudflare DNS-only, SES/SendGrid config vs delivery, FrankenPHP/Apache Adobe hatch, and that `magelift audit` is not a customer SOC 2 / ISO 27001 / GDPR certificate. `make lint` MUST be green before release.

#### Scenario: FAQ covers New Relic and SOC 2

- **WHEN** an operator reads FAQ after this change
- **THEN** New Relic is bounded GKE+ops evidence unless a Magento three-signal file exists, and SOC 2 copy points at `magelift audit` without certifying the customer
