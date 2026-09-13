# Known-content recovery fixture

This fixture contains no customer data and no secret values. It is used to
prove that a provider adapter restores every declared class rather than only
the database. The `configuration.env.template` file contains secret and object
references, never their resolved values.

Provider acceptance materializes these files into an isolated destination and
must report the manifest digest, content digest, permission digest, counts,
application reads, secret-reference resolution, service health, and restore
duration through the provider-neutral recovery contract.
