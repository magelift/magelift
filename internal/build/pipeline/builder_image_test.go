package pipeline

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuilderImageDoesNotLoadRuntimeDeploymentConfig(t *testing.T) {
	dockerfile, err := os.ReadFile(filepath.Join("..", "..", "..", "images", "php-nginx", "Dockerfile"))
	if err != nil {
		t.Fatal(err)
	}
	stages := dockerStages(string(dockerfile))
	runtime, ok := stages["runtime"]
	if !ok {
		t.Fatal("runtime stage missing")
	}
	builder, ok := stages["builder"]
	if !ok {
		t.Fatal("builder stage missing")
	}
	if !strings.Contains(runtime, "images/php-nginx/env.php") || !strings.Contains(runtime, "images/php-nginx/deployment-config.php") || !strings.Contains(runtime, "images/php-nginx/php.ini") {
		t.Fatal("runtime stage dropped the deployment scaffold")
	}
	if !strings.HasPrefix(strings.TrimSpace(builder), "FROM extensions AS builder") {
		t.Fatalf("builder must start from the extension image, got %q", firstLine(builder))
	}
	for _, forbidden := range []string{"env.php", "deployment-config.php", "images/php-nginx/php.ini", "auto_prepend", "FROM runtime"} {
		if strings.Contains(builder, forbidden) {
			t.Fatalf("builder stage contains %q", forbidden)
		}
	}
	if !strings.Contains(builder, "images/php-nginx/builder.ini") {
		t.Fatal("builder stage does not install builder.ini")
	}
	if !strings.Contains(builder, "io.magelift.php.base=\"${PHP_BASE}\"") || !strings.Contains(builder, "io.magelift.php.branch=\"${PHP_BRANCH}\"") {
		t.Fatal("builder stage must carry the same PHP base label as the runtime image")
	}

	ini, err := os.ReadFile(filepath.Join("..", "..", "..", "images", "php-nginx", "builder.ini"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(ini), "memory_limit = 2G") {
		t.Fatalf("builder.ini = %q", ini)
	}
	for _, line := range strings.Split(string(ini), "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "auto_prepend_file") {
			t.Fatalf("builder.ini sets auto_prepend_file: %q", line)
		}
	}
}

func dockerStages(dockerfile string) map[string]string {
	stages := map[string]string{}
	var name string
	var body strings.Builder
	flush := func() {
		if name != "" {
			stages[name] = body.String()
			body.Reset()
		}
	}
	for _, line := range strings.Split(dockerfile, "\n") {
		if strings.HasPrefix(line, "FROM ") {
			flush()
			name = stageName(line)
			body.WriteString(line)
			body.WriteByte('\n')
			continue
		}
		if name != "" {
			body.WriteString(line)
			body.WriteByte('\n')
		}
	}
	flush()
	return stages
}

func stageName(fromLine string) string {
	_, after, ok := strings.Cut(fromLine, " AS ")
	if !ok {
		return ""
	}
	return strings.TrimSpace(after)
}

func firstLine(text string) string {
	line, _, _ := strings.Cut(text, "\n")
	return line
}
