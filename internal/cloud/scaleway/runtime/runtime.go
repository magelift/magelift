package runtime

import (
	"errors"
	"fmt"
	"strings"

	"github.com/magelift/magelift/internal/cloud/kube"
	"github.com/magelift/magelift/internal/cloud/scaleway/naming"
	"github.com/magelift/magelift/internal/platform"
	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	appsv1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/apps/v1"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	"github.com/pulumi/pulumi-random/sdk/v4/go/random"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	scwk8s "github.com/pulumiverse/pulumi-scaleway/sdk/go/scaleway/kubernetes"
)

const TypeToken = "magelift:scaleway:KapsuleRuntime"
const ApplicationPort = 8080

type Args struct {
	ProjectID           string
	MagentoProject      string
	Environment         string
	Region              string
	PrivateNetworkID    pulumi.StringInput
	Image               string
	ApplicationMode     string
	ApplicationVersion  string
	WebRuntime          string
	DatabaseWriter      pulumi.StringInput
	DatabaseName        string
	DatabaseUsername    string
	DatabasePassword    pulumi.StringInput
	CacheEndpoint       pulumi.StringInput
	SessionEndpoint     pulumi.StringInput
	EncryptionKeySecret string
	KapsuleVersion      string
	NodeType            string
	NodeCount           int
	AvailabilityZones   []string
	CPURequest          string
	MemoryRequest       string
	DesiredWebReplicas  int
	QueueConsumerCount  int
	Labels              map[string]string
	Magento             platform.MagentoOverlays
}

type Component struct {
	pulumi.ResourceState
	ClusterName             pulumi.StringOutput
	ServiceName             pulumi.StringOutput
	ApplicationURL          pulumi.StringOutput
	ClusterEndpoint         pulumi.StringOutput
	DatabaseSecretName      pulumi.StringOutput
	EncryptionKeySecretName pulumi.StringOutput
	Kubeconfig              pulumi.StringOutput
}

func New(ctx *pulumi.Context, name string, args Args, opts ...pulumi.ResourceOption) (*Component, error) {
	if strings.TrimSpace(name) == "" {
		return nil, errors.New("runtime name is required")
	}
	if strings.TrimSpace(args.ProjectID) == "" || strings.TrimSpace(args.Region) == "" {
		return nil, errors.New("Scaleway project ID and region are required")
	}
	if strings.TrimSpace(args.MagentoProject) == "" || strings.TrimSpace(args.Environment) == "" {
		return nil, errors.New("Magento project and environment are required for cluster naming")
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
	if strings.TrimSpace(args.KapsuleVersion) == "" {
		args.KapsuleVersion = "1.36.4"
	}
	if strings.TrimSpace(args.NodeType) == "" {
		args.NodeType = "DEV1-M"
	}
	if args.NodeCount < 1 {
		args.NodeCount = 2
	}
	if args.CPURequest == "" {
		args.CPURequest = "500m"
	}
	if args.MemoryRequest == "" {
		args.MemoryRequest = kube.DefaultApplicationMemoryRequest
	}
	component := &Component{}
	if err := ctx.RegisterComponentResourceV2(TypeToken, name, pulumi.Map{
		"projectId": pulumi.String(args.ProjectID), "region": pulumi.String(args.Region),
	}, component, opts...); err != nil {
		return nil, err
	}
	parent := pulumi.Parent(component)

	tags := make(pulumi.StringArray, 0, len(args.Labels))
	for key, value := range args.Labels {
		tags = append(tags, pulumi.String(key+"="+value))
	}

	clusterName := naming.ClusterName(args.MagentoProject, args.Environment)
	cluster, err := scwk8s.NewCluster(ctx, name+"-cluster", &scwk8s.ClusterArgs{
		Name:                      pulumi.String(clusterName),
		Version:                   pulumi.String(args.KapsuleVersion),
		Cni:                       pulumi.String("cilium"),
		ProjectId:                 pulumi.String(args.ProjectID),
		Region:                    pulumi.String(args.Region),
		PrivateNetworkId:          args.PrivateNetworkID,
		DeleteAdditionalResources: pulumi.Bool(true),
		Tags:                      tags,
	}, parent)
	if err != nil {
		return nil, fmt.Errorf("create Scaleway Kapsule cluster: %w", err)
	}
	placements, err := poolPlacements(name, args.NodeCount, args.AvailabilityZones)
	if err != nil {
		return nil, fmt.Errorf("plan Scaleway Kapsule pools: %w", err)
	}
	poolResources := make([]pulumi.Resource, 0, len(placements))
	for _, placement := range placements {
		poolArgs := &scwk8s.PoolArgs{
			ClusterId: cluster.ID(),
			Version:   cluster.Version,
			Name:      pulumi.String(placement.Name),
			NodeType:  pulumi.String(args.NodeType),
			Size:      pulumi.Int(placement.Size),
			Region:    pulumi.String(args.Region),
		}
		if placement.Zone != "" {
			poolArgs.Zone = pulumi.StringPtr(placement.Zone)
		}
		pool, err := scwk8s.NewPool(ctx, placement.Name, poolArgs, parent, pulumi.DependsOn([]pulumi.Resource{cluster}))
		if err != nil {
			return nil, fmt.Errorf("create Scaleway Kapsule pool %s: %w", placement.Name, err)
		}
		poolResources = append(poolResources, pool)
	}

	clusterDependencies := append([]pulumi.Resource{cluster}, poolResources...)

	kubeconfig := pulumi.ToSecret(generateKubeconfig(clusterName, cluster.Kubeconfigs)).(pulumi.StringOutput)
	k8sProvider, err := kubernetes.NewProvider(ctx, name+"-k8s", &kubernetes.ProviderArgs{
		Kubeconfig:        kubeconfig,
		ClusterIdentifier: cluster.ID().ToStringOutput(),
	}, parent, pulumi.DependsOn(clusterDependencies))
	if err != nil {
		return nil, fmt.Errorf("create Kubernetes provider: %w", err)
	}
	k8sOpts := []pulumi.ResourceOption{parent, pulumi.Provider(k8sProvider), pulumi.DependsOn(clusterDependencies)}
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
	webLabels := pulumi.StringMap{"app": pulumi.String(name + "-web")}

	webContainers, err := kube.WebRuntimeContainers(args.WebRuntime, args.Image, env, args.CPURequest, args.MemoryRequest)
	if err != nil {
		return nil, err
	}
	web, err := appsv1.NewDeployment(ctx, name+"-web", &appsv1.DeploymentArgs{
		Metadata: &metav1.ObjectMetaArgs{Name: pulumi.String(name + "-web"), Annotations: skipAwait},
		Spec: &appsv1.DeploymentSpecArgs{
			Replicas: pulumi.Int(args.DesiredWebReplicas),
			Selector: &metav1.LabelSelectorArgs{MatchLabels: webLabels},
			Template: &corev1.PodTemplateSpecArgs{
				Metadata: &metav1.ObjectMetaArgs{Labels: webLabels},
				Spec: &corev1.PodSpecArgs{
					TopologySpreadConstraints: zoneSpreadConstraints(args.DesiredWebReplicas, args.AvailabilityZones, webLabels),
					Containers:                webContainers,
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
		queueLabels := pulumi.StringMap{"app": pulumi.String(name + "-queue")}
		_, err = appsv1.NewDeployment(ctx, name+"-queue", &appsv1.DeploymentArgs{
			Metadata: &metav1.ObjectMetaArgs{Name: pulumi.String(name + "-queue"), Annotations: skipAwait},
			Spec: &appsv1.DeploymentSpecArgs{
				Replicas: pulumi.Int(args.QueueConsumerCount),
				Selector: &metav1.LabelSelectorArgs{MatchLabels: queueLabels},
				Template: &corev1.PodTemplateSpecArgs{
					Metadata: &metav1.ObjectMetaArgs{Labels: queueLabels},
					Spec: &corev1.PodSpecArgs{
						TopologySpreadConstraints: zoneSpreadConstraints(args.QueueConsumerCount, args.AvailabilityZones, queueLabels),
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
	component.ServiceName = pulumi.String(name + "-web").ToStringOutput()
	component.ClusterEndpoint = cluster.ApiserverUrl
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

type poolPlacement struct {
	Name string
	Zone string
	Size int
}

func poolPlacements(name string, nodeCount int, zones []string) ([]poolPlacement, error) {
	if nodeCount < 1 {
		return nil, errors.New("node count must be at least one")
	}
	if len(zones) == 0 {
		return []poolPlacement{{Name: name + "-pool", Size: nodeCount}}, nil
	}

	seen := make(map[string]struct{}, len(zones))
	for _, zone := range zones {
		zone = strings.TrimSpace(zone)
		if zone == "" {
			return nil, errors.New("availability zones cannot be empty")
		}
		if _, exists := seen[zone]; exists {
			return nil, fmt.Errorf("availability zone %q is repeated", zone)
		}
		seen[zone] = struct{}{}
	}
	if nodeCount < len(zones) {
		return nil, fmt.Errorf("node count %d cannot place at least one node in each of %d availability zones", nodeCount, len(zones))
	}

	placements := make([]poolPlacement, 0, len(zones))
	baseSize := nodeCount / len(zones)
	extraNodes := nodeCount % len(zones)
	for index, zone := range zones {
		size := baseSize
		if index < extraNodes {
			size++
		}
		placements = append(placements, poolPlacement{Name: fmt.Sprintf("%s-pool-%d", name, index+1), Zone: strings.TrimSpace(zone), Size: size})
	}
	return placements, nil
}

func zoneSpreadConstraints(replicas int, zones []string, labels pulumi.StringMap) corev1.TopologySpreadConstraintArray {
	if len(zones) < 2 || replicas < len(zones) {
		return nil
	}
	return corev1.TopologySpreadConstraintArray{
		&corev1.TopologySpreadConstraintArgs{
			MaxSkew:           pulumi.Int(1),
			TopologyKey:       pulumi.String("topology.kubernetes.io/zone"),
			WhenUnsatisfiable: pulumi.String("DoNotSchedule"),
			LabelSelector:     &metav1.LabelSelectorArgs{MatchLabels: labels},
		},
	}
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

// generateKubeconfig builds a static token-authenticated kubeconfig from the
// cluster's first Scaleway-issued Kubeconfig entry, avoiding any exec plugin
// dependency at Magento runtime. See the Scaleway provider SDK examples for
// the Kubeconfigs[].{Host,Token,ClusterCaCertificate} shape.
func generateKubeconfig(clusterName string, kubeconfigs scwk8s.ClusterKubeconfigArrayOutput) pulumi.StringOutput {
	return kubeconfigs.ApplyT(func(configs []scwk8s.ClusterKubeconfig) (string, error) {
		if len(configs) == 0 {
			return "", errors.New("Scaleway cluster produced no kubeconfig entries")
		}
		cfg := configs[0]
		contextName := "magelift_" + clusterName
		server := naming.FormatURL(stringOrEmpty(cfg.Host))
		return kube.BuildStaticTokenKubeconfig(contextName, server, stringOrEmpty(cfg.ClusterCaCertificate), stringOrEmpty(cfg.Token)), nil
	}).(pulumi.StringOutput)
}

func stringOrEmpty(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
