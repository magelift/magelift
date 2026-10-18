package bootstrap

import (
	"context"
	"fmt"

	"google.golang.org/api/option"
)

// NewAccountPreparerFromContext wires production Google clients with optional ClientOptions.
func NewAccountPreparerFromContext(ctx context.Context, opts ...option.ClientOption) (*AccountPreparer, error) {
	apis, err := NewServiceEnablementClient(ctx, opts...)
	if err != nil {
		return nil, err
	}
	registry, err := NewArtifactRegistryClient(ctx, opts...)
	if err != nil {
		_ = apis.Close()
		return nil, err
	}
	secrets, err := NewStateSecretClient(ctx, opts...)
	if err != nil {
		_ = apis.Close()
		_ = registry.Close()
		return nil, err
	}
	checkpoints, err := newGCSCheckpointReader(ctx, opts...)
	if err != nil {
		_ = apis.Close()
		_ = registry.Close()
		_ = secrets.Close()
		return nil, err
	}
	locks, err := newGCSStateLockFactory(ctx, opts...)
	if err != nil {
		_ = apis.Close()
		_ = registry.Close()
		_ = secrets.Close()
		_ = checkpoints.Close()
		return nil, err
	}
	preparer, err := NewAccountPreparer(apis, registry, secrets, checkpoints, locks)
	if err != nil {
		_ = apis.Close()
		_ = registry.Close()
		_ = secrets.Close()
		_ = checkpoints.Close()
		_ = locks.Close()
		return nil, fmt.Errorf("create account preparer: %w", err)
	}
	return preparer, nil
}
