# FrankenPHP classic adapter

This image is the Debian Trixie adapter for the selectable `frankenphp-classic`
web runtime. It runs FrankenPHP's normal request mode with Magento's `pub`
directory as the document root. It does not enable worker mode.

The image is a runtime base. The Magento build pipeline must add the application
filesystem, required PHP extensions, and the artifact manifest before publishing a
digest for deployment.

The local Compose profile exposes HTTP on port 8080 and an opt-in HTTPS listener on
port 8443. HTTPS uses Caddy's internal development CA and stores its ephemeral state
under `/tmp`; it is intended for local secure-cookie and integration testing only.
