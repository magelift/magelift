# Security policy

## Supported versions

Until the first public release line (`v1.0.0-rc.1` and later) is listed here,
treat security support as best-effort on `main`. Once releases begin, this
document will list supported versions and security-maintenance windows.

## Reporting a vulnerability

Do not open a public issue. Use GitHub's
[private vulnerability reporting](https://docs.github.com/en/code-security/security-advisories/guidance-on-reporting-and-writing-information-about-vulnerabilities/privately-reporting-a-security-vulnerability)
for this repository (Security → Advise a vulnerability). If that feature is
unavailable, contact the maintainers through the private address listed in the
repository profile; do not include secrets in the initial message.

Include affected versions, impact, reproduction steps, and any suggested mitigation.

### Response SLA

| Step | Target |
| --- | --- |
| Acknowledge a complete report | **5 business days** |
| Initial severity / next steps | **10 business days** after ack |
| Fix or advisory for supported releases | Coordinated with reporter; no fixed calendar for zero-days |

Supported releases after the first public tag: the latest `v1.0.0-rc.*` line, then
the latest `v1.x` patch once `v1.0.0` ships (see Supported versions above).

Never test against infrastructure or accounts you do not own or have explicit
permission to assess. Good-faith research that follows this policy will not be
threatened with legal action by the project maintainers.
