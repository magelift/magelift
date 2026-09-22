package runtime

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

const testImage = "ghcr.io/magelift/shop@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
const testVarnishImage = "docker.io/library/varnish:8.0.2@sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"

type node struct {
	typeToken, name string
	inputs          resource.PropertyMap
}
type mocks struct {
	mu    sync.Mutex
	nodes []node
}

func (m *mocks) NewResource(args pulumi.MockResourceArgs) (string, resource.PropertyMap, error) {
	m.mu.Lock()
	m.nodes = append(m.nodes, node{args.TypeToken, args.Name, args.Inputs.Copy()})
	m.mu.Unlock()
	return args.Name + "-id", args.Inputs, nil
}
func (*mocks) Call(args pulumi.MockCallArgs) (resource.PropertyMap, error) { return args.Args, nil }

func TestRuntimeResourceGraphAndSecurityContract(t *testing.T) {
	t.Parallel()
	m := deploy(t, validArgs())
	want := []string{
		"aws:ec2/securityGroup:SecurityGroup:shop-web-sg", "aws:ecs/cluster:Cluster:shop-cluster", "aws:ecs/service:Service:shop-cron-service", "aws:ecs/service:Service:shop-web-service", "aws:ecs/taskDefinition:TaskDefinition:shop-cron-task", "aws:ecs/taskDefinition:TaskDefinition:shop-deploy-task", "aws:ecs/taskDefinition:TaskDefinition:shop-web-task",
		"aws:iam/role:Role:shop-deployment-role", "aws:iam/role:Role:shop-execution-role", "aws:iam/role:Role:shop-task-role", "aws:iam/rolePolicy:RolePolicy:shop-execution-logs-policy", "aws:iam/rolePolicy:RolePolicy:shop-execution-policy", TypeToken + ":shop",
	}
	if got := m.snapshot(); !reflect.DeepEqual(got, want) {
		t.Fatalf("resource graph:\n%s", strings.Join(got, "\n"))
	}
	task := m.named(t, "aws:ecs/taskDefinition:TaskDefinition", "shop-web-task")
	var definitions []map[string]any
	if err := json.Unmarshal([]byte(task.inputs["containerDefinitions"].StringValue()), &definitions); err != nil {
		t.Fatal(err)
	}
	if len(definitions) != 3 {
		t.Fatalf("integrated nginx-fpm task must contain php-fpm, web, and varnish containers: %#v", definitions)
	}
	containers := make(map[string]map[string]any, len(definitions))
	for _, definition := range definitions {
		name, ok := definition["name"].(string)
		if !ok {
			t.Fatalf("container definition has no name: %#v", definition)
		}
		containers[name] = definition
	}
	php := containers["php-fpm"]
	web := containers["web"]
	varnish := containers["varnish"]
	if php["image"] != testImage || php["user"] != "10001:10001" || php["readonlyRootFilesystem"] != false {
		t.Fatalf("php-fpm container definition = %#v", php)
	}
	if web["image"] != testImage || web["command"].([]any)[0] != "nginx" || web["portMappings"] != nil {
		t.Fatalf("nginx container definition = %#v", web)
	}
	if web["readonlyRootFilesystem"] != false {
		t.Fatalf("web must use a writable root on Fargate: %#v", web)
	}
	if _, hasMounts := web["mountPoints"]; hasMounts {
		t.Fatalf("web must not overlay Fargate empty volumes on /tmp: %#v", web["mountPoints"])
	}
	healthCheck, ok := web["healthCheck"].(map[string]any)
	if !ok {
		t.Fatalf("web must define an ECS health check for /health: %#v", web)
	}
	hcCommand, _ := healthCheck["command"].([]any)
	if len(hcCommand) < 2 || hcCommand[0] != "CMD-SHELL" || !strings.Contains(fmt.Sprint(hcCommand[1]), "127.0.0.1:8080/health") {
		t.Fatalf("web health check = %#v", healthCheck)
	}
	if varnish["image"] != testVarnishImage || varnish["user"] != "varnish" || varnish["readonlyRootFilesystem"] != false || varnish["portMappings"].([]any)[0].(map[string]any)["containerPort"] != float64(VarnishPort) {
		t.Fatalf("varnish container definition = %#v", varnish)
	}
	depends, _ := varnish["dependsOn"].([]any)
	if len(depends) != 1 {
		t.Fatalf("varnish dependsOn = %#v", varnish["dependsOn"])
	}
	dep, _ := depends[0].(map[string]any)
	if dep["containerName"] != "web" || dep["condition"] != "HEALTHY" {
		t.Fatalf("varnish must wait for healthy nginx: %#v", dep)
	}
	linuxParameters, ok := varnish["linuxParameters"].(map[string]any)
	if !ok {
		t.Fatalf("varnish Linux parameters are missing: %#v", varnish)
	}
	if _, hasTmpfs := linuxParameters["tmpfs"]; hasTmpfs {
		t.Fatalf("varnish must not use tmpfs for VSM: %#v", linuxParameters)
	}
	if _, hasMounts := varnish["mountPoints"]; hasMounts {
		t.Fatalf("varnish must use image /tmp, not a Fargate empty volume: %#v", varnish["mountPoints"])
	}
	command, _ := varnish["command"].([]any)
	if len(command) < 2 || command[0] != "-n" || command[1] != "/tmp/varnish" {
		t.Fatalf("varnish must set -n /tmp/varnish for Fargate VSM: %#v", command)
	}
	if env := environmentByName(varnish); env["VARNISH_SIZE"] != "64m" {
		t.Fatalf("varnish malloc for 512MiB task = %q, want 64m", env["VARNISH_SIZE"])
	}
	if webLinux, ok := web["linuxParameters"].(map[string]any); ok {
		if _, hasTmpfs := webLinux["tmpfs"]; hasTmpfs {
			t.Fatalf("web must not rely on Fargate tmpfs uid/gid options: %#v", webLinux)
		}
	}
	if _, hasSecrets := web["secrets"]; hasSecrets {
		t.Fatal("nginx sidecar received application secrets")
	}
	if !strings.Contains(task.inputs["containerDefinitions"].StringValue(), "MAGELIFT_WEB_RUNTIME") {
		t.Fatalf("container definitions lack runtime marker: %s", task.inputs["containerDefinitions"].StringValue())
	}
	if !strings.Contains(task.inputs["containerDefinitions"].StringValue(), "MAGELIFT_DATABASE_CREDENTIALS") {
		t.Fatalf("container definitions lack the managed database secret reference: %s", task.inputs["containerDefinitions"].StringValue())
	}
	if !strings.Contains(task.inputs["containerDefinitions"].StringValue(), "MAGENTO_DC_DB__CONNECTION__DEFAULT__PASSWORD") || !strings.Contains(task.inputs["containerDefinitions"].StringValue(), "database:password::") {
		t.Fatalf("container definitions do not select database JSON credentials: %s", task.inputs["containerDefinitions"].StringValue())
	}
	if strings.Contains(task.inputs["containerDefinitions"].StringValue(), "database:host::") || strings.Contains(task.inputs["containerDefinitions"].StringValue(), "database:port::") || strings.Contains(task.inputs["containerDefinitions"].StringValue(), "database:dbname::") {
		t.Fatalf("managed database secret must not select host/port/dbname JSON keys: %s", task.inputs["containerDefinitions"].StringValue())
	}
	if !strings.Contains(task.inputs["containerDefinitions"].StringValue(), "MAGENTO_DC_CRYPT__KEY") || !strings.Contains(task.inputs["containerDefinitions"].StringValue(), "encryption-key") {
		t.Fatalf("container definitions do not inject the Magento encryption key reference: %s", task.inputs["containerDefinitions"].StringValue())
	}
	if !strings.Contains(task.inputs["containerDefinitions"].StringValue(), `"logDriver":"awslogs"`) || !strings.Contains(task.inputs["containerDefinitions"].StringValue(), "/magelift/shop/preview/web") {
		t.Fatalf("container definitions lack awslogs configuration: %s", task.inputs["containerDefinitions"].StringValue())
	}
	if strings.Contains(task.inputs["containerDefinitions"].StringValue(), "plaintext") {
		t.Fatal("container definition contains plaintext secret material")
	}
	deployTask := m.named(t, "aws:ecs/taskDefinition:TaskDefinition", "shop-deploy-task")
	deployCmd := deployTask.inputs["containerDefinitions"].StringValue()
	if _, found := deployTask.inputs["taskRoleArn"]; !found || !strings.Contains(deployCmd, "app:config:import") || !strings.Contains(deployCmd, "setup:upgrade") || !strings.Contains(deployCmd, "cache:flush") {
		t.Fatalf("deploy task role or command is unsafe: inputs=%#v", deployTask.inputs)
	}
	if upgrade, importAt := strings.Index(deployCmd, "setup:upgrade"), strings.Index(deployCmd, "app:config:import"); upgrade < 0 || importAt < 0 || upgrade > importAt {
		t.Fatalf("deploy command = %q, want setup:upgrade before app:config:import", deployCmd)
	}
	if !strings.Contains(deployTask.inputs["containerDefinitions"].StringValue(), "MAGELIFT_DATABASE_CREDENTIALS") {
		t.Fatal("deploy task does not receive the managed database secret reference")
	}
	if !strings.Contains(deployTask.inputs["containerDefinitions"].StringValue(), "/magelift/shop/preview/deploy") {
		t.Fatal("deploy task lacks awslogs group for deploy workload")
	}
	service := m.one(t, "aws:ecs/service:Service")
	if !service.inputs["enableExecuteCommand"].BoolValue() {
		t.Fatal("ECS Exec is disabled on the web service")
	}
	network := service.inputs["networkConfiguration"].ObjectValue()
	if network["assignPublicIp"].BoolValue() {
		t.Fatal("service assigns a public IP")
	}
	if len(network["subnets"].ArrayValue()) != 2 {
		t.Fatal("service did not retain private subnet placement")
	}
	group := m.one(t, "aws:ec2/securityGroup:SecurityGroup")
	if ingress, found := group.inputs["ingress"]; found && len(ingress.ArrayValue()) != 0 {
		t.Fatal("web security group has inbound rules")
	}
}

func TestRuntimeSupportsFargateSpotCapacityProvider(t *testing.T) {
	t.Parallel()
	args := validArgs()
	args.ComputeMode = ComputeModeFargateSpot
	m := deploy(t, args)

	if !containsResource(m.snapshot(), "aws:ecs/clusterCapacityProviders:ClusterCapacityProviders:shop-capacity-providers") {
		t.Fatal("Fargate Spot must associate FARGATE and FARGATE_SPOT with the cluster")
	}
	service := m.named(t, "aws:ecs/service:Service", "shop-web-service")
	if _, found := service.inputs["launchType"]; found {
		t.Fatal("Fargate Spot service must use a capacity-provider strategy, not launchType")
	}
	strategies := service.inputs["capacityProviderStrategies"].ArrayValue()
	if len(strategies) != 1 || strategies[0].ObjectValue()["capacityProvider"].StringValue() != "FARGATE_SPOT" {
		t.Fatalf("Fargate Spot service strategy = %#v", strategies)
	}
	task := m.named(t, "aws:ecs/taskDefinition:TaskDefinition", "shop-web-task")
	if got := task.inputs["requiresCompatibilities"].ArrayValue(); len(got) != 1 || got[0].StringValue() != "FARGATE" {
		t.Fatalf("Fargate Spot task compatibility = %#v", got)
	}
	definitions := decodeDefinitions(t, task.inputs["containerDefinitions"].StringValue())
	for _, definition := range definitions {
		if got := int(definition["stopTimeout"].(float64)); got != fargateSpotStopTimeoutSeconds {
			t.Fatalf("Fargate Spot container %q stop timeout = %d, want %d", definition["name"], got, fargateSpotStopTimeoutSeconds)
		}
	}
}

func TestRuntimeOmitsFargateSpotStopTimeoutForRegularFargate(t *testing.T) {
	t.Parallel()
	m := deploy(t, validArgs())
	task := m.named(t, "aws:ecs/taskDefinition:TaskDefinition", "shop-web-task")
	for _, definition := range decodeDefinitions(t, task.inputs["containerDefinitions"].StringValue()) {
		if _, found := definition["stopTimeout"]; found {
			t.Fatalf("regular Fargate container %q unexpectedly has Spot stop timeout: %#v", definition["name"], definition["stopTimeout"])
		}
	}
}

func TestRuntimeSupportsEC2AutoScalingCapacity(t *testing.T) {
	t.Parallel()
	args := validArgs()
	args.ComputeMode = ComputeModeEC2AutoScaling
	args.InstanceAMI = "ami-0123456789abcdef0"
	args.InstanceType = "m7i.large"
	args.MinCapacity = 2
	args.MaxCapacity = 4
	m := deploy(t, args)
	resources := m.snapshot()
	for _, want := range []string{
		"aws:iam/role:Role:shop-ecs-host-role",
		"aws:iam/instanceProfile:InstanceProfile:shop-ecs-host-profile",
		"aws:ec2/launchTemplate:LaunchTemplate:shop-ecs-host-template",
		"aws:autoscaling/group:Group:shop-ecs-hosts",
		"aws:ecs/capacityProvider:CapacityProvider:shop-ecs-capacity-provider",
		"aws:ecs/clusterCapacityProviders:ClusterCapacityProviders:shop-capacity-providers",
	} {
		if !containsResource(resources, want) {
			t.Fatalf("EC2 capacity graph is missing %s:\n%s", want, strings.Join(resources, "\n"))
		}
	}
	service := m.named(t, "aws:ecs/service:Service", "shop-web-service")
	if _, found := service.inputs["launchType"]; found {
		t.Fatal("EC2 capacity service must use a capacity-provider strategy, not launchType")
	}
	if strategies := service.inputs["capacityProviderStrategies"].ArrayValue(); len(strategies) != 1 {
		t.Fatalf("EC2 capacity service strategies = %#v", strategies)
	}
	task := m.named(t, "aws:ecs/taskDefinition:TaskDefinition", "shop-web-task")
	if got := task.inputs["requiresCompatibilities"].ArrayValue(); len(got) != 1 || got[0].StringValue() != "EC2" {
		t.Fatalf("EC2 task compatibility = %#v", got)
	}
	launchTemplate := m.named(t, "aws:ec2/launchTemplate:LaunchTemplate", "shop-ecs-host-template")
	if launchTemplate.inputs["imageId"].StringValue() != args.InstanceAMI || launchTemplate.inputs["instanceType"].StringValue() != args.InstanceType {
		t.Fatalf("EC2 launch template pinning = %#v", launchTemplate.inputs)
	}
	provider := m.named(t, "aws:ecs/capacityProvider:CapacityProvider", "shop-ecs-capacity-provider")
	if got := provider.inputs["name"].StringValue(); got != "shop-ec2" {
		t.Fatalf("EC2 capacity provider name = %q, want shop-ec2", got)
	}
	group := m.named(t, "aws:autoscaling/group:Group", "shop-ecs-hosts")
	if group.inputs["minSize"].NumberValue() != 2 || group.inputs["desiredCapacity"].NumberValue() != 2 || group.inputs["maxSize"].NumberValue() != 4 {
		t.Fatalf("EC2 host capacity = %#v", group.inputs)
	}
}

func TestRuntimeEC2CapacityProviderNameAvoidsAWSPrefix(t *testing.T) {
	t.Parallel()
	args := validArgs()
	args.ComputeMode = ComputeModeEC2AutoScaling
	args.InstanceAMI = "ami-0123456789abcdef0"
	args.InstanceType = "t3.medium"
	m := &mocks{}
	if err := pulumi.RunErr(func(ctx *pulumi.Context) error {
		_, err := New(ctx, "awsap-preview-runtime", args)
		return err
	}, pulumi.WithMocks("project", "stack", m)); err != nil {
		t.Fatal(err)
	}
	provider := m.named(t, "aws:ecs/capacityProvider:CapacityProvider", "awsap-preview-runtime-ecs-capacity-provider")
	if got := provider.inputs["name"].StringValue(); got != "ml-awsap-preview-runtime-ec2" {
		t.Fatalf("capacity provider AWS name = %q, want ml-awsap-preview-runtime-ec2", got)
	}
}

func TestRuntimeSupportsManagedInstanceCapacity(t *testing.T) {
	t.Parallel()
	args := validArgs()
	args.ComputeMode = ComputeModeManagedInstance
	args.InstanceType = "c7i.large"
	m := deploy(t, args)
	resources := m.snapshot()
	for _, want := range []string{
		"aws:iam/role:Role:shop-ecs-managed-instance-role",
		"aws:iam/instanceProfile:InstanceProfile:shop-ecs-managed-instance-profile",
		"aws:iam/role:Role:shop-ecs-infrastructure-role",
		"aws:iam/rolePolicy:RolePolicy:shop-ecs-infrastructure-pass-role",
		"aws:ecs/capacityProvider:CapacityProvider:shop-ecs-managed-capacity-provider",
	} {
		if !containsResource(resources, want) {
			t.Fatalf("Managed Instance capacity graph is missing %s:\n%s", want, strings.Join(resources, "\n"))
		}
	}
	if containsResource(resources, "aws:ecs/clusterCapacityProviders:ClusterCapacityProviders:shop-capacity-providers") {
		t.Fatal("Managed Instance capacity must not call PutClusterCapacityProviders; the CP is cluster-scoped at create")
	}
	role := m.named(t, "aws:iam/role:Role", "shop-ecs-managed-instance-role")
	if role.inputs["name"].StringValue() != "ecsInstanceRole-shop" {
		t.Fatalf("Managed Instance role name = %#v, want ecsInstanceRole-shop", role.inputs["name"])
	}
	provider := m.named(t, "aws:ecs/capacityProvider:CapacityProvider", "shop-ecs-managed-capacity-provider")
	if _, found := provider.inputs["managedInstancesProvider"]; !found {
		t.Fatalf("Managed Instance capacity provider inputs = %#v", provider.inputs)
	}
	if tags, ok := provider.inputs["tags"]; !ok || tags.ObjectValue()["magelift:iam-propagated"].StringValue() != "ready" {
		t.Fatalf("Managed Instance capacity provider must wait on IAM propagation: %#v", provider.inputs["tags"])
	}
	if got := provider.inputs["name"].StringValue(); got != "shop-managed" {
		t.Fatalf("Managed Instance capacity provider name = %q, want shop-managed", got)
	}
	service := m.named(t, "aws:ecs/service:Service", "shop-web-service")
	if tags, ok := service.inputs["tags"]; !ok || tags.ObjectValue()["magelift:capacity-ready"].StringValue() != "ready" {
		t.Fatalf("Managed Instance service must wait for capacity provider ACTIVE: %#v", service.inputs["tags"])
	}
	if _, found := service.inputs["launchType"]; found {
		t.Fatal("Managed Instance service must use a capacity-provider strategy, not launchType")
	}
	task := m.named(t, "aws:ecs/taskDefinition:TaskDefinition", "shop-web-task")
	if got := task.inputs["requiresCompatibilities"].ArrayValue(); len(got) != 1 || got[0].StringValue() != "MANAGED_INSTANCES" {
		t.Fatalf("Managed Instance task compatibility = %#v", got)
	}
	reqs := provider.inputs["managedInstancesProvider"].ObjectValue()["instanceLaunchTemplate"].ObjectValue()["instanceRequirements"].ObjectValue()
	vcpu := reqs["vcpuCount"].ObjectValue()
	mem := reqs["memoryMib"].ObjectValue()
	if vcpu["min"].NumberValue() != 2 || vcpu["max"].NumberValue() != 2 {
		t.Fatalf("Managed Instance vcpuCount = %#v, want min=max=2", vcpu)
	}
	if mem["min"].NumberValue() != 4096 || mem["max"].NumberValue() != 4096 {
		t.Fatalf("Managed Instance memoryMib = %#v, want min=max=4096 for c7i.large", mem)
	}
	if _, found := reqs["instanceGenerations"]; found {
		t.Fatalf("Managed Instance instanceGenerations = %#v, want omitted with a pinned SKU", reqs["instanceGenerations"])
	}
}

func TestRuntimeRejectsUnpinnedEC2CapacityAMI(t *testing.T) {
	t.Parallel()
	args := validArgs()
	args.ComputeMode = ComputeModeEC2AutoScaling
	m := &mocks{}
	err := pulumi.RunErr(func(ctx *pulumi.Context) error { _, err := New(ctx, "shop", args); return err }, pulumi.WithMocks("project", "stack", m))
	if err == nil || !strings.Contains(err.Error(), "pinned ami") {
		t.Fatalf("unpinned EC2 capacity AMI was accepted: %v", err)
	}
	if len(m.snapshot()) == 0 {
		// The component and its cluster are registered before capacity validation;
		// this assertion documents that the error is raised before host mutation.
		return
	}
	for _, resource := range m.snapshot() {
		if strings.Contains(resource, "ecs-host") || strings.Contains(resource, "capacityProvider") {
			t.Fatalf("unpinned EC2 capacity registered host resources: %s", resource)
		}
	}
}

func TestRuntimeWiresMagentoOpenSearchEnvFromEndpoint(t *testing.T) {
	t.Parallel()
	args := validArgs()
	args.Capabilities = testCapabilities()
	args.Capabilities.SearchEndpoint = pulumi.String("https://vpc-shop.eu-west-3.es.amazonaws.com")
	m := deploy(t, args)
	task := m.named(t, "aws:ecs/taskDefinition:TaskDefinition", "shop-web-task")
	definitions := decodeDefinitions(t, task.inputs["containerDefinitions"].StringValue())
	if definitionsByName(definitions)["search-proxy"] != nil {
		t.Fatalf("AWS OpenSearch must not attach a signing sidecar: %#v", definitions)
	}
	php := definitionsByName(definitions)["php-fpm"]
	web := definitionsByName(definitions)["web"]
	if php["dependsOn"] != nil {
		t.Fatalf("php-fpm must not wait on a search sidecar: %#v", php["dependsOn"])
	}
	if web["dependsOn"] != nil {
		t.Fatalf("web dependsOn = %#v", web["dependsOn"])
	}
	environment := environmentByName(php)
	for name, value := range map[string]string{
		"MAGELIFT_SEARCH_ENDPOINT":                               "https://vpc-shop.eu-west-3.es.amazonaws.com",
		"MAGENTO_DC_CATALOG__SEARCH__ENGINE":                     "opensearch",
		"MAGENTO_DC_CATALOG__SEARCH__OPENSEARCH_SERVER_HOSTNAME": "https://vpc-shop.eu-west-3.es.amazonaws.com",
		"MAGENTO_DC_CATALOG__SEARCH__OPENSEARCH_SERVER_PORT":     "443",
		"MAGENTO_DC_CATALOG__SEARCH__OPENSEARCH_ENABLE_AUTH":     "0",
		"MAGENTO_DC_CATALOG__SEARCH__OPENSEARCH_INDEX_PREFIX":    "magento2",
		"MAGENTO_DC_ELASTICSUITE__ES_CLIENT__SERVERS":            "vpc-shop.eu-west-3.es.amazonaws.com:443",
		"MAGENTO_DC_ELASTICSUITE__ES_CLIENT__ENABLE_HTTPS_MODE":  "1",
		"MAGENTO_DC_ELASTICSUITE__ES_CLIENT__ENABLE_HTTP_AUTH":   "0",
	} {
		if environment[name] != value {
			t.Fatalf("%s = %q, want %q", name, environment[name], value)
		}
	}
}

func TestRuntimeWiresAOSSThroughSigningProxy(t *testing.T) {
	t.Parallel()
	args := validArgs()
	args.SearchProxyImage = testSearchProxyImage
	args.Capabilities = testCapabilities()
	args.Capabilities.SearchEndpoint = pulumi.String("https://abc.eu-west-3.aoss.amazonaws.com")
	m := deploy(t, args)
	task := m.named(t, "aws:ecs/taskDefinition:TaskDefinition", "shop-web-task")
	definitions := decodeDefinitions(t, task.inputs["containerDefinitions"].StringValue())
	if definitionsByName(definitions)["search-proxy"] == nil {
		t.Fatal("AOSS Magento search requires a signing sidecar")
	}
	php := definitionsByName(definitions)["php-fpm"]
	environment := environmentByName(php)
	for name, value := range map[string]string{
		"MAGELIFT_SEARCH_ENDPOINT":                               "https://abc.eu-west-3.aoss.amazonaws.com",
		"MAGENTO_DC_CATALOG__SEARCH__OPENSEARCH_SERVER_HOSTNAME": "127.0.0.1",
		"MAGENTO_DC_CATALOG__SEARCH__OPENSEARCH_SERVER_PORT":     "8081",
		"MAGENTO_DC_CATALOG__SEARCH__OPENSEARCH_ENABLE_AUTH":     "0",
		"MAGENTO_DC_ELASTICSUITE__ES_CLIENT__SERVERS":            "127.0.0.1:8081",
		"MAGENTO_DC_ELASTICSUITE__ES_CLIENT__ENABLE_HTTPS_MODE":  "0",
	} {
		if environment[name] != value {
			t.Fatalf("%s = %q, want %q", name, environment[name], value)
		}
	}
}

func TestRuntimeOmitsMagentoSearchEnvWhenEndpointEmpty(t *testing.T) {
	t.Parallel()
	args := validArgs()
	args.Capabilities = testCapabilities()
	args.Capabilities.SearchEndpoint = pulumi.String("")
	m := deploy(t, args)
	task := m.named(t, "aws:ecs/taskDefinition:TaskDefinition", "shop-web-task")
	definitions := decodeDefinitions(t, task.inputs["containerDefinitions"].StringValue())
	if definitionsByName(definitions)["search-proxy"] != nil {
		t.Fatalf("search proxy must be absent: %#v", definitions)
	}
	php := definitionsByName(definitions)["php-fpm"]
	environment := environmentByName(php)
	for _, name := range []string{
		"MAGENTO_DC_CATALOG__SEARCH__ENGINE",
		"MAGENTO_DC_CATALOG__SEARCH__OPENSEARCH_SERVER_HOSTNAME",
		"MAGENTO_DC_CATALOG__SEARCH__OPENSEARCH_SERVER_PORT",
	} {
		if _, ok := environment[name]; ok {
			t.Fatalf("%s must not be set when search endpoint is empty", name)
		}
	}
}

func TestSearchProxyListenAddr(t *testing.T) {
	t.Parallel()
	if got := searchProxyListenAddr(); got != ":8081" {
		t.Fatalf("searchProxyListenAddr() = %q, want %q", got, ":8081")
	}
}

func TestSearchProxyTargetServiceName(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		endpoint string
		wantHost string
		wantSvc  string
	}{
		{name: "provisioned_domain", endpoint: "https://shop.search.eu-west-3.es.amazonaws.com", wantHost: "shop.search.eu-west-3.es.amazonaws.com", wantSvc: "es"},
		{name: "serverless_aoss", endpoint: "https://abc.eu-west-3.aoss.amazonaws.com", wantHost: "abc.eu-west-3.aoss.amazonaws.com", wantSvc: "aoss"},
		{name: "bare_host", endpoint: "shop.search", wantHost: "shop.search", wantSvc: "es"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			host, service := searchProxyTarget(tc.endpoint)
			if host != tc.wantHost || service != tc.wantSvc {
				t.Fatalf("searchProxyTarget(%q) = (%q, %q), want (%q, %q)", tc.endpoint, host, service, tc.wantHost, tc.wantSvc)
			}
		})
	}
}

func TestRuntimeCreatesQueueConsumerServiceWhenRequested(t *testing.T) {
	t.Parallel()
	args := validArgs()
	args.QueueConsumerCount = 2
	args.Magento.ConsumerNames = []string{"product_action_attribute.update"}
	m := deploy(t, args)
	if !containsResource(m.snapshot(), "aws:ecs/service:Service:shop-queue-service") || !containsResource(m.snapshot(), "aws:ecs/taskDefinition:TaskDefinition:shop-queue-task") {
		t.Fatalf("queue consumer resources were not created: %v", m.snapshot())
	}
	queueTask := m.named(t, "aws:ecs/taskDefinition:TaskDefinition", "shop-queue-task")
	definitions := queueTask.inputs["containerDefinitions"].StringValue()
	if !strings.Contains(definitions, "queue:consumers:start") || !strings.Contains(definitions, "product_action_attribute.update") {
		t.Fatalf("queue task does not start the named Magento consumer: %s", definitions)
	}
}

func TestRuntimeWritesMagentoOverlayContract(t *testing.T) {
	t.Parallel()
	args := validArgs()
	args.Magento.FrontName = "backend"
	args.Magento.CookieDomain = ".shop.test"
	m := deploy(t, args)
	task := m.named(t, "aws:ecs/taskDefinition:TaskDefinition", "shop-web-task")
	definitions := task.inputs["containerDefinitions"].StringValue()
	if !strings.Contains(definitions, "MAGENTO_DC_BACKEND__FRONTNAME") || !strings.Contains(definitions, "backend") {
		t.Fatalf("web task missing Magento frontName overlay: %s", definitions)
	}
	if !strings.Contains(definitions, "CONFIG__DEFAULT__WEB__COOKIE__COOKIE_DOMAIN") || !strings.Contains(definitions, ".shop.test") {
		t.Fatalf("web task missing Magento cookie overlay: %s", definitions)
	}
}

func TestRuntimeSupportsRegisteredWebRuntimesAndRejectsWorker(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		webRuntime string
		command    string
	}{
		{webRuntime: "nginx-fpm", command: "nginx"},
		{webRuntime: "frankenphp-classic", command: "frankenphp"},
		{webRuntime: "php-apache", command: "sh"},
	} {
		test := test
		t.Run(test.webRuntime, func(t *testing.T) {
			t.Parallel()
			args := validArgs()
			args.WebRuntime = test.webRuntime
			args.ApplicationMode = "headless"
			args.ContainerPort = ApplicationPort
			args.VarnishImage = ""
			m := deploy(t, args)
			definitions := definitionsByName(decodeDefinitions(t, m.named(t, "aws:ecs/taskDefinition:TaskDefinition", "shop-web-task").inputs["containerDefinitions"].StringValue()))
			web := definitions["web"]
			if web == nil || web["command"].([]any)[0] != test.command {
				t.Fatalf("%s web container = %#v", test.webRuntime, web)
			}
			if test.webRuntime == "nginx-fpm" {
				if definitions["php-fpm"] == nil {
					t.Fatal("nginx-fpm must retain its PHP-FPM container")
				}
			} else {
				if definitions["php-fpm"] != nil {
					t.Fatalf("%s must run as one HTTP process container", test.webRuntime)
				}
				if _, hasEnvironment := web["environment"]; !hasEnvironment {
					t.Fatalf("%s web process must receive Magento configuration", test.webRuntime)
				}
			}
		})
	}
	args := validArgs()
	args.WebRuntime = "frankenphp-worker"
	m := &mocks{}
	err := pulumi.RunErr(func(ctx *pulumi.Context) error { _, err := New(ctx, "shop", args); return err }, pulumi.WithMocks("project", "stack", m))
	if err == nil || !strings.Contains(err.Error(), `plugin "frankenphp-worker" is not registered`) {
		t.Fatalf("frankenphp-worker error = %v", err)
	}
}

func TestRuntimeInjectsNonSecretCapabilityReferences(t *testing.T) {
	t.Parallel()
	for _, webRuntime := range []string{"nginx-fpm", "frankenphp-classic", "php-apache"} {
		webRuntime := webRuntime
		t.Run(webRuntime, func(t *testing.T) {
			t.Parallel()
			args := validArgs()
			args.WebRuntime = webRuntime
			args.Capabilities = testCapabilities()
			m := deploy(t, args)
			task := m.named(t, "aws:ecs/taskDefinition:TaskDefinition", "shop-web-task")
			definitions := decodeDefinitions(t, task.inputs["containerDefinitions"].StringValue())
			application := definitions[0]
			if webRuntime == "nginx-fpm" {
				application = definitionsByName(definitions)["php-fpm"]
				if _, exists := definitionsByName(definitions)["web"]["environment"]; exists {
					t.Fatal("nginx sidecar received capability configuration")
				}
			}
			environment := environmentByName(application)
			want := map[string]string{
				"MAGELIFT_APPLICATION_MODE": "integrated",
				"MAGELIFT_DATABASE_WRITER":  "shop.writer", "MAGELIFT_DATABASE_SECRET_ARN": "arn:aws:secretsmanager:eu-west-3:123456789012:secret:database",
				"MAGELIFT_CACHE_ENDPOINT": "shop.cache", "MAGELIFT_SESSION_ENDPOINT": "shop.sessions", "MAGELIFT_SEARCH_ENDPOINT": "https://shop.search",
				"MAGELIFT_QUEUE_MODE": "rabbitmq", "MAGELIFT_QUEUE_ENDPOINT": "amqps://shop.queue:5671", "MAGELIFT_MEDIA_BUCKET": "shop-media",
			}
			for name, value := range want {
				if environment[name] != value {
					t.Fatalf("%s = %q, want %q", name, environment[name], value)
				}
			}
			for name, value := range map[string]string{
				"MAGENTO_DC_DB__CONNECTION__DEFAULT__HOST":                     "shop.writer",
				"MAGENTO_DC_DB__CONNECTION__DEFAULT__PORT":                     "3306",
				"MAGENTO_DC_DB__CONNECTION__DEFAULT__DBNAME":                   "magento",
				"MAGENTO_DC_DB__CONNECTION__DEFAULT__MODEL":                    "mysql4",
				"MAGENTO_DC_CACHE__FRONTEND__DEFAULT__BACKEND_OPTIONS__SERVER": "shop.cache",
				"MAGENTO_DC_SESSION__REDIS_HOST":                               "shop.sessions",
				"MAGENTO_DC_QUEUE__DEFAULT_CONNECTION":                         "amqp",
			} {
				if environment[name] != value {
					t.Fatalf("%s = %q, want %q", name, environment[name], value)
				}
			}
			encoded := task.inputs["containerDefinitions"].StringValue()
			if strings.Contains(encoded, "database-password") || strings.Contains(encoded, "cache-token-value") {
				t.Fatal("container definitions contain plaintext secret material")
			}
		})
	}
}

func TestExecutionPolicyScopesSecretReadsToARNs(t *testing.T) {
	t.Parallel()
	m := deploy(t, validArgs())
	policy := m.named(t, "aws:iam/rolePolicy:RolePolicy", "shop-execution-policy").inputs["policy"].StringValue()
	for _, value := range []string{"secretsmanager:GetSecretValue", "ssm:GetParameters", "arn:aws:secretsmanager:eu-west-3:123456789012:secret:composer", "arn:aws:ssm:eu-west-3:123456789012:parameter/shop/license"} {
		if !strings.Contains(policy, value) {
			t.Fatalf("execution policy is missing %q: %s", value, policy)
		}
	}
	if strings.Contains(policy, `"Action":["secretsmanager:GetSecretValue"],"Resource":"*"`) {
		t.Fatal("secret read permission is unscoped")
	}
	logsPolicy := m.named(t, "aws:iam/rolePolicy:RolePolicy", "shop-execution-logs-policy")
	if !strings.Contains(logsPolicy.inputs["policy"].StringValue(), "logs:CreateLogStream") || !strings.Contains(logsPolicy.inputs["policy"].StringValue(), "/magelift/shop/preview/*") {
		t.Fatalf("execution logs policy missing awslogs permissions: %#v", logsPolicy.inputs)
	}
}

func TestRuntimeAttachesExistingSecurityGroupAndTargetGroup(t *testing.T) {
	t.Parallel()
	args := validArgs()
	args.WebSecurityGroupID = pulumi.String("sg-web")
	args.TargetGroupARN = pulumi.String("arn:aws:elasticloadbalancing:eu-west-3:123456789012:targetgroup/shop/target")
	m := deploy(t, args)
	for _, resource := range m.snapshot() {
		if strings.Contains(resource, "securityGroup:SecurityGroup") {
			t.Fatalf("runtime created a duplicate web security group: %s", resource)
		}
	}
	service := m.named(t, "aws:ecs/service:Service", "shop-web-service")
	loadBalancers := service.inputs["loadBalancers"].ArrayValue()
	if len(loadBalancers) != 1 {
		t.Fatalf("service load balancer attachment = %#v", loadBalancers)
	}
	lb := loadBalancers[0].ObjectValue()
	if lb["targetGroupArn"].StringValue() != string(args.TargetGroupARN.(pulumi.String)) {
		t.Fatalf("service load balancer attachment = %#v", loadBalancers)
	}
	if lb["containerName"].StringValue() != "varnish" || lb["containerPort"].NumberValue() != float64(VarnishPort) {
		t.Fatalf("integrated load balancer must target varnish:%d, got %#v", VarnishPort, lb)
	}
}

func TestRuntimeRejectsUnsafeInputsBeforeRegistration(t *testing.T) {
	t.Parallel()
	tests := []func(*Args){
		func(args *Args) { args.Image = "ghcr.io/magelift/shop:latest" },
		func(args *Args) { args.WebRuntime = "frankenphp-worker" },
		func(args *Args) { args.PrivateSubnetIDs = nil },
		func(args *Args) { args.ContainerPort = 22 },
		func(args *Args) { args.TaskCPU = "" },
		func(args *Args) { args.DesiredCount = 0 },
		func(args *Args) { args.Secrets[0].ARN = "plaintext" },
		func(args *Args) { args.Secrets = append(args.Secrets, args.Secrets[0]) },
		func(args *Args) {
			args.Capabilities = &CapabilityConfig{DatabaseWriterEndpoint: pulumi.String("shop.writer")}
		},
	}
	for index, mutate := range tests {
		args := validArgs()
		mutate(&args)
		m := &mocks{}
		err := pulumi.RunErr(func(ctx *pulumi.Context) error { _, err := New(ctx, "shop", args); return err }, pulumi.WithMocks("project", "stack", m))
		if index == 3 {
			// Port 22 is a container port, not an inbound rule. It remains valid.
			if err != nil {
				t.Fatalf("container port 22 was incorrectly treated as SSH ingress: %v", err)
			}
			continue
		}
		if err == nil {
			t.Fatalf("case %d was accepted", index)
		}
		if len(m.snapshot()) != 0 {
			t.Fatalf("case %d registered resources before validation", index)
		}
	}
}

func TestRuntimeRequiresStableEncryptionSecretReference(t *testing.T) {
	args := validArgs()
	args.Secrets = args.Secrets[:2]
	m := &mocks{}
	err := pulumi.RunErr(func(ctx *pulumi.Context) error { _, err := New(ctx, "shop", args); return err }, pulumi.WithMocks("project", "stack", m))
	if err == nil || !strings.Contains(err.Error(), "MAGENTO_DC_CRYPT__KEY") {
		t.Fatalf("missing encryption secret was accepted: %v", err)
	}
	if len(m.snapshot()) != 0 {
		t.Fatal("runtime registered resources before rejecting the missing encryption secret")
	}
}

func TestAMQPSettingsNormalizeBrokerEndpoint(t *testing.T) {
	if host, port, ssl := amqpSettings("amqps://broker.example:5671"); host != "broker.example" || port != "5671" || ssl != "1" {
		t.Fatalf("amqp settings = %q:%q ssl=%q", host, port, ssl)
	}
	if host, port, ssl := amqpSettings("not-an-endpoint"); host != "" || port != "" || ssl != "" {
		t.Fatalf("invalid endpoint returned settings = %q:%q ssl=%q", host, port, ssl)
	}
}

func TestFrontendPortSelectsIntegratedVarnishBoundary(t *testing.T) {
	if got := FrontendPort("integrated"); got != VarnishPort {
		t.Fatalf("integrated frontend port = %d, want %d", got, VarnishPort)
	}
	if got := FrontendPort("headless"); got != ApplicationPort {
		t.Fatalf("headless frontend port = %d, want %d", got, ApplicationPort)
	}
}

const testSearchProxyImage = "public.ecr.aws/aws-observability/aws-sigv4-proxy:1.11.1@sha256:34bbec3cb98403d3e040ec1dadb53bb02285f70d2f0ead2d16435fd30980abaa"

func TestRuntimeContainerGraphInteractions(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		mutate func(*Args)
		check  func(*testing.T, *mocks)
	}{
		{
			name: "U1_nginx_fpm_headless_search_proxy_depends_on_web",
			mutate: func(args *Args) {
				args.WebRuntime = "nginx-fpm"
				args.ApplicationMode = "headless"
				args.ContainerPort = ApplicationPort
				args.VarnishImage = ""
				args.SearchProxyImage = testSearchProxyImage
				args.Capabilities = testCapabilities()
			},
			check: func(t *testing.T, m *mocks) {
				byName := taskContainers(t, m, "shop-web-task")
				if byName["search-proxy"] == nil || byName["php-fpm"] == nil || byName["web"] == nil {
					names := make([]string, 0, len(byName))
					for name := range byName {
						names = append(names, name)
					}
					t.Fatalf("headless nginx-fpm containers = %v", names)
				}
				if _, hasVarnish := byName["varnish"]; hasVarnish {
					t.Fatal("headless nginx-fpm must not append varnish")
				}
				assertDependsOnSearchProxy(t, byName["php-fpm"])
			},
		},
		{
			name: "U2_nginx_fpm_integrated_appends_varnish",
			mutate: func(args *Args) {
				args.WebRuntime = "nginx-fpm"
				args.ApplicationMode = "integrated"
				args.ContainerPort = VarnishPort
				args.VarnishImage = testVarnishImage
			},
			check: func(t *testing.T, m *mocks) {
				byName := taskContainers(t, m, "shop-web-task")
				if byName["php-fpm"] == nil || byName["web"] == nil || byName["varnish"] == nil {
					t.Fatalf("nginx-fpm integrated containers = %#v", byName)
				}
				if _, hasProxy := byName["search-proxy"]; hasProxy {
					t.Fatal("unexpected search-proxy without SearchProxyImage")
				}
				if byName["varnish"]["image"] != testVarnishImage {
					t.Fatalf("varnish image = %#v", byName["varnish"]["image"])
				}
			},
		},
		{
			name: "U3_process_runtime_search_proxy_depends_on_web",
			mutate: func(args *Args) {
				args.WebRuntime = "frankenphp-classic"
				args.ApplicationMode = "headless"
				args.ContainerPort = ApplicationPort
				args.VarnishImage = ""
				args.SearchProxyImage = testSearchProxyImage
				args.Capabilities = testCapabilities()
			},
			check: func(t *testing.T, m *mocks) {
				byName := taskContainers(t, m, "shop-web-task")
				if byName["web"] == nil || byName["search-proxy"] == nil || byName["php-fpm"] != nil {
					t.Fatalf("frankenphp-classic containers = %#v", byName)
				}
				assertDependsOnSearchProxy(t, byName["web"])
			},
		},
		{
			name: "U3_queue_consumers_carry_search_proxy",
			mutate: func(args *Args) {
				args.QueueConsumerCount = 2
				args.SearchProxyImage = testSearchProxyImage
				args.Capabilities = testCapabilities()
			},
			check: func(t *testing.T, m *mocks) {
				byName := taskContainers(t, m, "shop-queue-task")
				if byName["queue"] == nil || byName["search-proxy"] == nil {
					t.Fatalf("queue task containers = %#v", byName)
				}
			},
		},
		{
			name: "U4_deploy_and_cron_carry_search_proxy",
			mutate: func(args *Args) {
				args.SearchProxyImage = testSearchProxyImage
				args.Capabilities = testCapabilities()
			},
			check: func(t *testing.T, m *mocks) {
				deploy := taskContainers(t, m, "shop-deploy-task")
				if deploy["deploy"] == nil || deploy["search-proxy"] == nil {
					t.Fatalf("deploy containers = %#v", deploy)
				}
				if deploy["search-proxy"]["essential"] != false {
					t.Fatalf("deploy search-proxy must not be essential: %#v", deploy["search-proxy"])
				}
				cron := taskContainers(t, m, "shop-cron-task")
				if cron["cron"] == nil || cron["search-proxy"] == nil {
					t.Fatalf("cron containers = %#v", cron)
				}
				if cron["search-proxy"]["essential"] != true {
					t.Fatalf("cron search-proxy must stay essential: %#v", cron["search-proxy"])
				}
			},
		},
		{
			name: "U5_queue_mode_db_sets_magento_connection",
			mutate: func(args *Args) {
				caps := testCapabilities()
				caps.QueueMode = pulumi.String("db")
				caps.QueueEndpoint = pulumi.String("")
				args.Capabilities = caps
			},
			check: func(t *testing.T, m *mocks) {
				byName := taskContainers(t, m, "shop-web-task")
				env := environmentByName(byName["php-fpm"])
				if got := env["MAGENTO_DC_QUEUE__DEFAULT_CONNECTION"]; got != "db" {
					t.Fatalf("MAGENTO_DC_QUEUE__DEFAULT_CONNECTION = %q, want db", got)
				}
				if got := env["MAGELIFT_QUEUE_MODE"]; got != "db" {
					t.Fatalf("MAGELIFT_QUEUE_MODE = %q, want db", got)
				}
			},
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			args := validArgs()
			args.Secrets = append([]SecretReference(nil), args.Secrets...)
			tt.mutate(&args)
			m := deploy(t, args)
			tt.check(t, m)
		})
	}
}

func TestAMQPSettingsBrokerEndpointShapes(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		endpoint string
		wantHost string
		wantPort string
		wantSSL  string
	}{
		{name: "amqps_explicit_port", endpoint: "amqps://broker.example:5671", wantHost: "broker.example", wantPort: "5671", wantSSL: "1"},
		{name: "amqp_explicit_port", endpoint: "amqp://broker.example:5672", wantHost: "broker.example", wantPort: "5672", wantSSL: "0"},
		{name: "amqp_default_port", endpoint: "amqp://broker.example", wantHost: "broker.example", wantPort: "5672", wantSSL: "0"},
		{name: "amqps_default_port", endpoint: "amqps://broker.example", wantHost: "broker.example", wantPort: "5671", wantSSL: "1"},
		{name: "artemis_custom_port", endpoint: "amqp://artemis.example:61616", wantHost: "artemis.example", wantPort: "61616", wantSSL: "0"},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			host, port, ssl := amqpSettings(tt.endpoint)
			if host != tt.wantHost || port != tt.wantPort || ssl != tt.wantSSL {
				t.Fatalf("amqpSettings(%q) = %q:%q ssl=%q, want %q:%q ssl=%q", tt.endpoint, host, port, ssl, tt.wantHost, tt.wantPort, tt.wantSSL)
			}
		})
	}
}

func taskContainers(t *testing.T, m *mocks, taskName string) map[string]map[string]any {
	t.Helper()
	task := m.named(t, "aws:ecs/taskDefinition:TaskDefinition", taskName)
	return definitionsByName(decodeDefinitions(t, task.inputs["containerDefinitions"].StringValue()))
}

func assertDependsOnSearchProxy(t *testing.T, definition map[string]any) {
	t.Helper()
	if definition == nil {
		t.Fatal("container definition is nil")
	}
	deps, ok := definition["dependsOn"].([]any)
	if !ok || len(deps) == 0 {
		t.Fatalf("missing search-proxy DependsOn: %#v", definition)
	}
	for _, raw := range deps {
		dep := raw.(map[string]any)
		if dep["containerName"] == "search-proxy" {
			return
		}
	}
	t.Fatalf("DependsOn lacks search-proxy: %#v", definition["dependsOn"])
}

func testCapabilities() *CapabilityConfig {
	return &CapabilityConfig{
		DatabaseWriterEndpoint: pulumi.String("shop.writer"), DatabaseName: "magento", DatabaseSecretARN: pulumi.String("arn:aws:secretsmanager:eu-west-3:123456789012:secret:database"),
		CacheEndpoint: pulumi.String("shop.cache"), SessionEndpoint: pulumi.String("shop.sessions"), SearchEndpoint: pulumi.String("https://shop.search"),
		QueueMode: pulumi.String("rabbitmq"), QueueEndpoint: pulumi.String("amqps://shop.queue:5671"), QueueUsername: pulumi.String("magento_admin"), MediaBucket: pulumi.String("shop-media"),
	}
}

func decodeDefinitions(t *testing.T, encoded string) []map[string]any {
	t.Helper()
	var definitions []map[string]any
	if err := json.Unmarshal([]byte(encoded), &definitions); err != nil {
		t.Fatal(err)
	}
	return definitions
}

func definitionsByName(definitions []map[string]any) map[string]map[string]any {
	result := make(map[string]map[string]any, len(definitions))
	for _, definition := range definitions {
		result[definition["name"].(string)] = definition
	}
	return result
}

func environmentByName(definition map[string]any) map[string]string {
	result := map[string]string{}
	for _, item := range definition["environment"].([]any) {
		value := item.(map[string]any)
		result[value["name"].(string)] = value["value"].(string)
	}
	return result
}

func validArgs() Args {
	return Args{Region: "eu-west-3", ApplicationMode: "integrated", WebRuntime: "nginx-fpm", VpcID: pulumi.String("vpc-private"), PrivateSubnetIDs: pulumi.StringArray{pulumi.String("subnet-private-a"), pulumi.String("subnet-private-b")}, Image: testImage, VarnishImage: testVarnishImage, DatabaseSecretARN: pulumi.String("arn:aws:secretsmanager:eu-west-3:123456789012:secret:database"), EncryptionKeyARN: pulumi.String("arn:aws:secretsmanager:eu-west-3:123456789012:secret:encryption-key"), ContainerPort: VarnishPort, TaskCPU: "256", TaskMemory: "512", DesiredCount: 2, LogGroupPrefix: "/magelift/shop/preview", Secrets: []SecretReference{
		{Name: "COMPOSER_AUTH", ARN: "arn:aws:secretsmanager:eu-west-3:123456789012:secret:composer"},
		{Name: "MAGENTO_LICENSE", ARN: "arn:aws:ssm:eu-west-3:123456789012:parameter/shop/license"},
		{Name: "MAGENTO_DC_CRYPT__KEY", ARN: "arn:aws:secretsmanager:eu-west-3:123456789012:secret:encryption-key"},
	}}
}

func deploy(t *testing.T, args Args) *mocks {
	t.Helper()
	m := &mocks{}
	if err := pulumi.RunErr(func(ctx *pulumi.Context) error { _, err := New(ctx, "shop", args); return err }, pulumi.WithMocks("project", "stack", m)); err != nil {
		t.Fatal(err)
	}
	return m
}
func (m *mocks) snapshot() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]string, len(m.nodes))
	for index, n := range m.nodes {
		result[index] = n.typeToken + ":" + n.name
	}
	sort.Strings(result)
	return result
}
func (m *mocks) one(t *testing.T, token string) node {
	t.Helper()
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, n := range m.nodes {
		if n.typeToken == token {
			return n
		}
	}
	t.Fatalf("resource %s not found", token)
	return node{}
}

func (m *mocks) named(t *testing.T, token, name string) node {
	t.Helper()
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, n := range m.nodes {
		if n.typeToken == token && n.name == name {
			return n
		}
	}
	t.Fatalf("resource %s/%s not found", token, name)
	return node{}
}

func containsResource(resources []string, want string) bool {
	for _, resource := range resources {
		if resource == want {
			return true
		}
	}
	return false
}
