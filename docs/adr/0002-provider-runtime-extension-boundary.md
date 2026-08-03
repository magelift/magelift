# ADR 0002: Keep provider topology outside portable contracts

- Status: Accepted
- Date: 2026-07-17

## Context

Magento build and deployment phases are largely portable. Infrastructure is not. An
Aurora cluster, an ECS service, and an EKS workload have different failure modes,
security controls, scaling behavior, and costs. Hiding those differences behind a
generic YAML model would make validation weaker and operations less predictable.

V1 needs a clear extension boundary without suggesting that untested providers or
runtimes are supported.

## Decision

MageLift separates portable contracts from infrastructure topology.

The application model, artifact manifest, build lifecycle, release identity, and
capability requirements are portable. They describe what Magento needs without naming
the resources that provide it.

Web runtime implementations are also portable across targets. The certified default
is nginx with PHP-FPM. FrankenPHP classic mode may run the same Magento application on
ECS, a future EKS target, or another cloud target once it passes the runtime acceptance
suite. Target implementations provide the surrounding compute and network topology;
they do not redefine Magento's web process contract.

FrankenPHP worker mode is reserved for later research. MageLift will not expose it in
configuration until Magento request isolation, extension behavior, memory use, and
deployment lifecycle compatibility are proven under sustained tests.

A versioned `Target` interface owns provider and runtime topology. Versioned
`CapabilityProvider` interfaces supply explicit capabilities such as database, cache,
search, queue, object storage, edge, and observability. Implementations may expose
typed provider-specific options through compiled extensions, but raw provider schemas
do not enter `magelift.yaml`.

AWS ECS Fargate is the only certified v1 target. A future AWS EKS or generic Kubernetes
runtime must implement the target and capability interfaces and pass the same
application-level acceptance suite. Other clouds follow the same rule and keep their
managed-service choices explicit.

MageLift will not claim multi-cloud support until at least two providers pass that
suite.

Local development is a separate execution context. It uses local capability
implementations and the same application lifecycle, but it does not implement the
`Target` interface and does not create Pulumi state. This keeps development fast and
offline while preserving an honest boundary around tests that require AWS behavior.

## Consequences

Projects on the certified path remain YAML-only. Advanced teams can compile an
extension when they need supported imports or resource transforms.

Provider implementations may differ substantially. A queue capability can use Amazon
MQ on AWS and a different managed service elsewhere. The interface captures Magento's
requirements; it does not pretend those services have identical operations.

Some configuration cannot move unchanged between targets. This is intentional. The
portable application and build contracts survive a target change, while topology,
cost, recovery procedures, and provider controls remain target-specific.

FrankenPHP classic is available as an image adapter and has container-level tests.
It remains uncertified for Magento production use until the full Magento acceptance
matrix passes. Nginx with PHP-FPM remains the default certified web runtime.

## Alternatives considered

- A universal infrastructure schema was rejected because it would expose only the
  features shared by every provider.
- Raw Pulumi or cloud-provider options in project YAML were rejected because they make
  the public configuration contract unstable and difficult to validate.
- Shipping several providers before one target is certified was rejected because the
  test and operational evidence would be too shallow.

## Provenance

This is an original project decision informed by public cloud and Pulumi extension
models. No third-party source code was used.
