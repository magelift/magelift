package runtime

import (
	"errors"
	"fmt"
	"strings"

	"github.com/acourtiol/magelift/internal/cloud/kube"
	"github.com/acourtiol/magelift/internal/cloud/scaleway/naming"
	"github.com/acourtiol/magelift/internal/platform"
	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	appsv1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/apps/v1"
	batchv1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/batch/v1"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
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
	WebRuntime          string
	DatabaseWriter      pulumi.StringInput
	DatabaseName        string
	CacheEndpoint       pulumi.StringInput
	SessionEndpoint     pulumi.StringInput
	EncryptionKeySecret string
	KapsuleVersion      string
	NodeType            string
	NodeCount           int
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
	if args.DesiredWebReplicas < 1 {
		args.DesiredWebReplicas = 1
	}
	if strings.TrimSpace(args.KapsuleVersion) == "" {
		args.KapsuleVersion = "1.29.1"
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
		args.MemoryRequest = "1Gi"
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
	pool, err := scwk8s.NewPool(ctx, name+"-pool", &scwk8s.PoolArgs{
		ClusterId: cluster.ID(),
		Version:   cluster.Version,
		Name:      pulumi.String(name + "-pool"),
		NodeType:  pulumi.String(args.NodeType),
		Size:      pulumi.Int(args.NodeCount),
		Region:    pulumi.String(args.Region),
	}, parent, pulumi.DependsOn([]pulumi.Resource{cluster}))
	if err != nil {
		return nil, fmt.Errorf("create Scaleway Kapsule pool: %w", err)
	}

	kubeconfig := generateKubeconfig(clusterName, cluster.Kubeconfigs)
	k8sProvider, err := kubernetes.NewProvider(ctx, name+"-k8s", &kubernetes.ProviderArgs{
		Kubeconfig:        pulumi.ToSecret(kubeconfig).(pulumi.StringOutput),
		ClusterIdentifier: cluster.ID().ToStringOutput(),
	}, parent, pulumi.DependsOn([]pulumi.Resource{cluster, pool}))
	if err != nil {
		return nil, fmt.Errorf("create Kubernetes provider: %w", err)
	}
	k8sOpts := []pulumi.ResourceOption{parent, pulumi.Provider(k8sProvider), pulumi.DependsOn([]pulumi.Resource{cluster, pool})}
	skipAwait := kube.SkipAwaitAnnotations()

	env := containerEnv(args)

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

	_, err = batchv1.NewJob(ctx, name+"-deploy", &batchv1.JobArgs{
		Metadata: &metav1.ObjectMetaArgs{Name: pulumi.String(name + "-deploy"), Annotations: skipAwait},
		Spec: &batchv1.JobSpecArgs{
			Template: &corev1.PodTemplateSpecArgs{
				Spec: &corev1.PodSpecArgs{
					RestartPolicy: pulumi.String("Never"),
					Containers: corev1.ContainerArray{
						&corev1.ContainerArgs{
							Name:    pulumi.String("deploy"),
							Image:   pulumi.String(args.Image),
							Command: kube.ToStringArray(platform.MagentoMigrationShell()),
							Env:     env,
						},
					},
				},
			},
		},
	}, k8sOpts...)
	if err != nil {
		return nil, fmt.Errorf("create deploy Job: %w", err)
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
	component.ClusterEndpoint = cluster.ApiserverUrl
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
	}); err != nil {
		return nil, err
	}
	return component, nil
}

func containerEnv(args Args) corev1.EnvVarArrayOutput {
	return pulumi.All(args.DatabaseWriter, args.CacheEndpoint, args.SessionEndpoint).ApplyT(func(values []interface{}) []corev1.EnvVar {
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
