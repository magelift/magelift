package pipeline

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func materializeEmbeddedRecipeDockerfile(contextDir string, recipeFingerprint string) (string, error) {
	basePath := filepath.Join(contextDir, "images", "php-nginx", "Dockerfile")
	base, err := os.ReadFile(basePath)
	if err != nil {
		return "", fmt.Errorf("read embedded recipe Dockerfile: %w", err)
	}
	fragment, err := embeddedApplicationFragment(recipeFingerprint)
	if err != nil {
		return "", err
	}
	composed := append(bytes.TrimSpace(base), '\n', '\n')
	composed = append(composed, fragment...)
	dockerfilePath := filepath.Join(contextDir, "Dockerfile")
	if err := writeNewRegularFile(dockerfilePath, composed, 0o600); err != nil {
		return "", fmt.Errorf("materialize composed recipe Dockerfile: %w", err)
	}
	return dockerfilePath, nil
}

func embeddedApplicationFragment(recipeFingerprint string) ([]byte, error) {
	lines := strings.Split(string(applicationDockerfile), "\n")
	filtered := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "# syntax=") {
			continue
		}
		if trimmed == "ARG RUNTIME_BASE=scratch" {
			continue
		}
		if trimmed == "FROM ${RUNTIME_BASE}" {
			filtered = append(filtered, "FROM runtime")
			continue
		}
		filtered = append(filtered, line)
	}
	body := strings.Join(filtered, "\n")
	if !strings.Contains(body, "ARG RECIPE_SHA256") {
		body = strings.Replace(body, "ARG RUNTIME_IMAGE_DIGEST", "ARG RECIPE_SHA256\nARG RUNTIME_IMAGE_DIGEST", 1)
	}
	if !strings.Contains(body, "io.magelift.recipe.sha256") {
		body = strings.Replace(body, "io.magelift.runtime.image=\"${RUNTIME_IMAGE_DIGEST}\"",
			"io.magelift.runtime.image=\"${RUNTIME_IMAGE_DIGEST}\" \\\n      io.magelift.recipe.sha256=\"${RECIPE_SHA256}\"", 1)
	}
	return []byte(body), nil
}

func recipeRuntimeTraceIdentity(fingerprint string) string {
	if fingerprint == "" {
		return ""
	}
	return "recipe:sha256:" + fingerprint
}

// recipeBaseImages records the actual pinned defaults in the bundled Dockerfile,
// avoiding a second catalog of image digests.
func recipeBaseImages(contextDir string) (map[string]string, error) {
	data, err := os.ReadFile(filepath.Join(contextDir, "images", "php-nginx", "Dockerfile"))
	if err != nil {
		return nil, err
	}
	bases := map[string]string{}
	for _, line := range strings.Split(string(data), "\n") {
		assignment, ok := strings.CutPrefix(line, "ARG ")
		if !ok {
			continue
		}
		role, value, ok := strings.Cut(assignment, "=")
		if !ok {
			continue
		}
		switch role {
		case "PHP_BASE", "COMPOSER_BASE", "NGINX_BASE":
		default:
			continue
		}
		if _, exists := bases[role]; exists {
			return nil, fmt.Errorf("duplicate bundled base role %s", role)
		}
		if !repositoryDigest.MatchString(value) {
			return nil, fmt.Errorf("bundled base role %s is not pinned", role)
		}
		bases[role] = value
	}
	if len(bases) != 3 {
		return nil, fmt.Errorf("bundled recipe must define three pinned upstream bases")
	}
	return bases, nil
}
