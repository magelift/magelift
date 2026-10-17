package cli

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
)

func TestWriteWorkflowCreatesAbsentFileWithoutClobberRace(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".github", "workflows", "magelift.yml")
	contents := []byte("name: MageLift CI\n")

	var ready sync.WaitGroup
	ready.Add(2)
	var start sync.WaitGroup
	start.Add(1)
	var done sync.WaitGroup
	done.Add(2)
	var firstErr error
	var secondErr error
	go func() {
		ready.Done()
		start.Wait()
		_, firstErr = writeWorkflow(path, contents, false)
		done.Done()
	}()
	go func() {
		ready.Done()
		start.Wait()
		_, secondErr = writeWorkflow(path, contents, false)
		done.Done()
	}()
	ready.Wait()
	start.Done()
	done.Wait()
	if firstErr != nil && secondErr != nil {
		t.Fatalf("both writes failed: %v ; %v", firstErr, secondErr)
	}
	onDisk, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(onDisk) != string(contents) {
		t.Fatalf("workflow = %q", onDisk)
	}
}

func TestWriteWorkflowRefusesSymlinkTarget(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink refusal is exercised on Unix-like systems")
	}
	dir := t.TempDir()
	target := filepath.Join(dir, "magelift.yml")
	if err := os.WriteFile(target, []byte("name: custom\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "workflow-link.yml")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	_, err := writeWorkflow(link, []byte("name: template\n"), true)
	if err == nil || ExitCode(err) != 2 || !strings.Contains(err.Error(), "non-regular") {
		t.Fatalf("error = %v", err)
	}
	unchanged, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(unchanged) != "name: custom\n" {
		t.Fatalf("target was modified: %q", unchanged)
	}
}

func TestValidateWorkflowFileRejectsEmptyWorkflow(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "magelift.yml")
	if err := os.WriteFile(path, []byte("   \n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := validateWorkflowFile(path, []byte("name: MageLift CI\n"), false)
	if err == nil || ExitCode(err) != 2 || !strings.Contains(err.Error(), "empty") {
		t.Fatalf("error = %v", err)
	}
}

func TestWriteWorkflowConflictReportsExactCandidateAndPreservesCustomPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "custom workflow.yml")
	original := []byte("name: agency CI\n")
	if err := os.WriteFile(path, original, 0o640); err != nil {
		t.Fatal(err)
	}
	result, err := writeWorkflow(path, []byte("name: MageLift CI\n"), false)
	if err == nil || result.CandidatePath == "" {
		t.Fatalf("result = %+v, error = %v", result, err)
	}
	if !strings.Contains(err.Error(), shellQuote(path)) || !strings.Contains(err.Error(), shellQuote(result.CandidatePath)) || !strings.Contains(err.Error(), "original ci generate command") {
		t.Fatalf("missing concrete review instructions: %v", err)
	}
	actual, readErr := os.ReadFile(path)
	if readErr != nil || string(actual) != string(original) {
		t.Fatalf("custom workflow changed: %q, %v", actual, readErr)
	}
}
