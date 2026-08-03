# Cloudflare DNS for MageLift cutover

**Authorized zones (2026-07-30):** any subdomain of `alexandrecourtiol.com` or `acourtiol.com`.

**Preferred preview host:** `magelift-preview.alexandrecourtiol.com`

## Auth note

Wrangler OAuth (`wrangler whoami`) currently has `zone:read` only — list zones works; DNS record create/update returns Authentication error.

Phase 7 DNS cutover needs a Cloudflare API token with:
- Zone → DNS → Edit (and Zone → Zone → Read) on those two zones

Export for the harness (do not commit):
```bash
export CLOUDFLARE_API_TOKEN=...   # or CF_API_TOKEN
export MAGELIFT_CUTOVER_HOST=magelift-preview.alexandrecourtiol.com
```

Cleanup: delete the cutover CNAME/A after the rehearsal.
