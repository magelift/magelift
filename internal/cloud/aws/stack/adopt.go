package stack

import (
	"fmt"
	"strings"

	"github.com/acourtiol/magelift/internal/platform"
	sdk "github.com/acourtiol/magelift/sdk/v1"
)

// AdoptEntry is one operator-visible adopted resource (D-01 / ATTACH-01).
type AdoptEntry struct {
	Kind       string
	ExternalID string
	Label      string
}

// Line returns the stable ADOPT preview line (e.g. "ADOPT network vpc-…").
func (e AdoptEntry) Line() string {
	return fmt.Sprintf("ADOPT %s %s", e.Kind, e.ExternalID)
}

// AdoptReport lists existing resources MageLift references without owning
// (D-01 / ATTACH-03). Network and database entries are independent.
func AdoptReport(spec Spec) []AdoptEntry {
	var entries []AdoptEntry
	if ref := spec.Existing.Network; ref != nil {
		label := string(ref.ID)
		if label == "" {
			label = "network"
		}
		entries = append(entries, AdoptEntry{
			Kind:       string(sdk.ExistingNetwork),
			ExternalID: ref.ExternalID,
			Label:      label,
		})
	}
	if ref := spec.Existing.Database; ref != nil {
		label := string(ref.ID)
		if label == "" {
			label = "database"
		}
		entries = append(entries, AdoptEntry{
			Kind:       string(sdk.ExistingDatabase),
			ExternalID: ref.ExternalID,
			Label:      label,
		})
	}
	return entries
}

// RefuseAdoptedMutation fails closed when intent is destroy or replace of an
// adopted network or database MageLift does not own (D-02 / ATTACH-03). Preview
// and Magento-scoped update/destroy of MageLift-owned children pass Intent "".
func RefuseAdoptedMutation(spec Spec, intent platform.AdoptMutationIntent) error {
	return refuseAdoptedMutation(spec, intent)
}

func refuseAdoptedMutation(spec Spec, intent platform.AdoptMutationIntent) error {
	if intent != platform.AdoptIntentDestroy && intent != platform.AdoptIntentReplace {
		return nil
	}
	entries := AdoptReport(spec)
	if len(entries) == 0 {
		return nil
	}
	parts := make([]string, 0, len(entries))
	for _, entry := range entries {
		label := entry.Label
		if label == "" {
			label = entry.Kind
		}
		parts = append(parts, fmt.Sprintf("%s (%s)", label, entry.ExternalID))
	}
	noun := "resource"
	if len(parts) > 1 {
		noun = "resources"
	}
	return fmt.Errorf("adopted %s %s: MageLift does not own this resource", noun, strings.Join(parts, ", "))
}

// AdoptedResourceLines implements platform.BrownfieldAttach.
func (p Planned) AdoptedResourceLines() []string {
	entries := AdoptReport(p.Spec)
	if len(entries) == 0 {
		return nil
	}
	lines := make([]string, 0, len(entries))
	for _, entry := range entries {
		lines = append(lines, entry.Line())
	}
	return lines
}

// RefuseAdoptedMutation implements platform.BrownfieldAttach.
func (p Planned) RefuseAdoptedMutation(intent platform.AdoptMutationIntent) error {
	return refuseAdoptedMutation(p.Spec, intent)
}
