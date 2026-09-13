## ADDED Requirements

### Requirement: External vendors use thin paid credits

Live Cloudflare, Fastly, New Relic, and SendGrid certification MUST assume thin paid credits, exact cleanup of DNS, CDN services, APM apps, and sender identities, and MUST NOT remain wired overnight unless KEEP is explicit and bounded.

#### Scenario: SendGrid sender identity is cleaned up

- **WHEN** a SendGrid live cell ends
- **THEN** evidence records cleanup of MageLift-owned sender or API resources and does not leave a billed sender identity unmarked
