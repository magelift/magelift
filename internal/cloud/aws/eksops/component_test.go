package eksops

import (
	"context"
	"encoding/base64"
	"io"
	"net/netip"
	"strings"
	"sync"
	"testing"

	"github.com/magelift/magelift/internal/automation"
	"github.com/magelift/magelift/internal/cloud/kube"
	"github.com/magelift/magelift/internal/config"
	"github.com/magelift/magelift/internal/platform"
	"github.com/magelift/magelift/sdk"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

func TestPlanFromConfigRejectsECSTarget(t *testing.T) {
	_, err := PlanFromConfig(config.Config{
		Target: config.Target{Provider: "aws", Runtime: "ecs-fargate"},
	}, "staging")
	if err == nil || !strings.Contains(err.Error(), "EKS stack requires") {
		t.Fatalf("expected EKS target rejection, got %v", err)
	}
}

func TestModuleRegistersExperimentalOutputs(t *testing.T) {
	registry := platform.NewModuleRegistry()
	if err := registry.RegisterModule(Module{}); err != nil {
		t.Fatal(err)
	}
	module, found := registry.Module("aws", "eks")
	if !found || module.CertificationTier() != platform.TierExperimental {
		t.Fatal("expected experimental EKS module")
	}
	planned := Planned{Spec: validSpec()}
	if planned.StackName() != "shop-preview-aws-eks" {
		t.Fatalf("stack name = %q", planned.StackName())
	}
}

func TestProgramBuildsMockGraph(t *testing.T) {
	spec := validSpec()
	if err := spec.Validate(); err != nil {
		t.Fatal(err)
	}
	mocks := &stackMocks{}
	err := pulumi.RunErr(Program(spec), pulumi.WithMocks("magelift", "shop-preview", mocks))
	if err != nil {
		t.Fatal(err)
	}
	if componentCount(mocks, "magelift:aws:Network") != 1 || componentCount(mocks, "magelift:aws:EKSRuntime") != 1 {
		t.Fatalf("expected network + EKS runtime components, got %#v", resourceTokens(mocks))
	}
	if componentCount(mocks, "aws:eks/cluster:Cluster") != 1 {
		t.Fatal("expected EKS Auto Mode cluster resource")
	}
	if componentCount(mocks, "aws:eks/addon:Addon") != 1 {
		t.Fatalf("expected the EKS CloudWatch observability add-on, got %#v", resourceTokens(mocks))
	}
	if got := resourceInputStringNamed(mocks, "aws:eks/addon:Addon", "shop-preview-runtime-cloudwatch-observability", "addonName"); got != "amazon-cloudwatch-observability" {
		t.Fatalf("CloudWatch EKS add-on = %q", got)
	}
	cluster := resourceNamed(mocks, "aws:eks/cluster:Cluster", "shop-preview-runtime-cluster")
	if _, found := cluster.Inputs[resource.PropertyKey("enabledClusterLogTypes")]; !found {
		t.Fatal("EKS cluster must enable control-plane CloudWatch log types")
	}
	if got := resourceInputStringNamed(mocks, "aws:iam/rolePolicyAttachment:RolePolicyAttachment", "shop-preview-runtime-node-worker", "policyArn"); got != "arn:aws:iam::aws:policy/AmazonEKSWorkerNodeMinimalPolicy" {
		t.Fatalf("Auto Mode node policy = %q", got)
	}
	if componentCount(mocks, "kubernetes:storage.k8s.io/v1:StorageClass") != 1 {
		t.Fatalf("expected EKS Auto Mode StorageClass, got %#v", resourceTokens(mocks))
	}
	if got := resourceInputString(mocks, "kubernetes:storage.k8s.io/v1:StorageClass", "provisioner"); got != "ebs.csi.eks.amazonaws.com" {
		t.Fatalf("EKS StorageClass provisioner = %q", got)
	}
	if componentCount(mocks, "aws:vpc/securityGroupIngressRule:SecurityGroupIngressRule") != 8 {
		t.Fatalf("expected six shared and two EKS service ingress rules, got %#v", resourceTokens(mocks))
	}
	if got := resourceInputStringNamed(mocks, "aws:vpc/securityGroupIngressRule:SecurityGroupIngressRule", "shop-preview-runtime-ingress-eks-data-mysql", "referencedSecurityGroupId"); got != "sg-shop-preview-runtime-cluster" {
		t.Fatalf("EKS database ingress source group = %q", got)
	}
	if componentCount(mocks, "kubernetes:core/v1:Secret") != 3 {
		t.Fatalf("expected database, database-admin, and encryption-key Kubernetes Secrets, got %#v", resourceTokens(mocks))
	}
	if componentCount(mocks, "kubernetes:batch/v1:Job") != 1 {
		t.Fatalf("runtime must create only the database grant Job before the shared deploy flow, got %#v", resourceTokens(mocks))
	}
	if got := serviceAnnotation(mocks, "shop-preview-runtime-web-svc", "service.beta.kubernetes.io/aws-load-balancer-scheme"); got != "internet-facing" {
		t.Fatalf("Auto Mode web Service scheme = %q, want internet-facing", got)
	}
}

func TestPlanMapsEKSDefaultsForStandardPreset(t *testing.T) {
	plan, err := PlanFromConfig(eksConfig(), "staging")
	if err != nil {
		t.Fatal(err)
	}
	if plan.Catalog.KubernetesVersion != DefaultKubernetesVersion {
		t.Fatalf("Kubernetes version = %q, want %q", plan.Catalog.KubernetesVersion, DefaultKubernetesVersion)
	}
	if plan.Catalog.SearchMode != SearchModeOpenSearch || plan.Catalog.SearchReplicas != 1 {
		t.Fatalf("standard search catalog = %#v", plan.Catalog)
	}
	if plan.Catalog.QueueMode != QueueModeRabbitMQ || plan.Catalog.QueueReplicas != 1 || plan.Catalog.QueueConsumerCount != 1 {
		t.Fatalf("standard queue catalog = %#v", plan.Catalog)
	}
}

func TestPlanMapsManagedDurabilityOverridesForEKS(t *testing.T) {
	cfg := eksConfig()
	cfg.Target.AWS.Catalog.DatabaseBackupWindow = "03:00-04:00"
	cfg.Target.AWS.Catalog.DatabaseMaintenanceWindow = "sun:05:00-sun:06:00"
	cfg.Target.AWS.Catalog.DatabaseDeletionProtection = eksBoolPtr(false)
	cfg.Target.AWS.Catalog.DatabaseDeleteAutomatedBackups = eksBoolPtr(true)
	cacheRetention := 7
	cfg.Target.AWS.Catalog.CacheSnapshotRetentionLimit = &cacheRetention
	cfg.Target.AWS.Catalog.CacheSnapshotWindow = "03:00-04:00"

	plan, err := PlanFromConfig(cfg, "staging")
	if err != nil {
		t.Fatal(err)
	}
	if plan.Catalog.DatabaseBackupWindow != "03:00-04:00" || plan.Catalog.DatabaseMaintenanceWindow != "sun:05:00-sun:06:00" || plan.Catalog.DatabaseDeletionProtection == nil || *plan.Catalog.DatabaseDeletionProtection || plan.Catalog.DatabaseDeleteAutomatedBackups == nil || !*plan.Catalog.DatabaseDeleteAutomatedBackups || plan.Catalog.CacheSnapshotRetentionLimit == nil || *plan.Catalog.CacheSnapshotRetentionLimit != 7 || plan.Catalog.CacheSnapshotWindow != "03:00-04:00" {
		t.Fatalf("EKS managed-service durability = %#v", plan.Catalog)
	}
}

func eksBoolPtr(value bool) *bool { return &value }

func TestSpecRejectsInvalidEKSWorkloadSelections(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*Spec)
		message string
	}{
		{name: "old Kubernetes", mutate: func(spec *Spec) { spec.Catalog.KubernetesVersion = "1.28" }, message: "1.29"},
		{name: "search replica with disabled search", mutate: func(spec *Spec) {
			spec.Catalog.SearchMode = SearchModeDisabled
			spec.Catalog.SearchReplicas = 1
		}, message: "disabled search"},
		{name: "queue replica with database queue", mutate: func(spec *Spec) {
			spec.Catalog.QueueMode = QueueModeDatabase
			spec.Catalog.QueueReplicas = 1
		}, message: "database queue"},
		{name: "in-place rabbitmq 1 to quorum", mutate: func(spec *Spec) {
			spec.Catalog.QueueMode = QueueModeRabbitMQ
			spec.Catalog.LiveQueueReplicas = 1
			spec.Catalog.QueueReplicas = 3
		}, message: "in-place RabbitMQ"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			spec := validSpec()
			test.mutate(&spec)
			err := spec.Validate()
			if err == nil || !strings.Contains(err.Error(), test.message) {
				t.Fatalf("expected validation error containing %q, got %v", test.message, err)
			}
		})
	}
}

func TestBindLiveQueueReplicasRefusesInPlaceQuorum(t *testing.T) {
	spec := validSpec()
	spec.Catalog.QueueMode = QueueModeRabbitMQ
	spec.Catalog.QueueReplicas = 3
	_, err := platform.BindLiveQueueReplicas(Planned{Spec: spec}, map[string]any{platform.OutputQueueReplicas: 1})
	if err == nil || !strings.Contains(err.Error(), "in-place RabbitMQ") {
		t.Fatalf("bind live replicas = %v", err)
	}
}

func TestProgramBuildsEKSOpenSearchAndRabbitMQGraph(t *testing.T) {
	spec := validSpec()
	spec.Identity.Preset = sdk.PresetStandard
	spec.Dependencies.SessionSecretARN = "arn:aws:secretsmanager:eu-west-3:123456789012:secret:session"
	spec.Catalog.DatabaseEngine = DatabaseEngineAuroraMySQL
	spec.Catalog.AuroraMySQLVersion = "8.0.mysql_aurora.3.10.0"
	spec.Catalog.InstanceClass = "db.r7g.large"
	spec.Catalog.InstanceCount = 2
	spec.Catalog.ValkeyReplicaCount = 1
	spec.Catalog.KubernetesVersion = DefaultKubernetesVersion
	spec.Catalog.SearchMode = SearchModeOpenSearch
	spec.Catalog.SearchReplicas = 1
	spec.Catalog.QueueMode = QueueModeRabbitMQ
	spec.Catalog.QueueReplicas = 1
	spec.Catalog.QueueConsumerCount = 1
	if err := spec.Validate(); err != nil {
		t.Fatal(err)
	}
	mocks := &stackMocks{}
	if err := pulumi.RunErr(Program(spec), pulumi.WithMocks("magelift", "shop-standard", mocks)); err != nil {
		t.Fatal(err)
	}
	if componentCount(mocks, "magelift:aws:EKSOpenSearch") != 1 || componentCount(mocks, "magelift:aws:EKSRabbitMQ") != 1 {
		t.Fatalf("expected EKS search + queue components, got %#v", resourceTokens(mocks))
	}
	if got := resourceInputString(mocks, "aws:eks/cluster:Cluster", "version"); got != DefaultKubernetesVersion {
		t.Fatalf("EKS cluster version = %q, want %q", got, DefaultKubernetesVersion)
	}
}

func TestProgramBuildsManagedNodeGroupGraph(t *testing.T) {
	spec := validSpec()
	spec.Catalog.ComputeMode = ComputeModeManagedNodes
	spec.Catalog.NodeInstanceType = "m7i.large"
	spec.Catalog.NodeMinSize = 2
	spec.Catalog.NodeDesiredSize = 2
	spec.Catalog.NodeMaxSize = 4
	if err := spec.Validate(); err != nil {
		t.Fatal(err)
	}
	mocks := &stackMocks{}
	if err := pulumi.RunErr(Program(spec), pulumi.WithMocks("magelift", "shop-managed", mocks)); err != nil {
		t.Fatal(err)
	}
	if componentCount(mocks, "aws:eks/nodeGroup:NodeGroup") != 1 || componentCount(mocks, "aws:eks/addon:Addon") != 2 {
		t.Fatalf("managed-node-groups graph = %#v", resourceTokens(mocks))
	}
	if got := resourceInputStringNamed(mocks, "aws:eks/addon:Addon", "shop-preview-runtime-cloudwatch-observability", "addonName"); got != "amazon-cloudwatch-observability" {
		t.Fatalf("managed-node-groups CloudWatch add-on = %q", got)
	}
	if got := resourceInputString(mocks, "aws:eks/nodeGroup:NodeGroup", "capacityType"); got != "ON_DEMAND" {
		t.Fatalf("managed node capacity type = %q", got)
	}
	if got := resourceInputString(mocks, "kubernetes:storage.k8s.io/v1:StorageClass", "provisioner"); got != "ebs.csi.aws.com" {
		t.Fatalf("managed node StorageClass provisioner = %q", got)
	}
	if componentCount(mocks, "aws:iam/openIdConnectProvider:OpenIdConnectProvider") != 1 {
		t.Fatalf("managed node graph must create one stack-owned OIDC provider: %#v", resourceTokens(mocks))
	}
	if got := resourceInputStringNamed(mocks, "aws:eks/addon:Addon", "shop-preview-runtime-ebs-csi", "serviceAccountRoleArn"); got != "arn:aws:iam::123456789012:role/shop-preview-runtime-ebs-csi-role" {
		t.Fatalf("EBS CSI add-on service account role = %q", got)
	}
	if got := resourceInputStringNamed(mocks, "aws:iam/rolePolicyAttachment:RolePolicyAttachment", "shop-preview-runtime-ebs-csi-policy", "policyArn"); got != "arn:aws:iam::aws:policy/service-role/AmazonEBSCSIDriverPolicy" {
		t.Fatalf("EBS CSI IAM policy = %q", got)
	}
	trustPolicy := resourceInputStringNamed(mocks, "aws:iam/role:Role", "shop-preview-runtime-ebs-csi-role", "assumeRolePolicy")
	for _, fragment := range []string{
		"arn:aws:iam::123456789012:oidc-provider/oidc.eks.eu-west-3.amazonaws.com/id/mock",
		"oidc.eks.eu-west-3.amazonaws.com/id/mock:aud",
		"system:serviceaccount:kube-system:ebs-csi-controller-sa",
	} {
		if !strings.Contains(trustPolicy, fragment) {
			t.Fatalf("EBS CSI trust policy %q does not contain %q", trustPolicy, fragment)
		}
	}
	cluster := resourceNamed(mocks, "aws:eks/cluster:Cluster", "shop-preview-runtime-cluster")
	if _, found := cluster.Inputs[resource.PropertyKey("computeConfig")]; found {
		t.Fatal("managed node-group cluster must not enable EKS Auto Mode computeConfig")
	}
	if !cluster.Inputs[resource.PropertyKey("bootstrapSelfManagedAddons")].BoolValue() {
		t.Fatal("managed node-group cluster must bootstrap standard EKS add-ons")
	}
}

func TestProgramBuildsSelfManagedNodeGraph(t *testing.T) {
	spec := validSpec()
	spec.Catalog.ComputeMode = ComputeModeSelfManaged
	spec.Catalog.NodeAMI = "ami-0123456789abcdef0"
	spec.Catalog.NodeInstanceType = "m7i.large"
	spec.Catalog.NodeMinSize = 1
	spec.Catalog.NodeDesiredSize = 2
	spec.Catalog.NodeMaxSize = 3
	if err := spec.Validate(); err != nil {
		t.Fatal(err)
	}
	mocks := &stackMocks{}
	if err := pulumi.RunErr(Program(spec), pulumi.WithMocks("magelift", "shop-self-managed", mocks)); err != nil {
		t.Fatal(err)
	}
	for _, token := range []string{
		"aws:ec2/launchTemplate:LaunchTemplate",
		"aws:autoscaling/group:Group",
		"aws:eks/accessEntry:AccessEntry",
	} {
		if componentCount(mocks, token) != 1 {
			t.Fatalf("self-managed graph is missing %s: %#v", token, resourceTokens(mocks))
		}
	}
	if componentCount(mocks, "aws:eks/addon:Addon") != 2 {
		t.Fatalf("self-managed graph must include EBS and CloudWatch add-ons: %#v", resourceTokens(mocks))
	}
	if got := resourceInputStringNamed(mocks, "aws:ec2/launchTemplate:LaunchTemplate", "shop-preview-runtime-self-managed-node-template", "imageId"); got != spec.Catalog.NodeAMI {
		t.Fatalf("self-managed node AMI = %q", got)
	}
	userData, err := base64.StdEncoding.DecodeString(resourceInputStringNamed(mocks, "aws:ec2/launchTemplate:LaunchTemplate", "shop-preview-runtime-self-managed-node-template", "userData"))
	if err != nil {
		t.Fatalf("self-managed node user data is not base64: %v", err)
	}
	userDataText := string(userData)
	for _, fragment := range []string{
		"MIME-Version: 1.0",
		"Content-Type: multipart/mixed; boundary=\"//\"",
		"Content-Type: application/node.eks.aws",
		"command -v nodeadm",
		"apiVersion: node.eks.aws/v1alpha1",
		"apiServerEndpoint: https://eks.eu-west-3.amazonaws.com",
		"cidr: 172.20.0.0/16",
		"Content-Type: text/x-shellscript",
		"/etc/eks/bootstrap.sh",
	} {
		if !strings.Contains(userDataText, fragment) {
			t.Fatalf("self-managed node user data does not contain %q: %s", fragment, userDataText)
		}
	}
	if got := resourceInputString(mocks, "kubernetes:storage.k8s.io/v1:StorageClass", "provisioner"); got != "ebs.csi.aws.com" {
		t.Fatalf("self-managed StorageClass provisioner = %q", got)
	}
	access := resourceNamed(mocks, "aws:eks/accessEntry:AccessEntry", "shop-preview-runtime-self-managed-node-access")
	if got := resourceInputStringFrom(access.Inputs, "type"); got != "EC2_LINUX" {
		t.Fatalf("self-managed node access entry type = %q", got)
	}
}

func TestProgramBuildsFargateProfileGraph(t *testing.T) {
	spec := validSpec()
	spec.Catalog.ComputeMode = ComputeModeFargate
	if err := spec.Validate(); err != nil {
		t.Fatal(err)
	}
	mocks := &stackMocks{}
	if err := pulumi.RunErr(Program(spec), pulumi.WithMocks("magelift", "shop-fargate", mocks)); err != nil {
		t.Fatal(err)
	}
	if componentCount(mocks, "aws:eks/fargateProfile:FargateProfile") != 1 || componentCount(mocks, "aws:eks/addon:Addon") != 0 {
		t.Fatalf("Fargate graph = %#v", resourceTokens(mocks))
	}
	if componentCount(mocks, "kubernetes:storage.k8s.io/v1:StorageClass") != 0 {
		t.Fatal("EKS Fargate graph must not create an EBS StorageClass for rebuild-only workloads")
	}
	if componentCount(mocks, "kubernetes:apps/v1:DeploymentPatch") != 1 {
		t.Fatal("EKS Fargate graph must move CoreDNS onto Fargate")
	}
	if componentCount(mocks, "aws:iam/role:Role") == 0 || componentCount(mocks, "aws:iam/role:Role") > 4 {
		t.Fatalf("unexpected Fargate IAM roles = %d", componentCount(mocks, "aws:iam/role:Role"))
	}
}

func TestSpecRejectsFargatePersistentWorkloads(t *testing.T) {
	spec := validSpec()
	spec.Catalog.ComputeMode = ComputeModeFargate
	spec.Catalog.SearchMode = SearchModeOpenSearch
	spec.Catalog.SearchReplicas = 1
	if err := spec.Validate(); err == nil || !strings.Contains(err.Error(), "Fargate") {
		t.Fatalf("Fargate OpenSearch workload was accepted: %v", err)
	}
}

func TestOpsDeployStepsTypeIdentityAndObserveShared(t *testing.T) {
	ops := Ops{
		NewCandidate: func(context.Context, kube.Backend) (kube.CandidateRunner, error) {
			return stubCandidate{}, nil
		},
		NewRuntime: func(context.Context, kube.Backend) (kube.RuntimeChecker, error) {
			return stubRuntime{}, nil
		},
	}
	backend := &stubBackend{outputs: map[string]any{}}
	steps, err := ops.NewDeploySteps(t.Context(), backend, Planned{Spec: validSpec()}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := steps.(*kube.Steps); !ok {
		t.Fatalf("want *kube.Steps, got %T", steps)
	}
	if _, err := (Ops{}).NewDeploySteps(t.Context(), struct{}{}, Planned{Spec: validSpec()}, io.Discard); err == nil || !strings.Contains(err.Error(), "backend with outputs") {
		t.Fatalf("expected wrong-backend error, got %v", err)
	}
	observe := Module{}.RuntimeObserve()
	if _, ok := observe.(*kube.Observe); !ok {
		t.Fatalf("want *kube.Observe, got %T", observe)
	}
}

type stubCandidate struct{}

func (stubCandidate) RegisterCandidate(context.Context, kube.CandidateRequest) (kube.Candidate, error) {
	return kube.Candidate{}, nil
}
func (stubCandidate) RunMigrations(context.Context, kube.Candidate) error             { return nil }
func (stubCandidate) RunProbe(context.Context, kube.CandidateRequest, []string) error { return nil }
func (stubCandidate) Cleanup(context.Context, kube.Candidate) error                   { return nil }

type stubRuntime struct{}

func (stubRuntime) Check(context.Context, string, string) (kube.ServiceHealth, error) {
	return kube.ServiceHealth{DesiredReplicas: 1, ReadyReplicas: 1, Available: true}, nil
}

type stubBackend struct{ outputs map[string]any }

func (b *stubBackend) Outputs(context.Context) (map[string]any, error) { return b.outputs, nil }
func (*stubBackend) Preview(context.Context, automation.Request, io.Writer) (map[string]int, error) {
	return map[string]int{}, nil
}
func (*stubBackend) Update(context.Context, automation.Request, io.Writer) (map[string]int, error) {
	return map[string]int{}, nil
}
func (*stubBackend) Destroy(context.Context, automation.Request, io.Writer) (map[string]int, error) {
	return map[string]int{}, nil
}

func validSpec() Spec {
	return Spec{
		Identity: Identity{
			Project: "shop", Environment: "preview", AccountID: "123456789012", Region: "eu-west-3",
			EnvironmentClass: "preview", Preset: sdk.PresetPreview,
			Tags: map[string]string{"magelift:managed-by": "magelift"},
		},
		Application: Application{Edition: "open-source", Version: "2.4.8", Mode: "integrated", WebRuntime: "nginx-fpm"},
		Artifact:    Artifact{ImageDigest: "ghcr.io/magelift/magento@sha256:abcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcd"},
		Policy: NetworkPolicy{
			VPCCIDR: netip.MustParsePrefix("10.42.0.0/16"), AvailabilityZones: []string{"eu-west-3a", "eu-west-3b"}, NatMode: NatModeFckNat, NatTopology: "single-az",
		},
		Catalog: CatalogSelection{
			DatabaseEngine: DatabaseEngineRDSMySQL, KubernetesVersion: DefaultKubernetesVersion,
			SearchMode: SearchModeDisabled, QueueMode: QueueModeDatabase,
			InstanceClass: "db.t4g.micro", InstanceCount: 1,
			ValkeyNodeType: "cache.t4g.micro", ValkeyReplicaCount: 0,
			CPURequest: "500m", MemoryRequest: "1Gi", DesiredWebReplicas: 1,
			BackupDays: 1, MySQLVersion: "8.4.10", ValkeyVersion: "8.1",
		},
		Dependencies: Dependencies{
			KMSKeyARN:        "arn:aws:kms:eu-west-3:123456789012:key/00000000-0000-0000-0000-000000000000",
			CacheSecretARN:   "arn:aws:secretsmanager:eu-west-3:123456789012:secret:cache",
			EncryptionKeyARN: "arn:aws:secretsmanager:eu-west-3:123456789012:secret:crypt",
			DatabaseName:     "magento", MasterUsername: "magento",
		},
	}
}

func eksConfig() config.Config {
	return config.Config{
		SchemaVersion: 1,
		Project:       config.Project{Name: "shop"},
		Application: config.Application{
			Edition: "open-source", Version: "2.4.9", Mode: "integrated", WebRuntime: "nginx-fpm",
		},
		Target: config.Target{
			Provider: "aws", Runtime: RuntimeID,
			AWS: &config.AWSTarget{
				ImageDigest:            "ghcr.io/magelift/magento@sha256:abcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcd",
				CacheSecretARN:         "arn:aws:secretsmanager:eu-west-3:123456789012:secret:cache",
				SessionSecretARN:       "arn:aws:secretsmanager:eu-west-3:123456789012:secret:session",
				EncryptionKeySecretARN: "arn:aws:secretsmanager:eu-west-3:123456789012:secret:crypt",
				KMSKeyARN:              "arn:aws:kms:eu-west-3:123456789012:key/00000000-0000-0000-0000-000000000000",
				VPCCIDR:                "10.42.0.0/16",
				AvailabilityZones:      []string{"eu-west-3a", "eu-west-3b"},
				Catalog:                config.AWSCatalog{Valkey: config.AWSCatalogValkey{NodeType: "cache.t4g.micro"}},
			},
		},
		Defaults: config.Defaults{Region: "eu-west-3", Preset: "standard"},
		Account:  "123456789012",
		Class:    "staging",
	}
}

type stackMocks struct {
	mu        sync.Mutex
	resources []pulumi.MockResourceArgs
}

func (m *stackMocks) NewResource(args pulumi.MockResourceArgs) (string, resource.PropertyMap, error) {
	m.mu.Lock()
	m.resources = append(m.resources, args)
	m.mu.Unlock()
	state := args.Inputs.Copy()
	switch args.TypeToken {
	case "aws:ec2/vpc:Vpc":
		state["id"] = resource.NewStringProperty("vpc-shop")
	case "aws:ec2/subnet:Subnet":
		state["id"] = resource.NewStringProperty("subnet-" + args.Name)
	case "aws:ec2/securityGroup:SecurityGroup":
		state["id"] = resource.NewStringProperty("sg-" + args.Name)
	case "aws:iam/role:Role":
		state["arn"] = resource.NewStringProperty("arn:aws:iam::123456789012:role/" + args.Name)
		state["name"] = resource.NewStringProperty(args.Name)
	case "aws:rds/instance:Instance":
		state["arn"] = resource.NewStringProperty("arn:aws:rds:eu-west-3:123456789012:db:shop")
		state["address"] = resource.NewStringProperty("shop.writer")
		state["endpoint"] = resource.NewStringProperty("shop.writer:3306")
		state["masterUserSecrets"] = resource.NewArrayProperty([]resource.PropertyValue{resource.NewObjectProperty(resource.PropertyMap{
			"secretArn": resource.NewStringProperty("arn:aws:secretsmanager:eu-west-3:123456789012:secret:managed"),
		})})
	case "aws:rds/cluster:Cluster":
		state["arn"] = resource.NewStringProperty("arn:aws:rds:eu-west-3:123456789012:cluster:shop")
		state["endpoint"] = resource.NewStringProperty("shop.writer")
		state["readerEndpoint"] = resource.NewStringProperty("shop.reader")
		state["masterUserSecrets"] = resource.NewArrayProperty([]resource.PropertyValue{resource.NewObjectProperty(resource.PropertyMap{
			"secretArn": resource.NewStringProperty("arn:aws:secretsmanager:eu-west-3:123456789012:secret:managed"),
		})})
	case "aws:elasticache/replicationGroup:ReplicationGroup":
		state["primaryEndpointAddress"] = resource.NewStringProperty(args.Name + ".cache.amazonaws.com")
	case "aws:eks/cluster:Cluster":
		state["arn"] = resource.NewStringProperty("arn:aws:eks:eu-west-3:123456789012:cluster/" + args.Name)
		state["name"] = resource.NewStringProperty(args.Name)
		state["endpoint"] = resource.NewStringProperty("eks.eu-west-3.amazonaws.com")
		state["certificateAuthority"] = resource.NewObjectProperty(resource.PropertyMap{
			"data": resource.NewStringProperty("Y2E="),
		})
		state["kubernetesNetworkConfig"] = resource.NewObjectProperty(resource.PropertyMap{
			"serviceIpv4Cidr": resource.NewStringProperty("172.20.0.0/16"),
		})
		state["vpcConfig"] = resource.NewObjectProperty(resource.PropertyMap{
			"clusterSecurityGroupId": resource.NewStringProperty("sg-" + args.Name),
		})
		state["identities"] = resource.NewArrayProperty([]resource.PropertyValue{resource.NewObjectProperty(resource.PropertyMap{
			"oidcs": resource.NewArrayProperty([]resource.PropertyValue{resource.NewObjectProperty(resource.PropertyMap{
				"issuer": resource.NewStringProperty("https://oidc.eks.eu-west-3.amazonaws.com/id/mock"),
			})}),
		})})
	case "aws:iam/openIdConnectProvider:OpenIdConnectProvider":
		// IAM normalizes the provider URL output without the https:// scheme;
		// the EKS cluster issuer remains the canonical trust-policy input.
		state["url"] = resource.NewStringProperty("oidc.eks.eu-west-3.amazonaws.com/id/mock")
		state["arn"] = resource.NewStringProperty("arn:aws:iam::123456789012:oidc-provider/oidc.eks.eu-west-3.amazonaws.com/id/mock")
	case "kubernetes:core/v1:Service":
		state["status"] = resource.NewObjectProperty(resource.PropertyMap{
			"loadBalancer": resource.NewObjectProperty(resource.PropertyMap{
				"ingress": resource.NewArrayProperty([]resource.PropertyValue{
					resource.NewObjectProperty(resource.PropertyMap{"hostname": resource.NewStringProperty("k8s-shop.elb.amazonaws.com")}),
				}),
			}),
		})
		state["metadata"] = resource.NewObjectProperty(resource.PropertyMap{"name": resource.NewStringProperty(args.Name)})
	}
	return args.Name + "-id", state, nil
}

func (m *stackMocks) Call(args pulumi.MockCallArgs) (resource.PropertyMap, error) {
	secretID := ""
	if value, ok := args.Args["secretId"]; ok {
		secretID = value.StringValue()
	}
	value := "mock-secret"
	if strings.Contains(secretID, "managed") {
		value = `{"username":"magento","password":"mock-password"}`
	}
	return resource.PropertyMap{"secretString": resource.MakeSecret(resource.NewStringProperty(value))}, nil
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

func resourceTokens(m *stackMocks) []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	tokens := make([]string, 0, len(m.resources))
	for _, resource := range m.resources {
		tokens = append(tokens, resource.TypeToken)
	}
	return tokens
}

func resourceInputString(m *stackMocks, token, key string) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, item := range m.resources {
		if item.TypeToken == token {
			return item.Inputs[resource.PropertyKey(key)].StringValue()
		}
	}
	return ""
}

func resourceInputStringNamed(m *stackMocks, token, name, key string) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, item := range m.resources {
		if item.TypeToken == token && item.Name == name {
			return item.Inputs[resource.PropertyKey(key)].StringValue()
		}
	}
	return ""
}

func resourceNamed(m *stackMocks, token, name string) pulumi.MockResourceArgs {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, item := range m.resources {
		if item.TypeToken == token && item.Name == name {
			return item
		}
	}
	return pulumi.MockResourceArgs{}
}

func resourceInputStringFrom(inputs resource.PropertyMap, key string) string {
	return inputs[resource.PropertyKey(key)].StringValue()
}

func serviceAnnotation(m *stackMocks, name, key string) string {
	svc := resourceNamed(m, "kubernetes:core/v1:Service", name)
	metadata := svc.Inputs[resource.PropertyKey("metadata")]
	if !metadata.IsObject() {
		return ""
	}
	annotations := metadata.ObjectValue()[resource.PropertyKey("annotations")]
	if !annotations.IsObject() {
		return ""
	}
	value := annotations.ObjectValue()[resource.PropertyKey(key)]
	if !value.IsString() {
		return ""
	}
	return value.StringValue()
}

func TestExpiredPreviewCarriesAllowExpiredIntoProgram(t *testing.T) {
	cfg := eksConfig()
	cfg.Class = "preview"
	cfg.Preset = "preview"
	cfg.Defaults.Preset = "preview"
	cfg.ExpiresAt = "2020-01-01T00:00:00Z"
	if _, err := PlanFromConfig(cfg, "preview"); err == nil || !strings.Contains(err.Error(), "expired") {
		t.Fatalf("expired preview was accepted for normal planning: %v", err)
	}
	spec, err := PlanFromConfigWithOptions(cfg, "preview", PlanOptions{AllowExpiredPreview: true})
	if err != nil {
		t.Fatalf("expired preview was not accepted for destroy planning: %v", err)
	}
	if !spec.AllowExpiredPreview {
		t.Fatal("destroy planning did not record AllowExpiredPreview on the spec; the Pulumi program would reject the destroy")
	}
	if err := spec.Validate(); err == nil || !strings.Contains(err.Error(), "expired") {
		t.Fatalf("strict validation stopped rejecting the expired preview: %v", err)
	}
	if err := spec.ValidateAllowExpiredPreview(); err != nil {
		t.Fatalf("teardown validation rejected the expired preview: %v", err)
	}
	cfg.Target.AWS.ImageDigest = "not-a-digest"
	if _, err := PlanFromConfigWithOptions(cfg, "preview", PlanOptions{AllowExpiredPreview: true}); err == nil {
		t.Fatal("allow-expired planning forgave a non-expiry defect")
	}
}
