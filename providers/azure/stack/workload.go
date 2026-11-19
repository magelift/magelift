package stack

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"strings"

	"github.com/magelift/magelift/internal/cloud/kube"
	"github.com/magelift/magelift/internal/cloud/kube/search"
	"github.com/magelift/magelift/internal/platform"
	"github.com/pulumi/pulumi-azure-native-sdk/containerservice/v3"
	"github.com/pulumi/pulumi-azure-native-sdk/dbformysql/v3"
	"github.com/pulumi/pulumi-azure-native-sdk/keyvault/v3"
	"github.com/pulumi/pulumi-azure-native-sdk/storage/v3"
	azurenative "github.com/pulumi/pulumi-azure-native-sdk/v3"
	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	appsv1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/apps/v1"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	networkingv1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/networking/v1"
	"github.com/pulumi/pulumi-random/sdk/v4/go/random"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// addMagentoWorkloads builds the Magento runtime after its dependencies. The
// deployment lifecycle still needs to run Magento setup and health checks.
func addMagentoWorkloads(ctx *pulumi.Context, spec Spec, azure *azurenative.Provider, cluster *containerservice.ManagedCluster, database *dbformysql.Server, account *storage.StorageAccount, share *storage.FileShare, vault *keyvault.Vault, dbPassword pulumi.StringInput) error {
	credentials := containerservice.ListManagedClusterUserCredentialsOutput(ctx, containerservice.ListManagedClusterUserCredentialsOutputArgs{
		ResourceGroupName: pulumi.String(spec.ResourceGroup), ResourceName: cluster.Name,
	}, pulumi.Provider(azure))
	kubeconfig := credentials.Kubeconfigs().ApplyT(func(values []containerservice.CredentialResultResponse) (string, error) {
		if len(values) == 0 {
			return "", errors.New("AKS did not return Kubernetes credentials")
		}
		decoded, err := base64.StdEncoding.DecodeString(values[0].Value)
		if err != nil {
			return "", fmt.Errorf("decode AKS credentials: %w", err)
		}
		return string(decoded), nil
	}).(pulumi.StringOutput)
	kubeconfig = pulumi.ToSecret(kubeconfig).(pulumi.StringOutput)
	namespace := "default"
	volumeName := "magelift-media"
	if spec.PreviewPoolCluster != "" {
		namespace = spec.Namespace
		volumeName = spec.Namespace + "-media"
	}
	provider, err := kubernetes.NewProvider(ctx, "aks-kubernetes", &kubernetes.ProviderArgs{Kubeconfig: kubeconfig, Namespace: pulumi.String(namespace)}, pulumi.DependsOn([]pulumi.Resource{cluster}))
	if err != nil {
		return err
	}
	options := []pulumi.ResourceOption{pulumi.Provider(provider)}
	if spec.PreviewPoolCluster != "" {
		ns, err := corev1.NewNamespace(ctx, "preview-namespace", &corev1.NamespaceArgs{Metadata: &metav1.ObjectMetaArgs{Name: pulumi.String(namespace)}}, options...)
		if err != nil {
			return err
		}
		options = append(options, pulumi.DependsOn([]pulumi.Resource{ns}))
		_, err = networkingv1.NewNetworkPolicy(ctx, "namespace-ingress", &networkingv1.NetworkPolicyArgs{
			Metadata: &metav1.ObjectMetaArgs{Name: pulumi.String("magelift-namespace-ingress")},
			Spec: &networkingv1.NetworkPolicySpecArgs{PodSelector: &metav1.LabelSelectorArgs{}, PolicyTypes: pulumi.StringArray{pulumi.String("Ingress")},
				Ingress: networkingv1.NetworkPolicyIngressRuleArray{&networkingv1.NetworkPolicyIngressRuleArgs{From: networkingv1.NetworkPolicyPeerArray{&networkingv1.NetworkPolicyPeerArgs{PodSelector: &metav1.LabelSelectorArgs{}}}}}},
		}, options...)
		if err != nil {
			return err
		}
		_, err = networkingv1.NewNetworkPolicy(ctx, "web-ingress", &networkingv1.NetworkPolicyArgs{
			Metadata: &metav1.ObjectMetaArgs{Name: pulumi.String("magelift-web-ingress")},
			Spec: &networkingv1.NetworkPolicySpecArgs{PodSelector: &metav1.LabelSelectorArgs{MatchLabels: pulumi.StringMap{"app": pulumi.String("magelift-web")}}, PolicyTypes: pulumi.StringArray{pulumi.String("Ingress")},
				Ingress: networkingv1.NetworkPolicyIngressRuleArray{&networkingv1.NetworkPolicyIngressRuleArgs{Ports: networkingv1.NetworkPolicyPortArray{&networkingv1.NetworkPolicyPortArgs{Port: pulumi.Int(8080)}}}}},
		}, options...)
		if err != nil {
			return err
		}
	}

	var encryptionKey pulumi.StringOutput
	if spec.EncryptionKey == nil {
		generated, err := random.NewRandomPassword(ctx, "magento-encryption-key", &random.RandomPasswordArgs{Length: pulumi.Int(32), Special: pulumi.Bool(false), MinUpper: pulumi.Int(1), MinLower: pulumi.Int(1), MinNumeric: pulumi.Int(1)})
		if err != nil {
			return err
		}
		encryptionKey = generated.Result
	} else {
		encryptionKey = pulumi.ToSecret(spec.EncryptionKey).(pulumi.StringOutput)
	}
	_, err = keyvault.NewSecret(ctx, "magento-encryption-key-secret", &keyvault.SecretArgs{
		ResourceGroupName: pulumi.String(spec.ResourceGroup), VaultName: vault.Name, SecretName: pulumi.String("magento-encryption-key"),
		Properties: &keyvault.SecretPropertiesArgs{Value: encryptionKey.ToStringPtrOutput()},
	}, pulumi.Provider(azure))
	if err != nil {
		return err
	}
	keySecret, err := kube.NewMagentoEncryptionKeySecret(ctx, "magento", encryptionKey, options...)
	if err != nil {
		return err
	}
	dbSecret, err := kube.NewDatabaseCredentialsSecret(ctx, "magento", spec.MySQLUsername, dbPassword, options...)
	if err != nil {
		return err
	}

	keys := storage.ListStorageAccountKeysOutput(ctx, storage.ListStorageAccountKeysOutputArgs{ResourceGroupName: pulumi.String(spec.ResourceGroup), AccountName: account.Name}, pulumi.Provider(azure))
	storageKey := pulumi.ToSecret(keys.Keys().Index(pulumi.Int(0)).Value()).(pulumi.StringOutput)
	fileSecret, err := corev1.NewSecret(ctx, "azure-file-key", &corev1.SecretArgs{
		Metadata:   &metav1.ObjectMetaArgs{Name: pulumi.String("magelift-media-key")},
		StringData: pulumi.StringMap{"azurestorageaccountname": account.Name, "azurestorageaccountkey": storageKey},
	}, options...)
	if err != nil {
		return err
	}
	volume, err := corev1.NewPersistentVolume(ctx, "azure-media-volume", &corev1.PersistentVolumeArgs{
		Metadata: &metav1.ObjectMetaArgs{Name: pulumi.String(volumeName)},
		Spec: &corev1.PersistentVolumeSpecArgs{
			AccessModes: pulumi.StringArray{pulumi.String("ReadWriteMany")}, Capacity: pulumi.StringMap{"storage": pulumi.String("100Gi")},
			PersistentVolumeReclaimPolicy: pulumi.String("Retain"), StorageClassName: pulumi.String(""),
			Csi: &corev1.CSIPersistentVolumeSourceArgs{
				Driver: pulumi.String("file.csi.azure.com"), VolumeHandle: pulumi.Sprintf("%s#%s", account.Name, share.Name),
				VolumeAttributes:   pulumi.StringMap{"resourceGroup": pulumi.String(spec.ResourceGroup), "storageAccount": account.Name, "shareName": share.Name},
				NodeStageSecretRef: &corev1.SecretReferenceArgs{Name: pulumi.String("magelift-media-key"), Namespace: pulumi.String(namespace)},
			},
		},
	}, append(options, pulumi.DependsOn([]pulumi.Resource{share, fileSecret}))...)
	if err != nil {
		return err
	}
	claim, err := corev1.NewPersistentVolumeClaim(ctx, "azure-media-claim", &corev1.PersistentVolumeClaimArgs{
		Metadata: &metav1.ObjectMetaArgs{Name: pulumi.String("magelift-media")},
		Spec: &corev1.PersistentVolumeClaimSpecArgs{
			AccessModes: pulumi.StringArray{pulumi.String("ReadWriteMany")}, StorageClassName: pulumi.String(""), VolumeName: pulumi.String(volumeName),
			Resources: &corev1.VolumeResourceRequirementsArgs{Requests: pulumi.StringMap{"storage": pulumi.String("100Gi")}},
		},
	}, append(options, pulumi.DependsOn([]pulumi.Resource{volume}))...)
	if err != nil {
		return err
	}

	searchReplicas := 1
	if spec.HighAvailability {
		searchReplicas = 3
	}
	searchComponent, err := search.New(ctx, "magelift-search", search.Args{Replicas: searchReplicas, Image: spec.SearchImage, K8sProvider: provider, TypeToken: "magelift:azure:OpenSearch"}, options...)
	if err != nil {
		return err
	}
	cacheEndpoint := spec.CacheEndpoint
	if cacheEndpoint == "" {
		cacheEndpoint = "magelift-cache"
		labels := pulumi.StringMap{"app": pulumi.String(cacheEndpoint)}
		_, err = appsv1.NewDeployment(ctx, "cache", &appsv1.DeploymentArgs{
			Metadata: &metav1.ObjectMetaArgs{Name: pulumi.String(cacheEndpoint)},
			Spec: &appsv1.DeploymentSpecArgs{Replicas: pulumi.Int(1), Selector: &metav1.LabelSelectorArgs{MatchLabels: labels}, Template: &corev1.PodTemplateSpecArgs{
				Metadata: &metav1.ObjectMetaArgs{Labels: labels}, Spec: &corev1.PodSpecArgs{Containers: corev1.ContainerArray{&corev1.ContainerArgs{Name: pulumi.String("cache"), Image: pulumi.String(spec.CacheImage), Ports: corev1.ContainerPortArray{&corev1.ContainerPortArgs{ContainerPort: pulumi.Int(6379)}}}}},
			}},
		}, options...)
		if err != nil {
			return err
		}
		_, err = corev1.NewService(ctx, "cache-service", &corev1.ServiceArgs{Metadata: &metav1.ObjectMetaArgs{Name: pulumi.String(cacheEndpoint)}, Spec: &corev1.ServiceSpecArgs{Selector: labels, Ports: corev1.ServicePortArray{&corev1.ServicePortArgs{Port: pulumi.Int(6379)}}}}, options...)
		if err != nil {
			return err
		}
	}
	webRuntime := spec.WebRuntime
	if webRuntime == "" {
		webRuntime = "nginx-fpm"
	}
	mode := spec.ApplicationMode
	if mode == "" {
		mode = "integrated"
	}
	env := pulumi.All(database.FullyQualifiedDomainName, searchComponent.Endpoint).ApplyT(func(values []any) []corev1.EnvVar {
		return kube.EnvVars(platform.CoreEnvBindings(platform.CapabilityEndpoints{
			ApplicationMode: mode, ApplicationVersion: spec.MagentoVersion, WebRuntime: webRuntime,
			DatabaseWriter: values[0].(string), DatabaseName: "magento", CacheEndpoint: cacheEndpoint,
			SessionEndpoint: cacheEndpoint, SearchEndpoint: values[1].(string), QueueMode: "database", Magento: spec.Magento,
		}))
	}).(corev1.EnvVarArrayOutput)
	env = kube.AppendEncryptionKeyEnv(env, kube.EncryptionKeySecretName("magento"))
	env = kube.AppendDatabaseCredentialEnv(env, kube.DatabaseCredentialsSecretName("magento"))
	env = env.ApplyT(func(values []corev1.EnvVar) []corev1.EnvVar {
		return append(values, corev1.EnvVar{Name: "MAGELIFT_DATABASE_TLS_CA", Value: pulumi.StringRef("/etc/ssl/certs/ca-certificates.crt")})
	}).(corev1.EnvVarArrayOutput)
	containers, err := kube.WebRuntimeContainers(webRuntime, spec.ApplicationImage, env, "500m", "2Gi")
	if err != nil {
		return err
	}
	for _, input := range containers {
		container := input.(*corev1.ContainerArgs)
		container.VolumeMounts = corev1.VolumeMountArray{&corev1.VolumeMountArgs{Name: pulumi.String("media"), MountPath: pulumi.String("/app/pub/media")}}
	}
	pod := &corev1.PodSpecArgs{
		Containers: containers,
		Volumes:    corev1.VolumeArray{&corev1.VolumeArgs{Name: pulumi.String("media"), PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSourceArgs{ClaimName: pulumi.String("magelift-media")}}},
	}
	if strings.TrimSpace(spec.ImagePullSecret) != "" {
		pod.ImagePullSecrets = corev1.LocalObjectReferenceArray{&corev1.LocalObjectReferenceArgs{Name: pulumi.String(spec.ImagePullSecret)}}
	}
	labels := pulumi.StringMap{"app": pulumi.String("magelift-web")}
	replicas := 1
	if spec.HighAvailability {
		replicas = 3
		pod.TopologySpreadConstraints = corev1.TopologySpreadConstraintArray{&corev1.TopologySpreadConstraintArgs{
			MaxSkew: pulumi.Int(1), TopologyKey: pulumi.String("topology.kubernetes.io/zone"),
			WhenUnsatisfiable: pulumi.String("DoNotSchedule"), LabelSelector: &metav1.LabelSelectorArgs{MatchLabels: labels},
		}}
	}
	web, err := appsv1.NewDeployment(ctx, "magento-web", &appsv1.DeploymentArgs{
		Metadata: &metav1.ObjectMetaArgs{Name: pulumi.String("magelift-web"), Annotations: kube.SkipAwaitAnnotations()},
		Spec: &appsv1.DeploymentSpecArgs{Replicas: pulumi.Int(replicas), Selector: &metav1.LabelSelectorArgs{MatchLabels: labels},
			Template: &corev1.PodTemplateSpecArgs{Metadata: &metav1.ObjectMetaArgs{Labels: labels}, Spec: pod}},
	}, append(options, pulumi.DependsOn([]pulumi.Resource{dbSecret, keySecret, claim}))...)
	if err != nil {
		return err
	}
	cronLabels := pulumi.StringMap{"app": pulumi.String("magelift-cron")}
	cronPod := &corev1.PodSpecArgs{
		Containers: corev1.ContainerArray{&corev1.ContainerArgs{
			Name: pulumi.String("cron"), Image: pulumi.String(spec.ApplicationImage),
			Command: kube.ToStringArray(platform.MagentoCronShell()), Env: env,
			VolumeMounts: corev1.VolumeMountArray{&corev1.VolumeMountArgs{Name: pulumi.String("media"), MountPath: pulumi.String("/app/pub/media")}},
		}},
		Volumes:          corev1.VolumeArray{&corev1.VolumeArgs{Name: pulumi.String("media"), PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSourceArgs{ClaimName: pulumi.String("magelift-media")}}},
		ImagePullSecrets: pod.ImagePullSecrets,
	}
	_, err = appsv1.NewDeployment(ctx, "magento-cron", &appsv1.DeploymentArgs{
		Metadata: &metav1.ObjectMetaArgs{Name: pulumi.String("magelift-cron"), Annotations: kube.SkipAwaitAnnotations()},
		Spec: &appsv1.DeploymentSpecArgs{Replicas: pulumi.Int(1), Selector: &metav1.LabelSelectorArgs{MatchLabels: cronLabels},
			Template: &corev1.PodTemplateSpecArgs{Metadata: &metav1.ObjectMetaArgs{Labels: cronLabels}, Spec: cronPod}},
	}, append(options, pulumi.DependsOn([]pulumi.Resource{dbSecret, keySecret, claim}))...)
	if err != nil {
		return err
	}
	serviceType := "LoadBalancer"
	if spec.IngressClass != "" {
		serviceType = "ClusterIP"
	}
	service, err := corev1.NewService(ctx, "magento-web-service", &corev1.ServiceArgs{
		Metadata: &metav1.ObjectMetaArgs{Name: pulumi.String("magelift-web")},
		Spec:     &corev1.ServiceSpecArgs{Type: pulumi.String(serviceType), Selector: labels, Ports: corev1.ServicePortArray{&corev1.ServicePortArgs{Port: pulumi.Int(80), TargetPort: pulumi.Int(8080)}}},
	}, append(options, pulumi.DependsOn([]pulumi.Resource{web}))...)
	if err != nil {
		return err
	}
	ctx.Export("kubeconfig", kubeconfig)
	ctx.Export("clusterName", cluster.Name)
	ctx.Export("serviceName", pulumi.String("magelift-web"))
	ctx.Export("namespace", pulumi.String(namespace))
	ctx.Export("imagePullSecret", pulumi.String(spec.ImagePullSecret))
	ctx.Export("mediaClaim", pulumi.String("magelift-media"))
	ctx.Export("cacheEndpoint", pulumi.String(cacheEndpoint))
	ctx.Export("queueMode", pulumi.String("database"))
	applicationURL := service.Status.ApplyT(func(status *corev1.ServiceStatus) string {
		if status == nil || status.LoadBalancer == nil {
			return ""
		}
		for _, ingress := range status.LoadBalancer.Ingress {
			host := ""
			if ingress.Ip != nil {
				host = strings.TrimSpace(*ingress.Ip)
			} else if ingress.Hostname != nil {
				host = strings.TrimSpace(*ingress.Hostname)
			}
			if host != "" {
				if ip := net.ParseIP(host); ip != nil && strings.Contains(host, ":") {
					host = "[" + host + "]"
				}
				return "http://" + host + "/"
			}
		}
		return ""
	}).(pulumi.StringOutput)
	if spec.IngressClass != "" {
		rule := &networkingv1.IngressRuleArgs{Http: &networkingv1.HTTPIngressRuleValueArgs{Paths: networkingv1.HTTPIngressPathArray{&networkingv1.HTTPIngressPathArgs{
			Path: pulumi.String("/"), PathType: pulumi.String("Prefix"), Backend: &networkingv1.IngressBackendArgs{Service: &networkingv1.IngressServiceBackendArgs{Name: pulumi.String("magelift-web"), Port: &networkingv1.ServiceBackendPortArgs{Number: pulumi.Int(80)}}},
		}}}}
		if spec.Domain != "" {
			rule.Host = pulumi.String(spec.Domain)
		}
		ingressSpec := &networkingv1.IngressSpecArgs{IngressClassName: pulumi.String(spec.IngressClass), Rules: networkingv1.IngressRuleArray{rule}}
		if spec.TLSSecret != "" {
			ingressSpec.Tls = networkingv1.IngressTLSArray{&networkingv1.IngressTLSArgs{Hosts: pulumi.StringArray{pulumi.String(spec.Domain)}, SecretName: pulumi.String(spec.TLSSecret)}}
		}
		ingress, err := networkingv1.NewIngress(ctx, "magento-ingress", &networkingv1.IngressArgs{Metadata: &metav1.ObjectMetaArgs{Name: pulumi.String("magelift-web")}, Spec: ingressSpec}, append(options, pulumi.DependsOn([]pulumi.Resource{service}))...)
		if err != nil {
			return err
		}
		if spec.Domain != "" {
			scheme := "http://"
			if spec.TLSSecret != "" {
				scheme = "https://"
			}
			applicationURL = pulumi.String(scheme + spec.Domain + "/").ToStringOutput()
		} else {
			applicationURL = ingress.Status.ApplyT(func(status *networkingv1.IngressStatus) string {
				if status == nil || status.LoadBalancer == nil {
					return ""
				}
				for _, item := range status.LoadBalancer.Ingress {
					host := ""
					if item.Ip != nil {
						host = strings.TrimSpace(*item.Ip)
					} else if item.Hostname != nil {
						host = strings.TrimSpace(*item.Hostname)
					}
					if host != "" {
						if ip := net.ParseIP(host); ip != nil && strings.Contains(host, ":") {
							host = "[" + host + "]"
						}
						return "http://" + host + "/"
					}
				}
				return ""
			}).(pulumi.StringOutput)
		}
	}
	ctx.Export("applicationURL", applicationURL)
	ctx.Export("databaseSecretName", pulumi.String(kube.DatabaseCredentialsSecretName("magento")))
	ctx.Export("databaseTLSCA", pulumi.String("/etc/ssl/certs/ca-certificates.crt"))
	ctx.Export("encryptionKeySecretName", pulumi.String(kube.EncryptionKeySecretName("magento")))
	ctx.Export("searchEndpoint", searchComponent.Endpoint)
	return nil
}
