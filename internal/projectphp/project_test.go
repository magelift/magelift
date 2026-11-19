package projectphp

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestRenderAndEvaluate(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "magelift.php")
	input := map[string]any{
		"schemaVersion": 1,
		"project":       map[string]any{"name": "it's \\ a shop"},
		"environments":  map[string]any{"staging": map[string]any{"domain": "stage.example"}},
		"extensions":    map[string]any{},
	}
	data, err := Render(input)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := Evaluate(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	expected, _ := json.Marshal(input)
	var a, b any
	if err := json.Unmarshal(got, &a); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(expected, &b); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(a, b) {
		t.Fatalf("got %s, want %s", got, expected)
	}
}

func TestProjectTypedValuesAndManagedOverlay(t *testing.T) {
	path := filepath.Join(t.TempDir(), "magelift.php")
	source := `<?php
use MageLift\Project;
use MageLift\Environment;
return new Project(values: ['schemaVersion' => 1], name: 'shop', environments: [
    'production' => new Environment(domain: 'shop.example', class: 'production'),
    'staging' => new Environment(inherits: 'production', domain: 'stage.example'),
]);`
	if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	overlay := Overlay{Environments: map[string]map[string]any{
		"production": {"protection": true}, "preview": {"inherits": "production"},
	}, Removed: []string{"staging"}}
	data, err := RenderOverlay(overlay)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(OverlayPath(path), data, 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := Evaluate(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Environments map[string]map[string]any `json:"environments"`
	}
	if err := json.Unmarshal(result, &doc); err != nil {
		t.Fatal(err)
	}
	if _, ok := doc.Environments["staging"]; ok {
		t.Fatal("staging was not removed")
	}
	if doc.Environments["production"]["protection"] != true || doc.Environments["production"]["domain"] != "shop.example" {
		t.Fatalf("production = %#v", doc.Environments["production"])
	}
	if doc.Environments["preview"]["inherits"] != "production" {
		t.Fatalf("preview = %#v", doc.Environments["preview"])
	}
}

func TestEvaluateRejectsOutputAndDoesNotLeakIt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "magelift.php")
	if err := os.WriteFile(path, []byte("<?php echo 'sensitive output'; return new \\MageLift\\Project([]);"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Evaluate(context.Background(), path)
	if err == nil || strings.Contains(err.Error(), "sensitive output") {
		t.Fatalf("error = %v", err)
	}
}
