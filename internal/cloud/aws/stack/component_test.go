package stack

import (
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/magelift/magelift/internal/cloud/aws/queue"
	sdk "github.com/magelift/magelift/sdk/v1"
	awsprovider "github.com/pulumi/pulumi-aws/sdk/v7/go/aws"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type stackMocks struct {
	mu        sync.Mutex
	resources []pulumi.MockResourceArgs
	invokes   []pulumi.MockCallArgs
}

func (m *stackMocks) NewResource(args pulumi.MockResourceArgs) (string, resource.PropertyMap, error) {
	m.mu.Lock()
	m.resources = append(m.resources, args)
	m.mu.Unlock()
	state := args.Inputs.Copy()
	switch args.TypeToken {
	case "aws:lb/loadBalancer:LoadBalancer":
		state["arn"] = resource.NewStringProperty("arn:aws:elasticloadbalancing:eu-west-3:123456789012:loadbalancer/app/shop/abc")
		state["arnSuffix"] = resource.NewStringProperty("app/shop/abc")
		state["dnsName"] = resource.NewStringProperty("shop.eu-west-3.elb.amazonaws.com")
	case "aws:lb/targetGroup:TargetGroup":
		state["arn"] = resource.NewStringProperty("arn:aws:elasticloadbalancing:eu-west-3:123456789012:targetgroup/shop/def")
	case "aws:lb/listener:Listener":
		state["arn"] = resource.NewStringProperty("arn:aws:elasticloadbalancing:eu-west-3:123456789012:listener/app/shop/abc/ghi")
	case "aws:ecs/cluster:Cluster":
		state["arn"] = resource.NewStringProperty("arn:aws:ecs:eu-west-3:123456789012:cluster/shop")
		state["name"] = resource.NewStringProperty("shop-cluster")
	case "aws:ecs/service:Service":
		state["name"] = resource.NewStringProperty("shop-web-service")
	case "aws:ecs/taskDefinition:TaskDefinition":
		state["arn"] = resource.NewStringProperty("arn:aws:ecs:eu-west-3:123456789012:task-definition/shop-web:1")
	case "aws:iam/role:Role":
		state["arn"] = resource.NewStringProperty("arn:aws:iam::123456789012:role/" + args.Name)
		state["name"] = resource.NewStringProperty(args.Name)
	case "aws:rds/cluster:Cluster":
		state["arn"] = resource.NewStringProperty("arn:aws:rds:eu-west-3:123456789012:cluster/shop")
		state["endpoint"] = resource.NewStringProperty("shop.writer")
		state["readerEndpoint"] = resource.NewStringProperty("shop.reader")
		state["masterUserSecrets"] = resource.NewArrayProperty([]resource.PropertyValue{resource.NewObjectProperty(resource.PropertyMap{"secretArn": resource.NewStringProperty("arn:aws:secretsmanager:eu-west-3:123456789012:secret:managed")})})
	case "aws:rds/instance:Instance":
		state["arn"] = resource.NewStringProperty("arn:aws:rds:eu-west-3:123456789012:db:shop")
		state["address"] = resource.NewStringProperty("shop.writer")
		state["endpoint"] = resource.NewStringProperty("shop.writer:3306")
		state["masterUserSecrets"] = resource.NewArrayProperty([]resource.PropertyValue{resource.NewObjectProperty(resource.PropertyMap{"secretArn": resource.NewStringProperty("arn:aws:secretsmanager:eu-west-3:123456789012:secret:managed")})})
	case "aws:elasticache/replicationGroup:ReplicationGroup":
		state["primaryEndpointAddress"] = resource.NewStringProperty(args.Name + ".cache.amazonaws.com")
	case "aws:ec2/instance:Instance":
		state["id"] = resource.NewStringProperty("i-" + args.Name)
		state["privateIp"] = resource.NewStringProperty("10.42.0.10")
	case "aws:ec2/eip:Eip":
		state["id"] = resource.NewStringProperty("eipalloc-" + args.Name)
		state["publicIp"] = resource.NewStringProperty("203.0.113.10")
	case "aws:ec2/securityGroup:SecurityGroup":
		state["id"] = resource.NewStringProperty("sg-" + args.Name)
	case "aws:ec2/ami:Ami":
		state["id"] = resource.NewStringProperty("ami-fcknat")
	case "aws:mq/broker:Broker":
		state["arn"] = resource.NewStringProperty("arn:aws:mq:eu-west-3:123456789012:broker:shop")
		state["instances"] = resource.NewArrayProperty([]resource.PropertyValue{resource.NewObjectProperty(resource.PropertyMap{
			"endpoints":  resource.NewArrayProperty([]resource.PropertyValue{resource.NewStringProperty("amqps://shop.mq.eu-west-3.amazonaws.com:5671")}),
			"consoleUrl": resource.NewStringProperty("https://shop.mq.eu-west-3.amazonaws.com:15671"),
		})})
	case "aws:opensearch/serverlessCollection:ServerlessCollection":
		state["arn"] = resource.NewStringProperty("arn:aws:aoss:eu-west-3:123456789012:collection/shop")
		state["collectionEndpoint"] = resource.NewStringProperty("https://shop.eu-west-3.aoss.amazonaws.com")
		state["dashboardEndpoint"] = resource.NewStringProperty("https://shop.eu-west-3.aoss.amazonaws.com/_dashboards")
	case "aws:opensearch/domain:Domain":
		state["arn"] = resource.NewStringProperty("arn:aws:es:eu-west-3:123456789012:domain/shop")
		state["endpoint"] = resource.NewStringProperty("shop.eu-west-3.es.amazonaws.com")
		state["dashboardEndpoint"] = resource.NewStringProperty("shop.eu-west-3.es.amazonaws.com/_dashboards")
	case "aws:s3/bucket:Bucket":
		state["arn"] = resource.NewStringProperty("arn:aws:s3:::shop-media")
		state["bucket"] = resource.NewStringProperty("shop-media")
		state["bucketRegionalDomainName"] = resource.NewStringProperty("shop-media.s3.eu-west-3.amazonaws.com")
	case "aws:cloudfront/distribution:Distribution":
		state["arn"] = resource.NewStringProperty("arn:aws:cloudfront::123456789012:distribution/EDFDVBD6EXAMPLE")
		state["domainName"] = resource.NewStringProperty("d111111abcdef8.cloudfront.net")
		state["hostedZoneId"] = resource.NewStringProperty("Z2FDTNDATAQYW2")
	case "aws:wafv2/webAcl:WebAcl":
		state["arn"] = resource.NewStringProperty("arn:aws:wafv2:us-east-1:123456789012:global/webacl/shop/abc")
	case "aws:cloudwatch/metricAlarm:MetricAlarm":
		state["arn"] = resource.NewStringProperty("arn:aws:cloudwatch:eu-west-3:123456789012:alarm:" + args.Name)
	}
	return args.Name + "-id", state, nil
}

func (m *stackMocks) Call(args pulumi.MockCallArgs) (resource.PropertyMap, error) {
	m.mu.Lock()
	m.invokes = append(m.invokes, args)
	m.mu.Unlock()
	switch args.Token {
	case "aws:ec2/getAmi:getAmi", "aws:index/getAmi:getAmi":
		return resource.PropertyMap{
			"id":           resource.NewStringProperty("ami-fcknat"),
			"architecture": resource.NewStringProperty("arm64"),
		}, nil
	default:
		return resource.PropertyMap{"secretString": resource.MakeSecret(resource.NewStringProperty("mock-secret"))}, nil
	}
}

func TestNewComposesPreviewAWSStackWithPulumiOutputs(t *testing.T) {
	t.Parallel()
	m := &stackMocks{}
	var outputs []string
	var exported pulumi.Map
	err := pulumi.RunErr(func(ctx *pulumi.Context) error {
		providers, err := NewProviders(ctx, "shop", "eu-west-3")
		if err != nil {
			return err
		}
		component, err := New(ctx, "shop-preview-1", validSpec(), providers)
		if err != nil {
			return err
		}
		exported = component.Outputs()
		for key, value := range exported {
			ctx.Export(key, value)
		}
		pulumi.All(component.Edge.DistributionDomainName, component.Ingress.TargetGroupARN, component.Runtime.TaskRoleARN, component.Database.WriterEndpoint).ApplyT(func(values []interface{}) string {
			for _, value := range values {
				outputs = append(outputs, value.(string))
			}
			return strings.Join(outputs, ",")
		})
		return nil
	}, pulumi.WithMocks("magelift", "test", m))
	if err != nil {
		t.Fatal(err)
	}
	if componentCount(m, "magelift:aws:Network") != 1 || componentCount(m, "magelift:aws:Ingress") != 1 || componentCount(m, "magelift:aws:EcsRuntime") != 1 || componentCount(m, "magelift:aws:AuroraMysql") != 1 || componentCount(m, "magelift:aws:Valkey") != 1 || componentCount(m, "magelift:aws:OpenSearch") != 1 || componentCount(m, "magelift:aws:MediaStorage") != 1 || componentCount(m, "magelift:aws:Edge") != 1 {
		t.Fatalf("stack did not compose expected components")
	}
	if componentCount(m, "magelift:aws:EcsRuntimeIdentity") != 1 {
		t.Fatal("stack did not split runtime identity from task definitions")
	}
	if len(outputs) != 4 {
		t.Fatalf("stack outputs did not resolve through Pulumi graph: %#v", outputs)
	}
	for _, key := range []string{"clusterName", "serviceName", "deployTaskDefinitionArn", "securityGroupId", "applicationURL", "mediaURL", "mediaBucket", "privateSubnetIds"} {
		if _, ok := exported[key]; !ok {
			t.Fatalf("Component.Outputs missing %q", key)
		}
	}
	policy := resourceInput(m, "aws:iam/rolePolicy:RolePolicy", "shop-preview-1-task-policy")
	policyText := policy["policy"].StringValue()
	for _, required := range []string{"s3:GetObject", "aoss:APIAccessAll", "secretsmanager:GetSecretValue", "kms:Decrypt", "ssmmessages:OpenDataChannel"} {
		if !strings.Contains(policyText, required) {
			t.Fatalf("task policy is missing %q: %s", required, policyText)
		}
	}
	deploymentPolicy := resourceInput(m, "aws:iam/rolePolicy:RolePolicy", "shop-preview-1-deployment-policy")
	if deploymentPolicy == nil || !strings.Contains(deploymentPolicy["policy"].StringValue(), "s3:GetObject") {
		t.Fatal("deployment task capability policy was not registered")
	}
	executionDatabasePolicy := resourceInput(m, "aws:iam/rolePolicy:RolePolicy", "shop-preview-1-execution-database-policy")
	if executionDatabasePolicy == nil || !strings.Contains(executionDatabasePolicy["policy"].StringValue(), "secretsmanager:GetSecretValue") || !strings.Contains(executionDatabasePolicy["policy"].StringValue(), "arn:aws:kms:eu-west-3:123456789012:key/11111111-2222-3333-4444-555555555555") {
		t.Fatalf("ECS execution role cannot read the managed database secret: %#v", executionDatabasePolicy)
	}
	webTask := resourceInput(m, "aws:ecs/taskDefinition:TaskDefinition", "shop-preview-1-runtime-web-task")
	definitions := webTask["containerDefinitions"].StringValue()
	for _, required := range []string{"MAGELIFT_DATABASE_WRITER", "shop.writer", "MAGELIFT_DATABASE_SECRET_ARN", "arn:aws:secretsmanager:eu-west-3:123456789012:secret:managed", "MAGELIFT_CACHE_ENDPOINT", "MAGELIFT_SEARCH_ENDPOINT", "MAGELIFT_MEDIA_BUCKET", "shop-media", "aws-observability/aws-sigv4-proxy:1.11.1", "docker.io/library/varnish:8.0.2", "VARNISH_HTTP_PORT", "6081", "MAGENTO_DC_CATALOG__SEARCH__ENGINE", "127.0.0.1"} {
		if !strings.Contains(definitions, required) {
			t.Fatalf("runtime capability configuration is missing %q: %s", required, definitions)
		}
	}
	targetGroup := resourceInput(m, "aws:lb/targetGroup:TargetGroup", "shop-preview-1-ingress-web")
	if targetGroup["port"].NumberValue() != 6081 {
		t.Fatalf("integrated target group port = %v, want 6081", targetGroup["port"])
	}
	if !strings.Contains(definitions, "MAGELIFT_DATABASE_CREDENTIALS") {
		t.Fatal("runtime task does not receive the managed database secret reference")
	}
	if strings.Contains(definitions, "mock-secret") {
		t.Fatal("runtime capability configuration contains a resolved secret value")
	}
	deployTask := resourceInput(m, "aws:ecs/taskDefinition:TaskDefinition", "shop-preview-1-runtime-deploy-task")
	if !strings.Contains(deployTask["taskRoleArn"].StringValue(), "deployment-role") {
		t.Fatalf("deploy task does not use the deployment identity: %v", deployTask)
	}
}

func TestNewComposesExistingDatabaseWithoutRDSCreates(t *testing.T) {
	t.Parallel()
	const (
		identifier = "db-magento-prod"
		endpoint   = "magento.xxxxx.eu-west-3.rds.amazonaws.com"
		secretARN  = "arn:aws:secretsmanager:eu-west-3:123456789012:secret:shop-db-master"
	)
	spec := validSpec()
	ref := sdk.ExistingResourceRef{ID: "database", Provider: "aws", Kind: sdk.ExistingDatabase, ExternalID: identifier}
	spec.Existing.Database = &ref
	spec.Existing.DatabaseSecretARN = secretARN
	spec.Existing.DatabaseEndpoint = endpoint

	m := &stackMocks{}
	err := pulumi.RunErr(func(ctx *pulumi.Context) error {
		providers, err := NewProviders(ctx, "shop", "eu-west-3")
		if err != nil {
			return err
		}
		_, err = New(ctx, "shop-preview-1", spec, providers)
		return err
	}, pulumi.WithMocks("magelift", "test", m))
	if err != nil {
		t.Fatal(err)
	}
	if componentCount(m, "aws:rds/cluster:Cluster")+componentCount(m, "aws:rds/instance:Instance")+componentCount(m, "aws:rds/subnetGroup:SubnetGroup") != 0 {
		t.Fatal("existing database adopt created managed RDS resources")
	}
	if componentCount(m, "magelift:aws:AuroraMysql") != 1 {
		t.Fatal("existing database component was not registered")
	}
	dbComp := resourceInput(m, "magelift:aws:AuroraMysql", "shop-preview-1-database")
	if dbComp == nil || dbComp["existingEndpoint"].StringValue() != endpoint || dbComp["existingSecretArn"].StringValue() != secretARN || dbComp["existingIdentifier"].StringValue() != identifier {
		t.Fatalf("database Existing refs not wired into database.New: %#v", dbComp)
	}
	executionDatabasePolicy := resourceInput(m, "aws:iam/rolePolicy:RolePolicy", "shop-preview-1-execution-database-policy")
	if executionDatabasePolicy == nil || !strings.Contains(executionDatabasePolicy["policy"].StringValue(), secretARN) {
		t.Fatalf("ECS execution role must scope GetSecretValue to the adopted secret ARN: %#v", executionDatabasePolicy)
	}
	policyText := executionDatabasePolicy["policy"].StringValue()
	if strings.Contains(policyText, "arn:aws:secretsmanager:*") || strings.Contains(policyText, `"Resource":"*"`) {
		t.Fatalf("execution database policy broadened beyond single adopted ARN: %s", policyText)
	}
	webTask := resourceInput(m, "aws:ecs/taskDefinition:TaskDefinition", "shop-preview-1-runtime-web-task")
	definitions := webTask["containerDefinitions"].StringValue()
	if !strings.Contains(definitions, endpoint) || !strings.Contains(definitions, secretARN) {
		t.Fatalf("runtime must receive adopted endpoint and secret ARN: %s", definitions)
	}
}

func TestNewComposesStandardThreeZoneStack(t *testing.T) {
	t.Parallel()
	m := &stackMocks{}
	spec := validSpec()
	spec.Identity.Environment = "staging-1"
	spec.Identity.EnvironmentClass = "staging"
	spec.Identity.Preset = "standard"
	spec.Lifecycle.ExpiresAt = time.Time{}
	spec.Policy.AvailabilityZones = []string{"eu-west-3a", "eu-west-3b", "eu-west-3c"}
	spec.Dependencies.SessionSecretARN = "arn:aws:secretsmanager:eu-west-3:123456789012:secret:shop-session-token"
	spec.Dependencies.QueueSecretARN = "arn:aws:secretsmanager:eu-west-3:123456789012:secret:shop-queue-token"
	spec.Catalog.Valkey.ReplicaCount = 1
	spec.Catalog.Fargate.DesiredCount = 2
	spec.Catalog.SearchMode = SearchModeProvisioned
	spec.Catalog.AuroraProvisioned = AuroraProvisionedProfile{InstanceClass: "db.r8g.large", InstanceCount: 2}
	spec.Catalog.SearchProvisioned = SearchProvisionedProfile{InstanceType: "m7g.large.search", InstanceCount: 2, EBSVolumeType: "gp3", EBSVolumeSizeGiB: 200}
	spec.Catalog.RabbitMQ = RabbitMQProfile{InstanceType: "mq.m7g.large"}
	err := pulumi.RunErr(func(ctx *pulumi.Context) error {
		providers, err := NewProviders(ctx, "shop", "eu-west-3")
		if err != nil {
			return err
		}
		_, err = New(ctx, "shop-staging-1", spec, providers)
		return err
	}, pulumi.WithMocks("magelift", "test", m))
	if err != nil {
		t.Fatal(err)
	}
	if componentCount(m, "magelift:aws:RabbitMqQueue") != 1 || componentCount(m, "aws:mq/broker:Broker") != 1 || componentCount(m, "aws:rds/clusterInstance:ClusterInstance") != 2 {
		t.Fatalf("standard stack did not create managed production capabilities")
	}
}

func TestNewComposesHighAvailabilityStack(t *testing.T) {
	t.Parallel()
	m := &stackMocks{}
	spec := validSpec()
	spec.Identity.Environment = "production"
	spec.Identity.EnvironmentClass = "production"
	spec.Identity.Preset = "high-availability"
	spec.Lifecycle.ExpiresAt = time.Time{}
	spec.Policy.AvailabilityZones = []string{"eu-west-3a", "eu-west-3b", "eu-west-3c"}
	spec.Dependencies.SessionSecretARN = "arn:aws:secretsmanager:eu-west-3:123456789012:secret:shop-session-token"
	spec.Dependencies.QueueSecretARN = "arn:aws:secretsmanager:eu-west-3:123456789012:secret:shop-queue-token"
	spec.Catalog.Valkey.ReplicaCount = 2
	spec.Catalog.Fargate.DesiredCount = 3
	spec.Catalog.SearchMode = SearchModeProvisioned
	spec.Catalog.AuroraProvisioned = AuroraProvisionedProfile{InstanceClass: "db.r8g.large", InstanceCount: 3}
	spec.Catalog.SearchProvisioned = SearchProvisionedProfile{InstanceType: "m7g.large.search", InstanceCount: 3, DedicatedMasterType: "m7g.large.search", DedicatedMasterCount: 3, EBSVolumeType: "gp3", EBSVolumeSizeGiB: 400}
	spec.Catalog.RabbitMQ = RabbitMQProfile{InstanceType: "mq.m7g.large"}
	err := pulumi.RunErr(func(ctx *pulumi.Context) error {
		providers, err := NewProviders(ctx, "shop", "eu-west-3")
		if err != nil {
			return err
		}
		_, err = New(ctx, "shop-production", spec, providers)
		return err
	}, pulumi.WithMocks("magelift", "test", m))
	if err != nil {
		t.Fatal(err)
	}
	if componentCount(m, "magelift:aws:RabbitMqQueue") != 1 || componentCount(m, "aws:rds/clusterInstance:ClusterInstance") != 3 || componentCount(m, "aws:opensearch/domain:Domain") != 1 {
		t.Fatalf("high-availability stack did not create the selected topology")
	}
}

func TestNewRejectsInvalidPlanBeforePulumiRegistration(t *testing.T) {
	m := &stackMocks{}
	spec := validSpec()
	spec.Artifact.ImageDigest = "latest"
	err := pulumi.RunErr(func(ctx *pulumi.Context) error {
		providers, err := NewProviders(ctx, "shop", "eu-west-3")
		if err != nil {
			return err
		}
		_, err = New(ctx, "shop-preview-1", spec, providers)
		return err
	}, pulumi.WithMocks("magelift", "test", m))
	if err == nil || len(m.resources) != 2 {
		t.Fatalf("invalid plan registration = %d resources, error = %v", len(m.resources), err)
	}
}

func TestNewComposesPreviewEscapeHatches(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		edit func(*Spec)
		want map[string]int
		deny []string
	}{
		{
			name: "fck-nat",
			edit: func(spec *Spec) { spec.Policy.NatMode = NatModeFckNat },
			want: map[string]int{
				"aws:ec2/instance:Instance":     1,
				"aws:ec2/natGateway:NatGateway": 0,
			},
		},
		{
			name: "rds-mysql",
			edit: func(spec *Spec) {
				spec.Catalog.DatabaseEngine = DatabaseEngineRDSMySQL
				spec.Catalog.Versions.MySQL = "8.4.10"
				spec.Catalog.AuroraProvisioned = AuroraProvisionedProfile{InstanceClass: "db.t4g.micro"}
			},
			want: map[string]int{
				"magelift:aws:AuroraMysql":  0,
				"magelift:aws:RdsMysql":     1,
				"aws:rds/instance:Instance": 1,
				"aws:rds/cluster:Cluster":   0,
			},
		},
		{
			name: "search-disabled",
			edit: func(spec *Spec) { spec.Catalog.SearchMode = SearchModeDisabled },
			want: map[string]int{
				"magelift:aws:OpenSearch":                                  0,
				"aws:opensearch/serverlessCollection:ServerlessCollection": 0,
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			m := &stackMocks{}
			spec := validSpec()
			test.edit(&spec)
			if err := spec.Validate(); err != nil {
				t.Fatal(err)
			}
			err := pulumi.RunErr(func(ctx *pulumi.Context) error {
				providers, err := NewProviders(ctx, "shop", "eu-west-3")
				if err != nil {
					return err
				}
				_, err = New(ctx, "shop-preview-1", spec, providers)
				return err
			}, pulumi.WithMocks("magelift", "test", m))
			if err != nil {
				t.Fatal(err)
			}
			for token, count := range test.want {
				if got := componentCount(m, token); got != count {
					t.Fatalf("%s count = %d, want %d", token, got, count)
				}
			}
			for _, token := range test.deny {
				if componentCount(m, token) != 0 {
					t.Fatalf("unexpected resource %s", token)
				}
			}
		})
	}
}

func componentCount(m *stackMocks, token string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	count := 0
	for _, resource := range m.resources {
		if resource.TypeToken == token {
			count++
		}
	}
	return count
}

func resourceInput(m *stackMocks, token, name string) resource.PropertyMap {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, item := range m.resources {
		if item.TypeToken == token && item.Name == name {
			return item.Inputs
		}
	}
	return nil
}

func TestCatalogCellProjectionMatrix(t *testing.T) {
	t.Parallel()

	webRuntimes := []string{"nginx-fpm", "frankenphp-classic"}
	type cell struct {
		name             string
		preset           sdk.PresetID
		searchMode       string
		queueMode        string
		webRuntime       string
		leaveSearchUnset bool
		leaveQueueUnset  bool
		wantRejected     bool
		wantRejectReason string
	}

	var tests []cell
	for _, searchMode := range []string{SearchModeDisabled, SearchModeServerless} {
		for _, queueMode := range []string{QueueModeDB, QueueModeECSRabbitMQ, QueueModeECSArtemis} {
			for _, webRuntime := range webRuntimes {
				tests = append(tests, cell{
					name:       "preview/" + searchMode + "/" + queueMode + "/" + webRuntime,
					preset:     sdk.PresetPreview,
					searchMode: searchMode,
					queueMode:  queueMode,
					webRuntime: webRuntime,
				})
			}
		}
	}
	for _, searchMode := range []string{SearchModeDisabled, SearchModeProvisioned} {
		for _, queueMode := range []string{QueueModeDB, QueueModeAmazonMQ, QueueModeECSRabbitMQ, QueueModeECSArtemis} {
			for _, webRuntime := range webRuntimes {
				tests = append(tests, cell{
					name:       "standard/" + searchMode + "/" + queueMode + "/" + webRuntime,
					preset:     sdk.PresetStandard,
					searchMode: searchMode,
					queueMode:  queueMode,
					webRuntime: webRuntime,
				})
			}
		}
	}
	tests = append(tests,
		cell{name: "preview/defaults", preset: sdk.PresetPreview, webRuntime: "nginx-fpm", leaveSearchUnset: true, leaveQueueUnset: true},
		cell{name: "standard/defaults", preset: sdk.PresetStandard, webRuntime: "nginx-fpm", leaveSearchUnset: true, leaveQueueUnset: true},
		cell{
			name: "preview/amazon-mq/rejected", preset: sdk.PresetPreview, searchMode: SearchModeServerless,
			queueMode: QueueModeAmazonMQ, webRuntime: "nginx-fpm", wantRejected: true,
			wantRejectReason: "amazon-mq cluster deployment requires exactly three unique availability zones",
		},
	)

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if tt.wantRejected {
				assertPreviewAmazonMQRejected(t, tt.wantRejectReason)
				return
			}

			spec := validSpec()
			spec.Identity.Preset = tt.preset
			spec.Application.WebRuntime = tt.webRuntime
			if tt.leaveSearchUnset {
				spec.Catalog.SearchMode = ""
			} else {
				spec.Catalog.SearchMode = tt.searchMode
			}
			if tt.leaveQueueUnset {
				spec.Catalog.QueueMode = ""
			} else {
				spec.Catalog.QueueMode = tt.queueMode
			}

			resolvedSearch := resolveSearchMode(spec.Catalog.SearchMode, tt.preset)
			resolvedQueue := resolveQueueMode(spec.Catalog.QueueMode, tt.preset)
			if tt.leaveSearchUnset {
				wantSearch := SearchModeServerless
				if tt.preset == sdk.PresetStandard {
					wantSearch = SearchModeProvisioned
				}
				if resolvedSearch != wantSearch {
					t.Fatalf("resolveSearchMode default = %q, want %q", resolvedSearch, wantSearch)
				}
				spec.Catalog.SearchMode = resolvedSearch
			}
			if tt.leaveQueueUnset {
				wantQueue := QueueModeDB
				if tt.preset == sdk.PresetStandard {
					wantQueue = QueueModeAmazonMQ
				}
				if resolvedQueue != wantQueue {
					t.Fatalf("resolveQueueMode default = %q, want %q", resolvedQueue, wantQueue)
				}
			}

			wantConsumers := 2
			effectiveQueue := effectiveQueueMode(spec)
			if effectiveQueue == QueueModeDB {
				wantConsumers = 0
			}
			if got := queueConsumerCount(spec); got != wantConsumers {
				t.Fatalf("queueConsumerCount = %d, want %d (queueMode=%q effective=%q)", got, wantConsumers, spec.Catalog.QueueMode, effectiveQueue)
			}
			if !tt.leaveQueueUnset && effectiveQueue != tt.queueMode {
				t.Fatalf("effectiveQueueMode = %q, want %q", effectiveQueue, tt.queueMode)
			}
			if tt.leaveQueueUnset && effectiveQueue != resolvedQueue {
				t.Fatalf("effectiveQueueMode default = %q, want %q", effectiveQueue, resolvedQueue)
			}

			wantVarnish := varnishImage
			if spec.Application.Mode != "integrated" {
				wantVarnish = ""
			}
			if got := varnishImageFor(spec.Application.Mode); got != wantVarnish {
				t.Fatalf("varnishImageFor = %q, want %q", got, wantVarnish)
			}

			// Mirrors component.go searchProxyImage projection.
			searchProxyImage := ""
			if spec.Catalog.SearchMode != SearchModeDisabled {
				searchProxyImage = sigV4ProxyImage
			}
			if spec.Catalog.SearchMode == SearchModeDisabled {
				if searchProxyImage != "" {
					t.Fatalf("searchProxyImage = %q, want empty for disabled search", searchProxyImage)
				}
			} else if searchProxyImage != sigV4ProxyImage {
				t.Fatalf("searchProxyImage = %q, want %q", searchProxyImage, sigV4ProxyImage)
			}
			if spec.Application.WebRuntime != tt.webRuntime {
				t.Fatalf("WebRuntime = %q, want %q", spec.Application.WebRuntime, tt.webRuntime)
			}
		})
	}
}

// assertPreviewAmazonMQRejected exercises the AZ guard that makes preview × amazon-mq
// incompatible by design (2-AZ preview vs CLUSTER_MULTI_AZ's three zones).
func assertPreviewAmazonMQRejected(t *testing.T, wantReason string) {
	t.Helper()
	spec := validSpec()
	if len(spec.Policy.AvailabilityZones) != 2 {
		t.Fatalf("preview fixture must stay 2-AZ to keep the amazon-mq guard testable, got %d", len(spec.Policy.AvailabilityZones))
	}
	m := &stackMocks{}
	err := pulumi.RunErr(func(ctx *pulumi.Context) error {
		_, err := queue.New(ctx, "shop", queue.Args{
			Mode: QueueModeAmazonMQ, Topology: queue.Topology(spec.Identity.Preset), Region: spec.Identity.Region,
			EngineVersion: spec.Catalog.Versions.RabbitMQ, InstanceType: "mq.m7g.large",
			AvailabilityZones: append([]string(nil), spec.Policy.AvailabilityZones...), SubnetIDs: []string{"subnet-a", "subnet-b"},
			SecurityGroupIDs: []string{"sg-queue"}, KMSKeyARN: spec.Dependencies.KMSKeyARN,
			Credentials: queue.Credentials{
				SecretARN: "arn:aws:secretsmanager:eu-west-3:123456789012:secret:shop-queue-token",
				Username:  spec.Dependencies.MasterUsername,
			},
			// Non-nil provider reaches the AZ check without registering a provider resource.
			Provider: &awsprovider.Provider{},
		})
		return err
	}, pulumi.WithMocks("magelift", "test", m))
	if err == nil || !strings.Contains(err.Error(), wantReason) {
		t.Fatalf("preview×amazon-mq rejection = %v, want reason containing %q", err, wantReason)
	}
	if componentCount(m, "aws:mq/broker:Broker") != 0 {
		t.Fatal("amazon-mq broker registered before the AZ guard rejected the plan")
	}
}
