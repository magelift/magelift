package runtime

import (
	"errors"
	"fmt"
	"strings"

	"github.com/magelift/magelift/internal/cloud/kube"
	"github.com/magelift/magelift/internal/cloud/ovh/naming"
	"github.com/magelift/magelift/internal/platform"
	"github.com/ovh/pulumi-ovh/sdk/v2/go/ovh/cloudproject"
	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	appsv1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/apps/v1"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	"github.com/pulumi/pulumi-random/sdk/v4/go/random"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

const TypeToken = "magelift:ovh:MKSRuntime"
const ApplicationPort = 8080

type Args struct {
	ServiceName                    string
	Region                         string
	ProjectName                    string
	Environment                    string
	NetworkID                      pulumi.StringInput
	SubnetID                       pulumi.StringInput
	AttachFloatingIPs              bool
	PrivateNetworkRoutingAsDefault bool
	Image                          string
	ApplicationMode                string
	ApplicationVersion             string
	WebRuntime                     string
	DatabaseWriter                 pulumi.StringInput
	DatabaseName                   string
	DatabaseUsername               string
	DatabasePassword               pulumi.StringInput
	CacheEndpoint                  pulumi.StringInput
	SessionEndpoint                pulumi.StringInput
	EncryptionKeySecret            string
	CPURequest                     string
	MemoryRequest                  string
	DesiredWebReplicas             int
	QueueConsumerCount             int
	MKSPlan                        string
	NodeFlavor                     string
	NodeCount                      int
	AvailabilityZones              []string
	Magento                        platform.MagentoOverlays
}

type Component struct {
	pulumi.ResourceState
	ClusterName             pulumi.StringOutput
	ClusterIdentifier       pulumi.StringOutput
	ServiceName             pulumi.StringOutput
	ApplicationURL          pulumi.StringOutput
	DatabaseSecretName      pulumi.StringOutput
	EncryptionKeySecretName pulumi.StringOutput
	Kubeconfig              pulumi.StringOutput
}

func New(ctx *pulumi.Context, name string, args Args, opts ...pulumi.ResourceOption) (*Component, error) {
	if strings.TrimSpace(name) == "" {
		return nil, errors.New("runtime name is required")
	}
	if strings.TrimSpace(args.ServiceName) == "" || strings.TrimSpace(args.Region) == "" {
		return nil, errors.New("OVH service name and region are required")
	}
	if strings.TrimSpace(args.Image) == "" {
		return nil, errors.New("container image digest is required")
	}
	if strings.TrimSpace(args.DatabaseUsername) == "" || args.DatabasePassword == nil {
		return nil, errors.New("database username and password are required")
	}
	if strings.TrimSpace(args.EncryptionKeySecret) == "" {
		return nil, errors.New("Kubernetes Secret name for Magento encryption key is required")
	}
	if args.DesiredWebReplicas < 1 {
		args.DesiredWebReplicas = 1
	}
	if args.CPURequest == "" {
		args.CPURequest = "500m"
	}
	if args.MemoryRequest == "" {
		args.MemoryRequest = kube.DefaultApplicationMemoryRequest
	}
	if args.MKSPlan == "" {
		args.MKSPlan = "standard"
	}
	if args.MKSPlan != "free" && args.MKSPlan != "standard" {
		return nil, fmt.Errorf("unsupported OVH MKS plan %q", args.MKSPlan)
	}
	if args.NodeFlavor == "" {
		args.NodeFlavor = "b3-8"
	}
	if args.NodeCount < 1 {
		args.NodeCount = 1
	}
	availabilityZones, err := normalizeAvailabilityZones(args.AvailabilityZones)
	if err != nil {
		return nil, err
	}
	if args.MKSPlan == "free" && len(availabilityZones) > 1 {
		return nil, errors.New("OVH MKS free plan supports one availability zone; use the standard plan for multi-zone workers")
	}
	if len(availabilityZones) > 1 && args.NodeCount < len(availabilityZones) {
		return nil, fmt.Errorf("OVH MKS node count %d is too small for %d availability zones; configure at least one node per zone", args.NodeCount, len(availabilityZones))
	}
	if args.SubnetID != nil && len(availabilityZones) == 0 {
		return nil, errors.New("OVH MKS availability zones are required for a private-network cluster")
	}
	component := &Component{}
	if err := ctx.RegisterComponentResourceV2(TypeToken, name, pulumi.Map{
		"serviceName": pulumi.String(args.ServiceName),
	}, component, opts...); err != nil {
		return nil, err
	}
	parent := pulumi.Parent(component)

	clusterName := naming.ClusterName(args.ProjectName, args.Environment)
	kubeArgs := &cloudproject.KubeArgs{
		ServiceName:  pulumi.String(args.ServiceName),
		Name:         pulumi.String(clusterName),
		Region:       pulumi.String(args.Region),
		Plan:         pulumi.String(args.MKSPlan),
		UpdatePolicy: pulumi.String("MINIMAL_DOWNTIME"),
	}
	if args.NetworkID != nil {
		kubeArgs.PrivateNetworkId = args.NetworkID.ToStringOutput().ToStringPtrOutput()
	}
	if args.SubnetID != nil {
		// Modern OVH regions require the nodes subnet explicitly. The network
		// component creates the matching OpenStack gateway before this cluster.
		kubeArgs.NodesSubnetId = args.SubnetID.ToStringOutput().ToStringPtrOutput()
	}
	if args.NetworkID != nil && args.SubnetID != nil && args.PrivateNetworkRoutingAsDefault {
		// OVH's documented DHCP-gateway mode requires an empty
		// defaultVrackGateway. A non-empty value is a separate custom-gateway
		// contract and must not be inferred from the subnet's first usable IP.
		kubeArgs.PrivateNetworkConfiguration = cloudproject.KubePrivateNetworkConfigurationArgs{
			DefaultVrackGateway:            pulumi.String(""),
			PrivateNetworkRoutingAsDefault: pulumi.Bool(true),
		}
	}
	cluster, err := cloudproject.NewKube(ctx, name+"-cluster", kubeArgs, parent)
	if err != nil {
		return nil, fmt.Errorf("create OVH Managed Kubernetes cluster: %w", err)
	}

	// OVH associates a node pool with one availability zone. A multi-zone
	// cluster therefore needs one pool per zone; putting every zone in one pool
	// is rejected by the provider and would not guarantee balanced capacity.
	poolCount := len(availabilityZones)
	if poolCount == 0 {
		poolCount = 1
	}
	nodesPerPool := distributeNodes(args.NodeCount, poolCount)
	pools := make([]pulumi.Resource, 0, poolCount)
	for index := range poolCount {
		poolName := strings.ReplaceAll(name, "_", "-") + "-pool"
		poolZone := ""
		if len(availabilityZones) > 0 {
			poolZone = availabilityZones[index]
			poolName = poolNameForZone(name, poolZone, len(availabilityZones) > 1)
		}
		nodePoolArgs := &cloudproject.KubeNodePoolArgs{
			ServiceName:  pulumi.String(args.ServiceName),
			KubeId:       cluster.ID().ToStringOutput(),
			Name:         pulumi.String(poolName),
			FlavorName:   pulumi.String(args.NodeFlavor),
			DesiredNodes: pulumi.Int(nodesPerPool[index]),
			MinNodes:     pulumi.Int(nodesPerPool[index]),
			MaxNodes:     pulumi.Int(nodesPerPool[index]),
		}
		if poolZone != "" {
			nodePoolArgs.AvailabilityZones = pulumi.StringArray{pulumi.String(poolZone)}
		}
		if args.AttachFloatingIPs {
			nodePoolArgs.AttachFloatingIps = cloudproject.KubeNodePoolAttachFloatingIpsArgs{
				Enabled: pulumi.BoolPtr(true),
			}
		}
		resourceName := name + "-pool"
		if poolCount > 1 {
			resourceName = fmt.Sprintf("%s-pool-%d", name, index)
		}
		pool, err := cloudproject.NewKubeNodePool(ctx, resourceName, nodePoolArgs, parent, pulumi.DependsOn([]pulumi.Resource{cluster}))
		if err != nil {
			return nil, fmt.Errorf("create OVH node pool %s: %w", poolName, err)
		}
		pools = append(pools, pool)
	}
	poolDependencies := append([]pulumi.Resource{cluster}, pools...)

	// Workloads depend on every pool, not just the cluster: MKS schedules pods
	// and runs the LB controller on worker nodes, so every configured failure
	// domain must exist before workload resources are submitted.
	kubeconfig := pulumi.ToSecret(cluster.Kubeconfig).(pulumi.StringOutput)
	k8sProvider, err := kubernetes.NewProvider(ctx, name+"-k8s", &kubernetes.ProviderArgs{
		Kubeconfig:        kubeconfig,
		ClusterIdentifier: cluster.ID().ToStringOutput(),
	}, parent, pulumi.DependsOn(poolDependencies))
	if err != nil {
		return nil, fmt.Errorf("create Kubernetes provider: %w", err)
	}
	k8sOpts := []pulumi.ResourceOption{parent, pulumi.Provider(k8sProvider), pulumi.DependsOn(poolDependencies)}
	skipAwait := kube.SkipAwaitAnnotations()

	env := containerEnv(args)
	databaseSecret, err := kube.NewDatabaseCredentialsSecret(ctx, name, args.DatabaseUsername, args.DatabasePassword, k8sOpts...)
	if err != nil {
		return nil, fmt.Errorf("create database credentials Secret: %w", err)
	}
	databaseSecretName := kube.DatabaseCredentialsSecretName(name)
	env = kube.AppendDatabaseCredentialEnv(env, databaseSecretName)
	encryptionKeySecretName := strings.TrimSpace(args.EncryptionKeySecret)
	encryptionKey, err := random.NewRandomPassword(ctx, name+"-encryption-key-value", &random.RandomPasswordArgs{
		Length: pulumi.Int(64), Special: pulumi.Bool(true), OverrideSpecial: pulumi.String("!@#%+=-"),
	}, parent)
	if err != nil {
		return nil, fmt.Errorf("generate Magento encryption key: %w", err)
	}
	encryptionSecret, err := kube.NewNamedMagentoEncryptionKeySecret(ctx, name, encryptionKeySecretName, pulumi.ToSecret(encryptionKey.Result).(pulumi.StringOutput), k8sOpts...)
	if err != nil {
		return nil, fmt.Errorf("create Magento encryption key Secret: %w", err)
	}
	env = kube.AppendEncryptionKeyEnv(env, encryptionKeySecretName)
	workloadOpts := append(k8sOpts, pulumi.DependsOn([]pulumi.Resource{databaseSecret, encryptionSecret}))

	webContainers, err := kube.WebRuntimeContainers(args.WebRuntime, args.Image, env, args.CPURequest, args.MemoryRequest)
	if err != nil {
		return nil, err
	}
	web, err := appsv1.NewDeployment(ctx, name+"-web", &appsv1.DeploymentArgs{
		Metadata: &metav1.ObjectMetaArgs{Name: pulumi.String(name + "-web"), Annotations: skipAwait},
		Spec: &appsv1.DeploymentSpecArgs{
			Replicas: pulumi.Int(args.DesiredWebReplicas),
			Selector: &metav1.LabelSelectorArgs{MatchLabels: pulumi.StringMap{"app": pulumi.String(name + "-web")}},
			Template: &corev1.PodTemplateSpecArgs{
				Metadata: &metav1.ObjectMetaArgs{Labels: pulumi.StringMap{"app": pulumi.String(name + "-web")}},
				Spec: &corev1.PodSpecArgs{
					Containers: webContainers,
				},
			},
		},
	}, workloadOpts...)
	if err != nil {
		return nil, fmt.Errorf("create web Deployment: %w", err)
	}

	service, err := corev1.NewService(ctx, name+"-web-svc", &corev1.ServiceArgs{
		Metadata: &metav1.ObjectMetaArgs{Name: pulumi.String(name + "-web"), Annotations: skipAwait},
		Spec: &corev1.ServiceSpecArgs{
			Type:     pulumi.String("LoadBalancer"),
			Selector: pulumi.StringMap{"app": pulumi.String(name + "-web")},
			Ports: corev1.ServicePortArray{
				&corev1.ServicePortArgs{Port: pulumi.Int(80), TargetPort: pulumi.Int(ApplicationPort)},
			},
		},
	}, append(k8sOpts, pulumi.DependsOn([]pulumi.Resource{web}))...)
	if err != nil {
		return nil, fmt.Errorf("create web Service: %w", err)
	}

	_, err = appsv1.NewDeployment(ctx, name+"-cron", &appsv1.DeploymentArgs{
		Metadata: &metav1.ObjectMetaArgs{Name: pulumi.String(name + "-cron"), Annotations: skipAwait},
		Spec: &appsv1.DeploymentSpecArgs{
			Replicas: pulumi.Int(1),
			Selector: &metav1.LabelSelectorArgs{MatchLabels: pulumi.StringMap{"app": pulumi.String(name + "-cron")}},
			Template: &corev1.PodTemplateSpecArgs{
				Metadata: &metav1.ObjectMetaArgs{Labels: pulumi.StringMap{"app": pulumi.String(name + "-cron")}},
				Spec: &corev1.PodSpecArgs{
					Containers: corev1.ContainerArray{
						&corev1.ContainerArgs{
							Name:    pulumi.String("cron"),
							Image:   pulumi.String(args.Image),
							Command: kube.ToStringArray(platform.MagentoCronShell()),
							Env:     env,
							Resources: &corev1.ResourceRequirementsArgs{
								Requests: pulumi.StringMap{"cpu": pulumi.String(args.CPURequest), "memory": pulumi.String(args.MemoryRequest)},
							},
						},
					},
				},
			},
		},
	}, workloadOpts...)
	if err != nil {
		return nil, fmt.Errorf("create cron Deployment: %w", err)
	}

	if args.QueueConsumerCount > 0 {
		_, err = appsv1.NewDeployment(ctx, name+"-queue", &appsv1.DeploymentArgs{
			Metadata: &metav1.ObjectMetaArgs{Name: pulumi.String(name + "-queue"), Annotations: skipAwait},
			Spec: &appsv1.DeploymentSpecArgs{
				Replicas: pulumi.Int(args.QueueConsumerCount),
				Selector: &metav1.LabelSelectorArgs{MatchLabels: pulumi.StringMap{"app": pulumi.String(name + "-queue")}},
				Template: &corev1.PodTemplateSpecArgs{
					Metadata: &metav1.ObjectMetaArgs{Labels: pulumi.StringMap{"app": pulumi.String(name + "-queue")}},
					Spec: &corev1.PodSpecArgs{
						Containers: corev1.ContainerArray{
							&corev1.ContainerArgs{
								Name:    pulumi.String("queue"),
								Image:   pulumi.String(args.Image),
								Command: kube.ToStringArray(platform.MagentoQueueArgsFor(args.Magento.ConsumerNames)),
								Env:     env,
								Resources: &corev1.ResourceRequirementsArgs{
									Requests: pulumi.StringMap{"cpu": pulumi.String(args.CPURequest), "memory": pulumi.String(args.MemoryRequest)},
								},
							},
						},
					},
				},
			},
		}, workloadOpts...)
		if err != nil {
			return nil, fmt.Errorf("create queue Deployment: %w", err)
		}
	}

	component.ClusterName = cluster.Name
	component.ClusterIdentifier = cluster.ID().ToStringOutput()
	component.ServiceName = pulumi.String(name + "-web").ToStringOutput()
	component.DatabaseSecretName = pulumi.String(databaseSecretName).ToStringOutput()
	component.EncryptionKeySecretName = pulumi.String(encryptionKeySecretName).ToStringOutput()
	component.Kubeconfig = kubeconfig
	component.ApplicationURL = service.Status.ApplyT(func(status *corev1.ServiceStatus) string {
		if status == nil || len(status.LoadBalancer.Ingress) == 0 {
			return ""
		}
		ingress := status.LoadBalancer.Ingress[0]
		if ingress.Hostname != nil && *ingress.Hostname != "" {
			return naming.FormatURL(*ingress.Hostname)
		}
		if ingress.Ip != nil {
			return naming.FormatURL(*ingress.Ip)
		}
		return ""
	}).(pulumi.StringOutput)

	if err := ctx.RegisterResourceOutputs(component, pulumi.Map{
		"clusterName": component.ClusterName, "serviceName": component.ServiceName, "applicationURL": component.ApplicationURL,
		"databaseSecretName":      component.DatabaseSecretName,
		"encryptionKeySecretName": component.EncryptionKeySecretName,
		"kubeconfig":              component.Kubeconfig,
	}); err != nil {
		return nil, err
	}
	return component, nil
}

func normalizeAvailabilityZones(values []string) ([]string, error) {
	if len(values) == 0 {
		return nil, nil
	}
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for index, value := range values {
		zone := strings.TrimSpace(value)
		if zone == "" {
			return nil, fmt.Errorf("OVH MKS availability zone %d is empty", index)
		}
		key := strings.ToLower(zone)
		if _, exists := seen[key]; exists {
			return nil, fmt.Errorf("OVH MKS availability zone %q is duplicated", zone)
		}
		seen[key] = struct{}{}
		result = append(result, zone)
	}
	return result, nil
}

func distributeNodes(total, poolCount int) []int {
	if poolCount < 1 {
		return nil
	}
	base, remainder := total/poolCount, total%poolCount
	result := make([]int, poolCount)
	for index := range result {
		result[index] = base
		if index < remainder {
			result[index]++
		}
	}
	return result
}

func poolNameForZone(name, zone string, multiZone bool) string {
	base := strings.ReplaceAll(name, "_", "-")
	if !multiZone {
		return base + "-pool"
	}
	zone = strings.NewReplacer("_", "-", ".", "-", "/", "-").Replace(strings.ToLower(zone))
	return base + "-" + zone + "-pool"
}

func containerEnv(args Args) corev1.EnvVarArrayOutput {
	return pulumi.All(args.DatabaseWriter, args.CacheEndpoint, args.SessionEndpoint).ApplyT(func(values []interface{}) []corev1.EnvVar {
		session := values[2].(string)
		if session == "" {
			session = values[1].(string)
		}
		bindings := platform.CoreEnvBindings(platform.CapabilityEndpoints{
			ApplicationMode:    args.ApplicationMode,
			ApplicationVersion: args.ApplicationVersion,
			WebRuntime:         args.WebRuntime,
			DatabaseWriter:     values[0].(string),
			DatabaseName:       args.DatabaseName,
			CacheEndpoint:      values[1].(string),
			SessionEndpoint:    session,
			Magento:            args.Magento,
		})
		return kube.EnvVars(bindings)
	}).(corev1.EnvVarArrayOutput)
}
