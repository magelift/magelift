package config

import (
	"errors"
	"fmt"
	"maps"
	"net"
	"net/url"
	"slices"
	"strings"

	"github.com/magelift/magelift/internal/searchplugins"
	"github.com/magelift/magelift/internal/secretref"
	"github.com/magelift/magelift/sdk"
	"go.yaml.in/yaml/v4"
	"k8s.io/apimachinery/pkg/util/validation"
)

// ServiceBinding is the public, provider-neutral Magento dependency contract.
type ServiceBinding = sdk.ServiceBinding

func resolveServiceBindings(value map[string]any, provenance map[string]Provenance) error {
	raw, ok := value["services"]
	if !ok {
		return nil
	}
	data, err := yaml.Marshal(raw)
	if err != nil {
		return err
	}
	var bindings map[string]ServiceBinding
	if err := yaml.Unmarshal(data, &bindings); err != nil {
		return fmt.Errorf("services: %w", err)
	}
	provider := nestedString(value, "target", "provider")
	for _, role := range slices.Sorted(maps.Keys(bindings)) {
		b := bindings[role]
		if err := validateServiceSettings(role, b); err != nil {
			return err
		}
		if b.Family == "" {
			return fmt.Errorf("services.%s requires a family", role)
		}
		if !slices.Contains([]string{"", "managed", "self-hosted", "existing", "external"}, b.Ownership) {
			return fmt.Errorf("services.%s has unknown ownership %q", role, b.Ownership)
		}
		if b.Endpoint != "" && !b.Attached() {
			return fmt.Errorf("services.%s.endpoint requires existing or external ownership", role)
		}
		if b.Attached() && (b.Adapter == "" || b.Endpoint != "") {
			if b.Endpoint == "" {
				return fmt.Errorf("services.%s requires an endpoint", role)
			}
			endpoint := b.Endpoint
			if !strings.Contains(endpoint, "://") {
				endpoint = "//" + endpoint
			}
			u, err := url.Parse(endpoint)
			if err != nil || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || strings.ContainsAny(b.Endpoint, " \r\n") {
				return fmt.Errorf("services.%s.endpoint must be a hostname or URL without credentials, whitespace or query parameters", role)
			}
		}
		if b.Port < 0 || b.Port > 65535 {
			return fmt.Errorf("invalid services.%s.port", role)
		}
		if b.Secret != "" {
			if _, err := parseBindingSecret(b.Secret); err != nil {
				return fmt.Errorf("services.%s.secret must be a secret reference", role)
			}
		}

		applyRole := func(path []string, selection any) {
			tree := map[string]any{}
			leaf := tree
			for _, key := range path[:len(path)-1] {
				child := map[string]any{}
				leaf[key] = child
				leaf = child
			}
			leaf[path[len(path)-1]] = selection
			apply(value, tree, "services."+role, "", provenance)
		}
		switch role {
		case "database", "cache", "session", "search", "queue":
			family := b.Family
			if role == "queue" && family == "db" {
				family = "database"
			}
			applyRole([]string{"local", role, "family"}, family)
			if b.Version != "" {
				applyRole([]string{"local", role, "version"}, b.Version)
			}
			if b.Image != "" {
				applyRole([]string{"local", role, "image"}, b.Image)
			}
		}
		if len(b.Parameters) > 0 {
			applyRole([]string{"local", role, "parameters"}, b.Parameters)
		}
		if b.MemoryMiB > 0 {
			applyRole([]string{"local", role, "memoryMiB"}, b.MemoryMiB)
		}
		if b.EvictionPolicy != "" {
			applyRole([]string{"local", role, "evictionPolicy"}, b.EvictionPolicy)
		}
		if len(b.Plugins) > 0 {
			applyRole([]string{"local", role, "plugins"}, b.Plugins)
		}
		if b.Attached() || b.Adapter != "" {
			continue
		}
		switch provider {
		case "aws":
			fields := []string{"target", "aws", "catalog"}
			if nestedString(value, "target", "runtime") == "eks" && (role == "search" || role == "queue") {
				mode := b.Family
				if mode == "db" {
					mode = "database"
				}
				applyRole([]string{"target", "aws", "catalog", "eks", role + "Mode"}, mode)
				continue
			}
			switch {
			case role == "database" && (b.Family == "mysql" || b.Family == "mariadb"):
				applyRole(append(slices.Clone(fields), "databaseEngine"), "rds-"+b.Family)
			case role == "search" && b.Family == "opensearch":
				applyRole(append(slices.Clone(fields), "searchMode"), "provisioned")
			case role == "queue" && b.Family == "rabbitmq":
				mode := "ecs-rabbitmq"
				if b.Ownership == "managed" {
					mode = "amazon-mq"
				}
				applyRole(append(slices.Clone(fields), "queueMode"), mode)
			case role == "queue" && (b.Family == "database" || b.Family == "db"):
				applyRole(append(slices.Clone(fields), "queueMode"), "db")
			}
			field := map[string]string{"mysql": "mysql", "mariadb": "mariaDb", "opensearch": "openSearch", "valkey": "valkey", "rabbitmq": "rabbitMq"}[b.Family]
			familyRole := map[string]string{"mysql": "database", "mariadb": "database", "opensearch": "search", "valkey": "cache", "rabbitmq": "queue"}[b.Family]
			if field != "" && familyRole == role && b.Version != "" {
				version := b.Version
				if field == "valkey" {
					version = b.ManagedBranch()
				} else if (field == "mysql" || field == "mariaDb") && !strings.Contains(version, ".") {
					version = selectedServiceBranch(version, nestedString(value, "target", "aws", "catalog", "versions", field))
				}
				if field == "openSearch" {
					version = "OpenSearch_" + selectedServiceBranch(b.Version, nestedString(value, "target", "aws", "catalog", "versions", "openSearch"))
				}
				applyRole(append(slices.Clone(fields), "versions", field), version)
			}
		case "gcp":
			g := func(field string, v any) { applyRole([]string{"target", "gcp", field}, v) }
			switch {
			case role == "database" && b.Family == "mysql":
				if b.Version != "" {
					g("cloudSqlDatabaseVersion", "MYSQL_"+strings.ReplaceAll(b.ManagedBranch(), ".", "_"))
				}
			case role == "cache" && b.Family == "valkey" && b.Ownership != "self-hosted":
				if b.Version != "" {
					g("memorystoreEngineVersion", "VALKEY_"+strings.ReplaceAll(b.ManagedBranch(), ".", "_"))
				}
			case role == "queue" && b.Family == "rabbitmq":
				g("queueMode", "rabbitmq")
				if b.Image != "" {
					g("rabbitMqImage", b.Image)
				} else if b.Version != "" {
					g("rabbitMqImage", "rabbitmq:"+b.Version+"-management")
				}
			case role == "search" && b.Family == "opensearch":
				g("openSearchMode", "opensearch")
				if b.Image != "" {
					g("openSearchImage", b.Image)
				} else if b.Version != "" {
					g("openSearchImage", "opensearchproject/opensearch:"+sdk.OpenSearchContainerTag(b.Version))
				}
			case role == "queue" && (b.Family == "database" || b.Family == "db"):
				if role == "queue" {
					g("queueMode", "database")
				}
			}
		}
	}
	return nil
}

// CheckServiceCapabilities prevents a built-in provisioner from creating a
// different dependency when its graph cannot materialize a selection.
func (c Config) CheckServiceCapabilities() error {
	var problems []error
	for _, role := range slices.Sorted(maps.Keys(c.Services)) {
		b := c.Services[role]
		if len(b.Parameters) > 0 && c.Target.Provider != "aws" && b.Ownership != "self-hosted" {
			problems = append(problems, fmt.Errorf("%s supports services.database.parameters on self-hosted MariaDB", c.Target.Provider))
			continue
		}
		if err := validateServiceSettings(role, b); err != nil {
			problems = append(problems, err)
			continue
		}
		if !slices.Contains([]string{"", "managed", "self-hosted", "existing", "external"}, b.Ownership) {
			problems = append(problems, fmt.Errorf("services.%s has unknown ownership %q", role, b.Ownership))
			continue
		}
		if b.Attached() {
			if err := validateServiceEndpoint(role, b); err != nil {
				problems = append(problems, err)
				continue
			}
		}
		if c.Target.Provider == "azure" && c.Target.Runtime == "aks" {
			switch role {
			case "database":
				if b.Attached() {
					if err := azureAttachedSecret(role, b); err != nil {
						problems = append(problems, err)
					}
					continue
				}
				managed := b.Family == "mysql" && (b.Ownership == "" || b.Ownership == "managed") && b.Version != "" && b.Image == ""
				selfHosted := b.Family == "mariadb" && b.Ownership == "self-hosted" && (b.Version != "" || b.Image != "")
				if (!managed && !selfHosted) || b.Endpoint != "" || b.Secret != "" || b.Username != "" || b.Port != 0 {
					problems = append(problems, errors.New("Azure requires managed MySQL with a version or self-hosted MariaDB with an image or version"))
				}
			case "cache", "session":
				if (b.Family != "redis" && b.Family != "valkey") || b.Secret != "" || b.Username != "" {
					problems = append(problems, errors.New("Azure requires an unauthenticated Redis or Valkey cache"))
				} else if b.Attached() {
					_, endpointPort, splitErr := net.SplitHostPort(b.Endpoint)
					if b.Image != "" || strings.Contains(b.Endpoint, "://") || (b.Port != 0 && b.Port != 6379) || (splitErr == nil && endpointPort != "6379") {
						problems = append(problems, errors.New("Azure external cache requires a hostname on port 6379 and no image"))
					}
				} else if (b.Ownership != "" && b.Ownership != "self-hosted") || (b.Image == "" && b.Version == "") || b.Endpoint != "" || b.Port != 0 {
					problems = append(problems, errors.New("Azure requires an explicit self-hosted cache image or external cache endpoint"))
				}
			case "search":
				if b.Family != "opensearch" || b.Attached() || (b.Ownership != "" && b.Ownership != "self-hosted") || (b.Image == "" && b.Version == "") || b.Endpoint != "" || b.Secret != "" || b.Username != "" || b.Port != 0 {
					problems = append(problems, errors.New("Azure requires a self-hosted OpenSearch image"))
				}
			case "queue":
				if b.Attached() {
					if err := azureAttachedSecret(role, b); err != nil {
						problems = append(problems, err)
					}
					continue
				}
				database := (b.Family == "database" || b.Family == "db") && b.Image == "" && (b.Ownership == "" || b.Ownership == "self-hosted")
				broker := b.Family == "rabbitmq" && b.Ownership == "self-hosted" && (b.Image != "" || b.Version != "")
				if (!database && !broker) || b.Endpoint != "" || b.Secret != "" || b.Username != "" || b.Port != 0 {
					problems = append(problems, errors.New("Azure supports the database queue or self-hosted RabbitMQ"))
				}
			default:
				problems = append(problems, fmt.Errorf("Azure does not yet support services.%s", role))
			}
			continue
		}
		gke := c.Target.Provider == "gcp" && (c.Target.Runtime == "gke-standard" || c.Target.Runtime == "gke-autopilot")
		store := role == "cache" || role == "session"
		if role == "session" && !gke && !(c.Target.Provider == "aws" && c.Target.Runtime == "ecs-fargate") {
			problems = append(problems, fmt.Errorf("%s/%s cannot provision separate sessions", c.Target.Provider, c.Target.Runtime))
			continue
		}
		known := map[string][]string{"session": {"valkey"}, "database": {"mysql", "mariadb"}, "cache": {"valkey"}, "search": {"opensearch"}, "queue": {"rabbitmq", "db", "database"}}
		available := slices.Contains(known[role], b.Family)
		if store && b.Family == "redis" && (b.Attached() || (gke && b.Ownership == "self-hosted")) {
			available = true
		}
		if !available || (c.Target.Provider == "gcp" && b.Family == "mariadb" && !b.Attached() && b.Ownership != "self-hosted") {
			problems = append(problems, fmt.Errorf("%s/%s needs a service adapter for %s family %q", c.Target.Provider, c.Target.Runtime, role, b.Family))
			continue
		}
		if b.Attached() {
			if c.Target.Provider == "gcp" && (c.Target.Runtime == "gke-autopilot" || c.Target.Runtime == "gke-standard") {
				if b.Endpoint == "" || b.Image != "" {
					problems = append(problems, fmt.Errorf("GCP external %s requires an endpoint and no image", role))
				}
				if role == "search" {
					if b.Secret != "" || b.Username != "" {
						problems = append(problems, fmt.Errorf("GCP external %s requires an adapter for authenticated endpoints", role))
					}
				} else if store && b.Secret == "" {
					if b.Username != "" {
						problems = append(problems, errors.New("GCP external cache username requires an adapter"))
					}
				} else {
					ref, err := secretref.Parse(b.Secret)
					if err != nil || ref.Kind != secretref.GCPSecretManager || ref.JSONField != "" || !strings.HasPrefix(ref.ID, "projects/") || !strings.Contains(ref.ID, "/versions/") {
						problems = append(problems, fmt.Errorf("GCP external %s requires a Secret Manager version reference", role))
					}
					if !store && b.Username == "" {
						problems = append(problems, fmt.Errorf("GCP external %s requires a username", role))
					}
					if store && b.Username != "" {
						problems = append(problems, errors.New("GCP external cache username requires an adapter"))
					}
				}
				if role == "database" || store {
					wantPort := "3306"
					if store {
						wantPort = "6379"
					}
					if strings.Contains(b.Endpoint, "://") {
						problems = append(problems, fmt.Errorf("GCP external %s endpoint must be a hostname", role))
					}
					if _, endpointPort, err := net.SplitHostPort(b.Endpoint); err == nil && endpointPort != wantPort {
						problems = append(problems, fmt.Errorf("GCP external %s endpoint uses an unsupported port", role))
					}
					if b.Port != 0 && fmt.Sprint(b.Port) != wantPort {
						problems = append(problems, fmt.Errorf("GCP external %s currently requires port %s", role, wantPort))
					}
				}
				if role == "queue" && b.Family != "rabbitmq" {
					problems = append(problems, errors.New("GCP external queue requires a RabbitMQ endpoint"))
				}
				continue
			}
			if c.Target.Provider != "aws" || c.Target.Runtime != "ecs-fargate" {
				problems = append(problems, fmt.Errorf("%s/%s needs a connection adapter for external %s", c.Target.Provider, c.Target.Runtime, role))
				continue
			}
			if b.Image != "" {
				problems = append(problems, fmt.Errorf("services.%s.image cannot be used with an external endpoint", role))
			}
			if role == "search" {
				if b.Secret != "" || b.Username != "" {
					problems = append(problems, errors.New("AWS external search requires an adapter for authenticated endpoints"))
				}
				continue
			}
			if role != "queue" && b.Username != "" {
				problems = append(problems, fmt.Errorf("AWS external %s username must be supplied through its secret", role))
			}
			ref, err := secretref.Parse(b.Secret)
			if err != nil || ref.Kind != secretref.SecretsManager || ref.JSONField != "" || !strings.HasPrefix(ref.ID, "arn:") || !strings.Contains(ref.ID, ":secretsmanager:") {
				problems = append(problems, fmt.Errorf("AWS external %s requires an AWS Secrets Manager ARN reference", role))
			}
			if (role == "database" || store) && strings.Contains(b.Endpoint, "://") {
				problems = append(problems, fmt.Errorf("AWS external %s endpoint must be a hostname", role))
			}
			if _, endpointPort, err := net.SplitHostPort(b.Endpoint); err == nil {
				if (role == "database" && endpointPort != "3306") || (store && endpointPort != "6379") {
					problems = append(problems, fmt.Errorf("AWS external %s endpoint uses an unsupported port", role))
				}
			}
			if role == "database" && b.Port != 0 && b.Port != 3306 {
				problems = append(problems, errors.New("AWS external database currently requires port 3306"))
			}
			if store && b.Port != 0 && b.Port != 6379 {
				problems = append(problems, errors.New("AWS external cache currently requires port 6379"))
			}
			if role == "queue" && b.Family == "rabbitmq" && b.Username == "" {
				problems = append(problems, errors.New("AWS external RabbitMQ requires a username"))
			}
			if role == "queue" && b.Family != "rabbitmq" {
				problems = append(problems, errors.New("AWS external queue requires a RabbitMQ endpoint"))
			}
			continue
		}
		if b.Endpoint != "" || b.Secret != "" || b.Port != 0 || b.Username != "" {
			problems = append(problems, fmt.Errorf("services.%s connection settings require external or existing ownership", role))
		}
		selfHostedGKE := gke && b.Ownership == "self-hosted" && (store || (role == "database" && b.Family == "mariadb"))
		if selfHostedGKE && b.Version == "" && b.Image == "" {
			problems = append(problems, fmt.Errorf("services.%s needs an image or version", role))
		}
		eksWorkload := c.Target.Provider == "aws" && c.Target.Runtime == "eks" && (role == "search" || role == "queue")
		if b.Ownership == "managed" && eksWorkload && (role == "search" || b.Family == "rabbitmq") {
			problems = append(problems, fmt.Errorf("AWS EKS provisions self-hosted %s", role))
		}
		if b.Ownership == "self-hosted" && !selfHostedGKE && !eksWorkload && (role == "database" || store || (role == "search" && c.Target.Provider == "aws")) {
			problems = append(problems, fmt.Errorf("%s/%s needs an adapter for self-hosted %s", c.Target.Provider, c.Target.Runtime, role))
		}
		if b.Ownership == "managed" && c.Target.Provider == "gcp" && (role == "search" || (role == "queue" && b.Family == "rabbitmq")) {
			problems = append(problems, fmt.Errorf("%s/%s needs an adapter for managed %s", c.Target.Provider, c.Target.Runtime, role))
		}
		if b.Image != "" && !selfHostedGKE && !eksWorkload && !(c.Target.Provider == "gcp" && (role == "search" || role == "queue")) {
			problems = append(problems, fmt.Errorf("%s/%s cannot use a custom image for %s", c.Target.Provider, c.Target.Runtime, role))
		}
	}
	return errors.Join(problems...)
}

// ValidateAdapterConnection verifies the materialized connection before a
// provider can consume it, including connections for external target modules.
func ValidateAdapterConnection(role string, b ServiceBinding) error {
	if !b.Attached() || b.Family == "" || b.Adapter != "" || b.Image != "" {
		return fmt.Errorf("services.%s requires a resolved external or existing connection with a family", role)
	}
	if b.Secret != "" {
		if _, err := parseBindingSecret(b.Secret); err != nil {
			return fmt.Errorf("services.%s.secret must be a secret reference", role)
		}
	}
	return validateServiceEndpoint(role, b)
}

func validateServiceEndpoint(role string, b ServiceBinding) error {
	if b.Endpoint == "" || b.Port < 0 || b.Port > 65535 || strings.ContainsAny(b.Endpoint, " \r\n") {
		return fmt.Errorf("services.%s requires a valid endpoint and port", role)
	}
	text := b.Endpoint
	if !strings.Contains(text, "://") {
		text = "//" + text
	}
	u, err := url.Parse(text)
	if err != nil || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return fmt.Errorf("services.%s endpoint must not contain credentials, a path, or query parameters", role)
	}
	if b.Port != 0 && u.Port() != "" && u.Port() != fmt.Sprint(b.Port) {
		return fmt.Errorf("services.%s endpoint and port disagree", role)
	}
	switch role {
	case "database", "cache", "session":
		if u.Scheme != "" {
			return fmt.Errorf("services.%s endpoint must be a hostname", role)
		}
	case "queue":
		if u.Scheme != "" && u.Scheme != "amqp" && u.Scheme != "amqps" {
			return fmt.Errorf("services.%s endpoint must use amqp or amqps", role)
		}
	case "search":
		if u.Scheme != "" && u.Scheme != "http" && u.Scheme != "https" {
			return fmt.Errorf("services.%s endpoint must use http or https", role)
		}
	}
	return nil
}

// A major-only service on the source can use the destination preset's minor within the same major.
func selectedServiceBranch(version, fallback string) string {
	major := strings.TrimPrefix(version, "OpenSearch_")
	if strings.Contains(major, ".") {
		return (sdk.ServiceBinding{Version: major}).ManagedBranch()
	}
	branch := strings.TrimPrefix(fallback, "OpenSearch_")
	if strings.HasPrefix(branch, major+".") {
		return (sdk.ServiceBinding{Version: branch}).ManagedBranch()
	}
	return major + ".0"
}

func validateServiceSettings(role string, b ServiceBinding) error {
	if len(b.Parameters) > 0 {
		if role != "database" || b.Attached() || (b.Family != "mysql" && b.Family != "mariadb") {
			return errors.New("services.database.parameters requires a provisioned MariaDB or MySQL database")
		}
		if err := sdk.ValidateDatabaseParameters(b.Parameters); err != nil {
			return err
		}
	}
	if len(b.Plugins) > 0 {
		if role != "search" || b.Family != "opensearch" {
			return errors.New("services.search.plugins requires OpenSearch")
		}
		if err := searchplugins.Validate(b.Plugins); err != nil {
			return err
		}
	}
	if b.EvictionPolicy != "" {
		if (role != "cache" && role != "session") || b.Attached() || !slices.Contains([]string{"noeviction", "allkeys-lru", "allkeys-lfu", "allkeys-random", "volatile-lru", "volatile-lfu", "volatile-random", "volatile-ttl"}, b.EvictionPolicy) {
			return fmt.Errorf("services.%s.evictionPolicy needs a provisioned Valkey or Redis store and a valid maxmemory policy", role)
		}
	}
	if b.MemoryMiB < 0 || (b.MemoryMiB > 0 && (b.Ownership != "self-hosted" || (role != "cache" && role != "session"))) {
		return fmt.Errorf("services.%s.memoryMiB requires a self-hosted cache or session store and a positive size", role)
	}
	if b.StorageGiB < 0 || (b.StorageGiB != 0 && (b.Ownership != "self-hosted" || (b.Family == "db" || b.Family == "database"))) {
		return fmt.Errorf("services.%s.storageGiB needs a self-hosted service and a positive volume size", role)
	}
	return nil
}

// A Kubernetes binding is resolved by the workload through a namespace-local SecretKeyRef.
func parseBindingSecret(value string) (secretref.Reference, error) {
	if name, ok := strings.CutPrefix(value, "kubernetes://"); ok {
		if len(validation.IsDNS1123Subdomain(name)) != 0 {
			return secretref.Reference{}, errors.New("invalid Kubernetes Secret name")
		}
		return secretref.Reference{Kind: "kubernetes", ID: name}, nil
	}
	return secretref.Parse(value)
}

func azureAttachedSecret(role string, b ServiceBinding) error {
	ref, err := parseBindingSecret(b.Secret)
	if err != nil || ref.Kind != "kubernetes" || b.Username == "" || b.Image != "" || b.Endpoint == "" {
		return fmt.Errorf("Azure external %s needs a hostname, username and namespace-local kubernetes:// secret containing password", role)
	}
	if role == "database" && (b.Family != "mysql" && b.Family != "mariadb" || b.Port != 0 && b.Port != 3306) {
		return errors.New("Azure external database requires MySQL or MariaDB on port 3306")
	}
	if role == "database" {
		if _, port, err := net.SplitHostPort(b.Endpoint); err == nil && port != "3306" {
			return errors.New("Azure external database requires port 3306")
		}
	}
	if role == "queue" && (b.Family != "rabbitmq" || !strings.HasPrefix(b.Endpoint, "amqps://")) {
		return errors.New("Azure external RabbitMQ needs an amqps:// endpoint")
	}
	return nil
}

// ServiceWarnings reports deployment checks for the selected topology.
func (c Config) ServiceWarnings() []string {
	var warnings []string
	search := c.Services["search"]
	if len(search.Plugins) > 0 && (search.Attached() || (c.Target.Provider == "aws" && c.Target.Runtime == "ecs-fargate")) {
		warnings = append(warnings, fmt.Sprintf("Check GET /_cat/plugins on the OpenSearch domain for %s. Its owner manages plugin availability.", strings.Join(search.Plugins, ", ")))
	}
	preset := c.Preset
	if preset == "" {
		preset = c.Defaults.Preset
	}
	if preset != string(sdk.PresetHighAvailability) {
		return warnings
	}
	for _, role := range slices.Sorted(maps.Keys(c.Services)) {
		b := c.Services[role]
		if b.Attached() || b.Adapter != "" {
			continue
		}
		singleReplica := false
		switch c.Target.Provider {
		case "aws":
			singleReplica = role == "queue" && c.Target.Runtime == "ecs-fargate" && c.Target.AWS != nil && c.Target.AWS.Catalog.QueueMode == "ecs-rabbitmq"
		case "gcp":
			switch role {
			case "database", "cache", "session":
				singleReplica = b.Ownership == "self-hosted"
			case "queue":
				singleReplica = b.Family == "rabbitmq" && c.Target.GCP != nil && c.Target.GCP.QueueReplicas == 1
			case "search":
				singleReplica = c.Target.GCP != nil && c.Target.GCP.OpenSearchReplicas == 1
			}
		case "azure":
			switch role {
			case "database":
				singleReplica = b.Ownership == "self-hosted"
			case "cache", "session":
				singleReplica = b.Family == "valkey" || b.Family == "redis"
			case "queue":
				singleReplica = b.Family == "rabbitmq"
			}
		}
		if singleReplica {
			warnings = append(warnings, fmt.Sprintf("services.%s (%s) runs one replica. Allow for downtime during replacement and rehearse recovery before sending production traffic.", role, b.Family))
		}
	}
	return warnings
}
