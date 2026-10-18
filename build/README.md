# MageLift Build

`magelift/build` contains the lifecycle contracts used to describe a Magento build
and deployment workflow.

This package defines the Magento and Composer lifecycle contracts, validates hooks,
and creates deterministic artifact manifests. The native runner executes the
validated preparation graph; the Go pipeline owns OCI packaging and promotion.

## Lifecycle model

A step has a stable ID, a lifecycle phase, dependency IDs, a timeout, retry
metadata, and a failure action. `LifecycleGraph` validates references and cycles,
then returns steps in deterministic dependency order.

Stable IDs use lowercase dot-separated segments, for example
`build.composer-install`. IDs are contracts: integrations should not derive them
from display labels or shell commands.

Hooks attach steps with `before`, `after`, `replace`, or `disable` relationships.
The graph rewrites dependency edges and rejects missing targets and cycles.

```php
use MageLift\Build\Lifecycle\FailureAction;
use MageLift\Build\Lifecycle\LifecycleGraph;
use MageLift\Build\Lifecycle\Phase;
use MageLift\Build\Lifecycle\RetryPolicy;
use MageLift\Build\Lifecycle\Step;

$graph = new LifecycleGraph([
    new Step('validate.composer', Phase::Validate),
    new Step(
        'build.composer-install',
        Phase::Build,
        ['validate.composer'],
        900,
        new RetryPolicy(2, 5),
        FailureAction::Abort,
    ),
]);

$orderedSteps = $graph->orderedSteps();
```

Magento commands are represented as argument arrays rather than shell strings. The
artifact manifest records the source revision, OCI digest, versions, enabled modules,
checksums, static content, runtime capabilities, and compatibility status.

Run `composer install` followed by `composer test`.
