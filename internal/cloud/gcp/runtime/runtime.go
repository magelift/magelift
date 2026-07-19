package runtime

import (
	"errors"
	"fmt"
	"strings"

	"github.com/acourtiol/magelift/internal/cloud/gcp/naming"
	"github.com/acourtiol/magelift/internal/platform"
	"github.com/pulumi/pulumi-gcp/sdk/v9/go/gcp/container"
	"github.com/pulumi/pulumi-gcp/sdk/v9/go/gcp/organizations"
	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	appsv1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/apps/v1"
	batchv1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/batch/v1"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

const TypeToken = "magelift:gcp:GKERuntime"
const ApplicationPort = 8080

type Args struct {
	Project             string
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
	if args.DesiredWebReplicas < 1 {
		args.DesiredWebReplicas = 1
	}
	if args.CPURequest == "" {
		args.CPURequest = "500m"
	}
	if args.MemoryRequest == "" {
		args.MemoryRequest = "1Gi"
	}
	component := &Component{}
	if err := ctx.RegisterComponentResourceV2(TypeToken, name, pulumi.Map{
		"project": pulumi.String(args.Project),
	}, component, opts...); err != nil {
		return nil, err
	}
	parent := pulumi.Parent(component)

	clusterName := naming.ClusterName(args.Project, name)
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

	kubeconfig := generateKubeconfig(ctx, args.Project, cluster.Name, cluster.Endpoint, cluster.MasterAuth)
	k8sProvider, err := kubernetes.NewProvider(ctx, name+"-k8s", &kubernetes.ProviderArgs{
		Kubeconfig:        pulumi.ToSecret(kubeconfig).(pulumi.StringOutput),
		ClusterIdentifier: cluster.ID().ToStringOutput(),
	}, parent, pulumi.DependsOn([]pulumi.Resource{cluster}))
	if err != nil {
		return nil, fmt.Errorf("create Kubernetes provider: %w", err)
	}
	k8sOpts := []pulumi.ResourceOption{parent, pulumi.Provider(k8sProvider), pulumi.DependsOn([]pulumi.Resource{cluster})}

	env := containerEnv(args)

	web, err := appsv1.NewDeployment(ctx, name+"-web", &appsv1.DeploymentArgs{
		Metadata: &metav1.ObjectMetaArgs{Name: pulumi.String(name + "-web")},
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
		Metadata: &metav1.ObjectMetaArgs{Name: pulumi.String(name + "-web")},
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
		Metadata: &metav1.ObjectMetaArgs{Name: pulumi.String(name + "-cron")},
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
							Command: pulumi.StringArray{pulumi.String("/bin/sh"), pulumi.String("-ec"), pulumi.String("while true; do bin/magento cron:run; sleep 60; done")},
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
		Metadata: &metav1.ObjectMetaArgs{Name: pulumi.String(name + "-deploy")},
		Spec: &batchv1.JobSpecArgs{
			Template: &corev1.PodTemplateSpecArgs{
				Spec: &corev1.PodSpecArgs{
					RestartPolicy: pulumi.String("Never"),
					Containers: corev1.ContainerArray{
						&corev1.ContainerArgs{
							Name:    pulumi.String("deploy"),
							Image:   pulumi.String(args.Image),
							Command: pulumi.StringArray{pulumi.String("/bin/sh"), pulumi.String("-ec"), pulumi.String("bin/magento app:config:import --no-interaction && bin/magento setup:upgrade --keep-generated --no-interaction && bin/magento cache:flush")},
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
			Metadata: &metav1.ObjectMetaArgs{Name: pulumi.String(name + "-queue")},
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
								Command: pulumi.StringArray{pulumi.String("bin/magento"), pulumi.String("queue:consumers:start"), pulumi.String("--max-messages=10000")},
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
		env := make([]corev1.EnvVar, 0, len(bindings))
		for _, binding := range bindings {
			value := binding.Value
			env = append(env, corev1.EnvVar{Name: binding.Name, Value: &value})
		}
		return env
	}).(corev1.EnvVarArrayOutput)
}

func generateKubeconfig(ctx *pulumi.Context, project string, name, endpoint pulumi.StringOutput, auth container.ClusterMasterAuthOutput) pulumi.StringOutput {
	clientConfig := organizations.GetClientConfigOutput(ctx)
	return pulumi.All(name, endpoint, auth, clientConfig.AccessToken()).ApplyT(func(values []interface{}) (string, error) {
		clusterName := values[0].(string)
		clusterEndpoint := values[1].(string)
		masterAuth := values[2].(container.ClusterMasterAuth)
		accessToken := values[3].(string)
		ca := ""
		if masterAuth.ClusterCaCertificate != nil {
			ca = *masterAuth.ClusterCaCertificate
		}
		return buildKubeconfig(project, clusterName, clusterEndpoint, ca, accessToken), nil
	}).(pulumi.StringOutput)
}

func buildKubeconfig(project, clusterName, endpoint, caCert, accessToken string) string {
	contextName := fmt.Sprintf("%s_magelift_%s", project, clusterName)
	return fmt.Sprintf(`apiVersion: v1
clusters:
- cluster:
    certificate-authority-data: %s
    server: https://%s
  name: %s
contexts:
- context:
    cluster: %s
    user: %s
  name: %s
current-context: %s
kind: Config
users:
- name: %s
  user:
    token: %s
`, caCert, endpoint, contextName, contextName, contextName, contextName, contextName, contextName, accessToken)
}
