package edge

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"sort"
	"strings"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"

	"github.com/magelift/magelift/sdk"
)

const (
	ovhEdgeAdapterID       = "ovh.edge.native"
	ovhServiceReference    = "ovh.kubernetes.service:"
	ovhServiceOperation    = "ovh.kubernetes.service"
	ovhOwnershipAnnotation = "magelift.io/ownership"
	ovhManagedAnnotation   = "magelift.io/managed-by"
	ovhManagedValue        = "magelift"
	ovhLoadBalancerClass   = "octavia"
	ovhLoadBalancerKey     = "loadbalancer.ovhcloud.com/class"
)

// ServiceAPI is the provider-owned subset of the official Kubernetes API used
// by the OVHcloud MKS load-balancer translator. Kubernetes request and
// response models stay in this package; only opaque service identities and
// normalized proof facts cross the SDK boundary.
type ServiceAPI interface {
	GetService(context.Context, string, string, metav1.GetOptions) (*corev1.Service, error)
	CreateService(context.Context, *corev1.Service, metav1.CreateOptions) (*corev1.Service, error)
	UpdateService(context.Context, *corev1.Service, metav1.UpdateOptions) (*corev1.Service, error)
	DeleteService(context.Context, string, string, metav1.DeleteOptions) error
	ListServices(context.Context, metav1.ListOptions) (*corev1.ServiceList, error)
}

type kubernetesServiceAPI struct{ client kubernetes.Interface }

func (api kubernetesServiceAPI) GetService(ctx context.Context, namespace, name string, options metav1.GetOptions) (*corev1.Service, error) {
	return api.client.CoreV1().Services(namespace).Get(ctx, name, options)
}

func (api kubernetesServiceAPI) CreateService(ctx context.Context, service *corev1.Service, options metav1.CreateOptions) (*corev1.Service, error) {
	return api.client.CoreV1().Services(service.Namespace).Create(ctx, service, options)
}

func (api kubernetesServiceAPI) UpdateService(ctx context.Context, service *corev1.Service, options metav1.UpdateOptions) (*corev1.Service, error) {
	return api.client.CoreV1().Services(service.Namespace).Update(ctx, service, options)
}

func (api kubernetesServiceAPI) DeleteService(ctx context.Context, namespace, name string, options metav1.DeleteOptions) error {
	return api.client.CoreV1().Services(namespace).Delete(ctx, name, options)
}

func (api kubernetesServiceAPI) ListServices(ctx context.Context, options metav1.ListOptions) (*corev1.ServiceList, error) {
	return api.client.CoreV1().Services(metav1.NamespaceAll).List(ctx, options)
}

// OriginHealthProbe is injected by the workload runtime. A service reference
// and an ownership marker are configuration; neither proves that an MKS
// load-balancer route is safe to expose.
type OriginHealthProbe interface {
	VerifyOrigin(context.Context, string) error
}

// NewKubernetesServiceAPI wraps the official Kubernetes client-go interface
// without exporting Kubernetes models through the public MageLift SDK.
func NewKubernetesServiceAPI(client kubernetes.Interface) (ServiceAPI, error) {
	if client == nil {
		return nil, errors.New("OVH Kubernetes client is required")
	}
	return kubernetesServiceAPI{client: client}, nil
}

// NativeAPI translates the portable edge lifecycle to the current OVHcloud
// MKS public-cloud Load Balancer path. OVH documents this path as an Octavia
// LoadBalancer service for MKS. It is intentionally L4-only here: native
// TLS termination, CDN/cache, and WAF are not claimed by this adapter.
type NativeAPI struct {
	api    ServiceAPI
	health OriginHealthProbe
}

func NewNativeAPI(api ServiceAPI, health OriginHealthProbe) (*NativeAPI, error) {
	if api == nil {
		return nil, errors.New("OVH service API is required")
	}
	if health == nil {
		return nil, errors.New("OVH edge origin health probe is required")
	}
	return &NativeAPI{api: api, health: health}, nil
}

type ovhEdgePlan struct {
	namespace       string
	serviceName     string
	servicePort     int32
	targetPort      intstr.IntOrString
	selector        map[string]string
	originHealthRef string
	ownershipMarker string
}

func (api *NativeAPI) Plan(ctx context.Context, request sdk.EdgePlanRequest) (sdk.EdgePlan, error) {
	if api == nil || api.api == nil {
		return sdk.EdgePlan{}, errors.New("OVH service API is required")
	}
	if ctx == nil {
		return sdk.EdgePlan{}, errors.New("OVH edge planning context is required")
	}
	if err := ctx.Err(); err != nil {
		return sdk.EdgePlan{}, err
	}
	if request.TargetProvider != "ovh" {
		return sdk.EdgePlan{}, fmt.Errorf("OVH edge target provider must be ovh, got %q", request.TargetProvider)
	}
	if err := sdk.ValidateEdgePlanRequest(request); err != nil {
		return sdk.EdgePlan{}, err
	}
	if request.Intent.Mode == "none" {
		return sdk.EdgePlan{AdapterID: ovhEdgeAdapterID, TargetProvider: request.TargetProvider, TargetRuntime: request.TargetRuntime}, nil
	}
	if request.Intent.Mode != "native" && request.Intent.Mode != "both" {
		return sdk.EdgePlan{}, fmt.Errorf("OVH edge adapter cannot handle edge mode %q", request.Intent.Mode)
	}
	switch request.Intent.NativeProvider {
	case "ovh-public-cloud-load-balancer":
	case "ovh-cdn":
		return sdk.EdgePlan{}, errors.New("OVH native CDN is unavailable for the MKS public-cloud edge adapter; use Fastly or another external edge adapter")
	default:
		return sdk.EdgePlan{}, fmt.Errorf("unsupported OVH native edge provider %q", request.Intent.NativeProvider)
	}
	if request.Intent.TLS {
		return sdk.EdgePlan{}, errors.New("OVH MKS public-cloud Load Balancer adapter is L4-only and cannot terminate TLS; configure TLS on an external edge or use an explicit passthrough path")
	}
	if request.Intent.CachePolicyRef != "" || request.Intent.PurgeOnDeploy || request.Intent.PurgePolicyRef != "" {
		return sdk.EdgePlan{}, errors.New("OVH MKS public-cloud Load Balancer adapter does not provide cache or purge lifecycle")
	}
	if request.Intent.WAFPolicyRef != "" {
		return sdk.EdgePlan{}, errors.New("OVH MKS public-cloud Load Balancer adapter does not provide native WAF lifecycle")
	}
	if request.Intent.FailoverPolicyRef != "" {
		return sdk.EdgePlan{}, sdk.EdgeCapabilityError{AdapterID: ovhEdgeAdapterID, Action: sdk.EdgeFailover, Status: sdk.EdgeCapabilityUnsupported, Reason: "OVH MKS public-cloud Load Balancer adapter does not provide native edge failover or rollback lifecycle"}
	}
	plan, err := parseServicePlan(request)
	if err != nil {
		return sdk.EdgePlan{}, fmt.Errorf("parse OVH MKS Load Balancer service: %w", err)
	}
	return sdk.EdgePlan{
		AdapterID:       ovhEdgeAdapterID,
		TargetProvider:  request.TargetProvider,
		TargetRuntime:   request.TargetRuntime,
		OwnershipMarker: plan.ownershipMarker,
		Outputs: []sdk.AdapterOutput{
			{Key: "serviceName"},
			{Key: "serviceNamespace"},
			{Key: "loadBalancerHostname"},
			{Key: "loadBalancerIP"},
		},
		Opaque: plan,
	}, nil
}

func (api *NativeAPI) Start(ctx context.Context, operation sdk.EdgeOperationRequest) (sdk.EdgeOperationObservation, error) {
	if api == nil || api.api == nil {
		return sdk.EdgeOperationObservation{}, errors.New("OVH service API is required")
	}
	if ctx == nil {
		return sdk.EdgeOperationObservation{}, errors.New("OVH edge execution context is required")
	}
	if operation.Provider != "ovh" {
		return sdk.EdgeOperationObservation{}, fmt.Errorf("OVH edge operation provider must be ovh, got %q", operation.Provider)
	}
	plan, ok := operation.Request.Plan.Opaque.(ovhEdgePlan)
	if !ok {
		return sdk.EdgeOperationObservation{}, errors.New("OVH edge plan is missing provider configuration")
	}
	if plan.ownershipMarker != operation.Request.OwnershipMarker {
		return sdk.EdgeOperationObservation{}, errors.New("OVH edge ownership marker does not match the plan")
	}
	if operation.Action == sdk.EdgeApply || operation.Action == sdk.EdgeVerify {
		if err := api.health.VerifyOrigin(ctx, plan.originHealthRef); err != nil {
			return sdk.EdgeOperationObservation{}, fmt.Errorf("verify OVH edge origin health before mutation: %w", err)
		}
	}
	switch operation.Action {
	case sdk.EdgeApply:
		if _, err := api.ensureService(ctx, plan); err != nil {
			return sdk.EdgeOperationObservation{}, err
		}
		return pendingObservation(sdk.EdgeApply, serviceOperationID(sdk.EdgeApply, plan.namespace, plan.serviceName, plan.ownershipMarker), plan.ownershipMarker), nil
	case sdk.EdgeVerify:
		if _, err := api.getOwnedService(ctx, plan.namespace, plan.serviceName, plan.ownershipMarker); err != nil {
			return sdk.EdgeOperationObservation{}, err
		}
		return pendingObservation(sdk.EdgeVerify, serviceOperationID(sdk.EdgeVerify, plan.namespace, plan.serviceName, plan.ownershipMarker), plan.ownershipMarker), nil
	case sdk.EdgeDestroy:
		namespace, name, err := serviceReferenceFromReferences(operation.Request.ResourceReferences)
		if err != nil {
			namespace, name = plan.namespace, plan.serviceName
		}
		service, getErr := api.api.GetService(ctx, namespace, name, metav1.GetOptions{})
		if getErr != nil {
			if apierrors.IsNotFound(getErr) {
				return succeededObservation(sdk.EdgeDestroy, serviceOperationID(sdk.EdgeDestroy, namespace, name, plan.ownershipMarker), plan.ownershipMarker, []string{serviceReference(namespace, name)}, []string{"ovh.edge.owning-service-inventory-empty"}), nil
			}
			return sdk.EdgeOperationObservation{}, fmt.Errorf("read OVH service for destroy: %w", getErr)
		}
		if !ownedService(service, plan.ownershipMarker) {
			return sdk.EdgeOperationObservation{}, errors.New("refusing to destroy an unowned OVH MKS Load Balancer service")
		}
		if err := api.api.DeleteService(ctx, namespace, name, metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
			return sdk.EdgeOperationObservation{}, fmt.Errorf("delete OVH MKS Load Balancer service: %w", err)
		}
		return pendingObservation(sdk.EdgeDestroy, serviceOperationID(sdk.EdgeDestroy, namespace, name, plan.ownershipMarker), plan.ownershipMarker), nil
	default:
		return sdk.EdgeOperationObservation{}, fmt.Errorf("unsupported OVH edge action %q", operation.Action)
	}
}

func (api *NativeAPI) Poll(ctx context.Context, operationID string) (sdk.EdgeOperationObservation, error) {
	if api == nil || api.api == nil {
		return sdk.EdgeOperationObservation{}, errors.New("OVH service API is required")
	}
	if ctx == nil {
		return sdk.EdgeOperationObservation{}, errors.New("OVH edge polling context is required")
	}
	if err := ctx.Err(); err != nil {
		return sdk.EdgeOperationObservation{}, err
	}
	action, namespace, name, expectedMarker, err := parseServiceOperationID(operationID)
	if err != nil {
		return sdk.EdgeOperationObservation{}, err
	}
	service, getErr := api.api.GetService(ctx, namespace, name, metav1.GetOptions{})
	if getErr != nil {
		if action == sdk.EdgeDestroy && apierrors.IsNotFound(getErr) {
			return succeededObservation(action, operationID, expectedMarker, []string{serviceReference(namespace, name)}, []string{"ovh.edge.owning-service-inventory-empty"}), nil
		}
		return sdk.EdgeOperationObservation{}, fmt.Errorf("poll OVH MKS Load Balancer service: %w", getErr)
	}
	marker := serviceMarker(service)
	if marker == "" {
		return failedObservation(action, operationID, "OVH MKS Load Balancer service ownership marker is missing"), nil
	}
	if marker != expectedMarker {
		return failedObservation(action, operationID, "OVH MKS Load Balancer service ownership marker does not match the operation scope"), nil
	}
	if action == sdk.EdgeDestroy {
		return pendingObservation(action, operationID, marker), nil
	}
	if !ownedServiceClass(service) || service.Spec.Type != corev1.ServiceTypeLoadBalancer {
		return failedObservation(action, operationID, "OVH MKS service no longer has the Octavia LoadBalancer contract"), nil
	}
	if !serviceReady(service) {
		return pendingObservation(action, operationID, marker), nil
	}
	return succeededObservationWithOutputs(action, operationID, marker, []string{serviceReference(namespace, name)}, serviceOutputs(service), []string{"ovh.edge.ownership", "ovh.edge.octavia", "ovh.edge.origin-health"}), nil
}

func (api *NativeAPI) Inventory(ctx context.Context, marker string) ([]sdk.EdgeInventoryResource, error) {
	if api == nil || api.api == nil {
		return nil, errors.New("OVH service API is required")
	}
	if ctx == nil {
		return nil, errors.New("OVH edge inventory context is required")
	}
	marker = strings.TrimSpace(marker)
	if marker == "" || strings.ContainsAny(marker, "\r\n\x00") {
		return nil, errors.New("OVH edge ownership marker is required and must be single-line")
	}
	list, err := api.api.ListServices(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list OVH Kubernetes services: %w", err)
	}
	resources := make([]sdk.EdgeInventoryResource, 0)
	if list != nil {
		for _, service := range list.Items {
			if serviceMarker(&service) != marker {
				continue
			}
			resources = append(resources, sdk.EdgeInventoryResource{Identity: serviceReference(service.Namespace, service.Name), OwnershipMarker: marker, Owned: true, Live: true})
		}
	}
	sort.Slice(resources, func(i, j int) bool { return resources[i].Identity < resources[j].Identity })
	return resources, nil
}

func (api *NativeAPI) ensureService(ctx context.Context, plan ovhEdgePlan) (*corev1.Service, error) {
	service, err := api.api.GetService(ctx, plan.namespace, plan.serviceName, metav1.GetOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return nil, fmt.Errorf("read OVH MKS Load Balancer service: %w", err)
	}
	if apierrors.IsNotFound(err) || service == nil {
		created, createErr := api.api.CreateService(ctx, desiredService(plan), metav1.CreateOptions{})
		if createErr != nil {
			return nil, fmt.Errorf("create OVH MKS Load Balancer service: %w", createErr)
		}
		return created, nil
	}
	if !ownedService(service, plan.ownershipMarker) {
		return nil, errors.New("refusing to adopt an unowned OVH MKS Load Balancer service")
	}
	if serviceMatchesPlan(service, plan) {
		return service, nil
	}
	updated := service.DeepCopy()
	if updated.Annotations == nil {
		updated.Annotations = make(map[string]string)
	}
	updated.Annotations[ovhLoadBalancerKey] = ovhLoadBalancerClass
	updated.Annotations[ovhOwnershipAnnotation] = plan.ownershipMarker
	updated.Annotations[ovhManagedAnnotation] = ovhManagedValue
	updated.Spec.Type = corev1.ServiceTypeLoadBalancer
	updated.Spec.Selector = cloneStringMap(plan.selector)
	updated.Spec.Ports = servicePorts(plan)
	result, updateErr := api.api.UpdateService(ctx, updated, metav1.UpdateOptions{})
	if updateErr != nil {
		return nil, fmt.Errorf("update OVH MKS Load Balancer service: %w", updateErr)
	}
	return result, nil
}

func (api *NativeAPI) getOwnedService(ctx context.Context, namespace, name, marker string) (*corev1.Service, error) {
	service, err := api.api.GetService(ctx, namespace, name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("read OVH MKS Load Balancer service: %w", err)
	}
	if !ownedService(service, marker) {
		return nil, errors.New("OVH MKS Load Balancer service ownership marker does not match")
	}
	return service, nil
}

func parseServicePlan(request sdk.EdgePlanRequest) (ovhEdgePlan, error) {
	namespace := strings.TrimSpace(stringOption(request.Configuration, "namespace"))
	name := strings.TrimSpace(stringOption(request.Configuration, "serviceName"))
	if namespace == "" || name == "" {
		return ovhEdgePlan{}, errors.New("configuration namespace and serviceName are required")
	}
	if strings.ContainsAny(namespace+name, "\r\n\x00") {
		return ovhEdgePlan{}, errors.New("configuration namespace and serviceName must be single-line")
	}
	port, err := integerOption(request.Configuration, "servicePort")
	if err != nil || port < 1 || port > 65535 {
		return ovhEdgePlan{}, errors.New("configuration servicePort must be an integer between 1 and 65535")
	}
	targetPort, err := targetPortOption(request.Configuration, "targetPort")
	if err != nil {
		return ovhEdgePlan{}, err
	}
	selector, err := selectorOption(request.Configuration, "selector")
	if err != nil {
		return ovhEdgePlan{}, err
	}
	marker := strings.TrimSpace(request.Intent.OwnershipMarker)
	if marker == "" || strings.ContainsAny(marker, "\r\n\x00") {
		return ovhEdgePlan{}, errors.New("OVH edge ownership marker is required and must be single-line")
	}
	return ovhEdgePlan{namespace: namespace, serviceName: name, servicePort: int32(port), targetPort: targetPort, selector: selector, originHealthRef: request.Intent.OriginHealthRef, ownershipMarker: marker}, nil
}

func desiredService(plan ovhEdgePlan) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: plan.serviceName, Namespace: plan.namespace, Annotations: map[string]string{ovhOwnershipAnnotation: plan.ownershipMarker, ovhManagedAnnotation: ovhManagedValue, ovhLoadBalancerKey: ovhLoadBalancerClass}},
		Spec:       corev1.ServiceSpec{Type: corev1.ServiceTypeLoadBalancer, Selector: cloneStringMap(plan.selector), Ports: servicePorts(plan)},
	}
}

func servicePorts(plan ovhEdgePlan) []corev1.ServicePort {
	return []corev1.ServicePort{{Name: "http", Protocol: corev1.ProtocolTCP, Port: plan.servicePort, TargetPort: plan.targetPort}}
}

func serviceMatchesPlan(service *corev1.Service, plan ovhEdgePlan) bool {
	if service == nil || !ownedService(service, plan.ownershipMarker) || service.Spec.Type != corev1.ServiceTypeLoadBalancer || !ownedServiceClass(service) || len(service.Spec.Ports) != 1 || service.Spec.Ports[0].Port != plan.servicePort || service.Spec.Ports[0].TargetPort != plan.targetPort || service.Spec.Ports[0].Protocol != corev1.ProtocolTCP {
		return false
	}
	return mapsEqual(service.Spec.Selector, plan.selector)
}

func ownedService(service *corev1.Service, marker string) bool {
	return service != nil && serviceMarker(service) == strings.TrimSpace(marker) && strings.TrimSpace(marker) != ""
}

func serviceMarker(service *corev1.Service) string {
	if service == nil || service.Annotations == nil {
		return ""
	}
	return strings.TrimSpace(service.Annotations[ovhOwnershipAnnotation])
}

func ownedServiceClass(service *corev1.Service) bool {
	return service != nil && service.Annotations != nil && service.Annotations[ovhLoadBalancerKey] == ovhLoadBalancerClass
}

func serviceReady(service *corev1.Service) bool {
	if service == nil {
		return false
	}
	for _, ingress := range service.Status.LoadBalancer.Ingress {
		if strings.TrimSpace(ingress.IP) != "" || strings.TrimSpace(ingress.Hostname) != "" {
			return true
		}
	}
	return false
}

func serviceReference(namespace, name string) string {
	return ovhServiceReference + base64.RawURLEncoding.EncodeToString([]byte(namespace+"\x00"+name))
}

func serviceReferenceFromReferences(references []string) (string, string, error) {
	var namespace, name string
	for _, reference := range references {
		if !strings.HasPrefix(reference, ovhServiceReference) {
			continue
		}
		encoded := strings.TrimPrefix(reference, ovhServiceReference)
		decoded, err := base64.RawURLEncoding.DecodeString(encoded)
		if err != nil {
			return "", "", fmt.Errorf("invalid OVH service reference %q", reference)
		}
		parts := strings.Split(string(decoded), "\x00")
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" || strings.ContainsAny(parts[0]+parts[1], "\r\n") {
			return "", "", fmt.Errorf("invalid OVH service reference %q", reference)
		}
		if namespace != "" && (namespace != parts[0] || name != parts[1]) {
			return "", "", errors.New("multiple OVH service references were supplied")
		}
		namespace, name = parts[0], parts[1]
	}
	if namespace == "" {
		return "", "", errors.New("OVH Kubernetes service reference is required")
	}
	return namespace, name, nil
}

func serviceOperationID(action sdk.EdgeAction, namespace, name, marker string) string {
	return ovhServiceOperation + ":service:" + string(action) + ":" + base64.RawURLEncoding.EncodeToString([]byte(namespace+"\x00"+name)) + ":" + base64.RawURLEncoding.EncodeToString([]byte(marker))
}

func parseServiceOperationID(value string) (sdk.EdgeAction, string, string, string, error) {
	parts := strings.Split(value, ":")
	if len(parts) != 5 || parts[0] != "ovh.kubernetes.service" || parts[1] != "service" || parts[2] == "" || parts[4] == "" {
		return "", "", "", "", fmt.Errorf("invalid OVH service operation ID %q", value)
	}
	action := sdk.EdgeAction(parts[2])
	if action != sdk.EdgeApply && action != sdk.EdgeVerify && action != sdk.EdgeDestroy {
		return "", "", "", "", fmt.Errorf("invalid OVH service operation action %q", parts[2])
	}
	decoded, err := base64.RawURLEncoding.DecodeString(parts[3])
	if err != nil {
		return "", "", "", "", fmt.Errorf("invalid OVH service operation reference: %w", err)
	}
	parts = strings.Split(string(decoded), "\x00")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", "", "", fmt.Errorf("invalid OVH service operation reference %q", value)
	}
	markerBytes, err := base64.RawURLEncoding.DecodeString(strings.Split(value, ":")[4])
	if err != nil || len(markerBytes) == 0 || strings.ContainsAny(string(markerBytes), "\r\n\x00") {
		return "", "", "", "", fmt.Errorf("invalid OVH service operation ownership marker %q", value)
	}
	return action, parts[0], parts[1], string(markerBytes), nil
}

func serviceOutputs(service *corev1.Service) []sdk.AdapterOutput {
	outputs := []sdk.AdapterOutput{{Key: "serviceName", Value: service.Name}, {Key: "serviceNamespace", Value: service.Namespace}}
	for _, ingress := range service.Status.LoadBalancer.Ingress {
		if strings.TrimSpace(ingress.Hostname) != "" {
			outputs = append(outputs, sdk.AdapterOutput{Key: "loadBalancerHostname", Value: ingress.Hostname})
		}
		if strings.TrimSpace(ingress.IP) != "" {
			outputs = append(outputs, sdk.AdapterOutput{Key: "loadBalancerIP", Value: ingress.IP})
		}
	}
	return outputs
}

func pendingObservation(action sdk.EdgeAction, operationID, marker string) sdk.EdgeOperationObservation {
	return sdk.EdgeOperationObservation{Status: sdk.EdgeOperationPending, Action: action, OperationID: operationID, OwnershipMarker: marker, OwnershipVerified: true, IdempotencyVerified: true}
}

func succeededObservation(action sdk.EdgeAction, operationID, marker string, refs, proofs []string) sdk.EdgeOperationObservation {
	return sdk.EdgeOperationObservation{Status: sdk.EdgeOperationSucceeded, Action: action, OperationID: operationID, ResourceRefs: append([]string(nil), refs...), ProofRefs: append([]string(nil), proofs...), OwnershipMarker: marker, OwnershipVerified: true, IdempotencyVerified: true}
}

func succeededObservationWithOutputs(action sdk.EdgeAction, operationID, marker string, refs []string, outputs []sdk.AdapterOutput, proofs []string) sdk.EdgeOperationObservation {
	observation := succeededObservation(action, operationID, marker, refs, proofs)
	observation.Outputs = append([]sdk.AdapterOutput(nil), outputs...)
	return observation
}

func failedObservation(action sdk.EdgeAction, operationID, detail string) sdk.EdgeOperationObservation {
	return sdk.EdgeOperationObservation{Status: sdk.EdgeOperationFailed, Action: action, OperationID: operationID, Detail: detail}
}

func stringOption(configuration map[string]any, key string) string {
	value, ok := configuration[key].(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(value)
}

func integerOption(configuration map[string]any, key string) (int, error) {
	value, ok := configuration[key]
	if !ok {
		return 0, fmt.Errorf("configuration %s is required", key)
	}
	switch value := value.(type) {
	case int:
		return value, nil
	case int32:
		return int(value), nil
	case int64:
		return int(value), nil
	case float64:
		if value != float64(int(value)) {
			return 0, fmt.Errorf("configuration %s must be an integer", key)
		}
		return int(value), nil
	default:
		return 0, fmt.Errorf("configuration %s must be an integer", key)
	}
}

func targetPortOption(configuration map[string]any, key string) (intstr.IntOrString, error) {
	value, ok := configuration[key]
	if !ok {
		return intstr.IntOrString{}, fmt.Errorf("configuration %s is required", key)
	}
	if number, err := integerOption(configuration, key); err == nil {
		if number < 1 || number > 65535 {
			return intstr.IntOrString{}, fmt.Errorf("configuration %s must be between 1 and 65535", key)
		}
		return intstr.FromInt(number), nil
	}
	name, ok := value.(string)
	name = strings.TrimSpace(name)
	if !ok || name == "" || strings.ContainsAny(name, "\r\n\x00") {
		return intstr.IntOrString{}, fmt.Errorf("configuration %s must be an integer or port name", key)
	}
	return intstr.FromString(name), nil
}

func selectorOption(configuration map[string]any, key string) (map[string]string, error) {
	value, ok := configuration[key]
	if !ok {
		return nil, fmt.Errorf("configuration %s is required", key)
	}
	selector := make(map[string]string)
	switch values := value.(type) {
	case map[string]string:
		for key, value := range values {
			selector[key] = value
		}
	case map[string]any:
		for key, value := range values {
			text, ok := value.(string)
			if !ok || strings.TrimSpace(key) == "" || strings.ContainsAny(key+text, "\r\n\x00") {
				return nil, errors.New("configuration selector must contain only single-line string keys and values")
			}
			selector[key] = text
		}
	default:
		return nil, errors.New("configuration selector must be a string map")
	}
	if len(selector) == 0 {
		return nil, errors.New("configuration selector must not be empty")
	}
	return selector, nil
}

func cloneStringMap(input map[string]string) map[string]string {
	output := make(map[string]string, len(input))
	for key, value := range input {
		output[key] = value
	}
	return output
}

func mapsEqual(left, right map[string]string) bool {
	if len(left) != len(right) {
		return false
	}
	for key, value := range left {
		if right[key] != value {
			return false
		}
	}
	return true
}
