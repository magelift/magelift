## ADDED Requirements

### Requirement: Integrated PHP storefront is a certified path

When `application.mode` is `integrated`, build and deploy MUST apply the locale×theme static-content matrix for storefront themes, Magento base URLs and cookies on the shop hostname, static and media, and Varnish FPC when the catalog selects Varnish. Hyvä or other Node-built Magento theme compile MUST remain out of band until a typed Node hook exists. Free-form shell hooks MUST stay rejected. JavaScript storefronts MUST stay outside the CLI.

#### Scenario: Storefront themes are deployed

- **WHEN** an integrated project lists storefront themes and locales in `build.staticContent`
- **THEN** prepare runs `setup:static-content:deploy` for that matrix and the cloud runtime serves those static files through nginx-fpm

#### Scenario: Hyvä Node compile is not a Magelift hook

- **WHEN** a project needs a Node or Vite compile for a Magento theme
- **THEN** Magelift does not accept a shell hook for that compile and docs name it as an out-of-band step
