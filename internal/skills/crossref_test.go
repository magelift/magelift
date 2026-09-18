// Package skills_test holds the cross-track drift guard: it resolves every
// `magelift ...` command span and every dotted YAML key span in the five user
// skills against the real Cobra tree and the generated JSON schema, checks the
// AGENTS.md router table against the skill dirs, and asserts the embed
// boundary that keeps contributor runbooks out of the binary.
//
// NOTE: this file must never contain the literal "contrib" + "/skills" — the
// boundary check below scans its own package, exactly like the spec scenario.
package skills_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"unicode"

	internalcli "github.com/magelift/magelift/internal/cli"
	parent "github.com/magelift/magelift/internal/skills"
	"github.com/spf13/cobra"
)

var userSkills = []string{
	"magelift-configure",
	"magelift-dependencies",
	"magelift-local-runtime",
	"magelift-migrate",
	"magelift-operate",
}

func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 6; i++ {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			if _, err := os.Stat(filepath.Join(dir, "agents", "skills")); err == nil {
				return dir
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	t.Fatal("repo root not found above working directory")
	return ""
}

func readSkill(t *testing.T, root, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, "agents", "skills", name, "SKILL.md"))
	if err != nil {
		t.Fatalf("read skill %s: %v", name, err)
	}
	return string(data)
}

type textSpan struct {
	skill string
	line  int
	argv  string
}

// commandSpans extracts `magelift ...` argv strings from skill prose, joining
// backslash continuations. It matches the literal token "magelift" followed
// by whitespace, so skill names (magelift-configure), domains (magelift.dev),
// and the capitalized product name never match.
func commandSpans(skill, text string) []textSpan {
	var out []textSpan
	lines := strings.Split(text, "\n")
	for i := 0; i < len(lines); i++ {
		start := i + 1
		logical := lines[i]
		for strings.HasSuffix(strings.TrimRight(logical, " \t"), "\\") && i+1 < len(lines) {
			trimmed := strings.TrimRight(logical, " \t")
			logical = trimmed[:len(trimmed)-1] + " " + strings.TrimLeft(lines[i+1], " \t")
			i++
		}
		rest := logical
		for {
			idx := strings.Index(rest, "magelift")
			if idx < 0 {
				break
			}
			after := rest[idx+len("magelift"):]
			if after == "" || (after[0] != ' ' && after[0] != '\t' && after[0] != '\n') {
				rest = rest[idx+len("magelift"):]
				continue
			}
			argv := strings.TrimSpace(after)
			if end := strings.Index(argv, "`"); end >= 0 {
				argv = strings.TrimSpace(argv[:end])
			}
			if argv != "" {
				out = append(out, textSpan{skill: skill, line: start, argv: "magelift " + argv})
			}
			rest = after
		}
	}
	return out
}

func findChild(node *cobra.Command, name string) *cobra.Command {
	for _, child := range node.Commands() {
		if child.Name() == name {
			return child
		}
		for _, alias := range child.Aliases {
			if alias == name {
				return child
			}
		}
	}
	return nil
}

// resolveArgv walks argv against the Cobra tree. Flags (leading dash) are
// skipped with their values; a bare `--` ends command parsing. It reports
// whether the span resolved and whether it was skipped as prose.
func resolveArgv(root *cobra.Command, span string) (resolved bool, skipped bool, detail string) {
	tokens := strings.Fields(span)
	if len(tokens) == 0 || tokens[0] != "magelift" {
		return false, true, "not a magelift span"
	}
	tokens = tokens[1:]
	node := root
	descended := 0
	sawContent := false
	for i := 0; i < len(tokens); i++ {
		token := tokens[i]
		if token == "--" {
			break
		}
		if len(token) > 1 && strings.HasPrefix(token, "-") {
			if !strings.Contains(token, "=") && i+1 < len(tokens) {
				next := tokens[i+1]
				if !strings.HasPrefix(next, "-") && findChild(node, next) == nil {
					i++
				}
			}
			continue
		}
		if child := findChild(node, token); child != nil {
			node = child
			descended++
			sawContent = true
			continue
		}
		if !sawContent && descended == 0 {
			if r := []rune(token); len(r) > 0 && unicode.IsUpper(r[0]) {
				return false, true, fmt.Sprintf("prose span starting %q", token)
			}
			return false, false, fmt.Sprintf("unknown command %q", token)
		}
		if isBareWord(token) {
			return false, false, fmt.Sprintf("unresolved token %q after %q (subcommand typo?)", token, node.CommandPath())
		}
		sawContent = true
	}
	if descended == 0 {
		return false, true, "no command tokens"
	}
	return true, false, ""
}

// isBareWord reports whether token looks like a mistyped subcommand rather
// than a positional literal (versions, digests, paths, file names).
func isBareWord(token string) bool {
	if token == "" {
		return false
	}
	for _, r := range token {
		if r == '@' || r == '.' || r == ':' || r == '/' || r == '_' {
			return false
		}
		if unicode.IsDigit(r) {
			return false
		}
		if unicode.IsUpper(r) {
			return false
		}
	}
	return true
}

func TestCommandSpansResolveAgainstCobraTree(t *testing.T) {
	root := repoRoot(t)
	tree := internalcli.New()
	checked := 0
	for _, name := range userSkills {
		for _, span := range commandSpans(name, readSkill(t, root, name)) {
			resolved, skipped, detail := resolveArgv(tree, span.argv)
			if skipped {
				continue
			}
			checked++
			if !resolved {
				t.Errorf("%s:%d: %q: %s", span.skill, span.line, span.argv, detail)
			}
		}
	}
	if checked == 0 {
		t.Fatal("no command spans resolved; extractor is blind")
	}
	t.Logf("resolved %d command spans across %d skills", checked, len(userSkills))
}

var dottedKeyPattern = regexp.MustCompile("`([A-Za-z][A-Za-z0-9_-]*(?:\\.[A-Za-z][A-Za-z0-9_-]*)+)`")

func TestYamlKeysResolveAgainstSchema(t *testing.T) {
	root := repoRoot(t)
	raw, err := os.ReadFile(filepath.Join(root, "schema", "magelift.schema.json"))
	if err != nil {
		t.Fatal(err)
	}
	var schema map[string]any
	if err := json.Unmarshal(raw, &schema); err != nil {
		t.Fatal(err)
	}
	checked := 0
	for _, name := range userSkills {
		text := readSkill(t, root, name)
		for _, match := range dottedKeyPattern.FindAllStringSubmatch(text, -1) {
			key := match[1]
			if key == "magelift.yaml" || strings.Contains(key, "/") {
				continue
			}
			checked++
			if !resolveSchemaKey(schema, strings.Split(key, ".")) {
				t.Errorf("%s: key %q not found in schema/magelift.schema.json", name, key)
			}
		}
	}
	if checked == 0 {
		t.Fatal("no YAML key spans found; extractor is blind")
	}
	t.Logf("resolved %d YAML key span(s)", checked)
}

func resolveSchemaKey(schema map[string]any, segments []string) bool {
	node := map[string]any{"properties": schema["properties"]}
	seen := map[string]bool{}
	for _, segment := range segments {
		for i := 0; i < 8; i++ {
			ref, ok := node["$ref"].(string)
			if !ok {
				break
			}
			if seen[ref] {
				return false
			}
			seen[ref] = true
			if !strings.HasPrefix(ref, "#/") {
				return false
			}
			node = schema
			for _, part := range strings.Split(ref[2:], "/") {
				child, ok := node[part].(map[string]any)
				if !ok {
					return false
				}
				node = child
			}
		}
		props, ok := node["properties"].(map[string]any)
		if !ok {
			return false
		}
		child, ok := props[segment].(map[string]any)
		if !ok {
			return false
		}
		node = child
	}
	return true
}

var routerRowPattern = regexp.MustCompile("`(agents|contrib)/skills/([a-z][a-z0-9-]+)`")

func TestRouterTableMatchesSkillTree(t *testing.T) {
	root := repoRoot(t)
	data, err := os.ReadFile(filepath.Join(root, "AGENTS.md"))
	if err != nil {
		t.Fatal(err)
	}
	mentions := map[string]int{}
	for _, match := range routerRowPattern.FindAllStringSubmatch(string(data), -1) {
		mentions[match[1]+"/skills/"+match[2]]++
	}
	var dirs []string
	for _, track := range []string{"agents", "contrib"} {
		entries, err := os.ReadDir(filepath.Join(root, track, "skills"))
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			skillFile := filepath.Join(root, track, "skills", entry.Name(), "SKILL.md")
			if _, err := os.Stat(skillFile); err != nil {
				t.Errorf("%s/skills/%s has no SKILL.md", track, entry.Name())
				continue
			}
			dirs = append(dirs, track+"/skills/"+entry.Name())
		}
	}
	if len(dirs) != 11 {
		t.Errorf("skill dirs = %d, want 11 (5 user + 6 contributor)", len(dirs))
	}
	for _, dir := range dirs {
		if mentions[dir] != 1 {
			t.Errorf("%s appears %d times in the AGENTS.md router table, want exactly once", dir, mentions[dir])
		}
	}
	for path, count := range mentions {
		found := false
		for _, dir := range dirs {
			if dir == path {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("router table names %s, which has no skill dir: %d mention(s)", path, count)
		}
	}
}

func TestEmbedBoundaryExcludesContributorTrack(t *testing.T) {
	root := repoRoot(t)
	needle := "contrib" + "/skills"
	paths := []string{filepath.Join(root, "internal", "cli", "skills.go")}
	entries, err := os.ReadDir(filepath.Join(root, "internal", "skills"))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") {
			continue
		}
		paths = append(paths, filepath.Join(root, "internal", "skills", entry.Name()))
	}
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(data), needle) {
			rel, _ := filepath.Rel(root, path)
			t.Errorf("%s references the contributor track; user-track code must not", rel)
		}
	}
}

func TestManifestMatchesSkillDirs(t *testing.T) {
	root := repoRoot(t)
	raw, err := os.ReadFile(filepath.Join(root, "agents", "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest []parent.ManifestEntry
	if err := json.Unmarshal(raw, &manifest); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(filepath.Join(root, "agents", "skills"))
	if err != nil {
		t.Fatal(err)
	}
	dirs := map[string]bool{}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		dirs[entry.Name()] = true
		data, err := os.ReadFile(filepath.Join(root, "agents", "skills", entry.Name(), "SKILL.md"))
		if err != nil {
			t.Fatalf("read skill %s: %v", entry.Name(), err)
		}
		sum := sha256.Sum256(data)
		want := hex.EncodeToString(sum[:])
		found := false
		for _, item := range manifest {
			if item.Name == entry.Name() {
				found = true
				if item.Digest != want {
					t.Errorf("skill %s digest %s does not match manifest %s", entry.Name(), want, item.Digest)
				}
			}
		}
		if !found {
			t.Errorf("skill dir %s has no manifest entry", entry.Name())
		}
	}
	for _, item := range manifest {
		if !dirs[item.Name] {
			t.Errorf("manifest entry %s has no skill dir", item.Name)
		}
	}
}
