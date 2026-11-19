package stack

import (
	"github.com/magelift/magelift/internal/platform"
	"github.com/magelift/magelift/sdk"
)

type AdoptEntry = sdk.AdoptedResource

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

// RefuseAdoptedMutation protects resources referenced from the shop's existing account.
func RefuseAdoptedMutation(spec Spec, intent platform.AdoptMutationIntent) error {
	return platform.AdoptedResources(AdoptReport(spec)).RefuseAdoptedMutation(intent)
}

func (p Planned) AdoptedResourceLines() []string {
	return platform.AdoptedResources(AdoptReport(p.Spec)).AdoptedResourceLines()
}

func (p Planned) RefuseAdoptedMutation(intent platform.AdoptMutationIntent) error {
	return RefuseAdoptedMutation(p.Spec, intent)
}
