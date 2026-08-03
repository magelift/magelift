# Security policy

## Supported versions

| Line | Support |
| --- | --- |
| `main` | Best-effort security fixes until the first public tag |
| `v1.0.0-rc.*` | Supported after the first public tag ships (listed here when cut) |
| `v1.x` | Supported after `v1.0.0` (latest patch of the current minor) |

Reporting against: **`main` only** until a release line appears in this table.
There is no supported release line yet.

## Reporting a vulnerability

Do not open a public issue. Use GitHub's
[private vulnerability reporting](https://docs.github.com/en/code-security/security-advisories/guidance-on-reporting-and-writing-information-about-vulnerabilities/privately-reporting-a-security-vulnerability)
for this repository (Security → Advise a vulnerability). If that path is
unavailable, email `contact@magelift.dev` without including secrets in the first
message.

Include affected versions, impact, reproduction steps, and any suggested mitigation.

### Response SLA

| Step | Target |
| --- | --- |
| Acknowledge a complete report | **5 business days** |
| Initial severity / next steps | **10 business days** after ack |
| Fix or advisory for supported releases | Coordinated with reporter; no fixed calendar for zero-days |

Supported releases after the first public tag: see the Supported versions table
above.

Never test against infrastructure or accounts you do not own or have explicit
permission to assess. Good-faith research that follows this policy will not be
threatened with legal action by the project maintainers.
