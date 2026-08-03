package platform

import (
	"context"
	"time"
)

// BootstrapRequest is Magento-shaped account preparation input.
// Provider product fields stay inside adapters.
type BootstrapRequest struct {
	AccessLogBucket string
	GitHubOwner     string
	GitHubRepo      string
}

// BootstrapResult is opaque DIY-backend metadata for CLI display.
type BootstrapResult struct {
	BackendURL string         `json:"backendURL" yaml:"backendURL"`
	KeyRef     string         `json:"keyRef,omitempty" yaml:"keyRef,omitempty"`
	Details    map[string]any `json:"details,omitempty" yaml:"details,omitempty"`
}

// Bootstrap prepares the Pulumi DIY backend and optional CI identity.
type Bootstrap interface {
	VerifyAccount(ctx context.Context, planned PlannedStack) error
	Ensure(ctx context.Context, planned PlannedStack, req BootstrapRequest) (BootstrapResult, error)
}

// HasBootstrap is implemented by StackModules that own account bootstrap.
type HasBootstrap interface {
	Bootstrap() Bootstrap
}

// ModuleBootstrap returns Bootstrap when the module implements it.
func ModuleBootstrap(module StackModule) Bootstrap {
	if module == nil {
		return nil
	}
	if provider, ok := module.(HasBootstrap); ok {
		return provider.Bootstrap()
	}
	return nil
}

// LockInfo describes a DIY deployment lock.
type LockInfo struct {
	Project     string    `json:"project" yaml:"project"`
	Environment string    `json:"environment" yaml:"environment"`
	Owner       string    `json:"owner" yaml:"owner"`
	AcquiredAt  time.Time `json:"acquiredAt" yaml:"acquiredAt"`
}

// BackupResult is opaque backup metadata.
type BackupResult struct {
	ID       string `json:"id,omitempty" yaml:"id,omitempty"`
	Location string `json:"location" yaml:"location"`
	ETag     string `json:"etag,omitempty" yaml:"etag,omitempty"`
}

// RestoreResult is opaque restore metadata.
type RestoreResult struct {
	ID       string `json:"id,omitempty" yaml:"id,omitempty"`
	Location string `json:"location" yaml:"location"`
}

// State manages DIY Pulumi backend locks and snapshots for an environment.
type State interface {
	Status(ctx context.Context, planned PlannedStack) (locked bool, info *LockInfo, backend string, err error)
	Lock(ctx context.Context, planned PlannedStack, owner string) (release func(context.Context) error, err error)
	Unlock(ctx context.Context, planned PlannedStack) (*LockInfo, error)
	Backup(ctx context.Context, planned PlannedStack) (BackupResult, error)
	Restore(ctx context.Context, planned PlannedStack, location string) (RestoreResult, error)
}

// HasState is implemented by StackModules that expose DIY state ops.
type HasState interface {
	State() State
}

// ModuleState returns State when the module implements it.
func ModuleState(module StackModule) State {
	if module == nil {
		return nil
	}
	if provider, ok := module.(HasState); ok {
		return provider.State()
	}
	return nil
}

// SecretMeta is a Magento application secret listing entry.
type SecretMeta struct {
	Name      string    `json:"name" yaml:"name"`
	UpdatedAt time.Time `json:"updatedAt,omitempty" yaml:"updatedAt,omitempty"`
}

// Secrets manages Magento application secrets (not Pulumi config).
type Secrets interface {
	List(ctx context.Context, planned PlannedStack) ([]SecretMeta, error)
	Set(ctx context.Context, planned PlannedStack, name string, value []byte) error
	Remove(ctx context.Context, planned PlannedStack, name string) error
}

// HasSecrets is implemented by StackModules that expose Secrets.
type HasSecrets interface {
	Secrets() Secrets
}

// ModuleSecrets returns Secrets when the module implements it.
func ModuleSecrets(module StackModule) Secrets {
	if module == nil {
		return nil
	}
	if provider, ok := module.(HasSecrets); ok {
		return provider.Secrets()
	}
	return nil
}
