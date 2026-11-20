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

// SchemaJSON returns the public configuration schema derived from the typed project model.
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

// ReferenceMarkdown returns the concise configuration reference derived from the typed project model.
func ReferenceMarkdown() []byte {
	var out bytes.Buffer
	out.WriteString("# Configuration reference\n")
	out.WriteString("\n")
	out.WriteString("Use `magelift.php` to choose a cloud, set service versions, and name environments such\n")
	out.WriteString("as staging and production. MageLift reads schema version 1 and reports unknown fields.\n")
	out.WriteString("Start with [Getting started](getting-started.md) if you are setting up a shop.\n")
	out.WriteString("\n")
	out.WriteString("A preset fills in common service and capacity settings. Values in the project file\n")
	out.WriteString("override the preset, and environment settings override project settings. Run\n")
	out.WriteString("`magelift config effective --env staging` to see the resulting values and where\n")
	out.WriteString("they came from. Set an optional environment value to `null` to remove an inherited\n")
	out.WriteString("value. Keep the cloud-specific settings under `target.aws`, `target.gcp`, or\n")
	out.WriteString("`target.azure`, according to `target.provider`.\n")
	out.WriteString("\n")
	out.WriteString("## Build and email\n")
	out.WriteString("\n")
	out.WriteString("`build.extensions` lists the PHP extensions your shop needs. The builder checks\n")
	out.WriteString("that it loads them and that its Composer version matches your request before it\n")
	out.WriteString("installs packages. Set `build.composer.version` to a branch such as `2.10`, an\n")
	out.WriteString("exact release such as `2.11.2`, a minimum such as `2.9.8+`, or a patch range\n")
	out.WriteString("such as `~2.9.8`. Set `build.composer.image` to a digest-pinned image if you\n")
	out.WriteString("need another Composer binary. [Composer build settings](builds.md#composer)\n")
	out.WriteString("\n")
	out.WriteString("Use `email` to configure a shop's cloud mail relay. Provide a secret reference in\n")
	out.WriteString("`email.credential` for authenticated SMTP. On AWS, use `email.mode: ses` with\n")
	out.WriteString("`email.managed.domain` and `email.managed.hostedZoneId` to let MageLift create the\n")
	out.WriteString("sender identity, SMTP credentials and DKIM records. Managed SES takes care of\n")
	out.WriteString("the host, port and login values. Preview environments use disabled email by\n")
	out.WriteString("default. Use `local.email` for Docker Compose and pass a password through the\n")
	out.WriteString("variable named by `local.email.credentialEnv`.\n")
	out.WriteString("\n")
	out.WriteString("## Cloud settings\n")
	out.WriteString("\n")
	out.WriteString("On AWS, `target.aws.encryptionKeySecretArn` refers to the Secrets Manager value\n")
	out.WriteString("for the Magento encryption key. Keep that value for as long as you keep the\n")
	out.WriteString("encrypted data. If you use an existing VPC, supply one public, private and data\n")
	out.WriteString("subnet ID per selected zone. The network must already have egress and access to\n")
	out.WriteString("the AWS services your shop uses. Set `target.aws.existing.database` to adopt an\n")
	out.WriteString("existing RDS instance. [Connect existing resources](brownfield-attach.md)\n")
	out.WriteString("\n")
	out.WriteString("GKE Autopilot and GKE Standard require `target.gcp.project`. Choose Standard for\n")
	out.WriteString("workloads such as multi-node OpenSearch that need control over the worker nodes.\n")
	out.WriteString("Set `memorystoreMode`, `memorystoreShardCount`, and `memorystoreReplicas` to\n")
	out.WriteString("select the cache configuration. Cluster mode disabled supports one shard.\n")
	out.WriteString("Choose cluster mode enabled for multiple shards.\n")
	out.WriteString("\n")
	out.WriteString("AWS and Google Cloud select backup retention and deletion protection from the\n")
	out.WriteString("preset and environment class. Preview defaults limit the resources retained\n")
	out.WriteString("after teardown. Check the effective configuration before deployment. Explicit\n")
	out.WriteString("`false` and `0` values are preserved. [Service settings](service-bindings.md)\n")
	out.WriteString("\n")
	out.WriteString("## Edge and telemetry\n")
	out.WriteString("\n")
	out.WriteString("Set `observability.nativeProvider` to `cloudwatch` on AWS or\n")
	out.WriteString("`google-cloud-operations` on Google Cloud. To send signals to an external\n")
	out.WriteString("adapter, use `observability.externalProvider` and put vendor-specific settings\n")
	out.WriteString("under `extensions.<vendor>`. `observability.nativeReference` identifies an\n")
	out.WriteString("existing destination, such as a Google Cloud Monitoring service. Store\n")
	out.WriteString("credentials as secret references.\n")
	out.WriteString("\n")
	out.WriteString("For an external edge adapter, set `edge.health.expectedCname` to the DNS\n")
	out.WriteString("target the provider gave you. For standalone `magelift edge apply`, set\n")
	out.WriteString("`edge.health.originUrl` to the origin's health endpoint. A deployment can use\n")
	out.WriteString("the `applicationURL` output to find that endpoint. Set `edge.health.originHost`\n")
	out.WriteString("if the certificate uses a different hostname. With `edge.tlsMode: external`,\n")
	out.WriteString("you supply and renew the certificate. [Operate a shop](operations.md)\n")
	out.WriteString("\n")
	out.WriteString("## Fields\n")
	out.WriteString("\n")
	out.WriteString("| Field | Type | Required | Accepted values | Description |\n")
	out.WriteString("| --- | --- | --- | --- | --- |\n")
	writeReferenceRows(&out, reflect.TypeOf(document{}), "", false, map[reflect.Type]bool{})
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
