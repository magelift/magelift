package platform

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"

	"github.com/magelift/magelift/internal/config"
	"github.com/magelift/magelift/sdk"
)

// RegisterServiceAdapter links a connection resolver into the current binary.
func (r *ModuleRegistry) RegisterServiceAdapter(adapter sdk.ServiceAdapter) error {
	if r == nil || adapter == nil {
		return errors.New("service adapter and module registry are required")
	}
	descriptor := adapter.ServiceDescriptor()
	if err := sdk.ValidateServiceAdapterDescriptor(descriptor); err != nil {
		return err
	}
	if _, exists := r.serviceAdapters[descriptor.ID]; exists {
		return fmt.Errorf("service adapter %q is already registered", descriptor.ID)
	}
	if r.serviceAdapters == nil {
		r.serviceAdapters = make(map[string]sdk.ServiceAdapter)
	}
	r.serviceAdapters[descriptor.ID] = adapter
	return nil
}

// ServiceAdapters lists the connection adapters explicitly linked into this binary.
func (r *ModuleRegistry) ServiceAdapters() []sdk.ServiceAdapterDescriptor {
	if r == nil {
		return nil
	}
	var descriptors []sdk.ServiceAdapterDescriptor
	for _, id := range slices.Sorted(maps.Keys(r.serviceAdapters)) {
		d := r.serviceAdapters[id].ServiceDescriptor()
		d.Roles = slices.Clone(d.Roles)
		d.Targets = slices.Clone(d.Targets)
		descriptors = append(descriptors, d)
	}
	return descriptors
}

func (r *ModuleRegistry) resolveServiceAdapters(ctx context.Context, cfg config.Config, environment string, target sdk.TargetDescriptor) (config.Config, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	cfg.Services = maps.Clone(cfg.Services)
	for _, role := range slices.Sorted(maps.Keys(cfg.Services)) {
		input := cfg.Services[role]
		if input.Adapter == "" {
			continue
		}
		adapter, ok := r.serviceAdapters[input.Adapter]
		if !ok {
			return cfg, fmt.Errorf("services.%s selects %q: link the service adapter into your MageLift binary", role, input.Adapter)
		}
		d := adapter.ServiceDescriptor()
		if !slices.Contains(d.Targets, target) || !slices.Contains(d.Roles, role) {
			return cfg, fmt.Errorf("service adapter %q does not handle %s on %s", d.ID, role, target.ID)
		}
		options, err := json.Marshal(cfg.Extensions[d.ID])
		if err != nil {
			return cfg, fmt.Errorf("encode extensions.%s: %w", d.ID, err)
		}
		output, err := adapter.ResolveService(ctx, sdk.ServiceRequest{
			Project: cfg.Project.Name, Environment: environment, Target: target, Role: role, Binding: input, Options: options,
		})
		if err != nil {
			return cfg, fmt.Errorf("resolve services.%s with %s: %w", role, d.ID, err)
		}
		if err := config.ValidateAdapterConnection(role, output); err != nil {
			return cfg, fmt.Errorf("service adapter %q: %w", d.ID, err)
		}
		if output.Version == "" {
			output.Version = input.Version
		} else if input.Version != "" && output.Version != input.Version {
			return cfg, fmt.Errorf("service adapter %q changed the selected %s version", d.ID, role)
		}
		cfg.Services[role] = output
	}
	return cfg, nil
}
