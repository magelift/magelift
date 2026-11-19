package runtime

import (
	"fmt"

	"github.com/magelift/magelift/sdk"

	appsv1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/apps/v1"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// newStatefulService creates one persistent instance; replication and backups
// remain operator responsibilities. Stable PVCs are retained after deletion.
func newStatefulService(ctx *pulumi.Context, name, image string, port int, storageGiB, memoryMiB int, retain bool, command []string, env corev1.EnvVarArray, opts ...pulumi.ResourceOption) (pulumi.Resource, error) {
	if image == "" {
		return nil, fmt.Errorf("%s requires an explicit container image", name)
	}
	if storageGiB == 0 {
		storageGiB = 20
	}
	if storageGiB < 1 {
		return nil, fmt.Errorf("%s storage must be positive", name)
	}
	labels := pulumi.StringMap{"app": pulumi.String(name), "magelift.io/capability": pulumi.String("service")}
	service, err := corev1.NewService(ctx, name+"-svc", &corev1.ServiceArgs{Metadata: &metav1.ObjectMetaArgs{Name: pulumi.String(name)}, Spec: &corev1.ServiceSpecArgs{Selector: labels, Ports: corev1.ServicePortArray{&corev1.ServicePortArgs{Port: pulumi.Int(port)}}}}, opts...)
	if err != nil {
		return nil, err
	}
	container := &corev1.ContainerArgs{Name: pulumi.String("service"), Image: pulumi.String(image), Env: env, Ports: corev1.ContainerPortArray{&corev1.ContainerPortArgs{ContainerPort: pulumi.Int(port)}}, VolumeMounts: corev1.VolumeMountArray{&corev1.VolumeMountArgs{Name: pulumi.String("data"), MountPath: pulumi.String("/var/lib/mysql")}}, Resources: &corev1.ResourceRequirementsArgs{Requests: pulumi.StringMap{"cpu": pulumi.String("500m"), "memory": pulumi.String("1Gi")}}}
	probe := func() *corev1.ProbeArgs {
		return &corev1.ProbeArgs{TcpSocket: &corev1.TCPSocketActionArgs{Port: pulumi.Int(port)}, PeriodSeconds: pulumi.Int(5), FailureThreshold: pulumi.Int(60)}
	}
	container.StartupProbe = probe()
	container.ReadinessProbe = probe()
	if port == 6379 {
		container.SecurityContext = &corev1.SecurityContextArgs{RunAsUser: pulumi.Int(999), RunAsGroup: pulumi.Int(1000), AllowPrivilegeEscalation: pulumi.Bool(false)}
		container.VolumeMounts = corev1.VolumeMountArray{&corev1.VolumeMountArgs{Name: pulumi.String("data"), MountPath: pulumi.String("/data")}}
	}
	if port == 6379 && memoryMiB > 0 {
		memory := pulumi.String(fmt.Sprintf("%dMi", memoryMiB+512))
		container.Resources = &corev1.ResourceRequirementsArgs{Requests: pulumi.StringMap{"cpu": pulumi.String("500m"), "memory": memory}, Limits: pulumi.StringMap{"memory": memory}}
	}
	if len(command) > 0 {
		values := pulumi.StringArray{}
		for _, v := range command {
			values = append(values, pulumi.String(v))
		}
		if port == 3306 {
			container.Args = values
		} else {
			container.Command = values
		}
	}
	retention := "Delete"
	if retain {
		retention = "Retain"
	}

	args := &appsv1.StatefulSetArgs{
		Metadata: &metav1.ObjectMetaArgs{Name: pulumi.String(name)},
		Spec: &appsv1.StatefulSetSpecArgs{
			Replicas: pulumi.Int(1),
			PersistentVolumeClaimRetentionPolicy: &appsv1.StatefulSetPersistentVolumeClaimRetentionPolicyArgs{
				WhenDeleted: pulumi.String(retention), WhenScaled: pulumi.String("Retain"),
			},
			ServiceName: pulumi.String(name),
			Selector:    &metav1.LabelSelectorArgs{MatchLabels: labels},
			Template: &corev1.PodTemplateSpecArgs{
				Metadata: &metav1.ObjectMetaArgs{Labels: labels},
				Spec:     &corev1.PodSpecArgs{Containers: corev1.ContainerArray{container}, SecurityContext: &corev1.PodSecurityContextArgs{FsGroup: pulumi.Int(1000)}},
			},
			VolumeClaimTemplates: corev1.PersistentVolumeClaimTypeArray{
				&corev1.PersistentVolumeClaimTypeArgs{
					Metadata: &metav1.ObjectMetaArgs{Name: pulumi.String("data")},
					Spec: &corev1.PersistentVolumeClaimSpecArgs{
						AccessModes: pulumi.StringArray{pulumi.String("ReadWriteOnce")},
						Resources:   &corev1.VolumeResourceRequirementsArgs{Requests: pulumi.StringMap{"storage": pulumi.String(fmt.Sprintf("%dGi", storageGiB))}},
					},
				},
			},
		},
	}
	return appsv1.NewStatefulSet(ctx, name, args, append(opts, pulumi.DependsOn([]pulumi.Resource{service}))...)
}

func secretEnv(name, secret, key string) *corev1.EnvVarArgs {
	return &corev1.EnvVarArgs{Name: pulumi.String(name), ValueFrom: &corev1.EnvVarSourceArgs{SecretKeyRef: &corev1.SecretKeySelectorArgs{Name: pulumi.String(secret), Key: pulumi.String(key)}}}
}

func serviceImage(binding sdk.ServiceBinding) string {
	if binding.Image != "" {
		return binding.Image
	}
	if binding.Version == "" {
		return ""
	}
	if binding.Family == "mariadb" {
		return "mariadb:" + binding.Version
	}
	if binding.Family == "redis" {
		return "redis:" + binding.Version
	}
	if binding.Family == "valkey" {
		return "valkey/valkey:" + binding.Version
	}
	return ""
}
