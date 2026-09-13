# New Relic NerdGraph operational-object acceptance — 2026-08-13

Status: **PASS for the bounded provider-local operational-object cell; not a
full New Relic observability certification**.

The disposable cell used the authenticated New Relic `default` profile for a
redacted account in the EU NerdGraph region. The Go provider adapter received
the user key only through the secret-reference boundary. The acceptance marker
was `magelift/acceptance/newrelic-operations/20260813-nrops-9`; its derived
ownership digest was `31c68f3d53dfc5a0`.

The command was:

```sh
MAGELIFT_NEWRELIC_OPERATIONS_ACCEPTANCE=1 \
MAGELIFT_NEWRELIC_PROFILE=default \
MAGELIFT_NEWRELIC_ACCOUNT_ID=<redacted> \
MAGELIFT_NEWRELIC_NERDGRAPH_ENDPOINT=https://api.eu.newrelic.com/graphql \
MAGELIFT_NEWRELIC_OPERATIONS_RUN_ID=20260813-nrops-9 \
MAGELIFT_NEWRELIC_OPERATIONS_MARKER=magelift/acceptance/newrelic-operations/20260813-nrops-9 \
./scripts/newrelic-operations-acceptance-local.sh
```

The live result was:

```text
newrelic operations acceptance PASS account=<redacted> marker=magelift/acceptance/newrelic-operations/20260813-nrops-9 alerts=true dashboards=true slos=true cleanup=verified
```

The provider-local adapter created and verified, under the exact ownership
marker:

- one `PER_CONDITION` alert policy and one warning NRQL condition for the
  `metrics` signal, with the declared `gt` operator, 60-second aggregation
  window, owner, HTTPS runbook, and deduplication metadata;
- one private dashboard with a signal page and a metrics billboard widget
  whose query carried the same ownership marker; and
- one application-health service level with a one-day rolling window and a
  99.9% target against the discovered disposable service entity.

The Go command verified the returned provider configuration before reporting
success, then destroyed service levels, dashboards, alert conditions, and the
alert policy through their owning NerdGraph APIs. The final provider-local
inventory was empty for this marker. A separate read-only audit found zero
active policy, condition, dashboard-parent, or service-level objects with the
same derived digest. Dashboard page entities are provider index records and
are not treated as independently owned resources; the adapter polls the exact
parent dashboard and uses `actor.entity` to distinguish a live parent from a
logical-delete tombstone.

This cell also exercised the failure boundary for partial NerdGraph mutation
responses: New Relic can return an object in GraphQL data while also returning
a top-level error, so the adapter records any returned identity immediately
and rolls it back before surfacing the operation error.

This evidence does **not** claim New Relic collector deployment on ECS or
Kubernetes, alert notification delivery or firing, retention enforcement,
redaction across provider pipelines, native-provider composition, or
provider-wide architecture coverage. Those remain open in the observability
and certification matrix.

