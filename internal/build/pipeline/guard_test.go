package pipeline

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestApplicationDockerfileGuardAcceptsRuntimeTemplate executes the baked
// config guard predicates from the actual generated application
// Dockerfile against the supported runtime template. The template and
// the guard evolve together: a template change that trips the guard, or
// a guard rewrite this test can no longer find, both fail here instead
// of at image build time.
func TestApplicationDockerfileGuardAcceptsRuntimeTemplate(t *testing.T) {
	predicates := guardPredicates(t, string(applicationDockerfile))
	if len(predicates) < 3 {
		t.Fatalf("found %d guard predicates in assets/application.Dockerfile, want at least 3 (marker grep, secret check, media URL check)", len(predicates))
	}

	stage := t.TempDir()
	template, err := os.ReadFile(filepath.Join("..", "..", "..", "images", "php-nginx", "env.php"))
	if err != nil {
		t.Fatal(err)
	}
	appDir := filepath.Join(stage, "app", "etc")
	if err := os.MkdirAll(appDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(appDir, "env.php"), template, 0o644); err != nil {
		t.Fatal(err)
	}
	// A shop config without remote media URLs passes.
	cleanConfig := `<?php return ['system' => ['default' => ['web' => ['unsecure' => ['base_media_url' => '{{unsecure_base_url}}media/']]]]];`
	if err := os.WriteFile(filepath.Join(appDir, "config.php"), []byte(cleanConfig), 0o644); err != nil {
		t.Fatal(err)
	}

	phpAvailable := true
	if _, err := exec.LookPath("php"); err != nil {
		phpAvailable = false
		t.Log("php is not installed; the php predicates run where php exists")
	}
	for _, predicate := range predicates {
		predicate := predicate
		if strings.HasPrefix(predicate, "php ") && !phpAvailable {
			continue
		}
		command := exec.Command("sh", "-c", predicate)
		command.Dir = stage
		if output, err := command.CombinedOutput(); err != nil {
			t.Errorf("guard predicate failed: %s\n%s", predicate, output)
		}
	}
}

// TestApplicationDockerfileGuardRejectsRemoteMediaURL stages a shop
// config pointing base_media_url at object storage and requires the
// media URL predicate to fail. Delivery runs through the storefront;
// direct bucket URLs would bypass the private bucket.
func TestApplicationDockerfileGuardRejectsRemoteMediaURL(t *testing.T) {
	if _, err := exec.LookPath("php"); err != nil {
		t.Skip("php is not installed; the rejection runs where php exists")
	}
	var mediaPredicate string
	for _, predicate := range guardPredicates(t, string(applicationDockerfile)) {
		if strings.Contains(predicate, "config.php") {
			mediaPredicate = predicate
		}
	}
	if mediaPredicate == "" {
		t.Fatal("no config.php guard predicate found in assets/application.Dockerfile")
	}
	stage := t.TempDir()
	appDir := filepath.Join(stage, "app", "etc")
	if err := os.MkdirAll(appDir, 0o755); err != nil {
		t.Fatal(err)
	}
	remoteConfig := `<?php return ['system' => ['default' => ['web' => ['unsecure' => ['base_media_url' => 'https://storage.googleapis.com/shop-media/media/']]]]];`
	if err := os.WriteFile(filepath.Join(appDir, "config.php"), []byte(remoteConfig), 0o644); err != nil {
		t.Fatal(err)
	}
	command := exec.Command("sh", "-c", mediaPredicate)
	command.Dir = stage
	if output, err := command.CombinedOutput(); err == nil {
		t.Fatalf("guard accepted a remote base_media_url:\n%s", output)
	}
}

// guardPredicates extracts the env.php validation segments from the
// generated Dockerfile's RUN chain: the unresolved-marker grep and the
// secret-value php check. Segments are matched by behavior (they name
// env.php and a check), not by position. Splitting respects single
// quotes because the php predicate chains && itself.
func guardPredicates(t *testing.T, dockerfile string) []string {
	t.Helper()
	var predicates []string
	for _, instruction := range runInstructions(dockerfile) {
		for _, segment := range splitChain(instruction) {
			trimmed := strings.TrimSpace(segment)
			if !strings.Contains(trimmed, "app/etc/") {
				continue
			}
			if strings.Contains(trimmed, "grep") || strings.HasPrefix(trimmed, "php ") {
				predicates = append(predicates, trimmed)
			}
		}
	}
	return predicates
}

// runInstructions returns RUN instruction bodies with continuations
// joined, so comments and other instructions never match the guard.
func runInstructions(dockerfile string) []string {
	lines := strings.Split(dockerfile, "\n")
	var instructions []string
	var current strings.Builder
	inRun := false
	flush := func() {
		if inRun {
			instructions = append(instructions, current.String())
		}
		current.Reset()
		inRun = false
	}
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !inRun {
			if rest, ok := strings.CutPrefix(trimmed, "RUN "); ok {
				inRun = true
				current.WriteString(strings.TrimSuffix(rest, "\\"))
				if !strings.HasSuffix(trimmed, "\\") {
					flush()
				}
			}
			continue
		}
		current.WriteString(" " + strings.TrimSuffix(trimmed, "\\"))
		if !strings.HasSuffix(trimmed, "\\") {
			flush()
		}
	}
	flush()
	return instructions
}

// splitChain splits a shell && chain without splitting inside single
// quotes.
func splitChain(chain string) []string {
	var segments []string
	var current strings.Builder
	inSingle := false
	for i := 0; i < len(chain); i++ {
		char := chain[i]
		if char == '\'' {
			inSingle = !inSingle
			current.WriteByte(char)
			continue
		}
		if !inSingle && char == '&' && i+1 < len(chain) && chain[i+1] == '&' {
			segments = append(segments, current.String())
			current.Reset()
			i++
			continue
		}
		current.WriteByte(char)
	}
	return append(segments, current.String())
}
