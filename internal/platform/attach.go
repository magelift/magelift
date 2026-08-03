package platform

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
