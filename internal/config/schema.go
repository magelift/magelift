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
	out.WriteString("# Configuration reference\n\n")
	out.WriteString("This page is generated from the MageLift configuration schema. Do not edit it by hand.\n\n")
	out.WriteString("`magelift.yaml` uses schema version 1. Unknown fields are rejected.\n\n")
	out.WriteString("When a provider target block is present, the resolver applies the selected preset's\n")
	out.WriteString("safe provider shape defaults and the service versions from the Magento compatibility catalog.\n")
	out.WriteString("Project and environment values override them. The defaults do not create secrets,\n")
	out.WriteString("certificates, or existing-resource references; provider and preset catalogs do\n")
	out.WriteString("materialize named capacity shapes so advanced YAML can override them deliberately\n")
	out.WriteString("instead of inheriting an undocumented SDK default.\n")
	out.WriteString("Use `config effective` to inspect the values and their provenance.\n\n")
	out.WriteString("See the [advanced AWS example](https://github.com/magelift/magelift/blob/main/examples/advanced-aws-fck-nat.yaml) for a complete\n")
	out.WriteString("typed configuration that composes multi-AZ fck-nat, native and external edge,\n")
	out.WriteString("native and external observability, recovery policy, and a community extension.\n\n")
	out.WriteString("Configuration has four intentional levels: the project/defaults/preset layer is the\n")
	out.WriteString("easy path for most users; environment overlays specialize it without duplicating the\n")
	out.WriteString("whole file; provider target fields select supported semantic infrastructure choices;\n")
	out.WriteString("and namespaced `extensions.<vendor>` fields hold provider-owned integration options.\n")
	out.WriteString("Provider target fields are validated semantic escape hatches, not an unversioned dump\n")
	out.WriteString("of raw provider SDK arguments. Omitted values resolve to named defaults and appear in\n")
	out.WriteString("the effective configuration, provenance, plan, and architecture fingerprint.\n\n")
	out.WriteString("Cloud transactional email is `email` on the project or an environment overlay.\n")
	out.WriteString("`sendgrid` and `ses` require a provider-matching secret reference in `email.credential`;\n")
	out.WriteString("plaintext is rejected before Magento SMTP is written. Validation success is not certified delivery.\n")
	out.WriteString("`local.email` remains the workstation overlay and uses `credentialEnv`. A preview environment that\n")
	out.WriteString("omits `email` resolves to `disabled` and does not inherit production SendGrid or SES credentials.\n\n")
	out.WriteString("Resilience projection targets are optional advanced settings: use `resilience.projection.runtime: ecs` with an ECS `cluster`,\n")
	out.WriteString("`service` (preferred) or pinned `task`, and `container`; use `runtime: kubernetes` with `namespace`, `workload`, and an optional\n")
	out.WriteString("`container`. These are workload identities only; credentials and application verifiers stay provider-owned.\n\n")
	out.WriteString("`target.provider: gcp` with `runtime: gke-autopilot` is the certified first-party Magento path for evidenced cells. `runtime: gke-standard` is experimental and remains the node-capable path for workloads such as multi-node OpenSearch.\n")
	out.WriteString("Historical 2.4.6-p15 and 2.4.9 topology/runtime passes remain scoped evidence. The complete Adobe cache intersection and the rest of the release matrix stay open.\n")
	out.WriteString("Both require `target.gcp.project` and keep GCP topology under\n")
	out.WriteString("`target.gcp` only. Do not reuse `target.aws` fields for GCP.\n\n")
	out.WriteString("GCP Memorystore capacity and failure-domain choices are explicit advanced fields: `memorystoreShardCount`,\n")
	out.WriteString("`memorystoreReplicas`, `memorystoreMode`, and `memorystoreZoneDistributionMode`/`memorystoreZone`.\n")
	out.WriteString("Cluster Mode Disabled accepts one shard; Cluster Mode Enabled is the path for multiple shards, and\n")
	out.WriteString("the selected mode is translated to the current Memorystore API rather than left to an SDK default.\n\n")
	out.WriteString("`target.provider: ovh` / `runtime: mks` and `target.provider: scaleway` / `runtime: kapsule`\n")
	out.WriteString("are also experimental. See [ovh-experimental.md](ovh-experimental.md) and\n")
	out.WriteString("[scaleway-experimental.md](scaleway-experimental.md). Scaleway requires\n")
	out.WriteString("`cacheMode: redis` (no managed Valkey yet); `databaseHighAvailability` and `redisClusterSize`\n")
	out.WriteString("select its supported RDB/Redis failure-domain shapes. Redis cluster mode (3-6 nodes) remains\n")
	out.WriteString("blocked until the Magento connector carries cluster discovery endpoints. OVH exposes\n")
	out.WriteString("`mksPlan`, `zones`, `nodeCount`, `databaseNodeCount`, and `valkeyNodeCount`; `zones` are MKS\n")
	out.WriteString("availability zones inside the selected OVH region, not database regions. Free MKS is single-zone;\n")
	out.WriteString("Standard multi-zone MKS creates one worker pool per listed zone and requires at least one worker per zone.\n")
	out.WriteString("The preview preset selects the current low-cost discovery one-node shape; standard and high-availability select the current production two-node shape. Advanced YAML can choose discovery/essential, business/production, or enterprise/advanced plans plus explicit flavors and node counts, and provider admission checks the selected region before mutation.\n")
	out.WriteString("On Scaleway, `zone` selects the managed-cache zone and `zones`\n")
	out.WriteString("selects the Kapsule worker zones; multi-zone plans create one worker pool per listed zone\n")
	out.WriteString("and require at least one configured node per zone. The preview preset may use one zone;\n")
	out.WriteString("standard requires two and high-availability requires three, so an underprovisioned list\n")
	out.WriteString("is rejected before Pulumi mutation.\n\n")
	out.WriteString("Managed-service durability is customizable at the provider boundary while keeping the core contract portable.\n")
	out.WriteString("AWS exposes RDS/Aurora backup and maintenance windows, database deletion-protection and automated-backup deletion policy, and ElastiCache Valkey snapshot retention/window choices. Preview materializes zero Valkey snapshot-retention days; standard and high-availability materialize seven days. Database deletion protection and automated-backup cleanup are materialized from the environment class, with production retaining protection and backups by default. Explicit `false` and `0` values are preserved as intentional advanced choices.\n")
	out.WriteString("The AWS ECS presets materialize task CPU/memory, capacity mode, database/cache/search capacity, and queue shape;\n")
	out.WriteString("the EKS presets materialize compute mode, Kubernetes version, requests, workload replicas, and the shared\n")
	out.WriteString("managed database/cache durability shape.\n")
	out.WriteString("GCP exposes Cloud SQL automated-backup, binary-log, retained-backup-count, transaction-log, schedule,\n")
	out.WriteString("location, and deletion-protection choices plus Memorystore deletion protection and the PSC connection limit.\n")
	out.WriteString("The standard and high-availability GCP presets name eight retained automated backups and seven days of\n")
	out.WriteString("transaction-log retention; the preview preset disables Cloud SQL automated backups and binary logging.\n")
	out.WriteString("Cloud Armor is materialized for durable presets and for production environments, including production\n")
	out.WriteString("environments that deliberately select preview capacity; an explicit `enableCloudArmor` value overrides it.\n")
	out.WriteString("Scaleway exposes RDB backup enablement, frequency, retention, same-region placement, and encryption at rest;\n")
	out.WriteString("standard and high-availability presets name daily backups (24 hours), seven-day retention, same-region\n")
	out.WriteString("placement, and encryption at rest; preview explicitly disables automated backups to avoid orphaned\n")
	out.WriteString("backup cost during disposable environments.\n")
	out.WriteString("OVH exposes managed MySQL/Valkey backup time, destination regions, and deletion protection. Retention is\n")
	out.WriteString("provider-native: GCP's retained-backup count, Scaleway's retention days, and OVH's plan-controlled policy\n")
	out.WriteString("are not silently converted into the generic recovery policy. Cache authentication or transport-encryption\n")
	out.WriteString("modes are only exposed once the Magento connection, secret, certificate, health, and recovery contracts\n")
	out.WriteString("can honor them end to end; otherwise they fail as typed unsupported choices before mutation.\n\n")
	out.WriteString("Deployments require `target.aws.encryptionKeySecretArn` to reference a stable\n")
	out.WriteString("Secrets Manager value. MageLift injects it at task start as the Magento encryption\n")
	out.WriteString("key. The key must remain stable for the lifetime of encrypted Magento data.\n\n")
	out.WriteString("`target.aws.existing.network` enables an existing VPC. When it is set, provide one\n")
	out.WriteString("public, private, and data subnet ID for every configured availability zone. MageLift\n")
	out.WriteString("does not create NAT gateways, route tables, or VPC endpoints in this mode, so the\n")
	out.WriteString("imported network must already provide the required egress and private AWS service\n")
	out.WriteString("access. The VPC CIDR is still required for security-group rules.\n\n")
	out.WriteString("For an owned AWS network, omitting `target.aws.vpcCidr` and\n")
	out.WriteString("`target.aws.availabilityZones` selects the named preview/standard/high-availability\n")
	out.WriteString("network defaults (`10.42.0.0/16` and two or three zones derived from the configured\n")
	out.WriteString("region). The effective configuration shows these values and their provenance;\n")
	out.WriteString("override them when the account's available zones or address plan differs.\n\n")
	out.WriteString("`target.aws.existing.database` adopts an existing AWS RDS MySQL instance. When it is\n")
	out.WriteString("set, provide `provider: aws`, `kind: database`, `externalId` (instance ID or ARN),\n")
	out.WriteString("`secretArn` (Secrets Manager master-user secret ARN; never an inline password),\n")
	out.WriteString("and `endpoint` (writer hostname). MageLift does not create RDS when this block is\n")
	out.WriteString("set. GCP managed Cloud SQL settings, when used, belong under `target.gcp`; they are\n")
	out.WriteString("not overloaded into the AWS existing-resource shape.\n\n")
	out.WriteString("For AWS fck-nat, `natMode`, `natTopology`, and `natReplacementMode` form one\n")
	out.WriteString("explicit network boundary. Selecting fck-nat materializes the preview or durable-preset\n")
	out.WriteString("topology, replacement policy, and one cost-optimized ARM64 NAT instance in effective YAML;\n")
	out.WriteString("multi-AZ fck-nat uses one failure-domain target per selected zone and automatic\n")
	out.WriteString("replacement through an Auto Scaling group unless `none` is explicitly selected.\n")
	out.WriteString("`target.aws.natInstanceType` overrides the ARM64 Graviton size; it defaults to the\n")
	out.WriteString("cost-optimized `t4g.nano`, and x86 instance types are rejected for the first-party\n")
	out.WriteString("ARM64 fck-nat AMI.\n")
	out.WriteString("A high-availability fck-nat plan requires multi-AZ plus automatic replacement; a\n")
	out.WriteString("single instance is never labeled highly available.\n\n")
	out.WriteString("`build.staticContent.strategy` and `build.staticContent.threads` map PaaS\n")
	out.WriteString("`SCD_STRATEGY` / `SCD_THREADS` on import and become Magento SCD `-s` / `-j`.\n")
	out.WriteString("See [ece-tools parity](ece-parity.md) for the closed vs intentional-gap matrix.\n\n")
	out.WriteString("`build.extensions` declares the PHP extensions that the selected builder must load.\n")
	out.WriteString("`build.composer.version` accepts a Composer 2 major.minor or patch version; append `+` for a minimum. Both\n")
	out.WriteString("requirements are checked inside the isolated builder before dependency installation; an\n")
	out.WriteString("unsupported extension or Composer version fails the build instead of being ignored.\n\n")
	out.WriteString("`observability.nativeProvider` selects a matching cloud telemetry destination and\n")
	out.WriteString("`observability.externalProvider` selects an extension-owned adapter without putting vendor credentials\n")
	out.WriteString("in the portable configuration. `observability.nativeReference` is an opaque identity\n")
	out.WriteString("owned by that native provider; it is not a credential, selector, or provider SDK\n")
	out.WriteString("object. For example, OVHcloud uses it to identify an existing Logs Data Platform\n")
	out.WriteString("stream. `cloudwatch`, `google-cloud-operations`, `scaleway-cockpit`, and\n")
	out.WriteString("`ovh-logs-data-platform` are first-party native adapters for their matching clouds.\n")
	out.WriteString("New Relic, Datadog, OTLP, and other external providers are extension-owned. Put\n")
	out.WriteString("their credentials and provider-specific options under `extensions.<vendor>`.\n")
	out.WriteString("The core preserves an extension namespace as opaque YAML; its registered adapter\n")
	out.WriteString("must strict-decode and validate that payload before planning or mutation. A\n")
	out.WriteString("first-party adapter rejects signals it cannot configure rather than silently\n")
	out.WriteString("dropping them.\n\n")
	out.WriteString("The first-release schema has no singular `edge.provider` or\n")
	out.WriteString("`observability.provider` field and has no compatibility alias for either one.\n")
	out.WriteString("Use `nativeProvider` and `externalProvider` to compose destinations;\n")
	out.WriteString("`provider` on `target` and existing-resource identities remains identity data.\n\n")
	out.WriteString("External edge apply requires a real `edge.health` policy. Set\n")
	out.WriteString("`edge.health.expectedCname` to the provider's DNS target and, for a standalone\n")
	out.WriteString("`edge apply`, set `edge.health.originUrl` to the origin health endpoint. A normal\n")
	out.WriteString("deployment may derive the origin URL from the newly provisioned\n")
	out.WriteString("`applicationURL` output. If the endpoint uses a generated load-balancer hostname with a certificate\n")
	out.WriteString("for the application domain, set `edge.health.originHost` for TLS SNI and HTTP Host.\n")
	out.WriteString("When one edge domain is configured, the first-party Fastly adapter uses that\n")
	out.WriteString("domain as the default origin host; set `originHost` when the certificate differs.\n")
	out.WriteString("An `originHealthRef` is a reference for evidence and\n")
	out.WriteString("never substitutes for an HTTP health proof. `tlsMode: external` means that\n")
	out.WriteString("certificate ownership and renewal stay outside MageLift; the Fastly adapter\n")
	out.WriteString("maps it to customer-provided TLS and does not create a TLS subscription.\n\n")
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
