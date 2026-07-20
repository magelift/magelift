// Package search provisions Magento OpenSearch on GKE (GCP has no managed OpenSearch).
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

type Args struct {
	Replicas    int
	K8sProvider *kubernetes.Provider
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
	component := &Component{}
	if err := ctx.RegisterComponentResourceV2(TypeToken, name, pulumi.Map{}, component, opts...); err != nil {
		return nil, err
	}
	parent := pulumi.Parent(component)
	k8sOpts := []pulumi.ResourceOption{parent, pulumi.Provider(args.K8sProvider)}
	labels := pulumi.StringMap{"app": pulumi.String(name), "magelift.io/capability": pulumi.String("search")}
	skipAwait := pulumi.StringMap{"pulumi.com/skipAwait": pulumi.String("true")}

	_, err := appsv1.NewDeployment(ctx, name, &appsv1.DeploymentArgs{
		Metadata: &metav1.ObjectMetaArgs{Name: pulumi.String(name), Annotations: skipAwait},
		Spec: &appsv1.DeploymentSpecArgs{
			Replicas: pulumi.Int(args.Replicas),
			Selector: &metav1.LabelSelectorArgs{MatchLabels: labels},
			Template: &corev1.PodTemplateSpecArgs{
				Metadata: &metav1.ObjectMetaArgs{Labels: labels},
				Spec: &corev1.PodSpecArgs{
					Containers: corev1.ContainerArray{
						&corev1.ContainerArgs{
							Name:  pulumi.String("opensearch"),
							Image: pulumi.String("opensearchproject/opensearch:2.19.0"),
							Ports: corev1.ContainerPortArray{&corev1.ContainerPortArgs{ContainerPort: pulumi.Int(OpenSearchPort)}},
							Env: corev1.EnvVarArray{
								&corev1.EnvVarArgs{Name: pulumi.String("discovery.type"), Value: pulumi.String("single-node")},
								&corev1.EnvVarArgs{Name: pulumi.String("DISABLE_SECURITY_PLUGIN"), Value: pulumi.String("true")},
								&corev1.EnvVarArgs{Name: pulumi.String("OPENSEARCH_JAVA_OPTS"), Value: pulumi.String("-Xms512m -Xmx512m")},
							},
							Resources: &corev1.ResourceRequirementsArgs{
								Requests: pulumi.StringMap{"cpu": pulumi.String("500m"), "memory": pulumi.String("1Gi")},
							},
						},
					},
				},
			},
		},
	}, k8sOpts...)
	if err != nil {
		return nil, fmt.Errorf("create OpenSearch Deployment: %w", err)
	}
	svc, err := corev1.NewService(ctx, name+"-svc", &corev1.ServiceArgs{
		Metadata: &metav1.ObjectMetaArgs{Name: pulumi.String(name), Annotations: skipAwait},
		Spec: &corev1.ServiceSpecArgs{
			Selector: labels,
			Ports: corev1.ServicePortArray{
				&corev1.ServicePortArgs{Port: pulumi.Int(OpenSearchPort), TargetPort: pulumi.Int(OpenSearchPort)},
			},
		},
	}, k8sOpts...)
	if err != nil {
		return nil, fmt.Errorf("create OpenSearch Service: %w", err)
	}
	_ = svc
	component.Endpoint = pulumi.String(name).ToStringOutput()
	if err := ctx.RegisterResourceOutputs(component, pulumi.Map{"endpoint": component.Endpoint}); err != nil {
		return nil, err
	}
	return component, nil
}
