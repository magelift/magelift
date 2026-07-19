package config

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestSchemaJSONIsValidAndStrict(t *testing.T) {
	data, err := SchemaJSON()
	if err != nil {
		t.Fatal(err)
	}
	var schema map[string]any
	if err := json.Unmarshal(data, &schema); err != nil {
		t.Fatal(err)
	}
	if schema["additionalProperties"] != false {
		t.Fatal("root schema must reject unknown fields")
	}
	if !strings.Contains(string(data), `"const": 1`) {
		t.Fatal("schemaVersion constraint is missing")
	}
	for _, field := range []string{`"aws"`, `"catalog"`, `"expiresAt"`, `"monthlyBudgetCents"`} {
		if !strings.Contains(string(data), field) {
			t.Fatalf("schema field %s is missing", field)
		}
	}
	if !strings.Contains(string(data), `"type": "number"`) {
		t.Fatal("floating-point catalog fields must be JSON numbers")
	}
}

func TestReferenceMarkdownIsDeterministic(t *testing.T) {
	first := ReferenceMarkdown()
	second := ReferenceMarkdown()
	if string(first) != string(second) {
		t.Fatal("configuration reference is not deterministic")
	}
	if !strings.Contains(string(first), "`environments.*.preset`") {
		t.Fatal("environment fields are missing")
	}
}

func TestEnvironmentSchemaUsesPartialOverlays(t *testing.T) {
	file, err := Load([]byte(`schemaVersion: 1
project: {name: shop}
application: {edition: open-source, version: 2.4.8-p5, mode: integrated}
build: {php: "8.3"}
target: {provider: aws, runtime: ecs-fargate}
defaults: {preset: preview}
environments:
  staging:
    application: {mode: headless}
    build: {staticContent: {locales: [en_US]}}
    target: {runtime: ecs-fargate}
`))
	if err != nil {
		t.Fatal(err)
	}
	effective, err := file.Resolve("staging", ResolveOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if effective.Config.Application.Mode != "headless" {
		t.Fatalf("application mode = %q", effective.Config.Application.Mode)
	}

	data, err := SchemaJSON()
	if err != nil {
		t.Fatal(err)
	}
	var schema struct {
		Definitions map[string]struct {
			Required []string `json:"required"`
		} `json:"$defs"`
	}
	if err := json.Unmarshal(data, &schema); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"applicationOverlay", "buildOverlay", "targetOverlay"} {
		definition, ok := schema.Definitions[name]
		if !ok {
			t.Fatalf("%s definition is missing", name)
		}
		if len(definition.Required) != 0 {
			t.Fatalf("%s requires overlay fields: %v", name, definition.Required)
		}
	}

	reference := string(ReferenceMarkdown())
	for _, row := range []string{
		"| `environments.*.application.mode` | string | no |",
		"| `environments.*.build.php` | string | no |",
		"| `environments.*.target.runtime` | string | no |",
	} {
		if !strings.Contains(reference, row) {
			t.Fatalf("partial overlay row missing from reference: %s", row)
		}
	}
}
