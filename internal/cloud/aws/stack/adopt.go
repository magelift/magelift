package stack

import (
	"fmt"

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

// AdoptReport lists existing resources MageLift references without owning.
// Database adopt fields are out of scope for the network slice (08-02/03).
func AdoptReport(spec Spec) []AdoptEntry {
	if spec.Existing.Network == nil {
		return nil
	}
	ref := *spec.Existing.Network
	label := string(ref.ID)
	if label == "" {
		label = "network"
	}
	return []AdoptEntry{{
		Kind:       string(sdk.ExistingNetwork),
		ExternalID: ref.ExternalID,
		Label:      label,
	}}
}

// RefuseAdoptedMutation fails closed when intent is destroy or replace of an
// adopted network MageLift does not own (D-02 / ATTACH-03). Preview and
// Magento-scoped update/destroy of MageLift-owned children pass Intent "".
func RefuseAdoptedMutation(spec Spec, intent platform.AdoptMutationIntent) error {
	return refuseAdoptedMutation(spec, intent)
}

func refuseAdoptedMutation(spec Spec, intent platform.AdoptMutationIntent) error {
	if intent != platform.AdoptIntentDestroy && intent != platform.AdoptIntentReplace {
		return nil
	}
	ref := spec.Existing.Network
	if ref == nil {
		return nil
	}
	label := string(ref.ID)
	if label == "" {
		label = "network"
	}
	return fmt.Errorf("adopted resource %s (%s): MageLift does not own this resource", label, ref.ExternalID)
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
