// Package agents exposes MageLift's first-party agent skills to the CLI.
package agents

import "embed"

//go:generate go run ../cmd/genskills

// Files contains the tracked first-party skill files. The CLI copies these
// files without executing their contents.
//
//go:embed skills/*/SKILL.md
var Files embed.FS
