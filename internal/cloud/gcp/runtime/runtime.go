package runtime

import (
	"errors"
	"fmt"
	"strings"

	"github.com/acourtiol/magelift/internal/cloud/gcp/naming"
	"github.com/acourtiol/magelift/internal/cloud/gcp/queue"
	"github.com/acourtiol/magelift/internal/cloud/gcp/search"
	"github.com/acourtiol/magelift/internal/cloud/kube"
	"github.com/acourtiol/magelift/internal/platform"
	"github.com/pulumi/pulumi-gcp/sdk/v9/go/gcp/container"
	"github.com/pulumi/pulumi-gcp/sdk/v9/go/gcp/organizations"
	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	appsv1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/apps/v1"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

const TypeToken = "magelift:gcp:GKERuntime"
const ApplicationPort = 8080

type Args struct {
	Project             string
	MagentoProject      string
	Environment         string
	Region              string
	NetworkSelfLink     pulumi.StringInput
	PrivateSubnetNames  pulumi.StringArrayInput
	Image               string
	ApplicationMode     string
	WebRuntime          string
	DatabaseWriter      pulumi.StringInput
	DatabaseName        string
	CacheEndpoint       pulumi.StringInput
	SessionEndpoint     pulumi.StringInput
	SearchMode          string
	SearchReplicas      int
	QueueMode           string
	QueueReplicas       int
	MediaBucket         pulumi.StringInput
	MediaURL            pulumi.StringInput
	EncryptionKeySecret string
	CPURequest          string
	MemoryRequest       string
	DesiredWebReplicas  int
	QueueConsumerCount  int
	Labels              map[string]string
}

type Component struct {
	pulumi.ResourceState
	ClusterName     pulumi.StringOutput
	ServiceName     pulumi.StringOutput
	ApplicationURL  pulumi.StringOutput
	ClusterEndpoint pulumi.StringOutput
	SearchEndpoint  pulumi.StringOutput
	QueueHost       pulumi.StringOutput
	QueueMode       pulumi.StringOutput
	Kubeconfig      pulumi.StringOutput
}

func New(ctx *pulumi.Context, name string, args Args, opts ...pulumi.ResourceOption) (*Component, error) {
	if strings.TrimSpace(name) == "" {
		return nil, errors.New("runtime name is required")
	}
	if strings.TrimSpace(args.Project) == "" || strings.TrimSpace(args.Region) == "" {
		return nil, errors.New("GCP project and region are required")
	}
	if strings.TrimSpace(args.Image) == "" {
		return nil, errors.New("container image digest is required")
	}
	if strings.TrimSpace(args.MagentoProject) == "" || strings.TrimSpace(args.Environment) == "" {
		return nil, errors.New("Magento project and environment are required for GKE cluster naming")
	}
	if args.DesiredWebReplicas < 1 {
		args.DesiredWebReplicas = 1
	}
	if args.CPURequest == "" {
		args.CPURequest = "500m"
	}
	if args.MemoryRequest == "" {
		args.MemoryRequest = "1Gi"
	}
	if args.QueueMode == "" {
		args.QueueMode = "database"
	}
	component := &Component{}
	if err := ctx.RegisterComponentResourceV2(TypeToken, name, pulumi.Map{
		"project": pulumi.String(args.Project),
	}, component, opts...); err != nil {
		return nil, err
	}
	parent := pulumi.Parent(component)
	skipAwait := kube.SkipAwaitAnnotations()

	clusterName := naming.ClusterName(args.MagentoProject, args.Environment)
	cluster, err := container.NewCluster(ctx, name+"-cluster", &container.ClusterArgs{
		Project:         pulumi.String(args.Project),
		Name:            pulumi.String(clusterName),
		Location:        pulumi.String(args.Region),
		EnableAutopilot: pulumi.Bool(true),
		Network:         args.NetworkSelfLink,
		Subnetwork:      args.PrivateSubnetNames.ToStringArrayOutput().Index(pulumi.Int(0)),
		IpAllocationPolicy: &container.ClusterIpAllocationPolicyArgs{
			ClusterIpv4CidrBlock:  pulumi.String("/17"),
			ServicesIpv4CidrBlock: pulumi.String("/22"),
		},
		ReleaseChannel: &container.ClusterReleaseChannelArgs{
			Channel: pulumi.String("REGULAR"),
		},
		DeletionProtection: pulumi.Bool(false),
		ResourceLabels:     pulumi.ToStringMap(args.Labels),
	}, parent)
	if err != nil {
		return nil, fmt.Errorf("create GKE Autopilot cluster: %w", err)
	}

	kubeconfig := pulumi.ToSecret(generateKubeconfig(ctx, args.Project, cluster.Name, cluster.Endpoint, cluster.MasterAuth)).(pulumi.StringOutput)
	k8sProvider, err := kubernetes.NewProvider(ctx, name+"-k8s", &kubernetes.ProviderArgs{
		Kubeconfig:        kubeconfig,
		ClusterIdentifier: cluster.ID().ToStringOutput(),
	}, parent, pulumi.DependsOn([]pulumi.Resource{cluster}))
	if err != nil {
		return nil, fmt.Errorf("create Kubernetes provider: %w", err)
	}
	k8sOpts := []pulumi.ResourceOption{parent, pulumi.Provider(k8sProvider), pulumi.DependsOn([]pulumi.Resource{cluster})}

	searchEndpoint := pulumi.String("").ToStringOutput()
	if args.SearchMode == "opensearch" {
		searchComp, err := search.New(ctx, naming.Resource(args.MagentoProject, args.Environment, "search"), search.Args{
			Replicas: args.SearchReplicas, K8sProvider: k8sProvider,
		}, k8sOpts...)
		if err != nil {
			return nil, fmt.Errorf("create OpenSearch: %w", err)
		}
		searchEndpoint = searchComp.Endpoint
	}

	queueHost := pulumi.String("").ToStringOutput()
	queueUser := ""
	if args.QueueMode == "rabbitmq" {
		queueComp, err := queue.New(ctx, naming.Resource(args.MagentoProject, args.Environment, "rabbitmq"), queue.Args{
			Replicas: args.QueueReplicas, K8sProvider: k8sProvider, Username: "magento",
		}, k8sOpts...)
		if err != nil {
			return nil, fmt.Errorf("create RabbitMQ: %w", err)
		}
		queueHost = queueComp.Host
		queueUser = "magento"
	}

	env := containerEnv(args, searchEndpoint, queueHost, queueUser)

	web, err := appsv1.NewDeployment(ctx, name+"-web", &appsv1.DeploymentArgs{
		Metadata: &metav1.ObjectMetaArgs{Name: pulumi.String(name + "-web"), Annotations: skipAwait},
		Spec: &appsv1.DeploymentSpecArgs{
			Replicas: pulumi.Int(args.DesiredWebReplicas),
			Selector: &metav1.LabelSelectorArgs{MatchLabels: pulumi.StringMap{"app": pulumi.String(name + "-web")}},
			Template: &corev1.PodTemplateSpecArgs{
				Metadata: &metav1.ObjectMetaArgs{Labels: pulumi.StringMap{"app": pulumi.String(name + "-web")}},
				Spec: &corev1.PodSpecArgs{
					Containers: corev1.ContainerArray{
						&corev1.ContainerArgs{
							Name:  pulumi.String("web"),
							Image: pulumi.String(args.Image),
							Ports: corev1.ContainerPortArray{&corev1.ContainerPortArgs{ContainerPort: pulumi.Int(ApplicationPort)}},
							Env:   env,
							Resources: &corev1.ResourceRequirementsArgs{
								Requests: pulumi.StringMap{"cpu": pulumi.String(args.CPURequest), "memory": pulumi.String(args.MemoryRequest)},
							},
						},
					},
				},
			},
		},
	}, k8sOpts...)
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
	}, k8sOpts...)
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
								Command: kube.ToStringArray(platform.MagentoQueueArgs()),
								Env:     env,
								Resources: &corev1.ResourceRequirementsArgs{
									Requests: pulumi.StringMap{"cpu": pulumi.String(args.CPURequest), "memory": pulumi.String(args.MemoryRequest)},
								},
							},
						},
					},
				},
			},
		}, k8sOpts...)
		if err != nil {
			return nil, fmt.Errorf("create queue Deployment: %w", err)
		}
	}

	component.ClusterName = cluster.Name
	component.ServiceName = pulumi.String(name + "-web").ToStringOutput()
	component.ClusterEndpoint = cluster.Endpoint
	component.SearchEndpoint = searchEndpoint
	component.QueueHost = queueHost
	component.QueueMode = pulumi.String(args.QueueMode).ToStringOutput()
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
		"clusterName": component.ClusterName, "serviceName": component.ServiceName,
		"applicationURL": component.ApplicationURL, "searchEndpoint": component.SearchEndpoint,
		"queueMode": component.QueueMode, "queueHost": component.QueueHost,
		"kubeconfig": component.Kubeconfig,
	}); err != nil {
		return nil, err
	}
	return component, nil
}

func containerEnv(args Args, searchEndpoint, queueHost pulumi.StringOutput, queueUser string) corev1.EnvVarArrayOutput {
	mediaBucket := args.MediaBucket
	if mediaBucket == nil {
		mediaBucket = pulumi.String("")
	}
	mediaURL := args.MediaURL
	if mediaURL == nil {
		mediaURL = pulumi.String("")
	}
	return pulumi.All(args.DatabaseWriter, args.CacheEndpoint, args.SessionEndpoint, searchEndpoint, queueHost, mediaBucket, mediaURL).ApplyT(func(values []interface{}) []corev1.EnvVar {
		session := values[2].(string)
		if session == "" {
			session = values[1].(string)
		}
		bindings := platform.CoreEnvBindings(platform.CapabilityEndpoints{
			ApplicationMode: args.ApplicationMode,
			WebRuntime:      args.WebRuntime,
			DatabaseWriter:  values[0].(string),
			DatabaseName:    args.DatabaseName,
			CacheEndpoint:   values[1].(string),
			SessionEndpoint: session,
			SearchEndpoint:  values[3].(string),
			QueueMode:       args.QueueMode,
			QueueHost:       values[4].(string),
			QueueUsername:   queueUser,
			MediaBucket:     values[5].(string),
			MediaURL:        values[6].(string),
		})
		return kube.EnvVars(bindings)
	}).(corev1.EnvVarArrayOutput)
}

func generateKubeconfig(ctx *pulumi.Context, project string, name, endpoint pulumi.StringOutput, auth container.ClusterMasterAuthOutput) pulumi.StringOutput {
	return pulumi.All(name, endpoint, auth.ClusterCaCertificate(), organizations.GetClientConfigOutput(ctx).AccessToken()).ApplyT(func(values []interface{}) (string, error) {
		contextName := fmt.Sprintf("%s_magelift_%s", project, asString(values[0]))
		server := "https://" + asString(values[1])
		return kube.BuildStaticTokenKubeconfig(contextName, server, asString(values[2]), asString(values[3])), nil
	}).(pulumi.StringOutput)
}

func asString(value interface{}) string {
	switch typed := value.(type) {
	case string:
		return typed
	case *string:
		if typed == nil {
			return ""
		}
		return *typed
	default:
		return fmt.Sprintf("%v", value)
	}
}
