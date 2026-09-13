// Package queue provisions the provider-neutral Magento RabbitMQ workload used
// by Kubernetes runtime adapters.
package queue

import (
	"errors"
	"fmt"
	"strings"

	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	appsv1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/apps/v1"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	rbacv1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/rbac/v1"
	"github.com/pulumi/pulumi-random/sdk/v4/go/random"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

const TypeToken = "magelift:gcp:RabbitMQ"

const (
	AMQPPort       = 5672
	EPMDPort       = 4369
	InterNodePort  = 25672
	ManagementPort = 15672
	DefaultImage   = "rabbitmq:4.2-management-alpine@sha256:b774118f6eee1edfb133c26f6d9f52b09d2813a6f65a2c9d991a6f6e3db7fa19"
)

type Args struct {
	Replicas         int
	Username         string
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
	if strings.TrimSpace(args.Image) == "" {
		args.Image = DefaultImage
	}
	if args.Username == "" {
		args.Username = "magento"
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
	labels := pulumi.StringMap{"app": pulumi.String(name), "magelift.io/capability": pulumi.String("queue")}
	skipAwait := pulumi.StringMap{"pulumi.com/skipAwait": pulumi.String("true")}
	headlessName := name + "-headless"
	serviceAccountName := name + "-peer-discovery"
	credentialsName := name + "-credentials"
	var storageClassName pulumi.StringPtrInput
	if strings.TrimSpace(args.StorageClassName) != "" {
		storageClassName = pulumi.StringPtr(args.StorageClassName)
	}

	password, err := random.NewRandomPassword(ctx, name+"-password", &random.RandomPasswordArgs{
		Length:  pulumi.Int(24),
		Special: pulumi.Bool(false),
	}, parent)
	if err != nil {
		return nil, fmt.Errorf("generate RabbitMQ password: %w", err)
	}
	erlangCookie, err := random.NewRandomPassword(ctx, name+"-erlang-cookie", &random.RandomPasswordArgs{
		Length:  pulumi.Int(32),
		Special: pulumi.Bool(false),
	}, parent)
	if err != nil {
		return nil, fmt.Errorf("generate RabbitMQ Erlang cookie: %w", err)
	}
	credentials, err := corev1.NewSecret(ctx, name+"-credentials", &corev1.SecretArgs{
		Metadata: &metav1.ObjectMetaArgs{Name: pulumi.String(credentialsName), Annotations: skipAwait},
		StringData: pulumi.StringMap{
			"username":      pulumi.String(args.Username),
			"password":      password.Result,
			"erlang-cookie": erlangCookie.Result,
		},
	}, k8sOpts...)
	if err != nil {
		return nil, fmt.Errorf("create RabbitMQ credentials Secret: %w", err)
	}

	config, err := corev1.NewConfigMap(ctx, name+"-config", &corev1.ConfigMapArgs{
		Metadata: &metav1.ObjectMetaArgs{Name: pulumi.String(name + "-config"), Annotations: skipAwait},
		Data: pulumi.StringMap{
			"rabbitmq.conf": pulumi.String(rabbitmqConfig(headlessName)),
		},
	}, k8sOpts...)
	if err != nil {
		return nil, fmt.Errorf("create RabbitMQ configuration ConfigMap: %w", err)
	}

	serviceAccount, err := corev1.NewServiceAccount(ctx, name+"-peer-discovery", &corev1.ServiceAccountArgs{
		Metadata: &metav1.ObjectMetaArgs{Name: pulumi.String(serviceAccountName), Annotations: skipAwait},
	}, k8sOpts...)
	if err != nil {
		return nil, fmt.Errorf("create RabbitMQ peer-discovery ServiceAccount: %w", err)
	}
	role, err := rbacv1.NewRole(ctx, name+"-peer-discovery", &rbacv1.RoleArgs{
		Metadata: &metav1.ObjectMetaArgs{Name: pulumi.String(serviceAccountName), Annotations: skipAwait},
		Rules: rbacv1.PolicyRuleArray{
			&rbacv1.PolicyRuleArgs{
				ApiGroups: pulumi.StringArray{pulumi.String("")},
				Resources: pulumi.StringArray{pulumi.String("endpoints")},
				Verbs:     pulumi.StringArray{pulumi.String("get")},
			},
			&rbacv1.PolicyRuleArgs{
				ApiGroups: pulumi.StringArray{pulumi.String("")},
				Resources: pulumi.StringArray{pulumi.String("events")},
				Verbs:     pulumi.StringArray{pulumi.String("create")},
			},
		},
	}, k8sOpts...)
	if err != nil {
		return nil, fmt.Errorf("create RabbitMQ peer-discovery Role: %w", err)
	}
	roleBinding, err := rbacv1.NewRoleBinding(ctx, name+"-peer-discovery-binding", &rbacv1.RoleBindingArgs{
		Metadata: &metav1.ObjectMetaArgs{Name: pulumi.String(serviceAccountName), Annotations: skipAwait},
		RoleRef: &rbacv1.RoleRefArgs{
			ApiGroup: pulumi.String("rbac.authorization.k8s.io"),
			Kind:     pulumi.String("Role"),
			Name:     pulumi.String(serviceAccountName),
		},
		Subjects: rbacv1.SubjectArray{
			&rbacv1.SubjectArgs{
				Kind:      pulumi.String("ServiceAccount"),
				Name:      pulumi.String(serviceAccountName),
				Namespace: pulumi.String("default"),
			},
		},
	}, append(k8sOpts, pulumi.DependsOn([]pulumi.Resource{serviceAccount, role}))...)
	if err != nil {
		return nil, fmt.Errorf("create RabbitMQ peer-discovery RoleBinding: %w", err)
	}

	headless, err := corev1.NewService(ctx, name+"-headless", &corev1.ServiceArgs{
		Metadata: &metav1.ObjectMetaArgs{Name: pulumi.String(headlessName), Annotations: skipAwait},
		Spec: &corev1.ServiceSpecArgs{
			ClusterIP:                pulumi.String("None"),
			PublishNotReadyAddresses: pulumi.Bool(true),
			Selector:                 labels,
			Ports:                    rabbitmqServicePorts(),
		},
	}, k8sOpts...)
	if err != nil {
		return nil, fmt.Errorf("create RabbitMQ headless Service: %w", err)
	}
	service, err := corev1.NewService(ctx, name+"-svc", &corev1.ServiceArgs{
		Metadata: &metav1.ObjectMetaArgs{Name: pulumi.String(name), Annotations: skipAwait},
		Spec: &corev1.ServiceSpecArgs{
			Selector: labels,
			Ports: corev1.ServicePortArray{
				&corev1.ServicePortArgs{Name: pulumi.String("amqp"), Port: pulumi.Int(AMQPPort), TargetPort: pulumi.Int(AMQPPort)},
				&corev1.ServicePortArgs{Name: pulumi.String("management"), Port: pulumi.Int(ManagementPort), TargetPort: pulumi.Int(ManagementPort)},
			},
		},
	}, k8sOpts...)
	if err != nil {
		return nil, fmt.Errorf("create RabbitMQ Service: %w", err)
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
					ServiceAccountName: pulumi.String(serviceAccountName),
					TopologySpreadConstraints: corev1.TopologySpreadConstraintArray{
						&corev1.TopologySpreadConstraintArgs{
							MaxSkew:           pulumi.Int(1),
							TopologyKey:       pulumi.String("topology.kubernetes.io/zone"),
							WhenUnsatisfiable: pulumi.String("DoNotSchedule"),
							LabelSelector:     &metav1.LabelSelectorArgs{MatchLabels: labels},
						},
					},
					InitContainers: corev1.ContainerArray{
						&corev1.ContainerArgs{
							Name:    pulumi.String("rabbitmq-configure"),
							Image:   pulumi.String(args.Image),
							Command: pulumi.StringArray{pulumi.String("/bin/sh"), pulumi.String("-ec")},
							Args:    pulumi.StringArray{pulumi.String("cp /config/rabbitmq.conf /etc/rabbitmq/rabbitmq.conf && rabbitmq-plugins --offline enable rabbitmq_peer_discovery_k8s && chown -R 100:101 /etc/rabbitmq /var/lib/rabbitmq && if [ -f /var/lib/rabbitmq/.erlang.cookie ]; then chmod 600 /var/lib/rabbitmq/.erlang.cookie; fi")},
							SecurityContext: &corev1.SecurityContextArgs{
								RunAsUser:    pulumi.Int(0),
								RunAsGroup:   pulumi.Int(0),
								RunAsNonRoot: pulumi.Bool(false),
							},
							Resources: &corev1.ResourceRequirementsArgs{
								Requests: pulumi.StringMap{"cpu": pulumi.String("50m"), "memory": pulumi.String("64Mi")},
							},
							VolumeMounts: corev1.VolumeMountArray{
								&corev1.VolumeMountArgs{Name: pulumi.String("rabbitmq-config"), MountPath: pulumi.String("/config"), ReadOnly: pulumi.Bool(true)},
								&corev1.VolumeMountArgs{Name: pulumi.String("rabbitmq-etc"), MountPath: pulumi.String("/etc/rabbitmq")},
								&corev1.VolumeMountArgs{Name: pulumi.String("data"), MountPath: pulumi.String("/var/lib/rabbitmq")},
							},
						},
					},
					Containers: corev1.ContainerArray{
						&corev1.ContainerArgs{
							Name:  pulumi.String("rabbitmq"),
							Image: pulumi.String(args.Image),
							Ports: corev1.ContainerPortArray{
								&corev1.ContainerPortArgs{Name: pulumi.String("amqp"), ContainerPort: pulumi.Int(AMQPPort)},
								&corev1.ContainerPortArgs{Name: pulumi.String("epmd"), ContainerPort: pulumi.Int(EPMDPort)},
								&corev1.ContainerPortArgs{Name: pulumi.String("inter-node"), ContainerPort: pulumi.Int(InterNodePort)},
								&corev1.ContainerPortArgs{Name: pulumi.String("management"), ContainerPort: pulumi.Int(ManagementPort)},
							},
							Env: rabbitmqEnv(credentialsName, headlessName, args.Username),
							ReadinessProbe: &corev1.ProbeArgs{
								Exec:                &corev1.ExecActionArgs{Command: pulumi.StringArray{pulumi.String("rabbitmq-diagnostics"), pulumi.String("-q"), pulumi.String("ping")}},
								InitialDelaySeconds: pulumi.Int(30), PeriodSeconds: pulumi.Int(10), TimeoutSeconds: pulumi.Int(10), FailureThreshold: pulumi.Int(30),
							},
							StartupProbe: &corev1.ProbeArgs{
								Exec:                &corev1.ExecActionArgs{Command: pulumi.StringArray{pulumi.String("rabbitmq-diagnostics"), pulumi.String("-q"), pulumi.String("ping")}},
								InitialDelaySeconds: pulumi.Int(30), PeriodSeconds: pulumi.Int(10), TimeoutSeconds: pulumi.Int(10), FailureThreshold: pulumi.Int(60),
							},
							LivenessProbe: &corev1.ProbeArgs{
								Exec:                &corev1.ExecActionArgs{Command: pulumi.StringArray{pulumi.String("rabbitmq-diagnostics"), pulumi.String("ping")}},
								InitialDelaySeconds: pulumi.Int(120), PeriodSeconds: pulumi.Int(30), TimeoutSeconds: pulumi.Int(10), FailureThreshold: pulumi.Int(5),
							},
							Resources: &corev1.ResourceRequirementsArgs{
								Requests: pulumi.StringMap{"cpu": pulumi.String("250m"), "memory": pulumi.String("512Mi")},
								Limits:   pulumi.StringMap{"cpu": pulumi.String("500m"), "memory": pulumi.String("1Gi")},
							},
							VolumeMounts: corev1.VolumeMountArray{
								&corev1.VolumeMountArgs{Name: pulumi.String("rabbitmq-etc"), MountPath: pulumi.String("/etc/rabbitmq")},
								&corev1.VolumeMountArgs{Name: pulumi.String("data"), MountPath: pulumi.String("/var/lib/rabbitmq")},
							},
						},
					},
					Volumes: corev1.VolumeArray{
						&corev1.VolumeArgs{Name: pulumi.String("rabbitmq-config"), ConfigMap: &corev1.ConfigMapVolumeSourceArgs{Name: pulumi.String(name + "-config")}},
						&corev1.VolumeArgs{Name: pulumi.String("rabbitmq-etc"), EmptyDir: &corev1.EmptyDirVolumeSourceArgs{}},
					},
				},
			},
			VolumeClaimTemplates: corev1.PersistentVolumeClaimTypeArray{
				&corev1.PersistentVolumeClaimTypeArgs{
					Metadata: &metav1.ObjectMetaArgs{Name: pulumi.String("data"), Annotations: skipAwait},
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
	}, append(k8sOpts, pulumi.DependsOn([]pulumi.Resource{config, credentials, serviceAccount, roleBinding, headless, service}))...)
	if err != nil {
		return nil, fmt.Errorf("create RabbitMQ StatefulSet: %w", err)
	}

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

func rabbitmqConfig(headlessName string) string {
	return strings.Join([]string{
		"cluster_formation.peer_discovery_backend = k8s",
		"cluster_formation.k8s.service_name = " + headlessName,
		"cluster_formation.k8s.address_type = hostname",
		"cluster_formation.k8s.hostname_suffix = ." + headlessName + ".default.svc.cluster.local",
		"cluster_partition_handling = pause_minority",
	}, "\n") + "\n"
}

func rabbitmqServicePorts() corev1.ServicePortArray {
	return corev1.ServicePortArray{
		&corev1.ServicePortArgs{Name: pulumi.String("amqp"), Port: pulumi.Int(AMQPPort), TargetPort: pulumi.Int(AMQPPort)},
		&corev1.ServicePortArgs{Name: pulumi.String("epmd"), Port: pulumi.Int(EPMDPort), TargetPort: pulumi.Int(EPMDPort)},
		&corev1.ServicePortArgs{Name: pulumi.String("inter-node"), Port: pulumi.Int(InterNodePort), TargetPort: pulumi.Int(InterNodePort)},
		&corev1.ServicePortArgs{Name: pulumi.String("management"), Port: pulumi.Int(ManagementPort), TargetPort: pulumi.Int(ManagementPort)},
	}
}

func rabbitmqEnv(credentialsName, headlessName, username string) corev1.EnvVarArray {
	return corev1.EnvVarArray{
		&corev1.EnvVarArgs{Name: pulumi.String("POD_NAME"), ValueFrom: &corev1.EnvVarSourceArgs{FieldRef: &corev1.ObjectFieldSelectorArgs{FieldPath: pulumi.String("metadata.name")}}},
		&corev1.EnvVarArgs{Name: pulumi.String("RABBITMQ_NODENAME"), Value: pulumi.String("rabbit@$(POD_NAME)." + headlessName + ".default.svc.cluster.local")},
		&corev1.EnvVarArgs{Name: pulumi.String("RABBITMQ_USE_LONGNAME"), Value: pulumi.String("true")},
		&corev1.EnvVarArgs{Name: pulumi.String("RABBITMQ_DEFAULT_USER"), Value: pulumi.String(username)},
		&corev1.EnvVarArgs{Name: pulumi.String("RABBITMQ_DEFAULT_PASS"), ValueFrom: secretKeyRef(credentialsName, "password")},
		&corev1.EnvVarArgs{Name: pulumi.String("RABBITMQ_ERLANG_COOKIE"), ValueFrom: secretKeyRef(credentialsName, "erlang-cookie")},
	}
}

func secretKeyRef(name, key string) *corev1.EnvVarSourceArgs {
	return &corev1.EnvVarSourceArgs{SecretKeyRef: &corev1.SecretKeySelectorArgs{Name: pulumi.String(name), Key: pulumi.String(key)}}
}
