// Package eks provisions an experimental EKS Auto Mode Magento runtime.
package eks

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/acourtiol/magelift/internal/platform"
	"github.com/pulumi/pulumi-aws/sdk/v7/go/aws/eks"
	"github.com/pulumi/pulumi-aws/sdk/v7/go/aws/iam"
	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	appsv1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/apps/v1"
	batchv1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/batch/v1"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

const TypeToken = "magelift:aws:EKSRuntime"
const ApplicationPort = 8080

type Args struct {
	Region             string
	PrivateSubnetIDs   pulumi.StringArrayInput
	Image              string
	ApplicationMode    string
	WebRuntime         string
	DatabaseWriter     pulumi.StringInput
	DatabaseName       string
	CacheEndpoint      pulumi.StringInput
	SessionEndpoint    pulumi.StringInput
	CPURequest         string
	MemoryRequest      string
	DesiredWebReplicas int
	QueueConsumerCount int
	Tags               map[string]string
}

type Component struct {
	pulumi.ResourceState
	ClusterName    pulumi.StringOutput
	ServiceName    pulumi.StringOutput
	ApplicationURL pulumi.StringOutput
	ClusterARN     pulumi.StringOutput
	Kubeconfig     pulumi.StringOutput
}

func New(ctx *pulumi.Context, name string, args Args, opts ...pulumi.ResourceOption) (*Component, error) {
	if strings.TrimSpace(name) == "" {
		return nil, errors.New("runtime name is required")
	}
	if strings.TrimSpace(args.Region) == "" {
		return nil, errors.New("region is required")
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
		"region": pulumi.String(args.Region),
	}, component, opts...); err != nil {
		return nil, err
	}
	parent := pulumi.Parent(component)

	clusterRole, err := iam.NewRole(ctx, name+"-cluster-role", &iam.RoleArgs{
		AssumeRolePolicy: pulumi.String(`{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"eks.amazonaws.com"},"Action":["sts:AssumeRole","sts:TagSession"]}]}`),
		Tags:             pulumi.ToStringMap(args.Tags),
	}, parent)
	if err != nil {
		return nil, fmt.Errorf("create EKS cluster role: %w", err)
	}
	clusterPolicies := []struct {
		suffix string
		arn    string
	}{
		{"cluster", "arn:aws:iam::aws:policy/AmazonEKSClusterPolicy"},
		{"compute", "arn:aws:iam::aws:policy/AmazonEKSComputePolicy"},
		{"block", "arn:aws:iam::aws:policy/AmazonEKSBlockStoragePolicy"},
		{"lb", "arn:aws:iam::aws:policy/AmazonEKSLoadBalancingPolicy"},
		{"net", "arn:aws:iam::aws:policy/AmazonEKSNetworkingPolicy"},
	}
	var clusterPolicyAttachments []pulumi.Resource
	for _, policy := range clusterPolicies {
		attachment, err := iam.NewRolePolicyAttachment(ctx, name+"-cluster-"+policy.suffix, &iam.RolePolicyAttachmentArgs{
			Role:      clusterRole.Name,
			PolicyArn: pulumi.String(policy.arn),
		}, parent)
		if err != nil {
			return nil, fmt.Errorf("attach %s policy: %w", policy.suffix, err)
		}
		clusterPolicyAttachments = append(clusterPolicyAttachments, attachment)
	}

	nodeRole, err := iam.NewRole(ctx, name+"-node-role", &iam.RoleArgs{
		AssumeRolePolicy: pulumi.String(`{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"ec2.amazonaws.com"},"Action":"sts:AssumeRole"}]}`),
		Tags:             pulumi.ToStringMap(args.Tags),
	}, parent)
	if err != nil {
		return nil, fmt.Errorf("create EKS node role: %w", err)
	}
	for _, policy := range []struct {
		suffix string
		arn    string
	}{
		{"worker", "arn:aws:iam::aws:policy/AmazonEKSWorkerNodeMinimalPolicy"},
		{"ecr", "arn:aws:iam::aws:policy/AmazonEC2ContainerRegistryPullOnly"},
	} {
		if _, err := iam.NewRolePolicyAttachment(ctx, name+"-node-"+policy.suffix, &iam.RolePolicyAttachmentArgs{
			Role: nodeRole.Name, PolicyArn: pulumi.String(policy.arn),
		}, parent); err != nil {
			return nil, fmt.Errorf("attach node %s policy: %w", policy.suffix, err)
		}
	}

	clusterDeps := append([]pulumi.Resource{}, clusterPolicyAttachments...)
	cluster, err := eks.NewCluster(ctx, name+"-cluster", &eks.ClusterArgs{
		Name:                       pulumi.String(name),
		RoleArn:                    clusterRole.Arn,
		Version:                    pulumi.String("1.32"),
		BootstrapSelfManagedAddons: pulumi.Bool(false),
		AccessConfig: &eks.ClusterAccessConfigArgs{
			AuthenticationMode: pulumi.String("API"),
		},
		ComputeConfig: &eks.ClusterComputeConfigArgs{
			Enabled:     pulumi.Bool(true),
			NodePools:   pulumi.StringArray{pulumi.String("general-purpose"), pulumi.String("system")},
			NodeRoleArn: nodeRole.Arn,
		},
		KubernetesNetworkConfig: &eks.ClusterKubernetesNetworkConfigArgs{
			ElasticLoadBalancing: &eks.ClusterKubernetesNetworkConfigElasticLoadBalancingArgs{Enabled: pulumi.Bool(true)},
		},
		StorageConfig: &eks.ClusterStorageConfigArgs{
			BlockStorage: &eks.ClusterStorageConfigBlockStorageArgs{Enabled: pulumi.Bool(true)},
		},
		VpcConfig: &eks.ClusterVpcConfigArgs{
			EndpointPrivateAccess: pulumi.Bool(true),
			EndpointPublicAccess:  pulumi.Bool(true),
			SubnetIds:             args.PrivateSubnetIDs,
		},
		Tags: pulumi.ToStringMap(args.Tags),
	}, append([]pulumi.ResourceOption{parent}, pulumi.DependsOn(clusterDeps))...)
	if err != nil {
		return nil, fmt.Errorf("create EKS Auto Mode cluster: %w", err)
	}

	kubeconfig := pulumi.ToSecret(generateKubeconfig(cluster.Name, cluster.Endpoint, cluster.CertificateAuthority, args.Region)).(pulumi.StringOutput)
	k8sProvider, err := kubernetes.NewProvider(ctx, name+"-k8s", &kubernetes.ProviderArgs{
		Kubeconfig:        kubeconfig,
		ClusterIdentifier: cluster.Arn,
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
								Command: pulumi.StringArray{pulumi.String("bin/magento"), pulumi.String("queue:consumers:start"), pulumi.String("async.operations.all")},
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
	component.ClusterARN = cluster.Arn
	component.ServiceName = pulumi.String(name + "-web").ToStringOutput()
	component.Kubeconfig = kubeconfig
	component.ApplicationURL = service.Status.ApplyT(func(status *corev1.ServiceStatus) string {
		if status == nil || len(status.LoadBalancer.Ingress) == 0 {
			return ""
		}
		ingress := status.LoadBalancer.Ingress[0]
		if ingress.Hostname != nil && *ingress.Hostname != "" {
			return "https://" + *ingress.Hostname
		}
		if ingress.Ip != nil && *ingress.Ip != "" {
			return "https://" + *ingress.Ip
		}
		return ""
	}).(pulumi.StringOutput)

	if err := ctx.RegisterResourceOutputs(component, pulumi.Map{
		"clusterName": component.ClusterName, "serviceName": component.ServiceName,
		"applicationURL": component.ApplicationURL, "clusterArn": component.ClusterARN,
		"kubeconfig": component.Kubeconfig,
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

func generateKubeconfig(name, endpoint pulumi.StringOutput, ca eks.ClusterCertificateAuthorityOutput, region string) pulumi.StringOutput {
	return pulumi.All(name, endpoint, ca.Data()).ApplyT(func(values []interface{}) (string, error) {
		clusterName := values[0].(string)
		clusterEndpoint := values[1].(string)
		caData, _ := values[2].(*string)
		caValue := ""
		if caData != nil {
			caValue = *caData
		}
		execArgs, err := json.Marshal([]string{"eks", "get-token", "--cluster-name", clusterName, "--region", region})
		if err != nil {
			return "", err
		}
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
    exec:
      apiVersion: client.authentication.k8s.io/v1beta1
      command: aws
      args: %s
`, caValue, clusterEndpoint, clusterName, clusterName, clusterName, clusterName, clusterName, clusterName, string(execArgs)), nil
	}).(pulumi.StringOutput)
}
