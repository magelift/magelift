# MageLift build package

`magelift/build` plans Magento and Composer steps, validates hooks, and writes
artifact manifests. The PHP runner prepares the shop. The Go pipeline packages
and promotes the OCI image.

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

Each Magento command is an argument array. The
artifact manifest records the source revision, OCI digest, versions, enabled modules,
checksums, static content, runtime capabilities, and compatibility status.

Run `composer install` followed by `composer test`.
