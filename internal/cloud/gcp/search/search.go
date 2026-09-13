// Package search provisions the provider-neutral Magento OpenSearch workload
// used by Kubernetes runtime adapters.
package search

import (
	"errors"
	"fmt"
	"strings"

	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	appsv1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/apps/v1"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

const TypeToken = "magelift:gcp:OpenSearch"
const OpenSearchPort = 9200
const DefaultImage = "opensearchproject/opensearch:3@sha256:bcc1797519726ceb6d651d4a3e60b7c30da91793914a8dfe75fd441d4f641509"

type Args struct {
	Replicas         int
	Image            string
	K8sProvider      *kubernetes.Provider
	StorageClassName string
	Dependencies     []pulumi.Resource
	// TypeToken lets another first-party Kubernetes target reuse this
	// provider-neutral workload while keeping its Pulumi component identity.
	// An empty value preserves the historical GCP token.
	TypeToken string
}

type Component struct {
	pulumi.ResourceState
	Endpoint pulumi.StringOutput
}

func New(ctx *pulumi.Context, name string, args Args, opts ...pulumi.ResourceOption) (*Component, error) {
	if strings.TrimSpace(name) == "" {
		return nil, errors.New("search name is required")
	}
	if args.K8sProvider == nil {
		return nil, errors.New("Kubernetes provider is required for OpenSearch")
	}
	if args.Replicas < 1 {
		args.Replicas = 1
	}
	if strings.TrimSpace(args.Image) == "" {
		args.Image = DefaultImage
	}

	component := &Component{}
	resourceType := TypeToken
	if strings.TrimSpace(args.TypeToken) != "" {
		resourceType = args.TypeToken
	}
	if err := ctx.RegisterComponentResourceV2(resourceType, name, pulumi.Map{}, component, opts...); err != nil {
		return nil, err
	}
	parent := pulumi.Parent(component)
	k8sOpts := []pulumi.ResourceOption{parent, pulumi.Provider(args.K8sProvider)}
	if len(args.Dependencies) > 0 {
		k8sOpts = append(k8sOpts, pulumi.DependsOn(args.Dependencies))
	}
	labels := pulumi.StringMap{"app": pulumi.String(name), "magelift.io/capability": pulumi.String("search")}
	headlessName := name + "-headless"
	var storageClassName pulumi.StringPtrInput
	if strings.TrimSpace(args.StorageClassName) != "" {
		storageClassName = pulumi.StringPtr(args.StorageClassName)
	}

	headless, err := corev1.NewService(ctx, name+"-headless", &corev1.ServiceArgs{
		Metadata: &metav1.ObjectMetaArgs{Name: pulumi.String(headlessName)},
		Spec: &corev1.ServiceSpecArgs{
			ClusterIP:                pulumi.String("None"),
			PublishNotReadyAddresses: pulumi.Bool(true),
			Selector:                 labels,
			Ports:                    openSearchServicePorts(),
		},
	}, k8sOpts...)
	if err != nil {
		return nil, fmt.Errorf("create OpenSearch headless Service: %w", err)
	}

	service, err := corev1.NewService(ctx, name+"-svc", &corev1.ServiceArgs{
		Metadata: &metav1.ObjectMetaArgs{Name: pulumi.String(name)},
		Spec: &corev1.ServiceSpecArgs{
			Selector: labels,
			Ports:    openSearchServicePorts(),
		},
	}, k8sOpts...)
	if err != nil {
		return nil, fmt.Errorf("create OpenSearch Service: %w", err)
	}

	_, err = appsv1.NewStatefulSet(ctx, name, &appsv1.StatefulSetArgs{
		Metadata: &metav1.ObjectMetaArgs{Name: pulumi.String(name)},
		Spec: &appsv1.StatefulSetSpecArgs{
			PodManagementPolicy: pulumi.String("Parallel"),
			PersistentVolumeClaimRetentionPolicy: &appsv1.StatefulSetPersistentVolumeClaimRetentionPolicyArgs{
				WhenDeleted: pulumi.String("Delete"),
				WhenScaled:  pulumi.String("Retain"),
			},
			Replicas:    pulumi.Int(args.Replicas),
			Selector:    &metav1.LabelSelectorArgs{MatchLabels: labels},
			ServiceName: pulumi.String(headlessName),
			Template: &corev1.PodTemplateSpecArgs{
				Metadata: &metav1.ObjectMetaArgs{Labels: labels},
				Spec: &corev1.PodSpecArgs{
					SecurityContext:           &corev1.PodSecurityContextArgs{FsGroup: pulumi.Int(1000)},
					TopologySpreadConstraints: openSearchTopologySpread(args.Replicas, labels),
					Containers: corev1.ContainerArray{
						&corev1.ContainerArgs{
							Name:           pulumi.String("opensearch"),
							Image:          pulumi.String(args.Image),
							Ports:          corev1.ContainerPortArray{&corev1.ContainerPortArgs{Name: pulumi.String("http"), ContainerPort: pulumi.Int(OpenSearchPort)}},
							Env:            openSearchEnv(name, headlessName, args.Replicas),
							ReadinessProbe: openSearchReadinessProbe(),
							StartupProbe:   openSearchStartupProbe(),
							LivenessProbe:  openSearchLivenessProbe(),
							Resources:      &corev1.ResourceRequirementsArgs{Requests: pulumi.StringMap{"cpu": pulumi.String("500m"), "memory": pulumi.String("1Gi")}},
							VolumeMounts:   corev1.VolumeMountArray{&corev1.VolumeMountArgs{Name: pulumi.String("data"), MountPath: pulumi.String("/usr/share/opensearch/data")}},
						},
					},
				},
			},
			VolumeClaimTemplates: corev1.PersistentVolumeClaimTypeArray{
				&corev1.PersistentVolumeClaimTypeArgs{
					Metadata: &metav1.ObjectMetaArgs{Name: pulumi.String("data")},
					Spec: &corev1.PersistentVolumeClaimSpecArgs{
						AccessModes:      pulumi.StringArray{pulumi.String("ReadWriteOnce")},
						StorageClassName: storageClassName,
						Resources: &corev1.VolumeResourceRequirementsArgs{
							Requests: pulumi.StringMap{"storage": pulumi.String("10Gi")},
						},
					},
				},
			},
		},
	}, append(k8sOpts, pulumi.DependsOn([]pulumi.Resource{headless, service}))...)
	if err != nil {
		return nil, fmt.Errorf("create OpenSearch StatefulSet: %w", err)
	}

	component.Endpoint = pulumi.String(name).ToStringOutput()
	if err := ctx.RegisterResourceOutputs(component, pulumi.Map{"endpoint": component.Endpoint}); err != nil {
		return nil, err
	}
	return component, nil
}

func openSearchEnv(name, headlessName string, replicas int) corev1.EnvVarArray {
	env := corev1.EnvVarArray{
		&corev1.EnvVarArgs{Name: pulumi.String("DISABLE_INSTALL_DEMO_CONFIG"), Value: pulumi.String("true")},
		&corev1.EnvVarArgs{Name: pulumi.String("DISABLE_SECURITY_PLUGIN"), Value: pulumi.String("true")},
		&corev1.EnvVarArgs{Name: pulumi.String("OPENSEARCH_JAVA_OPTS"), Value: pulumi.String("-Xms512m -Xmx512m")},
	}
	if replicas == 1 {
		return append(env, &corev1.EnvVarArgs{Name: pulumi.String("discovery.type"), Value: pulumi.String("single-node")})
	}

	initialClusterManagers := make([]string, replicas)
	for index := range replicas {
		initialClusterManagers[index] = fmt.Sprintf("%s-%d", name, index)
	}
	return append(env,
		&corev1.EnvVarArgs{Name: pulumi.String("POD_NAME"), ValueFrom: &corev1.EnvVarSourceArgs{FieldRef: &corev1.ObjectFieldSelectorArgs{FieldPath: pulumi.String("metadata.name")}}},
		&corev1.EnvVarArgs{Name: pulumi.String("node.name"), Value: pulumi.String("$(POD_NAME)")},
		&corev1.EnvVarArgs{Name: pulumi.String("discovery.seed_hosts"), Value: pulumi.String(headlessName)},
		&corev1.EnvVarArgs{Name: pulumi.String("cluster.initial_cluster_manager_nodes"), Value: pulumi.String(strings.Join(initialClusterManagers, ","))},
	)
}

func openSearchServicePorts() corev1.ServicePortArray {
	return corev1.ServicePortArray{
		&corev1.ServicePortArgs{Name: pulumi.String("http"), Port: pulumi.Int(OpenSearchPort), TargetPort: pulumi.Int(OpenSearchPort)},
	}
}

func openSearchTopologySpread(replicas int, labels pulumi.StringMap) corev1.TopologySpreadConstraintArray {
	if replicas < 2 {
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

func openSearchReadinessProbe() *corev1.ProbeArgs {
	return &corev1.ProbeArgs{
		HttpGet:             &corev1.HTTPGetActionArgs{Path: pulumi.String("/_cluster/health?wait_for_status=yellow&timeout=5s"), Port: pulumi.Int(OpenSearchPort)},
		InitialDelaySeconds: pulumi.Int(30), PeriodSeconds: pulumi.Int(10), TimeoutSeconds: pulumi.Int(10), FailureThreshold: pulumi.Int(30),
	}
}

func openSearchStartupProbe() *corev1.ProbeArgs {
	return &corev1.ProbeArgs{
		HttpGet:             &corev1.HTTPGetActionArgs{Path: pulumi.String("/_cluster/health?wait_for_status=yellow&timeout=5s"), Port: pulumi.Int(OpenSearchPort)},
		InitialDelaySeconds: pulumi.Int(30), PeriodSeconds: pulumi.Int(10), TimeoutSeconds: pulumi.Int(10), FailureThreshold: pulumi.Int(60),
	}
}

func openSearchLivenessProbe() *corev1.ProbeArgs {
	return &corev1.ProbeArgs{
		HttpGet:             &corev1.HTTPGetActionArgs{Path: pulumi.String("/"), Port: pulumi.Int(OpenSearchPort)},
		InitialDelaySeconds: pulumi.Int(120), PeriodSeconds: pulumi.Int(30), TimeoutSeconds: pulumi.Int(10), FailureThreshold: pulumi.Int(5),
	}
}
