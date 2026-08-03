// Package lintcoverage guards the CI lint matrix against silently covering less
// than go list ./... reports (QUALITY-06 Pitfall 4).
package lintcoverage

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"go.yaml.in/yaml/v4"
)

const (
	ciWorkflowRel     = ".github/workflows/ci.yml"
	golangciConfigRel = ".golangci.yml"
	lintJobName       = "lint"
)

type workflowFile struct {
	Jobs map[string]workflowJob `yaml:"jobs"`
}

type workflowJob struct {
	Strategy *jobStrategy `yaml:"strategy"`
}

type jobStrategy struct {
	Matrix *jobMatrix `yaml:"matrix"`
}

type jobMatrix struct {
	Include []matrixEntry `yaml:"include"`
}

type matrixEntry struct {
	Name     string `yaml:"name"`
	Packages string `yaml:"packages"`
}

type golangciFile struct {
	Linters struct {
		Exclusions struct {
			Paths []string `yaml:"paths"`
		} `yaml:"exclusions"`
	} `yaml:"linters"`
}

func TestLintMatrixCoversEveryPackage(t *testing.T) {
	root := moduleRoot(t)

	entries := lintMatrixEntries(t, root)
	if len(entries) == 0 {
		t.Fatal("lint job strategy.matrix.include is empty")
	}

	excludeRes := golangciPathExclusions(t, root)
	allPkgs := goList(t, root, "./...")
	expected, excluded := subtractExcluded(allPkgs, modulePath(t, root), excludeRes)

	covered := make(map[string]string) // package → partition name
	var overlaps []string

	for _, entry := range entries {
		patterns := fieldsPreserve(entry.Packages)
		if len(patterns) == 0 {
			t.Fatalf("lint matrix entry %q has empty packages", entry.Name)
		}
		pkgs := goList(t, root, patterns...)
		for _, pkg := range pkgs {
			if prev, ok := covered[pkg]; ok {
				overlaps = append(overlaps, fmt.Sprintf("%s (partitions %q and %q)", pkg, prev, entry.Name))
				continue
			}
			covered[pkg] = entry.Name
		}
	}

	var uncovered []string
	for _, pkg := range expected {
		if _, ok := covered[pkg]; !ok {
			uncovered = append(uncovered, pkg)
		}
	}

	// Packages the matrix covers that golangci would skip are fine to ignore for
	// the uncovered check, but still count toward overlap.
	sort.Strings(uncovered)
	sort.Strings(overlaps)

	if len(uncovered) > 0 || len(overlaps) > 0 {
		var b strings.Builder
		if len(uncovered) > 0 {
			b.WriteString("lint matrix does not cover packages reported by go list ./...:\n")
			for _, pkg := range uncovered {
				b.WriteString("  - ")
				b.WriteString(pkg)
				b.WriteByte('\n')
			}
		}
		if len(overlaps) > 0 {
			b.WriteString("packages covered by more than one lint partition:\n")
			for _, line := range overlaps {
				b.WriteString("  - ")
				b.WriteString(line)
				b.WriteByte('\n')
			}
		}
		if len(excluded) > 0 {
			b.WriteString("golangci path exclusions (subtracted from expected set):\n")
			for _, pkg := range excluded {
				b.WriteString("  - ")
				b.WriteString(pkg)
				b.WriteByte('\n')
			}
		}
		t.Fatal(b.String())
	}
}

func TestLintMatrixFailsLoudlyWhenJobMissing(t *testing.T) {
	// Table-driven structural failure modes; each must not pass vacuously.
	cases := []struct {
		name string
		yaml string
	}{
		{
			name: "missing lint job",
			yaml: "jobs:\n  other:\n    runs-on: ubuntu-latest\n",
		},
		{
			name: "lint job without matrix and without golangci ./...",
			yaml: "jobs:\n  lint:\n    runs-on: ubuntu-latest\n",
		},
		{
			name: "lint job without matrix include and without golangci ./...",
			yaml: "jobs:\n  lint:\n    strategy:\n      fail-fast: false\n",
		},
		{
			name: "lint matrix without include",
			yaml: "jobs:\n  lint:\n    strategy:\n      matrix:\n        name: [aws]\n",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseLintMatrix([]byte(tc.yaml))
			if err == nil {
				t.Fatal("expected error for malformed workflow, got nil")
			}
		})
	}
}

func lintMatrixEntries(t *testing.T, root string) []matrixEntry {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, ciWorkflowRel))
	if err != nil {
		t.Fatalf("read %s: %v", ciWorkflowRel, err)
	}
	entries, err := parseLintMatrix(data)
	if err != nil {
		t.Fatal(err)
	}
	return entries
}

func parseLintMatrix(data []byte) ([]matrixEntry, error) {
	var wf workflowFile
	// Partial decode: ci.yml has many keys we do not model.
	if err := yaml.Unmarshal(data, &wf); err != nil {
		return nil, fmt.Errorf("decode %s: %w", ciWorkflowRel, err)
	}
	job, ok := wf.Jobs[lintJobName]
	if !ok {
		return nil, fmt.Errorf("%s: job %q not found", ciWorkflowRel, lintJobName)
	}
	// QUALITY-06: lint may be a single golangci-lint ./... job (no matrix) to
	// save Actions minutes, or a partitioned matrix. Both must cover go list ./....
	if job.Strategy == nil || job.Strategy.Matrix == nil || len(job.Strategy.Matrix.Include) == 0 {
		if !lintJobRunsAllPackages(data) {
			return nil, fmt.Errorf("%s: job %q has no strategy.matrix.include and does not run golangci-lint on ./...", ciWorkflowRel, lintJobName)
		}
		return []matrixEntry{{Name: "all", Packages: "./..."}}, nil
	}
	for i, entry := range job.Strategy.Matrix.Include {
		if strings.TrimSpace(entry.Name) == "" {
			return nil, fmt.Errorf("%s: matrix.include[%d] missing name", ciWorkflowRel, i)
		}
		if strings.TrimSpace(entry.Packages) == "" {
			return nil, fmt.Errorf("%s: matrix.include[%d] (%q) missing packages", ciWorkflowRel, i, entry.Name)
		}
	}
	return job.Strategy.Matrix.Include, nil
}

// lintJobRunsAllPackages reports whether the lint job invokes golangci-lint on ./....
func lintJobRunsAllPackages(data []byte) bool {
	// Keep this intentionally string-based: modeling every Actions step schema is
	// brittle, and the contract we care about is "args include ./...".
	return bytes.Contains(data, []byte("golangci-lint")) &&
		bytes.Contains(data, []byte("args: ./..."))
}

func golangciPathExclusions(t *testing.T, root string) []*regexp.Regexp {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, golangciConfigRel))
	if err != nil {
		t.Fatalf("read %s: %v", golangciConfigRel, err)
	}
	var cfg golangciFile
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("decode %s: %v", golangciConfigRel, err)
	}
	paths := cfg.Linters.Exclusions.Paths
	out := make([]*regexp.Regexp, 0, len(paths))
	for _, p := range paths {
		re, err := regexp.Compile(p)
		if err != nil {
			t.Fatalf("compile golangci path exclusion %q: %v", p, err)
		}
		out = append(out, re)
	}
	return out
}

func subtractExcluded(pkgs []string, mod string, excludeRes []*regexp.Regexp) (kept, excluded []string) {
	for _, pkg := range pkgs {
		rel := strings.TrimPrefix(pkg, mod+"/")
		if rel == pkg {
			// Module root package itself.
			rel = "."
		}
		if matchesAny(rel, excludeRes) {
			excluded = append(excluded, pkg)
			continue
		}
		kept = append(kept, pkg)
	}
	return kept, excluded
}

func matchesAny(rel string, excludeRes []*regexp.Regexp) bool {
	for _, re := range excludeRes {
		if re.MatchString(rel) {
			return true
		}
	}
	return false
}

func moduleRoot(t *testing.T) string {
	t.Helper()
	out := runGo(t, "", "list", "-m", "-f", "{{.Dir}}")
	root := strings.TrimSpace(out)
	if root == "" {
		t.Fatal("go list -m returned empty module dir")
	}
	return root
}

func modulePath(t *testing.T, root string) string {
	t.Helper()
	return strings.TrimSpace(runGo(t, root, "list", "-m"))
}

func goList(t *testing.T, root string, patterns ...string) []string {
	t.Helper()
	args := append([]string{"list"}, patterns...)
	out := runGo(t, root, args...)
	var pkgs []string
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			pkgs = append(pkgs, line)
		}
	}
	sort.Strings(pkgs)
	return pkgs
}

func runGo(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("go", args...)
	if dir != "" {
		cmd.Dir = dir
	}
	cmd.Env = append(os.Environ(), "GOMAXPROCS=1", "GOFLAGS=-p=1")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("go %s: %v\n%s", strings.Join(args, " "), err, stderr.String())
	}
	return stdout.String()
}

func fieldsPreserve(s string) []string {
	return strings.Fields(s)
}
