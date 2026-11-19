package platform

import (
	"fmt"
	"strings"

	"github.com/magelift/magelift/sdk"
)

// AdoptMutationIntent names a caller-declared action against an adopted resource.
type AdoptMutationIntent string

const (
	// AdoptIntentDestroy refuses destroy of an adopted resource MageLift does not own.
	AdoptIntentDestroy AdoptMutationIntent = "destroy"
	// AdoptIntentReplace refuses replace of an adopted resource MageLift does not own.
	AdoptIntentReplace AdoptMutationIntent = "replace"
)

// BrownfieldAttach is implemented by planned stacks that reference operator-owned
// cloud resources. Preview surfaces AdoptedResourceLines; mutate paths consult
// RefuseAdoptedMutation before provider calls.
type BrownfieldAttach interface {
	AdoptedResourceLines() []string
	RefuseAdoptedMutation(intent AdoptMutationIntent) error
}

type AdoptedResources []sdk.AdoptedResource

func (resources AdoptedResources) AdoptedResourceLines() []string {
	var lines []string
	for _, entry := range resources {
		lines = append(lines, entry.Line())
	}
	return lines
}

func (resources AdoptedResources) RefuseAdoptedMutation(intent AdoptMutationIntent) error {
	if len(resources) == 0 || intent != AdoptIntentDestroy && intent != AdoptIntentReplace {
		return nil
	}
	var parts []string
	for _, entry := range resources {
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
