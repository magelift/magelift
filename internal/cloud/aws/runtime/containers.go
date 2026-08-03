package runtime

import (
	"encoding/json"
	"errors"
	"strings"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

func containerDefinitionsInput(args Args, secrets []SecretReference) (pulumi.StringInput, error) {
	return pulumi.All(capabilityEnvironment(args), args.DatabaseSecretARN, args.EncryptionKeyARN, searchEndpointInput(args)).ApplyT(func(values []interface{}) (string, error) {
		environment, ok := values[0].([]containerEnvironment)
		if !ok {
			return "", errors.New("resolve runtime capability environment")
		}
		databaseARN, ok := values[1].(string)
		if !ok || strings.TrimSpace(databaseARN) == "" {
			return "", errors.New("resolve database secret reference")
		}
		encryptionARN, ok := values[2].(string)
		if !ok || strings.TrimSpace(encryptionARN) == "" {
			return "", errors.New("resolve Magento encryption key secret reference")
		}
		searchEndpoint, ok := values[3].(string)
		if !ok {
			return "", errors.New("resolve OpenSearch endpoint for signing proxy")
		}
		return containerDefinitions(args, appendEncryptionSecret(secrets, encryptionARN), environment, databaseARN, searchEndpoint)
	}).(pulumi.StringOutput), nil
}

func searchEndpointInput(args Args) pulumi.StringInput {
	if args.Capabilities == nil || args.Capabilities.SearchEndpoint == nil {
		return pulumi.String("")
	}
	return args.Capabilities.SearchEndpoint
}

// FrontendPort returns the task port reached by the load balancer. Integrated
// Magento requests enter through Varnish; headless requests enter the
// application listener directly.
func FrontendPort(applicationMode string) int {
	if applicationMode == "integrated" {
		return VarnishPort
	}
	return ApplicationPort
}

func containerDefinitions(args Args, secrets []SecretReference, environment []containerEnvironment, databaseARN, searchEndpoint string) (string, error) {
	if args.WebRuntime == "nginx-fpm" {
		php := baseContainer(args, appendDatabaseSecret(secrets, databaseARN), environment, "php-fpm", nil, false)
		web := baseContainer(args, nil, nil, "web", []string{"nginx", "-g", "daemon off;"}, args.ApplicationMode != "integrated")
		web.HealthCheck = nginxHealthCheck()
		containers, err := appendSearchProxy(args, []containerDefinition{php, web}, searchEndpoint)
		if err != nil {
			return "", err
		}
		containers, err = appendVarnish(args, containers)
		if err != nil {
			return "", err
		}
		encoded, err := json.Marshal(containers)
		return string(encoded), err
	}
	return containerDefinitionsFor(args, appendDatabaseSecret(secrets, databaseARN), environment, "web", nil, true, searchEndpoint)
}

type containerSecret struct {
	Name      string `json:"name"`
	ValueFrom string `json:"valueFrom"`
}

type containerMount struct {
	SourceVolume  string `json:"sourceVolume"`
	ContainerPath string `json:"containerPath"`
	ReadOnly      bool   `json:"readOnly"`
}

type containerPort struct {
	ContainerPort int    `json:"containerPort"`
	Protocol      string `json:"protocol"`
}

type containerLinux struct {
	InitProcessEnabled bool             `json:"initProcessEnabled"`
	Tmpfs              []containerTmpfs `json:"tmpfs,omitempty"`
}

type containerTmpfs struct {
	ContainerPath string   `json:"containerPath"`
	MountOptions  []string `json:"mountOptions,omitempty"`
	Size          int      `json:"size"`
}

type containerDependency struct {
	ContainerName string `json:"containerName"`
	Condition     string `json:"condition"`
}

type containerEnvironment struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type containerHealthCheck struct {
	Command     []string `json:"command"`
	Interval    int      `json:"interval"`
	Timeout     int      `json:"timeout"`
	Retries     int      `json:"retries"`
	StartPeriod int      `json:"startPeriod"`
}

type containerLogConfiguration struct {
	LogDriver string            `json:"logDriver"`
	Options   map[string]string `json:"options"`
}

type containerDefinition struct {
	Name                   string                     `json:"name"`
	Image                  string                     `json:"image"`
	Command                []string                   `json:"command,omitempty"`
	Essential              bool                       `json:"essential"`
	User                   string                     `json:"user,omitempty"`
	ReadonlyRootFilesystem bool                       `json:"readonlyRootFilesystem"`
	LinuxParameters        containerLinux             `json:"linuxParameters"`
	PortMappings           []containerPort            `json:"portMappings"`
	MountPoints            []containerMount           `json:"mountPoints,omitempty"`
	DependsOn              []containerDependency      `json:"dependsOn,omitempty"`
	HealthCheck            *containerHealthCheck      `json:"healthCheck,omitempty"`
	Secrets                []containerSecret          `json:"secrets,omitempty"`
	Environment            []containerEnvironment     `json:"environment,omitempty"`
	LogConfiguration       *containerLogConfiguration `json:"logConfiguration,omitempty"`
}

func containerDefinitionsForInput(args Args, secrets []SecretReference, name string, command []string, exposePort bool) (pulumi.StringInput, error) {
	return pulumi.All(capabilityEnvironment(args), args.DatabaseSecretARN, args.EncryptionKeyARN, searchEndpointInput(args)).ApplyT(func(values []interface{}) (string, error) {
		environment, ok := values[0].([]containerEnvironment)
		if !ok {
			return "", errors.New("resolve runtime capability environment")
		}
		databaseARN, ok := values[1].(string)
		if !ok || strings.TrimSpace(databaseARN) == "" {
			return "", errors.New("resolve database secret reference")
		}
		encryptionARN, ok := values[2].(string)
		if !ok || strings.TrimSpace(encryptionARN) == "" {
			return "", errors.New("resolve Magento encryption key secret reference")
		}
		searchEndpoint, ok := values[3].(string)
		if !ok {
			return "", errors.New("resolve OpenSearch endpoint for signing proxy")
		}
		return containerDefinitionsFor(args, appendDatabaseSecret(appendEncryptionSecret(secrets, encryptionARN), databaseARN), environment, name, command, exposePort, searchEndpoint)
	}).(pulumi.StringOutput), nil
}

func containerDefinitionsFor(args Args, secrets []SecretReference, environment []containerEnvironment, name string, command []string, exposePort bool, searchEndpoint string) (string, error) {
	containers, err := appendSearchProxy(args, []containerDefinition{baseContainer(args, secrets, environment, name, command, exposePort && args.ApplicationMode != "integrated")}, searchEndpoint)
	if err != nil {
		return "", err
	}
	if name == "web" {
		containers, err = appendVarnish(args, containers)
		if err != nil {
			return "", err
		}
	}
	encoded, err := json.Marshal(containers)
	return string(encoded), err
}

// nginxHealthCheck probes the ALB-facing /health location without booting Magento.
func nginxHealthCheck() *containerHealthCheck {
	return &containerHealthCheck{
		Command:     []string{"CMD-SHELL", "curl -sf http://127.0.0.1:8080/health"},
		Interval:    10,
		Timeout:     5,
		Retries:     3,
		StartPeriod: 30,
	}
}

func baseContainer(args Args, secrets []SecretReference, environment []containerEnvironment, name string, command []string, exposePort bool) containerDefinition {
	selected := make([]containerSecret, len(secrets))
	for index, value := range secrets {
		selected[index] = containerSecret{Name: value.Name, ValueFrom: value.ValueFrom()}
	}
	// Magento needs a writable root (env.php, generated files, nginx pid under
	// /tmp). Fargate empty volumes mount as root:root, so overlaying /tmp or
	// /app/var breaks non-root nginx/php — match prior Magento-on-AWS practice (writable root,
	// no bind-mount overlays) until EFS access points own the paths.
	container := containerDefinition{
		Name: name, Image: args.Image, Command: command, Essential: true, User: "10001:10001",
		ReadonlyRootFilesystem: false, LinuxParameters: containerLinux{InitProcessEnabled: true},
		Secrets: selected, Environment: environment, LogConfiguration: awslogsConfig(args, name),
	}
	if exposePort {
		container.PortMappings = []containerPort{{ContainerPort: args.ContainerPort, Protocol: "tcp"}}
	}
	return container
}

// awslogsConfig maps Magento workload containers onto the observability log
// groups (web/deploy/cron). Sidecars share the web group.
func awslogsConfig(args Args, containerName string) *containerLogConfiguration {
	prefix := strings.TrimSpace(args.LogGroupPrefix)
	if prefix == "" || strings.TrimSpace(args.Region) == "" {
		return nil
	}
	role := "web"
	switch containerName {
	case "deploy":
		role = "deploy"
	case "cron":
		role = "cron"
	}
	return &containerLogConfiguration{
		LogDriver: "awslogs",
		Options: map[string]string{
			"awslogs-group":         prefix + "/" + role,
			"awslogs-region":        args.Region,
			"awslogs-stream-prefix": "ecs",
		},
	}
}
