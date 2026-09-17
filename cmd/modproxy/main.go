// Command modproxy stages the MageLift modules from a working tree into a
// file proxy layout, so release sums can be computed before any tag
// exists. The staged zips are content-identical to what the tags will
// serve, so sums computed here match the published sums exactly and all
// module tags can push at once (staggered tag-then-request poisons the
// public proxy's negative cache; see the release skill).
//
// Usage: modproxy --root <repo> --version <v> --out <dir>
package main

import (
	"bytes"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/mod/module"
	modzip "golang.org/x/mod/zip"
)

var nestedModules = []string{"providers", "sdk"}

var skipTopLevel = map[string]bool{
	".git": true, "dist": true, ".venv": true, "node_modules": true, "website": true,
}

func main() {
	root := flag.String("root", "", "repository root holding go.work")
	version := flag.String("version", "", "version to stage (e.g. v0.1.0-alpha.1-rc.6)")
	out := flag.String("out", "", "proxy directory to write")
	flag.Parse()
	if err := run(*root, *version, *out); err != nil {
		fmt.Fprintf(os.Stderr, "modproxy: %v\n", err)
		os.Exit(1)
	}
}

func run(root, version, out string) error {
	if strings.TrimSpace(root) == "" || strings.TrimSpace(version) == "" || strings.TrimSpace(out) == "" {
		return fmt.Errorf("--root, --version, and --out are all required")
	}
	staged := []struct{ module, dir string }{}
	stage, err := os.MkdirTemp("", "modproxy-root")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	if err := copyTree(root, stage, true); err != nil {
		return fmt.Errorf("stage root: %w", err)
	}
	staged = append(staged,
		struct{ module, dir string }{"github.com/magelift/magelift", stage},
		struct {
			module, dir string
		}{"github.com/magelift/magelift/sdk", filepath.Join(root, "sdk")},
		struct {
			module, dir string
		}{"github.com/magelift/magelift/providers/gcp", filepath.Join(root, "providers", "gcp")},
	)
	for _, module := range staged {
		if err := publish(out, module.module, version, module.dir); err != nil {
			return err
		}
	}
	return nil
}

// copyTree copies src to dst. At the top level it skips nested Go
// modules (a parent zip must not contain them) plus caches and build
// outputs; symlinks and non-regular files never enter a staged tree.
func copyTree(src, dst string, top bool) error {
	return filepath.WalkDir(src, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if relative == "." {
			return os.MkdirAll(dst, 0o755)
		}
		first := relative
		if index := strings.IndexByte(relative, filepath.Separator); index >= 0 {
			first = relative[:index]
		}
		if top && (skipTopLevel[first] || isNestedModule(first)) {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		target := filepath.Join(dst, relative)
		if entry.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		contents, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, contents, 0o644)
	})
}

func isNestedModule(name string) bool {
	for _, nested := range nestedModules {
		if name == nested {
			return true
		}
	}
	return false
}

func publish(proxy, modulePath, version, srcDir string) error {
	escaped, err := module.EscapePath(modulePath)
	if err != nil {
		return err
	}
	escapedVersion, err := module.EscapeVersion(version)
	if err != nil {
		return err
	}
	atDir := filepath.Join(proxy, filepath.FromSlash(escaped), "@v")
	if err := os.MkdirAll(atDir, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(atDir, escapedVersion+".info"), []byte(`{"Version":"`+version+`"}`), 0o644); err != nil {
		return err
	}
	var archive bytes.Buffer
	if err := modzip.CreateFromDir(&archive, module.Version{Path: modulePath, Version: version}, srcDir); err != nil {
		return fmt.Errorf("zip %s: %w", modulePath, err)
	}
	if err := os.WriteFile(filepath.Join(atDir, escapedVersion+".zip"), archive.Bytes(), 0o644); err != nil {
		return err
	}
	goMod, err := os.ReadFile(filepath.Join(srcDir, "go.mod"))
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(atDir, escapedVersion+".mod"), goMod, 0o644)
}
