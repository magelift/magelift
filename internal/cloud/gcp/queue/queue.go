// Package queue provisions Magento RabbitMQ on GKE for non-preview presets.
package queue

import (
	"errors"
	"fmt"
	"strings"

	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	appsv1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/apps/v1"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	"github.com/pulumi/pulumi-random/sdk/v4/go/random"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

const TypeToken = "magelift:gcp:RabbitMQ"
const AMQPPort = 5672

type Args struct {
	Replicas    int
	Username    string
	K8sProvider *kubernetes.Provider
}

type Component struct {
	pulumi.ResourceState
	Host     pulumi.StringOutput
	Username pulumi.StringOutput
	Password pulumi.StringOutput
}

func New(ctx *pulumi.Context, name string, args Args, opts ...pulumi.ResourceOption) (*Component, error) {
	if strings.TrimSpace(name) == "" {
		return nil, errors.New("queue name is required")
	}
	if args.K8sProvider == nil {
		return nil, errors.New("Kubernetes provider is required for RabbitMQ")
	}
	if args.Replicas < 1 {
		args.Replicas = 1
	}
	if args.Username == "" {
		args.Username = "magento"
	}
	component := &Component{}
	if err := ctx.RegisterComponentResourceV2(TypeToken, name, pulumi.Map{}, component, opts...); err != nil {
		return nil, err
	}
	parent := pulumi.Parent(component)
	k8sOpts := []pulumi.ResourceOption{parent, pulumi.Provider(args.K8sProvider)}
	labels := pulumi.StringMap{"app": pulumi.String(name), "magelift.io/capability": pulumi.String("queue")}
	skipAwait := pulumi.StringMap{"pulumi.com/skipAwait": pulumi.String("true")}

	password, err := random.NewRandomPassword(ctx, name+"-password", &random.RandomPasswordArgs{
		Length:  pulumi.Int(24),
		Special: pulumi.Bool(false),
	}, parent)
	if err != nil {
		return nil, fmt.Errorf("generate RabbitMQ password: %w", err)
	}

	_, err = appsv1.NewDeployment(ctx, name, &appsv1.DeploymentArgs{
		Metadata: &metav1.ObjectMetaArgs{Name: pulumi.String(name), Annotations: skipAwait},
		Spec: &appsv1.DeploymentSpecArgs{
			Replicas: pulumi.Int(args.Replicas),
			Selector: &metav1.LabelSelectorArgs{MatchLabels: labels},
			Template: &corev1.PodTemplateSpecArgs{
				Metadata: &metav1.ObjectMetaArgs{Labels: labels},
				Spec: &corev1.PodSpecArgs{
					Containers: corev1.ContainerArray{
						&corev1.ContainerArgs{
							Name:  pulumi.String("rabbitmq"),
							Image: pulumi.String("rabbitmq:3.13-management-alpine"),
							Ports: corev1.ContainerPortArray{&corev1.ContainerPortArgs{ContainerPort: pulumi.Int(AMQPPort)}},
							Env: corev1.EnvVarArray{
								&corev1.EnvVarArgs{Name: pulumi.String("RABBITMQ_DEFAULT_USER"), Value: pulumi.String(args.Username)},
								&corev1.EnvVarArgs{Name: pulumi.String("RABBITMQ_DEFAULT_PASS"), Value: password.Result},
							},
							Resources: &corev1.ResourceRequirementsArgs{
								Requests: pulumi.StringMap{"cpu": pulumi.String("250m"), "memory": pulumi.String("512Mi")},
							},
						},
					},
				},
			},
		},
	}, k8sOpts...)
	if err != nil {
		return nil, fmt.Errorf("create RabbitMQ Deployment: %w", err)
	}
	svc, err := corev1.NewService(ctx, name+"-svc", &corev1.ServiceArgs{
		Metadata: &metav1.ObjectMetaArgs{Name: pulumi.String(name), Annotations: skipAwait},
		Spec: &corev1.ServiceSpecArgs{
			Selector: labels,
			Ports: corev1.ServicePortArray{
				&corev1.ServicePortArgs{Port: pulumi.Int(AMQPPort), TargetPort: pulumi.Int(AMQPPort)},
			},
		},
	}, k8sOpts...)
	if err != nil {
		return nil, fmt.Errorf("create RabbitMQ Service: %w", err)
	}
	_ = svc
	component.Host = pulumi.String(name).ToStringOutput()
	component.Username = pulumi.String(args.Username).ToStringOutput()
	component.Password = password.Result
	if err := ctx.RegisterResourceOutputs(component, pulumi.Map{
		"host": component.Host, "username": component.Username,
	}); err != nil {
		return nil, err
	}
	return component, nil
}
