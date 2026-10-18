package lintcoverage

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.yaml.in/yaml/v4"
)

var goJobMarkers = []string{
	"actions/setup-go@",
	"goreleaser/goreleaser-action@",
	"golangci/golangci-lint-action@",
	"golang/govulncheck-action@",
	"github/codeql-action/",
	"go test",
	"go build",
	"go run",
	"go mod ",
	"go list",
	"go vet",
	"go install",
	"gofmt",
}

func TestGoWorkflowJobsCapMemory(t *testing.T) {
	root := moduleRoot(t)
	workflows, err := filepath.Glob(filepath.Join(root, ".github/workflows/*.yml"))
	if err != nil {
		t.Fatal(err)
	}
	if len(workflows) == 0 {
		t.Fatal("no workflow files")
	}

	var missing []string
	var pinned []string
	for _, path := range workflows {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		var wf struct {
			Jobs map[string]yaml.Node `yaml:"jobs"`
		}
		if err := yaml.Unmarshal(data, &wf); err != nil {
			t.Fatalf("decode %s: %v", path, err)
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			rel = path
		}
		for name, node := range wf.Jobs {
			raw, err := yaml.Marshal(&node)
			if err != nil {
				t.Fatalf("marshal %s job %s: %v", rel, name, err)
			}
			if !jobRunsGo(raw) {
				continue
			}
			id := rel + " job " + name
			if !jobCapsMemory(raw) {
				missing = append(missing, id)
			}
			if jobPinsGoParallelism(raw) {
				pinned = append(pinned, id)
			}
		}
	}
	if len(missing) > 0 || len(pinned) > 0 {
		var b strings.Builder
		if len(missing) > 0 {
			b.WriteString("Go jobs must use .github/actions/go-memlimit before compiling:\n")
			for _, id := range missing {
				b.WriteString("  - ")
				b.WriteString(id)
				b.WriteByte('\n')
			}
		}
		if len(pinned) > 0 {
			b.WriteString("Go jobs must not pin GOMAXPROCS or go -p=1:\n")
			for _, id := range pinned {
				b.WriteString("  - ")
				b.WriteString(id)
				b.WriteByte('\n')
			}
		}
		t.Fatal(b.String())
	}
}

func TestMakefileLeavesGoSchedulingToToolchain(t *testing.T) {
	root := moduleRoot(t)
	data, err := os.ReadFile(filepath.Join(root, "Makefile"))
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range bytes.Split(data, []byte("\n")) {
		trim := bytes.TrimSpace(line)
		if bytes.HasPrefix(trim, []byte("#")) {
			continue
		}
		s := string(trim)
		if strings.HasPrefix(s, "export GOMAXPROCS") || strings.HasPrefix(s, "export GOFLAGS") {
			t.Fatalf("Makefile must not export GOMAXPROCS or GOFLAGS: %s", s)
		}
		if strings.Contains(s, "GOMEMLIMIT") && !strings.Contains(s, "go-memlimit.sh") && !strings.Contains(s, "unexport") {
			if strings.Contains(s, "export GOMEMLIMIT") && !strings.Contains(s, "go-memlimit.sh") {
				t.Fatalf("Makefile GOMEMLIMIT must come from scripts/go-memlimit.sh: %s", s)
			}
		}
	}
	if !bytes.Contains(data, []byte("scripts/go-memlimit.sh")) {
		t.Fatal("Makefile must set GOMEMLIMIT from scripts/go-memlimit.sh")
	}
}

func TestFmtUsesToolchainGofmt(t *testing.T) {
	root := moduleRoot(t)
	makefile, err := os.ReadFile(filepath.Join(root, "Makefile"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(makefile, []byte("go env GOROOT)/bin/gofmt")) {
		t.Fatal("Makefile fmt/fmt-check must invoke $(go env GOROOT)/bin/gofmt, not PATH gofmt")
	}
	if bytes.Contains(makefile, []byte("\tgofmt -")) || bytes.Contains(makefile, []byte("$$(gofmt -")) {
		t.Fatal("Makefile must not call PATH gofmt")
	}

	ci, err := os.ReadFile(filepath.Join(root, ".github/workflows/ci.yml"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(ci, []byte(`gofmt="$(go env GOROOT)/bin/gofmt"`)) {
		t.Fatal("go-verify must format with $(go env GOROOT)/bin/gofmt")
	}
}

func jobRunsGo(raw []byte) bool {
	s := string(raw)
	for _, marker := range goJobMarkers {
		if strings.Contains(s, marker) {
			return true
		}
	}
	return false
}

func jobCapsMemory(raw []byte) bool {
	s := string(raw)
	return strings.Contains(s, ".github/actions/go-memlimit") ||
		strings.Contains(s, "scripts/go-memlimit.sh")
}

func jobPinsGoParallelism(raw []byte) bool {
	s := string(raw)
	return strings.Contains(s, "GOMAXPROCS:") ||
		strings.Contains(s, "GOMAXPROCS=") ||
		strings.Contains(s, "GOFLAGS: -p=1") ||
		strings.Contains(s, "GOFLAGS: '-p=1'") ||
		strings.Contains(s, "GOFLAGS: \"-p=1\"")
}

func TestReleaseSerializesGoreleaserPipes(t *testing.T) {
	root := moduleRoot(t)
	data, err := os.ReadFile(filepath.Join(root, ".github/workflows/release.yml"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(data, []byte("release --clean --parallelism 1 --timeout 150m")) {
		t.Fatal("full-matrix goreleaser must pass --parallelism 1; concurrent Pulumi targets OOM ubuntu-latest")
	}
}
