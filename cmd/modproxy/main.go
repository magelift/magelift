// Command modproxy stages the MageLift modules from a commit into a file
// proxy layout, so release sums can be computed before any tag exists.
// Staging reads `git archive` output (exactly the tag content) and
// prunes only nested Go modules, mirroring proxy construction: staged
// zips are byte-identical to the tags, so the sums match exactly and
// all module tags can push at once (staggered tag-then-request poisons
// the public proxy's negative cache; see the release skill).
//
// Staging downloads must use an isolated module cache, never the
// shared one: a staged future version under a real version string
// would poison every later honest check on this machine.
//
// Usage: modproxy --root <repo> --ref <commit> --version <v> --out <dir>
package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"golang.org/x/mod/module"
	modzip "golang.org/x/mod/zip"
)

func main() {
	root := flag.String("root", "", "repository holding the modules")
	ref := flag.String("ref", "HEAD", "commit to stage (must be committed)")
	version := flag.String("version", "", "version to stage (e.g. v0.1.0-alpha.1-rc.8)")
	out := flag.String("out", "", "proxy directory to write")
	flag.Parse()
	if err := run(*root, *ref, *version, *out); err != nil {
		fmt.Fprintf(os.Stderr, "modproxy: %v\n", err)
		os.Exit(1)
	}
}

func run(root, ref, version, out string) error {
	if strings.TrimSpace(root) == "" || strings.TrimSpace(version) == "" || strings.TrimSpace(out) == "" {
		return fmt.Errorf("--root, --version, and --out are all required")
	}
	if strings.TrimSpace(ref) == "" {
		ref = "HEAD"
	}
	work, err := os.MkdirTemp("", "modproxy")
	if err != nil {
		return err
	}
	defer os.RemoveAll(work)
	if err := extractArchive(root, ref, ".", work); err != nil {
		return err
	}
	// Prune only nested modules: everything else the archive holds is
	// in the published zip exactly as archived.
	for _, nested := range []string{"providers", "sdk"} {
		if err := os.RemoveAll(filepath.Join(work, nested)); err != nil {
			return err
		}
	}
	staged := []struct{ module, dir string }{
		{"github.com/magelift/magelift", work},
	}
	for _, module := range []struct {
		path, dir string
	}{
		{"github.com/magelift/magelift/sdk", "sdk"},
		{"github.com/magelift/magelift/providers/gcp", "providers/gcp"},
	} {
		sub, err := os.MkdirTemp("", "modproxy-sub")
		if err != nil {
			return err
		}
		defer os.RemoveAll(sub)
		if err := extractArchive(root, ref, module.dir, sub); err != nil {
			return err
		}
		staged = append(staged, struct{ module, dir string }{module.path, filepath.Join(sub, module.dir)})
	}
	for _, module := range staged {
		if err := publish(out, module.module, version, module.dir); err != nil {
			return err
		}
	}
	return nil
}

func extractArchive(root, ref, path, dest string) error {
	archive := exec.Command("git", "-C", root, "archive", ref, path)
	extract := exec.Command("tar", "-x", "-C", dest)
	pipe, err := archive.StdoutPipe()
	if err != nil {
		return err
	}
	extract.Stdin = pipe
	extract.Stderr = os.Stderr
	if err := extract.Start(); err != nil {
		return fmt.Errorf("extract archive: %w", err)
	}
	if err := archive.Run(); err != nil {
		return fmt.Errorf("git archive %s: %w", ref, err)
	}
	return extract.Wait()
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
