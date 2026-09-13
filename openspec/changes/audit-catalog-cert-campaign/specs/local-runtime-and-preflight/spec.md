## ADDED Requirements

### Requirement: Local Compose uses the registered web-runtime plugin

`magelift local init` MUST plan the web service from the selected `application.webRuntime` plugin. Unregistered families MUST fail before Compose writes volumes. Registered FrankenPHP classic or Apache plugins MUST use their image and health contracts. nginx-fpm remains the default.

#### Scenario: FrankenPHP classic local stack

- **WHEN** the FrankenPHP classic plugin is registered, YAML selects it, and `compatibility.allowUnsupported` is set
- **THEN** Compose does not emit an nginx app service under that name

#### Scenario: Unknown local family still fails

- **WHEN** YAML selects a web runtime with no plugin
- **THEN** `local init` fails before creating volumes
