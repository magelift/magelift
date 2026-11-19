package stack

import (
	"fmt"
	"strings"

	"github.com/magelift/magelift/internal/cloud/kube"
	"github.com/magelift/magelift/sdk"

	appsv1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/apps/v1"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	"github.com/pulumi/pulumi-random/sdk/v4/go/random"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

func addSingleStateful(ctx *pulumi.Context, resourceName, name string, port, storageGiB int, container *corev1.ContainerArgs, opts []pulumi.ResourceOption) error {
	labels := pulumi.StringMap{"app": pulumi.String(name)}
	_, err := corev1.NewService(ctx, resourceName+"-service", &corev1.ServiceArgs{
		Metadata: &metav1.ObjectMetaArgs{Name: pulumi.String(name)},
		Spec:     &corev1.ServiceSpecArgs{Selector: labels, Ports: corev1.ServicePortArray{&corev1.ServicePortArgs{Port: pulumi.Int(port), TargetPort: pulumi.Int(port)}}},
	}, opts...)
	if err != nil {
		return err
	}
	_, err = appsv1.NewStatefulSet(ctx, resourceName, &appsv1.StatefulSetArgs{
		Metadata: &metav1.ObjectMetaArgs{Name: pulumi.String(name)},
		Spec: &appsv1.StatefulSetSpecArgs{
			ServiceName: pulumi.String(name), Replicas: pulumi.Int(1), Selector: &metav1.LabelSelectorArgs{MatchLabels: labels},
			VolumeClaimTemplates: corev1.PersistentVolumeClaimTypeArray{&corev1.PersistentVolumeClaimTypeArgs{
				Metadata: &metav1.ObjectMetaArgs{Name: pulumi.String("data")},
				Spec:     &corev1.PersistentVolumeClaimSpecArgs{AccessModes: pulumi.StringArray{pulumi.String("ReadWriteOnce")}, Resources: &corev1.VolumeResourceRequirementsArgs{Requests: pulumi.StringMap{"storage": pulumi.String(fmt.Sprintf("%dGi", storageGiB))}}},
			}},
			Template: &corev1.PodTemplateSpecArgs{Metadata: &metav1.ObjectMetaArgs{Labels: labels}, Spec: &corev1.PodSpecArgs{Containers: corev1.ContainerArray{container}}},
		},
	}, opts...)
	return err
}

func addMariaDB(ctx *pulumi.Context, spec Spec, credentials *corev1.Secret, opts []pulumi.ResourceOption) error {
	root, err := random.NewRandomPassword(ctx, "mariadb-root-password", &random.RandomPasswordArgs{Length: pulumi.Int(32), Special: pulumi.Bool(false)})
	if err != nil {
		return err
	}
	rootSecret, err := corev1.NewSecret(ctx, "mariadb-root-secret", &corev1.SecretArgs{Metadata: &metav1.ObjectMetaArgs{Name: pulumi.String("magelift-mariadb-root")}, StringData: pulumi.StringMap{"password": root.Result}}, opts...)
	if err != nil {
		return err
	}
	size := spec.DatabaseStorageGiB
	if size == 0 {
		size = 30
	}
	ref := func(secret, key string) *corev1.EnvVarSourceArgs {
		return &corev1.EnvVarSourceArgs{SecretKeyRef: &corev1.SecretKeySelectorArgs{Name: pulumi.String(secret), Key: pulumi.String(key)}}
	}
	container := &corev1.ContainerArgs{
		Name: pulumi.String("mariadb"), Image: pulumi.String(spec.DatabaseImage), Args: pulumi.ToStringArray(sdk.DatabaseArguments(spec.DatabaseParameters)),
		ReadinessProbe: &corev1.ProbeArgs{TcpSocket: &corev1.TCPSocketActionArgs{Port: pulumi.Int(3306)}, InitialDelaySeconds: pulumi.Int(10), PeriodSeconds: pulumi.Int(5)},
		Env: corev1.EnvVarArray{
			&corev1.EnvVarArgs{Name: pulumi.String("MARIADB_DATABASE"), Value: pulumi.String("magento")},
			&corev1.EnvVarArgs{Name: pulumi.String("MARIADB_USER"), ValueFrom: ref("magento-db-credentials", "username")},
			&corev1.EnvVarArgs{Name: pulumi.String("MARIADB_PASSWORD"), ValueFrom: ref("magento-db-credentials", "password")},
			&corev1.EnvVarArgs{Name: pulumi.String("MARIADB_ROOT_PASSWORD"), ValueFrom: ref("magelift-mariadb-root", "password")},
		},
		Ports:        corev1.ContainerPortArray{&corev1.ContainerPortArgs{ContainerPort: pulumi.Int(3306)}},
		VolumeMounts: corev1.VolumeMountArray{&corev1.VolumeMountArgs{Name: pulumi.String("data"), MountPath: pulumi.String("/var/lib/mysql")}},
	}
	return addSingleStateful(ctx, "mariadb", "magelift-database", 3306, size, container, append(opts, pulumi.DependsOn([]pulumi.Resource{credentials, rootSecret})))
}

func addValkey(ctx *pulumi.Context, name, image, policy string, memoryMiB, storageGiB int, opts []pulumi.ResourceOption) error {
	password, err := random.NewRandomPassword(ctx, name+"-password", &random.RandomPasswordArgs{Length: pulumi.Int(32), Special: pulumi.Bool(false)})
	if err != nil {
		return err
	}
	secret, err := kube.NewCachePasswordSecret(ctx, name, password.Result, opts...)
	if err != nil {
		return err
	}
	if memoryMiB == 0 {
		memoryMiB = 256
	}
	if storageGiB == 0 {
		storageGiB = 10
	}
	if strings.TrimSpace(policy) == "" {
		policy = "noeviction"
	}
	container := &corev1.ContainerArgs{
		Name: pulumi.String("redis"), Image: pulumi.String(image),
		ReadinessProbe: &corev1.ProbeArgs{TcpSocket: &corev1.TCPSocketActionArgs{Port: pulumi.Int(6379)}, PeriodSeconds: pulumi.Int(5)},
		Resources:      &corev1.ResourceRequirementsArgs{Limits: pulumi.StringMap{"memory": pulumi.String(fmt.Sprintf("%dMi", memoryMiB+256))}},
		Env:            corev1.EnvVarArray{&corev1.EnvVarArgs{Name: pulumi.String("STORE_PASSWORD"), ValueFrom: &corev1.EnvVarSourceArgs{SecretKeyRef: &corev1.SecretKeySelectorArgs{Name: secret.Metadata.Name(), Key: pulumi.String("password")}}}},
		Args:           pulumi.StringArray{pulumi.String("--requirepass"), pulumi.String("$(STORE_PASSWORD)"), pulumi.String("--appendonly"), pulumi.String("yes"), pulumi.String("--maxmemory"), pulumi.String(fmt.Sprintf("%dmb", memoryMiB)), pulumi.String("--maxmemory-policy"), pulumi.String(policy)},
		Ports:          corev1.ContainerPortArray{&corev1.ContainerPortArgs{ContainerPort: pulumi.Int(6379)}},
		VolumeMounts:   corev1.VolumeMountArray{&corev1.VolumeMountArgs{Name: pulumi.String("data"), MountPath: pulumi.String("/data")}},
	}
	return addSingleStateful(ctx, name, name, 6379, storageGiB, container, opts)
}
