package runtime

import (
	"encoding/json"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

const testImage = "ghcr.io/acourtiol/shop@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
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
		"aws:iam/role:Role:shop-deployment-role", "aws:iam/role:Role:shop-execution-role", "aws:iam/role:Role:shop-task-role", "aws:iam/rolePolicy:RolePolicy:shop-execution-policy", TypeToken + ":shop",
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
	if php["image"] != testImage || php["user"] != "10001:10001" || php["readonlyRootFilesystem"] != true {
		t.Fatalf("php-fpm container definition = %#v", php)
	}
	if web["image"] != testImage || web["command"].([]any)[0] != "nginx" || web["portMappings"] != nil {
		t.Fatalf("nginx container definition = %#v", web)
	}
	if varnish["image"] != testVarnishImage || varnish["user"] != "varnish" || varnish["readonlyRootFilesystem"] != true || varnish["portMappings"].([]any)[0].(map[string]any)["containerPort"] != float64(VarnishPort) {
		t.Fatalf("varnish container definition = %#v", varnish)
	}
	linuxParameters, ok := varnish["linuxParameters"].(map[string]any)
	if !ok {
		t.Fatalf("varnish Linux parameters are missing: %#v", varnish)
	}
	tmpfs, ok := linuxParameters["tmpfs"].([]any)
	if !ok || len(tmpfs) != 1 {
		t.Fatalf("varnish must use one task-scoped tmpfs: %#v", linuxParameters)
	}
	varnishTmpfs, ok := tmpfs[0].(map[string]any)
	if !ok || varnishTmpfs["containerPath"] != "/var/lib/varnish" || varnishTmpfs["size"] != float64(384) {
		t.Fatalf("varnish VSM tmpfs = %#v", varnishTmpfs)
	}
	if !reflect.DeepEqual(varnishTmpfs["mountOptions"], []any{"rw", "exec", "uid=1000", "gid=1000", "mode=0750"}) {
		t.Fatalf("varnish VSM tmpfs options = %#v", varnishTmpfs["mountOptions"])
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
	if !strings.Contains(task.inputs["containerDefinitions"].StringValue(), "MAGENTO_DC_CRYPT__KEY") || !strings.Contains(task.inputs["containerDefinitions"].StringValue(), "encryption-key") {
		t.Fatalf("container definitions do not inject the Magento encryption key reference: %s", task.inputs["containerDefinitions"].StringValue())
	}
	if strings.Contains(task.inputs["containerDefinitions"].StringValue(), "plaintext") {
		t.Fatal("container definition contains plaintext secret material")
	}
	deployTask := m.named(t, "aws:ecs/taskDefinition:TaskDefinition", "shop-deploy-task")
	if _, found := deployTask.inputs["taskRoleArn"]; !found || !strings.Contains(deployTask.inputs["containerDefinitions"].StringValue(), "app:config:import") || !strings.Contains(deployTask.inputs["containerDefinitions"].StringValue(), "setup:upgrade") || !strings.Contains(deployTask.inputs["containerDefinitions"].StringValue(), "cache:flush") {
		t.Fatalf("deploy task role or command is unsafe: inputs=%#v", deployTask.inputs)
	}
	if !strings.Contains(deployTask.inputs["containerDefinitions"].StringValue(), "MAGELIFT_DATABASE_CREDENTIALS") {
		t.Fatal("deploy task does not receive the managed database secret reference")
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

func TestRuntimeAddsSigV4ProxyForMagentoOpenSearch(t *testing.T) {
	t.Parallel()
	args := validArgs()
	args.Capabilities = testCapabilities()
	args.SearchProxyImage = "public.ecr.aws/aws-observability/aws-sigv4-proxy:1.11.1@sha256:34bbec3cb98403d3e040ec1dadb53bb02285f70d2f0ead2d16435fd30980abaa"
	m := deploy(t, args)
	task := m.named(t, "aws:ecs/taskDefinition:TaskDefinition", "shop-web-task")
	var definitions []map[string]any
	if err := json.Unmarshal([]byte(task.inputs["containerDefinitions"].StringValue()), &definitions); err != nil {
		t.Fatal(err)
	}
	proxy := definitionsByName(definitions)["search-proxy"]
	if proxy == nil {
		t.Fatalf("search proxy is missing: %#v", definitions)
	}
	if proxy["image"] != args.SearchProxyImage || proxy["essential"] != true {
		t.Fatalf("search proxy definition = %#v", proxy)
	}
	command := proxy["command"].([]any)
	for _, required := range []string{"--port", "8081", "--name", "es", "--region", "eu-west-3", "--host", "shop.search", "--sign-host", "shop.search"} {
		found := false
		for _, value := range command {
			if value == required {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("search proxy command lacks %q: %#v", required, command)
		}
	}
	if _, hasUser := proxy["user"]; hasUser {
		t.Fatalf("search proxy should use its image user: %#v", proxy)
	}
	php := definitionsByName(definitions)["php-fpm"]
	web := definitionsByName(definitions)["web"]
	if len(php["dependsOn"].([]any)) != 1 || web["dependsOn"] != nil {
		t.Fatalf("search proxy dependency placement = php=%#v web=%#v", php["dependsOn"], web["dependsOn"])
	}
	environment := environmentByName(php)
	for name, value := range map[string]string{
		"MAGENTO_DC_CATALOG__SEARCH__ENGINE":                     "opensearch",
		"MAGENTO_DC_CATALOG__SEARCH__OPENSEARCH_SERVER_HOSTNAME": "127.0.0.1",
		"MAGENTO_DC_CATALOG__SEARCH__OPENSEARCH_SERVER_PORT":     "8081",
		"MAGENTO_DC_CATALOG__SEARCH__OPENSEARCH_ENABLE_AUTH":     "0",
		"MAGENTO_DC_CATALOG__SEARCH__OPENSEARCH_INDEX_PREFIX":    "magento2",
		"MAGENTO_DC_CATALOG__SEARCH__OPENSEARCH_SERVER_TIMEOUT":  "15",
	} {
		if environment[name] != value {
			t.Fatalf("%s = %q, want %q", name, environment[name], value)
		}
	}
}

func TestRuntimeCreatesQueueConsumerServiceWhenRequested(t *testing.T) {
	t.Parallel()
	args := validArgs()
	args.QueueConsumerCount = 2
	m := deploy(t, args)
	if !containsResource(m.snapshot(), "aws:ecs/service:Service:shop-queue-service") || !containsResource(m.snapshot(), "aws:ecs/taskDefinition:TaskDefinition:shop-queue-task") {
		t.Fatalf("queue consumer resources were not created: %v", m.snapshot())
	}
	queueTask := m.named(t, "aws:ecs/taskDefinition:TaskDefinition", "shop-queue-task")
	if !strings.Contains(queueTask.inputs["containerDefinitions"].StringValue(), "queue:consumers:start") {
		t.Fatalf("queue task does not start the Magento consumer: %s", queueTask.inputs["containerDefinitions"].StringValue())
	}
}

func TestRuntimeFrankenPHPClassicUsesSingleHTTPContainer(t *testing.T) {
	t.Parallel()
	args := validArgs()
	args.WebRuntime = "frankenphp-classic"
	args.ApplicationMode = "headless"
	args.ContainerPort = ApplicationPort
	args.VarnishImage = ""
	m := deploy(t, args)
	task := m.named(t, "aws:ecs/taskDefinition:TaskDefinition", "shop-web-task")
	var definitions []map[string]any
	if err := json.Unmarshal([]byte(task.inputs["containerDefinitions"].StringValue()), &definitions); err != nil {
		t.Fatal(err)
	}
	if len(definitions) != 1 || definitions[0]["name"] != "web" || len(definitions[0]["portMappings"].([]any)) != 1 {
		t.Fatalf("FrankenPHP classic task containers = %#v", definitions)
	}
}

func TestRuntimeInjectsNonSecretCapabilityReferences(t *testing.T) {
	t.Parallel()
	for _, webRuntime := range []string{"nginx-fpm", "frankenphp-classic"} {
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
	policy := m.one(t, "aws:iam/rolePolicy:RolePolicy").inputs["policy"].StringValue()
	for _, value := range []string{"secretsmanager:GetSecretValue", "ssm:GetParameters", "arn:aws:secretsmanager:eu-west-3:123456789012:secret:composer", "arn:aws:ssm:eu-west-3:123456789012:parameter/shop/license"} {
		if !strings.Contains(policy, value) {
			t.Fatalf("execution policy is missing %q: %s", value, policy)
		}
	}
	if strings.Contains(policy, `"Action":["secretsmanager:GetSecretValue"],"Resource":"*"`) {
		t.Fatal("secret read permission is unscoped")
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
	if len(loadBalancers) != 1 || loadBalancers[0].ObjectValue()["targetGroupArn"].StringValue() != string(args.TargetGroupARN.(pulumi.String)) {
		t.Fatalf("service load balancer attachment = %#v", loadBalancers)
	}
}

func TestRuntimeRejectsUnsafeInputsBeforeRegistration(t *testing.T) {
	t.Parallel()
	tests := []func(*Args){
		func(args *Args) { args.Image = "ghcr.io/acourtiol/shop:latest" },
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

func testCapabilities() *CapabilityConfig {
	return &CapabilityConfig{
		DatabaseWriterEndpoint: pulumi.String("shop.writer"), DatabaseSecretARN: pulumi.String("arn:aws:secretsmanager:eu-west-3:123456789012:secret:database"),
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
	return Args{Region: "eu-west-3", ApplicationMode: "integrated", WebRuntime: "nginx-fpm", VpcID: pulumi.String("vpc-private"), PrivateSubnetIDs: pulumi.StringArray{pulumi.String("subnet-private-a"), pulumi.String("subnet-private-b")}, Image: testImage, VarnishImage: testVarnishImage, DatabaseSecretARN: pulumi.String("arn:aws:secretsmanager:eu-west-3:123456789012:secret:database"), EncryptionKeyARN: pulumi.String("arn:aws:secretsmanager:eu-west-3:123456789012:secret:encryption-key"), ContainerPort: VarnishPort, TaskCPU: "256", TaskMemory: "512", DesiredCount: 2, Secrets: []SecretReference{
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
