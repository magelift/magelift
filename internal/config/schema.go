package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"
)

const schemaID = "https://magelift.dev/schema/v1/magelift.schema.json"

// SchemaJSON returns the public configuration schema derived from the YAML model.
func SchemaJSON() ([]byte, error) {
	b := schemaBuilder{definitions: map[string]any{}}
	root := b.object(reflect.TypeOf(document{}), false)
	root["$schema"] = "https://json-schema.org/draft/2020-12/schema"
	root["$id"] = schemaID
	root["title"] = "MageLift configuration"
	root["$defs"] = b.definitions

	data, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode configuration schema: %w", err)
	}
	return append(data, '\n'), nil
}

// ReferenceMarkdown returns the concise configuration reference derived from the YAML model.
func ReferenceMarkdown() []byte {
	var out bytes.Buffer
	out.WriteString("# Configuration reference\n\n`magelift.yaml` uses schema version 1. Unknown fields are rejected.\n\n")
	out.WriteString("When `target.aws` is present, the resolver applies the selected preset's bounded\n")
	out.WriteString("topology defaults and the service versions from the Magento compatibility catalog.\n")
	out.WriteString("Project and environment values override them. The defaults do not create secrets,\n")
	out.WriteString("certificates, existing-resource references, or benchmark-selected instance sizes.\n")
	out.WriteString("Use `config effective` to inspect the values and their provenance.\n\n")
	out.WriteString("`target.provider: gcp` with `runtime: gke-autopilot` is an **experimental** first-party\n")
	out.WriteString("target (ADR 0007 / 0008). It requires `target.gcp.project` and keeps GCP topology under\n")
	out.WriteString("`target.gcp` only. Do not reuse `target.aws` fields for GCP.\n\n")
	out.WriteString("`target.provider: ovh` / `runtime: mks` and `target.provider: scaleway` / `runtime: kapsule`\n")
	out.WriteString("are also experimental. See [ovh-experimental.md](ovh-experimental.md) and\n")
	out.WriteString("[scaleway-experimental.md](scaleway-experimental.md). Scaleway requires\n")
	out.WriteString("`cacheMode: redis` (no managed Valkey yet).\n\n")
	out.WriteString("Deployments require `target.aws.encryptionKeySecretArn` to reference a stable\n")
	out.WriteString("Secrets Manager value. MageLift injects it at task start as the Magento encryption\n")
	out.WriteString("key. The key must remain stable for the lifetime of encrypted Magento data.\n\n")
	out.WriteString("`target.aws.existing.network` enables an existing VPC. When it is set, provide one\n")
	out.WriteString("public, private, and data subnet ID for every configured availability zone. MageLift\n")
	out.WriteString("does not create NAT gateways, route tables, or VPC endpoints in this mode, so the\n")
	out.WriteString("imported network must already provide the required egress and private AWS service\n")
	out.WriteString("access. The VPC CIDR is still required for security-group rules.\n\n")
	out.WriteString("`target.aws.existing.database` adopts an existing AWS RDS MySQL instance. When it is\n")
	out.WriteString("set, provide `provider: aws`, `kind: database`, `externalId` (instance ID or ARN),\n")
	out.WriteString("`secretArn` (Secrets Manager master-user secret ARN; never an inline password),\n")
	out.WriteString("and `endpoint` (writer hostname). MageLift does not create RDS when this block is\n")
	out.WriteString("set. Cloud SQL is not a configuration key in this milestone.\n\n")
	out.WriteString("`build.staticContent.strategy` and `build.staticContent.threads` map PaaS\n")
	out.WriteString("`SCD_STRATEGY` / `SCD_THREADS` on import and become Magento SCD `-s` / `-j`.\n")
	out.WriteString("See [ece-tools parity](ece-parity.md) for the closed vs intentional-gap matrix.\n\n")
	out.WriteString("| Field | Type | Required | Accepted values | Description |\n")
	out.WriteString("| --- | --- | --- | --- | --- |\n")
	writeReferenceRows(&out, reflect.TypeOf(document{}), "", false, map[reflect.Type]bool{})
	out.WriteString("\nEnvironment values override project values. A `null` environment value removes an optional inherited value. Entries under `extensions` may use namespaced fields that MageLift does not inspect.\n")
	return out.Bytes()
}

type schemaBuilder struct {
	definitions map[string]any
}

func (b *schemaBuilder) object(t reflect.Type, overlay bool) map[string]any {
	properties := map[string]any{}
	required := make([]string, 0, t.NumField())
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		name, optional := yamlField(field)
		if name == "" {
			continue
		}
		rules := schemaRules(field)
		childOverlay := overlay || t == reflect.TypeOf(Environment{})
		properties[name] = b.value(field.Type, rules, field.Tag.Get("config"), childOverlay)
		_, explicitlyRequired := rules["required"]
		if !overlay && (!optional || explicitlyRequired) {
			required = append(required, name)
		}
	}
	result := map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties":           properties,
	}
	if len(required) != 0 {
		result["required"] = required
	}
	return result
}

func (b *schemaBuilder) value(t reflect.Type, rules map[string]string, description string, overlay bool) map[string]any {
	_, nullable := rules["nullable"]
	if t.Kind() == reflect.Pointer {
		nullable = true
		t = t.Elem()
	}
	var value map[string]any
	switch t.Kind() {
	case reflect.Struct:
		name := definitionName(t)
		if overlay && t != reflect.TypeOf(Environment{}) {
			name += "Overlay"
		}
		if _, exists := b.definitions[name]; !exists {
			b.definitions[name] = map[string]any{}
			b.definitions[name] = b.object(t, overlay)
		}
		value = map[string]any{"$ref": "#/$defs/" + name}
	case reflect.Map:
		value = map[string]any{"type": "object"}
		if t.Elem().Kind() == reflect.Interface {
			value["additionalProperties"] = true
		} else {
			value["additionalProperties"] = b.value(t.Elem(), nil, "", overlay)
		}
	case reflect.Slice:
		value = map[string]any{"type": "array", "items": b.value(t.Elem(), nil, "", overlay)}
	case reflect.Bool:
		value = map[string]any{"type": "boolean"}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		value = map[string]any{"type": "integer"}
	case reflect.Float32, reflect.Float64:
		value = map[string]any{"type": "number"}
	default:
		value = map[string]any{"type": "string"}
	}
	if description != "" {
		value["description"] = description
	}
	applySchemaRules(value, rules)
	if nullable {
		value = map[string]any{"anyOf": []any{value, map[string]any{"type": "null"}}}
	}
	return value
}

func applySchemaRules(value map[string]any, rules map[string]string) {
	if constant := rules["const"]; constant != "" {
		if integer, err := strconv.Atoi(constant); err == nil {
			value["const"] = integer
		} else {
			value["const"] = constant
		}
	}
	if enum := rules["enum"]; enum != "" {
		value["enum"] = strings.Split(enum, "|")
	}
	if pattern := rules["pattern"]; pattern != "" {
		value["pattern"] = pattern
	}
	for _, key := range []string{"minLength", "minProperties", "minimum"} {
		if raw := rules[key]; raw != "" {
			integer, _ := strconv.Atoi(raw)
			value[key] = integer
		}
	}
}

func writeReferenceRows(out *bytes.Buffer, t reflect.Type, prefix string, overlay bool, visiting map[reflect.Type]bool) {
	if visiting[t] {
		return
	}
	visiting[t] = true
	defer delete(visiting, t)

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		name, optional := yamlField(field)
		if name == "" {
			continue
		}
		path := name
		if prefix != "" {
			path = prefix + "." + name
		}
		rules := schemaRules(field)
		required := "yes"
		_, explicitlyRequired := rules["required"]
		if overlay || (optional && !explicitlyRequired) {
			required = "no"
		}
		accepted := rules["const"]
		if accepted == "" {
			accepted = strings.ReplaceAll(rules["enum"], "|", ", ")
		}
		fieldType := markdownType(field.Type)
		if _, nullable := rules["nullable"]; nullable && !strings.Contains(fieldType, "null") {
			fieldType += " or null"
		}
		fmt.Fprintf(out, "| `%s` | %s | %s | %s | %s |\n", path, fieldType, required, accepted, field.Tag.Get("config"))

		nested := field.Type
		if nested.Kind() == reflect.Pointer {
			nested = nested.Elem()
		}
		if nested.Kind() == reflect.Struct {
			writeReferenceRows(out, nested, path, overlay || t == reflect.TypeOf(Environment{}), visiting)
		} else if nested.Kind() == reflect.Map && nested.Elem().Kind() == reflect.Struct {
			writeReferenceRows(out, nested.Elem(), path+".*", overlay, visiting)
		}
	}
}

func yamlField(field reflect.StructField) (string, bool) {
	tag := field.Tag.Get("yaml")
	parts := strings.Split(tag, ",")
	if parts[0] == "" || parts[0] == "-" {
		return "", false
	}
	return parts[0], len(parts) > 1 && parts[1] == "omitempty"
}

func schemaRules(field reflect.StructField) map[string]string {
	rules := map[string]string{}
	for _, rule := range strings.Split(field.Tag.Get("schema"), ",") {
		key, value, _ := strings.Cut(rule, "=")
		if key != "" {
			rules[key] = value
		}
	}
	return rules
}

func definitionName(t reflect.Type) string {
	name := t.Name()
	if name == "" {
		return "value"
	}
	return strings.ToLower(name[:1]) + name[1:]
}

func markdownType(t reflect.Type) string {
	if t.Kind() == reflect.Pointer {
		return markdownType(t.Elem()) + " or null"
	}
	switch t.Kind() {
	case reflect.Struct, reflect.Map:
		return "object"
	case reflect.Slice:
		return "array"
	case reflect.Bool:
		return "boolean"
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return "integer"
	case reflect.Float32, reflect.Float64:
		return "number"
	default:
		return "string"
	}
}

// SortedDefinitionNames supports deterministic generator tests without exposing schema internals.
func SortedDefinitionNames() []string {
	b := schemaBuilder{definitions: map[string]any{}}
	b.object(reflect.TypeOf(document{}), false)
	names := make([]string, 0, len(b.definitions))
	for name := range b.definitions {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
