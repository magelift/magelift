package v1

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

const CleanupLedgerVersion = 1

// Cleanup resource statuses are durable. Intended is written before the
// provider create call so an interrupted process can still be reconciled.
const (
	CleanupStatusIntended  = "intended"
	CleanupStatusClaimed   = "claimed"
	CleanupStatusDeleting  = "deleting"
	CleanupStatusGone      = "gone"
	CleanupStatusTombstone = "tombstone"
	CleanupStatusRefused   = "refused"
)

const (
	CleanupRoleSource   = "source"
	CleanupRoleRestore  = "restore"
	CleanupRoleSnapshot = "snapshot"
)

const (
	CleanupRankSnapshot = 10
	CleanupRankRestore  = 20
	CleanupRankSource   = 30
	CleanupRankDefault  = 50
)

// CleanupResource is one owned cloud or SaaS object. Identity may be empty
// while the status is intended; Name then binds the claim after create.
type CleanupResource struct {
	Kind     string `json:"kind" yaml:"kind"`
	Role     string `json:"role,omitempty" yaml:"role,omitempty"`
	Name     string `json:"name,omitempty" yaml:"name,omitempty"`
	Identity string `json:"identity,omitempty" yaml:"identity,omitempty"`
	Rank     int    `json:"rank" yaml:"rank"`
	Status   string `json:"status" yaml:"status"`
	Detail   string `json:"detail,omitempty" yaml:"detail,omitempty"`
}

// CleanupLedger is the restartable ownership document. It is the authority
// for interrupted cleanup: process traps are last-mile, not the lifecycle.
type CleanupLedger struct {
	Version   int               `json:"version" yaml:"version"`
	RunID     string            `json:"runId" yaml:"runId"`
	Marker    string            `json:"marker" yaml:"marker"`
	Provider  string            `json:"provider" yaml:"provider"`
	Region    string            `json:"region,omitempty" yaml:"region,omitempty"`
	Project   string            `json:"project,omitempty" yaml:"project,omitempty"`
	Profile   string            `json:"profile,omitempty" yaml:"profile,omitempty"`
	ClaimedAt string            `json:"claimedAt" yaml:"claimedAt"`
	Resources []CleanupResource `json:"resources" yaml:"resources"`
}

// CleanupInventoryResource is a direct owning-service observation. Owned must
// be true before ReconcileCleanup may delete the object.
type CleanupInventoryResource struct {
	Kind      string `json:"kind" yaml:"kind"`
	Role      string `json:"role,omitempty" yaml:"role,omitempty"`
	Name      string `json:"name,omitempty" yaml:"name,omitempty"`
	Identity  string `json:"identity" yaml:"identity"`
	Owned     bool   `json:"owned" yaml:"owned"`
	Live      bool   `json:"live" yaml:"live"`
	Delayed   bool   `json:"delayed" yaml:"delayed"`
	Protected bool   `json:"protected" yaml:"protected"`
}

// CleanupInventoryRequest is the only inventory input a provider adapter sees.
type CleanupInventoryRequest struct {
	Marker   string `json:"marker" yaml:"marker"`
	Provider string `json:"provider" yaml:"provider"`
	Region   string `json:"region,omitempty" yaml:"region,omitempty"`
	Project  string `json:"project,omitempty" yaml:"project,omitempty"`
}

// CleanupInventoryClient queries the service that owns the resources, not a
// local state cache or an eventually consistent secondary index.
type CleanupInventoryClient interface {
	Inventory(context.Context, CleanupInventoryRequest) ([]CleanupInventoryResource, error)
}

// CleanupDeleteClient removes one already-classified owned resource.
type CleanupDeleteClient interface {
	Delete(context.Context, CleanupResource) error
}

// CleanupReconcileOptions bounds mutation and optional durable persistence.
// Persist is invoked after every resource status change so a crash cannot
// lose a successful delete.
type CleanupReconcileOptions struct {
	DryRun  bool
	Persist func(CleanupLedger) error
}

// CleanupReport is the operator-visible result of one reconcile pass.
type CleanupReport struct {
	Status    string            `json:"status" yaml:"status"`
	Marker    string            `json:"marker" yaml:"marker"`
	Provider  string            `json:"provider" yaml:"provider"`
	DryRun    bool              `json:"dryRun" yaml:"dryRun"`
	Deleted   []string          `json:"deleted,omitempty" yaml:"deleted,omitempty"`
	Gone      []string          `json:"gone,omitempty" yaml:"gone,omitempty"`
	Pending   []string          `json:"pending,omitempty" yaml:"pending,omitempty"`
	Refused   []string          `json:"refused,omitempty" yaml:"refused,omitempty"`
	Remaining []CleanupResource `json:"remaining,omitempty" yaml:"remaining,omitempty"`
	Detail    string            `json:"detail,omitempty" yaml:"detail,omitempty"`
	Ledger    CleanupLedger     `json:"ledger" yaml:"ledger"`
}

// DefaultCleanupRank returns the deletion order for a resource role or kind.
// Snapshots and other derived backups are removed before restore outputs,
// which are removed before source instances.
func DefaultCleanupRank(kind, role string) int {
	switch strings.TrimSpace(role) {
	case CleanupRoleSnapshot:
		return CleanupRankSnapshot
	case CleanupRoleRestore:
		return CleanupRankRestore
	case CleanupRoleSource:
		return CleanupRankSource
	}
	switch strings.TrimSpace(kind) {
	case "rdb-snapshot", "snapshot", "backup", "cloudsql-backup":
		return CleanupRankSnapshot
	case "rdb-instance", "instance", "cloudsql-instance", "database-instance":
		return CleanupRankSource
	default:
		return CleanupRankDefault
	}
}

// ValidateCleanupLedger checks the durable document before any provider call.
func ValidateCleanupLedger(ledger CleanupLedger) error {
	if ledger.Version != CleanupLedgerVersion {
		return fmt.Errorf("cleanup ledger version %d is unsupported", ledger.Version)
	}
	if err := requireCleanupLine("cleanup ledger run ID", ledger.RunID); err != nil {
		return err
	}
	if err := requireCleanupLine("cleanup ledger ownership marker", ledger.Marker); err != nil {
		return err
	}
	if err := requireCleanupLine("cleanup ledger provider", ledger.Provider); err != nil {
		return err
	}
	if err := optionalCleanupLine("cleanup ledger region", ledger.Region); err != nil {
		return err
	}
	if err := optionalCleanupLine("cleanup ledger project", ledger.Project); err != nil {
		return err
	}
	if err := optionalCleanupLine("cleanup ledger profile", ledger.Profile); err != nil {
		return err
	}
	if err := requireCleanupLine("cleanup ledger claimed-at", ledger.ClaimedAt); err != nil {
		return err
	}
	if _, err := time.Parse(time.RFC3339, ledger.ClaimedAt); err != nil {
		return fmt.Errorf("cleanup ledger claimed-at must be RFC3339: %w", err)
	}
	if sensitiveField.MatchString(ledger.RunID + ledger.Marker + ledger.Provider + ledger.Region + ledger.Project + ledger.Profile) {
		return errors.New("cleanup ledger identity fields must not contain secret-bearing names")
	}
	seen := make(map[string]struct{}, len(ledger.Resources))
	for i, resource := range ledger.Resources {
		if err := validateCleanupResource(resource); err != nil {
			return fmt.Errorf("cleanup ledger resource %d: %w", i, err)
		}
		key := cleanupResourceKey(resource)
		if key == "" {
			return fmt.Errorf("cleanup ledger resource %d needs a name or identity", i)
		}
		if _, exists := seen[key]; exists {
			return fmt.Errorf("cleanup ledger resource %q is duplicated", key)
		}
		seen[key] = struct{}{}
	}
	return nil
}

// ReconcileCleanup unions the durable ledger with a direct owning-service
// inventory, deletes owned live resources in rank order, and is safe to call
// again after interruption. Foreign and unmarked resources are never deleted.
func ReconcileCleanup(ctx context.Context, ledger CleanupLedger, inventory CleanupInventoryClient, deleter CleanupDeleteClient, options CleanupReconcileOptions) (CleanupReport, error) {
	if ctx == nil {
		return CleanupReport{}, errors.New("cleanup reconcile context is required")
	}
	if err := ValidateCleanupLedger(ledger); err != nil {
		return CleanupReport{}, err
	}
	if inventory == nil {
		return CleanupReport{}, errors.New("cleanup inventory client is required")
	}
	if !options.DryRun && deleter == nil {
		return CleanupReport{}, errors.New("cleanup delete client is required unless dry-run is set")
	}

	observed, err := inventory.Inventory(ctx, CleanupInventoryRequest{
		Marker:   ledger.Marker,
		Provider: ledger.Provider,
		Region:   ledger.Region,
		Project:  ledger.Project,
	})
	if err != nil {
		return CleanupReport{}, fmt.Errorf("inventory owned cleanup resources: %w", err)
	}
	if err := validateCleanupInventory(observed); err != nil {
		return CleanupReport{}, err
	}

	ledger = mergeCleanupInventory(ledger, observed)
	if err := persistCleanupLedger(ledger, options); err != nil {
		return CleanupReport{}, err
	}

	byIdentity := indexCleanupInventory(observed)
	sorted := append([]CleanupResource(nil), ledger.Resources...)
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].Rank != sorted[j].Rank {
			return sorted[i].Rank < sorted[j].Rank
		}
		left := cleanupResourceKey(sorted[i])
		right := cleanupResourceKey(sorted[j])
		return left < right
	})

	report := CleanupReport{
		Marker:   ledger.Marker,
		Provider: ledger.Provider,
		DryRun:   options.DryRun,
		Ledger:   ledger,
	}

	for _, resource := range sorted {
		index := cleanupResourceIndex(ledger.Resources, resource)
		if index < 0 {
			continue
		}
		current := ledger.Resources[index]
		observation, found := lookupCleanupInventory(byIdentity, current)
		switch {
		case current.Status == CleanupStatusGone:
			report.Gone = append(report.Gone, cleanupResourceKey(current))
			continue
		case current.Status == CleanupStatusRefused:
			report.Refused = append(report.Refused, cleanupResourceKey(current))
			continue
		case current.Status == CleanupStatusTombstone:
			report.Pending = append(report.Pending, cleanupResourceKey(current))
			continue
		case found && !observation.Owned:
			current.Status = CleanupStatusRefused
			current.Detail = "owning-service inventory did not confirm ownership"
			ledger.Resources[index] = current
			report.Refused = append(report.Refused, cleanupResourceKey(current))
			if err := persistCleanupLedger(ledger, options); err != nil {
				return CleanupReport{}, err
			}
			continue
		case found && observation.Protected:
			current.Status = CleanupStatusClaimed
			current.Detail = "resource is protected; deletion was not attempted"
			ledger.Resources[index] = current
			report.Pending = append(report.Pending, cleanupResourceKey(current))
			if err := persistCleanupLedger(ledger, options); err != nil {
				return CleanupReport{}, err
			}
			continue
		case found && observation.Delayed:
			current.Status = CleanupStatusTombstone
			current.Detail = "provider-mandated deletion tombstone"
			ledger.Resources[index] = current
			report.Pending = append(report.Pending, cleanupResourceKey(current))
			if err := persistCleanupLedger(ledger, options); err != nil {
				return CleanupReport{}, err
			}
			continue
		case !found && current.Status == CleanupStatusIntended:
			continue
		case !found && current.Identity == "" && current.Status == CleanupStatusClaimed:
			report.Pending = append(report.Pending, cleanupResourceKey(current))
			continue
		case !found && (current.Status == CleanupStatusClaimed || current.Status == CleanupStatusDeleting):
			current.Status = CleanupStatusGone
			current.Detail = "owning-service inventory no longer contains this resource"
			ledger.Resources[index] = current
			report.Gone = append(report.Gone, cleanupResourceKey(current))
			if err := persistCleanupLedger(ledger, options); err != nil {
				return CleanupReport{}, err
			}
			continue
		case found && observation.Live:
			if options.DryRun {
				report.Pending = append(report.Pending, cleanupResourceKey(current))
				continue
			}
			current.Status = CleanupStatusDeleting
			ledger.Resources[index] = current
			if err := persistCleanupLedger(ledger, options); err != nil {
				return CleanupReport{}, err
			}
			if err := deleter.Delete(ctx, current); err != nil {
				current.Detail = err.Error()
				if strings.ContainsAny(current.Detail, "\r\n\x00") {
					current.Detail = "provider delete failed"
				}
				ledger.Resources[index] = current
				_ = persistCleanupLedger(ledger, options)
				return CleanupReport{}, fmt.Errorf("delete cleanup resource %q: %w", cleanupResourceKey(current), err)
			}
			current.Status = CleanupStatusGone
			current.Detail = "deleted through owning-service inventory"
			ledger.Resources[index] = current
			report.Deleted = append(report.Deleted, cleanupResourceKey(current))
			if err := persistCleanupLedger(ledger, options); err != nil {
				return CleanupReport{}, err
			}
		}
	}

	final, err := inventory.Inventory(ctx, CleanupInventoryRequest{
		Marker:   ledger.Marker,
		Provider: ledger.Provider,
		Region:   ledger.Region,
		Project:  ledger.Project,
	})
	if err != nil {
		return CleanupReport{}, fmt.Errorf("verify cleanup inventory: %w", err)
	}
	if err := validateCleanupInventory(final); err != nil {
		return CleanupReport{}, err
	}

	live := make([]string, 0)
	delayed := make([]string, 0)
	for _, observation := range final {
		if !observation.Owned {
			continue
		}
		if observation.Live {
			live = append(live, observation.Identity)
		}
		if observation.Delayed {
			delayed = append(delayed, observation.Identity)
		}
	}
	sort.Strings(live)
	sort.Strings(delayed)
	report.Ledger = ledger
	report.Remaining = remainingCleanupResources(ledger)
	activePending := pendingCleanupKeys(report.Ledger, report.Pending)
	switch {
	case len(report.Refused) > 0:
		report.Status = "failed"
		report.Detail = "owned live resources or refused identities remain"
	case options.DryRun && (len(live) > 0 || len(activePending) > 0):
		report.Status = "pending"
		report.Detail = "dry-run; owned live resources were not deleted"
		report.Pending = uniqueCleanupKeys(append(activePending, live...))
	case len(live) > 0:
		report.Status = "failed"
		report.Detail = "owned live resources or refused identities remain"
	case len(delayed) > 0 || len(activePending) > 0:
		report.Status = "pending"
		report.Detail = "provider deletion tombstones, protected resources, or unbound claims remain"
		report.Pending = uniqueCleanupKeys(append(activePending, delayed...))
	default:
		report.Status = "complete"
	}
	return report, nil
}

func validateCleanupResource(resource CleanupResource) error {
	if err := requireCleanupLine("kind", resource.Kind); err != nil {
		return err
	}
	if err := optionalCleanupLine("role", resource.Role); err != nil {
		return err
	}
	if err := optionalCleanupLine("name", resource.Name); err != nil {
		return err
	}
	if err := optionalCleanupLine("identity", resource.Identity); err != nil {
		return err
	}
	if err := optionalCleanupLine("detail", resource.Detail); err != nil {
		return err
	}
	if resource.Rank < 0 {
		return errors.New("rank cannot be negative")
	}
	switch resource.Status {
	case CleanupStatusIntended, CleanupStatusClaimed, CleanupStatusDeleting, CleanupStatusGone, CleanupStatusTombstone, CleanupStatusRefused:
	default:
		return fmt.Errorf("unsupported cleanup status %q", resource.Status)
	}
	if resource.Status == CleanupStatusIntended && strings.TrimSpace(resource.Name) == "" {
		return errors.New("intended resources require a name")
	}
	if resource.Status != CleanupStatusIntended && strings.TrimSpace(resource.Identity) == "" && strings.TrimSpace(resource.Name) == "" {
		return errors.New("claimed resources require a name or identity")
	}
	if sensitiveField.MatchString(resource.Kind + resource.Role + resource.Name + resource.Identity + resource.Detail) {
		return errors.New("cleanup resource fields must not contain secret-bearing names")
	}
	return nil
}

func validateCleanupInventory(resources []CleanupInventoryResource) error {
	seen := make(map[string]struct{}, len(resources))
	for i, resource := range resources {
		if err := requireCleanupLine("inventory identity", resource.Identity); err != nil {
			return fmt.Errorf("cleanup inventory resource %d: %w", i, err)
		}
		if err := requireCleanupLine("inventory kind", resource.Kind); err != nil {
			return fmt.Errorf("cleanup inventory resource %d: %w", i, err)
		}
		if err := optionalCleanupLine("inventory role", resource.Role); err != nil {
			return fmt.Errorf("cleanup inventory resource %d: %w", i, err)
		}
		if err := optionalCleanupLine("inventory name", resource.Name); err != nil {
			return fmt.Errorf("cleanup inventory resource %d: %w", i, err)
		}
		if resource.Live && resource.Delayed {
			return fmt.Errorf("cleanup inventory resource %q cannot be live and delayed", resource.Identity)
		}
		if _, exists := seen[resource.Identity]; exists {
			return fmt.Errorf("cleanup inventory identity %q is duplicated", resource.Identity)
		}
		seen[resource.Identity] = struct{}{}
	}
	return nil
}

func mergeCleanupInventory(ledger CleanupLedger, observed []CleanupInventoryResource) CleanupLedger {
	for _, observation := range observed {
		if !observation.Owned {
			continue
		}
		if index := cleanupInventoryMatch(ledger.Resources, observation); index >= 0 {
			current := ledger.Resources[index]
			if current.Identity == "" {
				current.Identity = observation.Identity
			}
			if current.Name == "" {
				current.Name = observation.Name
			}
			if current.Role == "" {
				current.Role = observation.Role
			}
			if current.Kind == "" {
				current.Kind = observation.Kind
			}
			if current.Rank == 0 {
				current.Rank = DefaultCleanupRank(current.Kind, current.Role)
			}
			if current.Status == CleanupStatusIntended {
				current.Status = CleanupStatusClaimed
			}
			ledger.Resources[index] = current
			continue
		}
		status := CleanupStatusClaimed
		if observation.Delayed {
			status = CleanupStatusTombstone
		}
		ledger.Resources = append(ledger.Resources, CleanupResource{
			Kind:     observation.Kind,
			Role:     observation.Role,
			Name:     observation.Name,
			Identity: observation.Identity,
			Rank:     DefaultCleanupRank(observation.Kind, observation.Role),
			Status:   status,
			Detail:   "adopted from owning-service inventory",
		})
	}
	return ledger
}

func cleanupInventoryMatch(resources []CleanupResource, observation CleanupInventoryResource) int {
	for i, resource := range resources {
		if resource.Identity != "" && resource.Identity == observation.Identity {
			return i
		}
	}
	for i, resource := range resources {
		if resource.Identity == "" && resource.Kind == observation.Kind && resource.Name != "" && resource.Name == observation.Name {
			return i
		}
	}
	return -1
}

func indexCleanupInventory(observed []CleanupInventoryResource) map[string]CleanupInventoryResource {
	index := make(map[string]CleanupInventoryResource, len(observed)*2)
	for _, observation := range observed {
		index["id:"+observation.Identity] = observation
		if observation.Name != "" {
			index["name:"+observation.Kind+":"+observation.Name] = observation
		}
	}
	return index
}

func lookupCleanupInventory(index map[string]CleanupInventoryResource, resource CleanupResource) (CleanupInventoryResource, bool) {
	if resource.Identity != "" {
		observation, ok := index["id:"+resource.Identity]
		return observation, ok
	}
	if resource.Name != "" {
		observation, ok := index["name:"+resource.Kind+":"+resource.Name]
		return observation, ok
	}
	return CleanupInventoryResource{}, false
}

func cleanupResourceIndex(resources []CleanupResource, wanted CleanupResource) int {
	key := cleanupResourceKey(wanted)
	for i, resource := range resources {
		if cleanupResourceKey(resource) == key {
			return i
		}
	}
	return -1
}

func cleanupResourceKey(resource CleanupResource) string {
	if strings.TrimSpace(resource.Identity) != "" {
		return resource.Kind + ":" + resource.Identity
	}
	if strings.TrimSpace(resource.Name) != "" {
		return resource.Kind + ":" + resource.Name
	}
	return ""
}

func remainingCleanupResources(ledger CleanupLedger) []CleanupResource {
	remaining := make([]CleanupResource, 0)
	for _, resource := range ledger.Resources {
		switch resource.Status {
		case CleanupStatusGone:
			continue
		default:
			remaining = append(remaining, resource)
		}
	}
	return remaining
}

func pendingCleanupKeys(ledger CleanupLedger, pending []string) []string {
	keys := append([]string(nil), pending...)
	for _, resource := range ledger.Resources {
		switch resource.Status {
		case CleanupStatusClaimed, CleanupStatusDeleting, CleanupStatusTombstone:
			keys = append(keys, cleanupResourceKey(resource))
		}
	}
	return uniqueCleanupKeys(keys)
}

func uniqueCleanupKeys(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func persistCleanupLedger(ledger CleanupLedger, options CleanupReconcileOptions) error {
	if options.Persist == nil {
		return nil
	}
	if err := options.Persist(ledger); err != nil {
		return fmt.Errorf("persist cleanup ledger: %w", err)
	}
	return nil
}

func requireCleanupLine(name, value string) error {
	if strings.TrimSpace(value) == "" || strings.ContainsAny(value, "\r\n\x00") {
		return fmt.Errorf("%s is required and must be single-line", name)
	}
	return nil
}

func optionalCleanupLine(name, value string) error {
	if value == "" {
		return nil
	}
	if strings.TrimSpace(value) == "" || strings.ContainsAny(value, "\r\n\x00") {
		return fmt.Errorf("%s must be single-line", name)
	}
	return nil
}
