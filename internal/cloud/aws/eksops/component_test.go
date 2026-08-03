package eksops

import (
	"context"
	"io"
	"net/netip"
	"strings"
	"sync"
	"testing"

	"github.com/magelift/magelift/internal/automation"
	"github.com/magelift/magelift/internal/cloud/kube"
	"github.com/magelift/magelift/internal/config"
	"github.com/magelift/magelift/internal/platform"
	sdk "github.com/magelift/magelift/sdk/v1"
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
	module, found := registry.Module("aws", "eks-autopilot")
	if !found || module.CertificationTier() != platform.TierExperimental {
		t.Fatal("expected experimental EKS module")
	}
	planned := Planned{Spec: validSpec()}
	if planned.StackName() != "shop-preview-aws-eks-autopilot" {
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
func (stubCandidate) RunMigrations(context.Context, kube.Candidate) error { return nil }
func (stubCandidate) Cleanup(context.Context, kube.Candidate) error       { return nil }

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
			VPCCIDR: netip.MustParsePrefix("10.42.0.0/16"), AvailabilityZones: []string{"eu-west-3a", "eu-west-3b"}, NatMode: NatModeFckNat,
		},
		Catalog: CatalogSelection{
			DatabaseEngine: DatabaseEngineRDSMySQL, InstanceClass: "db.t4g.micro", InstanceCount: 1,
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
	case "aws:elasticache/replicationGroup:ReplicationGroup":
		state["primaryEndpointAddress"] = resource.NewStringProperty(args.Name + ".cache.amazonaws.com")
	case "aws:eks/cluster:Cluster":
		state["arn"] = resource.NewStringProperty("arn:aws:eks:eu-west-3:123456789012:cluster/" + args.Name)
		state["name"] = resource.NewStringProperty(args.Name)
		state["endpoint"] = resource.NewStringProperty("eks.eu-west-3.amazonaws.com")
		state["certificateAuthority"] = resource.NewObjectProperty(resource.PropertyMap{
			"data": resource.NewStringProperty("Y2E="),
		})
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
	return resource.PropertyMap{"secretString": resource.MakeSecret(resource.NewStringProperty("mock-secret"))}, nil
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
