package edge

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/magelift/magelift/sdk"
)

type fakeServiceAPI struct {
	services    map[string]*corev1.Service
	createReady bool
	creates     int
	updates     int
	deletes     int
}

func newFakeServiceAPI() *fakeServiceAPI {
	return &fakeServiceAPI{services: make(map[string]*corev1.Service)}
}

func (api *fakeServiceAPI) GetService(_ context.Context, namespace, name string, _ metav1.GetOptions) (*corev1.Service, error) {
	service, ok := api.services[serviceKey(namespace, name)]
	if !ok {
		return nil, apierrors.NewNotFound(corev1.Resource("services"), name)
	}
	return service.DeepCopy(), nil
}

func (api *fakeServiceAPI) CreateService(_ context.Context, service *corev1.Service, _ metav1.CreateOptions) (*corev1.Service, error) {
	key := serviceKey(service.Namespace, service.Name)
	if _, exists := api.services[key]; exists {
		return nil, errors.New("service already exists")
	}
	api.creates++
	created := service.DeepCopy()
	if api.createReady {
		created.Status.LoadBalancer.Ingress = []corev1.LoadBalancerIngress{{Hostname: "lb.example.ovh"}}
	}
	api.services[key] = created
	return created.DeepCopy(), nil
}

func (api *fakeServiceAPI) UpdateService(_ context.Context, service *corev1.Service, _ metav1.UpdateOptions) (*corev1.Service, error) {
	key := serviceKey(service.Namespace, service.Name)
	if _, exists := api.services[key]; !exists {
		return nil, apierrors.NewNotFound(corev1.Resource("services"), service.Name)
	}
	api.updates++
	api.services[key] = service.DeepCopy()
	return service.DeepCopy(), nil
}

func (api *fakeServiceAPI) DeleteService(_ context.Context, namespace, name string, _ metav1.DeleteOptions) error {
	key := serviceKey(namespace, name)
	if _, exists := api.services[key]; !exists {
		return errors.New("service not found")
	}
	api.deletes++
	delete(api.services, key)
	return nil
}

func (api *fakeServiceAPI) ListServices(_ context.Context, _ metav1.ListOptions) (*corev1.ServiceList, error) {
	list := &corev1.ServiceList{}
	for _, service := range api.services {
		list.Items = append(list.Items, *service.DeepCopy())
	}
	return list, nil
}

type fakeOVHHealth struct{ calls int }

func (probe *fakeOVHHealth) VerifyOrigin(_ context.Context, reference string) error {
	probe.calls++
	if reference == "" {
		return errors.New("origin reference is required")
	}
	return nil
}

func ovhEdgeTestRequest() sdk.EdgePlanRequest {
	return sdk.EdgePlanRequest{TargetProvider: "ovh", TargetRuntime: "mks", Intent: sdk.EdgeIntent{Mode: "native", NativeProvider: "ovh-public-cloud-load-balancer", OriginHealthRef: "health/origin", OwnershipMarker: "magelift/test/ovh", Domains: []string{"shop.example.com"}}, Configuration: map[string]any{"namespace": "magelift", "serviceName": "web", "servicePort": 80, "targetPort": 8080, "selector": map[string]any{"app": "web"}}}
}

func ovhEdgeTestPolicy() sdk.EdgeOperationPolicy {
	return sdk.EdgeOperationPolicy{Timeout: time.Second, PollInterval: time.Millisecond, MaxAttempts: 4}
}

func TestOVHPlanRejectsUnsupportedNativeFeaturesBeforeMutation(t *testing.T) {
	api := newFakeServiceAPI()
	native, err := NewNativeAPI(api, &fakeOVHHealth{})
	if err != nil {
		t.Fatal(err)
	}
	request := ovhEdgeTestRequest()
	request.Intent.TLS = true
	request.Intent.TLSMode = "managed"
	request.Intent.DNSMode = "external"
	if _, err := native.Plan(context.Background(), request); err == nil || !containsError(err, "L4-only") {
		t.Fatalf("TLS plan error = %v", err)
	}
	request = ovhEdgeTestRequest()
	request.Intent.WAFPolicyRef = "waf/managed"
	if _, err := native.Plan(context.Background(), request); err == nil || !containsError(err, "native WAF") {
		t.Fatalf("WAF plan error = %v", err)
	}
	if api.creates != 0 || api.updates != 0 || api.deletes != 0 {
		t.Fatalf("unsupported plan mutated services: %#v", api)
	}
}

func TestOVHPlanKeepsUnsupportedCachePurgeTLSWAFAndFailoverExplicit(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*sdk.EdgePlanRequest)
		want   string
	}{
		{name: "cache", mutate: func(request *sdk.EdgePlanRequest) { request.Intent.CachePolicyRef = "cache/managed" }, want: "cache"},
		{name: "purge", mutate: func(request *sdk.EdgePlanRequest) { request.Intent.PurgeOnDeploy = true }, want: "cache or purge"},
		{name: "TLS", mutate: func(request *sdk.EdgePlanRequest) {
			request.Intent.TLS = true
			request.Intent.TLSMode = "managed"
			request.Intent.DNSMode = "external"
		}, want: "L4-only"},
		{name: "WAF", mutate: func(request *sdk.EdgePlanRequest) { request.Intent.WAFPolicyRef = "waf/managed" }, want: "native WAF"},
		{name: "failover", mutate: func(request *sdk.EdgePlanRequest) { request.Intent.FailoverPolicyRef = "ovh/failover" }, want: "failover"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			api := newFakeServiceAPI()
			native, err := NewNativeAPI(api, &fakeOVHHealth{})
			if err != nil {
				t.Fatal(err)
			}
			request := ovhEdgeTestRequest()
			test.mutate(&request)
			if _, err := native.Plan(context.Background(), request); err == nil || !strings.Contains(strings.ToLower(err.Error()), strings.ToLower(test.want)) {
				t.Fatalf("unsupported %s error = %v, want %q", test.name, err, test.want)
			}
			if api.creates != 0 || api.updates != 0 || api.deletes != 0 {
				t.Fatalf("unsupported %s plan mutated services: %#v", test.name, api)
			}
		})
	}
}

func TestOVHPlanReturnsTypedFailoverCapabilityError(t *testing.T) {
	native, err := NewNativeAPI(newFakeServiceAPI(), &fakeOVHHealth{})
	if err != nil {
		t.Fatal(err)
	}
	request := ovhEdgeTestRequest()
	request.Intent.FailoverPolicyRef = "ovh/failover"
	_, err = native.Plan(context.Background(), request)
	var capabilityErr sdk.EdgeCapabilityError
	if !errors.As(err, &capabilityErr) || capabilityErr.Status != sdk.EdgeCapabilityUnsupported || capabilityErr.Action != sdk.EdgeFailover {
		t.Fatalf("typed failover error = %v", err)
	}
}

func TestOVHKubernetesEdgeApplyVerifyDestroyUsesOwningService(t *testing.T) {
	api := newFakeServiceAPI()
	api.createReady = true
	health := &fakeOVHHealth{}
	native, err := NewNativeAPI(api, health)
	if err != nil {
		t.Fatal(err)
	}
	adapter, err := NewNativeLifecycleAdapter(native, ovhEdgeTestPolicy())
	if err != nil {
		t.Fatal(err)
	}
	request := ovhEdgeTestRequest()
	plan, err := adapter.PlanEdge(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	result, err := adapter.ExecuteEdge(context.Background(), sdk.EdgeExecutionRequest{Plan: plan, Action: sdk.EdgeApply, IdempotencyKey: "apply-1", OwnershipMarker: plan.OwnershipMarker})
	if err != nil {
		t.Fatal(err)
	}
	if api.creates != 1 || health.calls != 1 || len(result.ResourceRefs) != 1 || !containsString(result.ProofRefs, "ovh.edge.octavia") {
		t.Fatalf("apply result = %#v, API = %#v, health calls = %d", result, api, health.calls)
	}
	verify, err := adapter.ExecuteEdge(context.Background(), sdk.EdgeExecutionRequest{Plan: plan, Action: sdk.EdgeVerify, IdempotencyKey: "verify-1", OwnershipMarker: plan.OwnershipMarker, ResourceReferences: result.ResourceRefs})
	if err != nil {
		t.Fatal(err)
	}
	if verify.Action != sdk.EdgeVerify || health.calls != 2 {
		t.Fatalf("verify result = %#v, health calls = %d", verify, health.calls)
	}
	destroy, err := adapter.ExecuteEdge(context.Background(), sdk.EdgeExecutionRequest{Plan: plan, Action: sdk.EdgeDestroy, IdempotencyKey: "destroy-1", OwnershipMarker: plan.OwnershipMarker, ResourceReferences: result.ResourceRefs})
	if err != nil {
		t.Fatal(err)
	}
	if destroy.Action != sdk.EdgeDestroy || api.deletes != 1 {
		t.Fatalf("destroy result = %#v, API = %#v", destroy, api)
	}
	resources, err := native.Inventory(context.Background(), plan.OwnershipMarker)
	if err != nil || len(resources) != 0 {
		t.Fatalf("post-destroy inventory = %#v, err = %v", resources, err)
	}
}

func TestOVHEdgeRefusesUnownedService(t *testing.T) {
	api := newFakeServiceAPI()
	api.services[serviceKey("magelift", "web")] = &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "magelift", Annotations: map[string]string{ovhOwnershipAnnotation: "magelift/other", ovhLoadBalancerKey: ovhLoadBalancerClass}}, Spec: corev1.ServiceSpec{Type: corev1.ServiceTypeLoadBalancer}}
	native, err := NewNativeAPI(api, &fakeOVHHealth{})
	if err != nil {
		t.Fatal(err)
	}
	adapter, err := NewNativeLifecycleAdapter(native, ovhEdgeTestPolicy())
	if err != nil {
		t.Fatal(err)
	}
	request := ovhEdgeTestRequest()
	plan, err := adapter.PlanEdge(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := adapter.ExecuteEdge(context.Background(), sdk.EdgeExecutionRequest{Plan: plan, Action: sdk.EdgeApply, IdempotencyKey: "apply-unowned", OwnershipMarker: plan.OwnershipMarker}); err == nil || !containsError(err, "unowned") {
		t.Fatalf("unowned apply error = %v", err)
	}
	if api.creates != 0 || api.updates != 0 {
		t.Fatalf("unowned service was mutated: %#v", api)
	}
}

func TestNewKubernetesServiceAPIRequiresClient(t *testing.T) {
	if _, err := NewKubernetesServiceAPI(nil); err == nil {
		t.Fatal("nil Kubernetes client was accepted")
	}
	if _, err := NewKubernetesServiceAPI(fake.NewSimpleClientset()); err != nil {
		t.Fatal(err)
	}
}

func serviceKey(namespace, name string) string { return namespace + "/" + name }

func containsError(err error, value string) bool {
	return err != nil && strings.Contains(err.Error(), value)
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
