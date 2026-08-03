package infra

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"

	sdk "github.com/acourtiol/magelift/sdk/v1"
)

type targetKey struct {
	provider sdk.ProviderID
	runtime  sdk.RuntimeID
}

type Registry struct {
	mu                  sync.RWMutex
	targetsByID         map[sdk.TargetID]sdk.Target
	targetsByPlatform   map[targetKey]sdk.Target
	capabilitiesByID    map[sdk.CapabilityProviderID]sdk.CapabilityProvider
	capabilitiesByCloud map[sdk.ProviderID][]sdk.CapabilityProvider
	transformsByID      map[sdk.TransformID]transformEntry
	hooksByID           map[sdk.HookID]sdk.LifecycleHook
}

type transformEntry struct {
	descriptor sdk.TransformDescriptor
	apply      func(context.Context, sdk.ComponentID, sdk.ResourceID, sdk.ResourceOptions) (sdk.ResourceOptions, error)
}

func NewRegistry() *Registry {
	return &Registry{
		targetsByID:         make(map[sdk.TargetID]sdk.Target),
		targetsByPlatform:   make(map[targetKey]sdk.Target),
		capabilitiesByID:    make(map[sdk.CapabilityProviderID]sdk.CapabilityProvider),
		capabilitiesByCloud: make(map[sdk.ProviderID][]sdk.CapabilityProvider),
		transformsByID:      make(map[sdk.TransformID]transformEntry),
		hooksByID:           make(map[sdk.HookID]sdk.LifecycleHook),
	}
}

func (r *Registry) RegisterTarget(target sdk.Target) error {
	if r == nil {
		return errors.New("infrastructure registry is required")
	}
	if target == nil {
		return errors.New("target is required")
	}
	descriptor := target.Descriptor()
	if err := sdk.ValidateTargetDescriptor(descriptor); err != nil {
		return fmt.Errorf("register target: %w", err)
	}
	key := targetKey{provider: descriptor.Provider, runtime: descriptor.Runtime}

	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.targetsByID[descriptor.ID]; exists {
		return fmt.Errorf("target ID %q is already registered", descriptor.ID)
	}
	if _, exists := r.targetsByPlatform[key]; exists {
		return fmt.Errorf("target for provider %q and runtime %q is already registered", descriptor.Provider, descriptor.Runtime)
	}
	r.targetsByID[descriptor.ID] = target
	r.targetsByPlatform[key] = target
	return nil
}

func (r *Registry) Target(provider sdk.ProviderID, runtime sdk.RuntimeID) (sdk.Target, bool) {
	if r == nil {
		return nil, false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	target, found := r.targetsByPlatform[targetKey{provider: provider, runtime: runtime}]
	return target, found
}

func (r *Registry) RegisterCapability(provider sdk.CapabilityProvider) error {
	if r == nil {
		return errors.New("infrastructure registry is required")
	}
	if provider == nil {
		return errors.New("capability provider is required")
	}
	descriptor := provider.Descriptor()
	if err := sdk.ValidateCapabilityDescriptors([]sdk.CapabilityDescriptor{descriptor}); err != nil {
		return fmt.Errorf("register capability provider: %w", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.capabilitiesByID[descriptor.ID]; exists {
		return fmt.Errorf("capability provider ID %q is already registered", descriptor.ID)
	}
	r.capabilitiesByID[descriptor.ID] = provider
	r.capabilitiesByCloud[descriptor.Provider] = append(r.capabilitiesByCloud[descriptor.Provider], provider)
	return nil
}

func (r *Registry) Capabilities(provider sdk.ProviderID) []sdk.CapabilityProvider {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	registered := append([]sdk.CapabilityProvider(nil), r.capabilitiesByCloud[provider]...)
	r.mu.RUnlock()
	sort.Slice(registered, func(i, j int) bool {
		return registered[i].Descriptor().ID < registered[j].Descriptor().ID
	})
	return registered
}

// RegisterTransform keeps the extension implementation typed while allowing
// the registry to dispatch it by stable descriptor at component boundaries.
func RegisterTransform[T sdk.ResourceOptions](r *Registry, transform sdk.Transform[T]) error {
	if r == nil {
		return errors.New("infrastructure registry is required")
	}
	if transform == nil {
		return errors.New("transform is required")
	}
	descriptor := transform.Descriptor()
	if err := sdk.ValidateTransformDescriptor(descriptor); err != nil {
		return fmt.Errorf("register transform: %w", err)
	}
	entry := transformEntry{
		descriptor: descriptor,
		apply: func(ctx context.Context, component sdk.ComponentID, resource sdk.ResourceID, options sdk.ResourceOptions) (sdk.ResourceOptions, error) {
			typed, ok := options.(T)
			if !ok {
				return nil, fmt.Errorf("transform %q received options of type %T, want %T", descriptor.ID, options, typed)
			}
			return transform.Apply(ctx, sdk.TransformInput[T]{Component: component, Resource: resource, Options: typed})
		},
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.transformsByID[descriptor.ID]; exists {
		return fmt.Errorf("transform ID %q is already registered", descriptor.ID)
	}
	r.transformsByID[descriptor.ID] = entry
	return nil
}

func ApplyTransform[T sdk.ResourceOptions](ctx context.Context, r *Registry, id sdk.TransformID, component sdk.ComponentID, resource sdk.ResourceID, options T) (T, error) {
	var zero T
	if r == nil {
		return zero, errors.New("infrastructure registry is required")
	}
	r.mu.RLock()
	entry, found := r.transformsByID[id]
	r.mu.RUnlock()
	if !found {
		return zero, fmt.Errorf("transform ID %q is not registered", id)
	}
	if !containsComponent(entry.descriptor.Components, component) {
		return zero, fmt.Errorf("transform %q does not apply to component %q", id, component)
	}
	result, err := entry.apply(ctx, component, resource, options)
	if err != nil {
		return zero, fmt.Errorf("apply transform %q: %w", id, err)
	}
	typed, ok := result.(T)
	if !ok {
		return zero, fmt.Errorf("transform %q returned options of type %T, want %T", id, result, zero)
	}
	return typed, nil
}

func (r *Registry) RegisterLifecycleHook(hook sdk.LifecycleHook) error {
	if r == nil {
		return errors.New("infrastructure registry is required")
	}
	if hook == nil {
		return errors.New("lifecycle hook is required")
	}
	descriptor := hook.Descriptor()
	if err := sdk.ValidateLifecycleHooks([]sdk.LifecycleHookDescriptor{descriptor}); err != nil {
		return fmt.Errorf("register lifecycle hook: %w", err)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.hooksByID[descriptor.ID]; exists {
		return fmt.Errorf("lifecycle hook ID %q is already registered", descriptor.ID)
	}
	r.hooksByID[descriptor.ID] = hook
	return nil
}

func (r *Registry) LifecycleHooks(phase sdk.LifecyclePhase) []sdk.LifecycleHook {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	registered := make([]sdk.LifecycleHook, 0)
	for _, hook := range r.hooksByID {
		if hook.Descriptor().Phase == phase {
			registered = append(registered, hook)
		}
	}
	r.mu.RUnlock()
	sort.Slice(registered, func(i, j int) bool { return registered[i].Descriptor().ID < registered[j].Descriptor().ID })
	return registered
}

func containsComponent(components []sdk.ComponentID, wanted sdk.ComponentID) bool {
	for _, component := range components {
		if component == wanted {
			return true
		}
	}
	return false
}
