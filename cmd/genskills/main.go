// Command genskills writes the release-visible manifest for the embedded
// first-party skills. The runtime still derives and verifies the same digest
// from the embedded files.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	skillbundle "github.com/magelift/magelift/internal/skills"
)

func main() {
	manifest, err := skillbundle.BundledManifest()
	if err != nil {
		panic(err)
	}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		panic(err)
	}
	data = append(data, '\n')
	workingDirectory, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	path := filepath.Join(workingDirectory, "agents", "manifest.json")
	if filepath.Base(workingDirectory) == "agents" {
		path = filepath.Join(workingDirectory, "manifest.json")
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		panic(fmt.Errorf("write %s: %w", path, err))
	}
}
