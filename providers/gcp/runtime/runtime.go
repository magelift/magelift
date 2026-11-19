package runtime

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/magelift/magelift/internal/cloud/kube"
	"github.com/magelift/magelift/internal/cloud/kube/queue"
	"github.com/magelift/magelift/internal/cloud/kube/search"
	"github.com/magelift/magelift/internal/platform"
	"github.com/magelift/magelift/providers/gcp/naming"
	"github.com/magelift/magelift/sdk"
	"github.com/pulumi/pulumi-gcp/sdk/v10/go/gcp/container"
	"github.com/pulumi/pulumi-gcp/sdk/v10/go/gcp/organizations"
	"github.com/pulumi/pulumi-gcp/sdk/v10/go/gcp/serviceaccount"
	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	appsv1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/apps/v1"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	networkingv1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/networking/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

const (
	TypeToken                       = "magelift:gcp:GKERuntime"
	ApplicationPort                 = 8080
	DefaultApplicationMemoryRequest = kube.DefaultApplicationMemoryRequest
	// mediaS3Endpoint is the GCS S3-interop endpoint Magento's AwsS3
	// driver talks to with the media HMAC credentials.
	mediaS3Endpoint = "https://storage.googleapis.com"
	// mediaDriver selects Magento remote storage. GCP always uses the
	// AwsS3 driver against GCS interop; runtimes without object storage
	// omit the binding and keep local media via the template default.
	mediaDriver = "aws-s3"
)

type Args struct {
	ExistingCluster                               string
	Namespace                                     string
	IngressCIDRs                                  []string
	Project                                       string
	MagentoProject                                string
	Environment                                   string
	Region                                        string
	Runtime                                       string
	Zones                                         []string
	KubernetesVersion                             string
	ReleaseChannel                                string
	ClusterIPv4CIDR                               string
	ServicesIPv4CIDR                              string
	StandardNodeType                              string
	StandardNodeCount                             int
	StandardNodeMinCount                          int
	StandardNodeMaxCount                          int
	StandardNodeDiskType                          string
	StandardNodeDiskSizeGiB                       int
	StandardNodeImageType                         string
	StandardNodeSpot                              bool
	NetworkSelfLink                               pulumi.StringInput
	PrivateSubnetNames                            pulumi.StringArrayInput
	Image                                         string
	ApplicationMode                               string
	ApplicationVersion                            string
	WebRuntime                                    string
	DatabaseWriter                                pulumi.StringInput
	DatabaseName                                  string
	DatabaseUsername                              string
	DatabasePassword                              pulumi.StringInput
	DatabaseAdminUsername                         string
	DatabaseAdminPassword                         pulumi.StringInput
	DatabaseUsersReady                            []pulumi.Resource
	ExternalDatabase                              bool
	EncryptionKey                                 pulumi.StringInput
	ExternalCachePassword                         pulumi.StringInput
	ExternalSessionPassword                       pulumi.StringInput
	DatabaseService, CacheService, SessionService sdk.ServiceBinding
	SearchPlugins                                 []string
	SearchStorageGiB, QueueStorageGiB             int
	RetainServiceVolumes                          bool
	CacheEndpoint                                 pulumi.StringInput
	SessionEndpoint                               pulumi.StringInput
	ExternalSearchEndpoint                        string
	SearchMode                                    string
	SearchReplicas                                int
	SearchImage                                   string
	ExternalQueueHost                             string
	ExternalQueuePort                             string
	ExternalQueueSSL                              string
	ExternalQueueUsername                         string
	ExternalQueuePassword                         pulumi.StringInput
	QueueMode                                     string
	QueueReplicas                                 int
	QueueImage                                    string
	MediaBucket                                   pulumi.StringInput
	MediaHmacAccessID                             pulumi.StringInput
	MediaHmacSecret                               pulumi.StringInput
	MediaS3Prefix                                 string
	CPURequest                                    string
	MemoryRequest                                 string
	DesiredWebReplicas                            int
	QueueConsumerCount                            int
	NativeObservability                           bool
	NativeEdge                                    bool
	NativeEdgeBackendConfigName                   pulumi.StringInput
	Labels                                        map[string]string
	Magento                                       platform.MagentoOverlays
	SmtpHost                                      string
	SmtpPort                                      int
	SmtpUsername                                  string
	SmtpFrom                                      string
	SmtpPassword                                  pulumi.StringInput
}

type Component struct {
	pulumi.ResourceState
	ClusterName               pulumi.StringOutput
	ServiceName               pulumi.StringOutput
	ApplicationURL            pulumi.StringOutput
	ClusterEndpoint           pulumi.StringOutput
	SearchEndpoint            pulumi.StringOutput
	QueueHost                 pulumi.StringOutput
	Namespace                 pulumi.StringOutput
	WorkloadServiceAccount    pulumi.StringOutput
	QueueMode                 pulumi.StringOutput
	QueueUsername             pulumi.StringOutput
	QueuePort                 pulumi.StringOutput
	QueueSSL                  pulumi.StringOutput
	QueuePasswordSecretName   pulumi.StringOutput
	CachePasswordSecretName   pulumi.StringOutput
	SessionEndpoint           pulumi.StringOutput
	SessionPasswordSecretName pulumi.StringOutput
	DatabaseSecretName        pulumi.StringOutput
	EncryptionKeySecretName   pulumi.StringOutput
	Kubeconfig                pulumi.StringOutput
	KubernetesProvider        *kubernetes.Provider
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
	if strings.TrimSpace(args.MagentoProject) == "" || strings.TrimSpace(args.Environment) == "" {
		return nil, errors.New("Magento project and environment are required for GKE cluster naming")
	}
	if args.Runtime == "" {
		args.Runtime = "gke-autopilot"
	}
	if args.Runtime != "gke-autopilot" && args.Runtime != "gke-standard" {
		return nil, fmt.Errorf("unsupported GCP GKE runtime %q", args.Runtime)
	}
	if args.Runtime == "gke-standard" && len(args.Zones) == 0 {
		return nil, errors.New("GKE Standard runtime requires at least one node zone")
	}
	if strings.TrimSpace(args.ReleaseChannel) == "" {
		args.ReleaseChannel = "REGULAR"
	}
	if args.ReleaseChannel != "RAPID" && args.ReleaseChannel != "REGULAR" && args.ReleaseChannel != "STABLE" {
		return nil, fmt.Errorf("unsupported GKE release channel %q", args.ReleaseChannel)
	}
	if args.Runtime == "gke-standard" {
		if strings.TrimSpace(args.StandardNodeType) == "" {
			args.StandardNodeType = "e2-standard-4"
		}
		if args.StandardNodeCount < 1 {
			args.StandardNodeCount = 1
		}
		if args.StandardNodeMinCount < 1 {
			args.StandardNodeMinCount = args.StandardNodeCount
		}
		if args.StandardNodeMaxCount < args.StandardNodeCount {
			args.StandardNodeMaxCount = args.StandardNodeCount
		}
		if args.StandardNodeMinCount > args.StandardNodeCount || args.StandardNodeCount > args.StandardNodeMaxCount {
			return nil, errors.New("GKE Standard node count must be between the configured minimum and maximum")
		}
		if strings.TrimSpace(args.StandardNodeDiskType) == "" {
			args.StandardNodeDiskType = "pd-balanced"
		}
		if args.StandardNodeDiskSizeGiB < 10 {
			args.StandardNodeDiskSizeGiB = 50
		}
		if strings.TrimSpace(args.StandardNodeImageType) == "" {
			args.StandardNodeImageType = "COS_CONTAINERD"
		}
	}
	if strings.TrimSpace(args.DatabaseUsername) == "" || args.DatabasePassword == nil ||
		(!args.ExternalDatabase && (strings.TrimSpace(args.DatabaseAdminUsername) == "" || args.DatabaseAdminPassword == nil)) {
		return nil, errors.New("database username, password, admin username, and admin password are required")
	}
	if args.EncryptionKey == nil {
		return nil, errors.New("Magento encryption key is required")
	}
	if args.NativeEdge && args.NativeEdgeBackendConfigName == nil {
		return nil, errors.New("native GCP edge requires a BackendConfig name")
	}
	if args.DesiredWebReplicas < 1 {
		args.DesiredWebReplicas = 1
	}
	if args.CPURequest == "" {
		args.CPURequest = "500m"
	}
	if args.MemoryRequest == "" {
		args.MemoryRequest = DefaultApplicationMemoryRequest
	}
	if args.QueueMode == "" {
		args.QueueMode = "database"
	}
	component := &Component{}
	if err := ctx.RegisterComponentResourceV2(TypeToken, name, pulumi.Map{
		"project": pulumi.String(args.Project),
	}, component, opts...); err != nil {
		return nil, err
	}
	parent := pulumi.Parent(component)
	skipAwait := kube.SkipAwaitAnnotations()

	clusterName := naming.ClusterNameForRuntime(args.MagentoProject, args.Environment, args.Runtime)
	nodeServiceAccount := naming.NodeServiceAccountEmail(args.Project, args.MagentoProject, args.Environment)
	nodeScopes := pulumi.StringArray{pulumi.String("https://www.googleapis.com/auth/cloud-platform"), pulumi.String("https://www.googleapis.com/auth/userinfo.email")}
	ipAllocationPolicy := &container.ClusterIpAllocationPolicyArgs{
		ClusterIpv4CidrBlock:  pulumi.String("/17"),
		ServicesIpv4CidrBlock: pulumi.String("/22"),
	}
	if strings.TrimSpace(args.ClusterIPv4CIDR) != "" {
		ipAllocationPolicy.ClusterIpv4CidrBlock = pulumi.String(args.ClusterIPv4CIDR)
	}
	if strings.TrimSpace(args.ServicesIPv4CIDR) != "" {
		ipAllocationPolicy.ServicesIpv4CidrBlock = pulumi.String(args.ServicesIPv4CIDR)
	}
	clusterArgs := &container.ClusterArgs{
		Project:            pulumi.String(args.Project),
		Name:               pulumi.String(clusterName),
		Location:           pulumi.String(args.Region),
		Network:            args.NetworkSelfLink,
		Subnetwork:         args.PrivateSubnetNames.ToStringArrayOutput().Index(pulumi.Int(0)),
		IpAllocationPolicy: ipAllocationPolicy,
		ReleaseChannel: &container.ClusterReleaseChannelArgs{
			Channel: pulumi.String("REGULAR"),
		},
		DeletionProtection: pulumi.Bool(false),
		ResourceLabels:     pulumi.ToStringMap(args.Labels),
	}
	if strings.TrimSpace(args.KubernetesVersion) != "" {
		clusterArgs.MinMasterVersion = pulumi.String(args.KubernetesVersion)
	}
	clusterArgs.ReleaseChannel = &container.ClusterReleaseChannelArgs{Channel: pulumi.String(args.ReleaseChannel)}
	if args.NativeObservability {
		clusterArgs.LoggingService = pulumi.String("logging.googleapis.com/kubernetes")
		clusterArgs.LoggingConfig = &container.ClusterLoggingConfigArgs{EnableComponents: pulumi.StringArray{pulumi.String("SYSTEM_COMPONENTS"), pulumi.String("WORKLOADS")}}
		clusterArgs.MonitoringService = pulumi.String("monitoring.googleapis.com/kubernetes")
		clusterArgs.MonitoringConfig = &container.ClusterMonitoringConfigArgs{EnableComponents: pulumi.StringArray{pulumi.String("SYSTEM_COMPONENTS"), pulumi.String("POD"), pulumi.String("DEPLOYMENT"), pulumi.String("STATEFULSET"), pulumi.String("DAEMONSET")}}
	}
	if args.Runtime == "gke-standard" {
		clusterArgs.InitialNodeCount = pulumi.Int(args.StandardNodeCount)
		clusterArgs.RemoveDefaultNodePool = pulumi.Bool(true)
		clusterArgs.NodeConfig = &container.ClusterNodeConfigArgs{
			ServiceAccount: pulumi.String(nodeServiceAccount),
			OauthScopes:    nodeScopes,
		}
	} else {
		clusterArgs.EnableAutopilot = pulumi.Bool(true)
		// Autopilot node VMs use cluster_autoscaling.auto_provisioning_defaults,
		// not node_config alone (GKE docs, 2026-10-13).
		clusterArgs.ClusterAutoscaling = &container.ClusterClusterAutoscalingArgs{
			AutoProvisioningDefaults: &container.ClusterClusterAutoscalingAutoProvisioningDefaultsArgs{
				ServiceAccount: pulumi.String(nodeServiceAccount),
				OauthScopes:    nodeScopes,
			},
		}
	}
	clusterResources := make([]pulumi.Resource, 0, 2)
	// Node identities are immutable. Configure new environments without
	// replacing legacy clusters or pools when their code is updated.
	var nodeIdentityIgnoreChanges []string
	if args.Runtime == "gke-autopilot" {
		nodeIdentityIgnoreChanges = []string{
			"clusterAutoscaling.autoProvisioningDefaults.serviceAccount",
			"clusterAutoscaling.autoProvisioningDefaults.oauthScopes",
		}
	} else {
		nodeIdentityIgnoreChanges = []string{"nodeConfig.serviceAccount", "nodeConfig.oauthScopes"}
	}
	nodeIdentityCreationOnly := pulumi.IgnoreChanges(nodeIdentityIgnoreChanges)
	var cluster *container.Cluster
	var err error
	if args.ExistingCluster != "" {
		clusterID := args.ExistingCluster
		if !strings.HasPrefix(clusterID, "projects/") {
			clusterID = "projects/" + args.Project + "/locations/" + args.Region + "/clusters/" + clusterID
		}
		cluster, err = container.GetCluster(ctx, name+"-existing-cluster", pulumi.ID(clusterID), nil, parent)
		if err != nil {
			return nil, fmt.Errorf("read shared GKE cluster: %w", err)
		}
		clusterResources = append(clusterResources, cluster)
	} else {
		cluster, err = container.NewCluster(ctx, name+"-cluster", clusterArgs, parent, nodeIdentityCreationOnly)
		if err != nil {
			return nil, fmt.Errorf("create GKE %s cluster: %w", args.Runtime, err)
		}
		clusterResources = append(clusterResources, cluster)
		if args.Runtime == "gke-standard" {
			nodePool, err := container.NewNodePool(ctx, name+"-standard-nodes", &container.NodePoolArgs{
				Project:          pulumi.String(args.Project),
				Name:             pulumi.String(naming.NodePoolName(args.MagentoProject, args.Environment)),
				Cluster:          cluster.Name,
				Location:         pulumi.String(args.Region),
				InitialNodeCount: pulumi.Int(args.StandardNodeCount),
				NodeLocations:    pulumi.ToStringArray(args.Zones),
				NodeConfig: &container.NodePoolNodeConfigArgs{
					ServiceAccount: pulumi.String(nodeServiceAccount),
					OauthScopes:    nodeScopes,
					MachineType:    pulumi.String(args.StandardNodeType),
					DiskType:       pulumi.String(args.StandardNodeDiskType),
					DiskSizeGb:     pulumi.Int(args.StandardNodeDiskSizeGiB),
					ImageType:      pulumi.String(args.StandardNodeImageType),
					Spot:           pulumi.Bool(args.StandardNodeSpot),
					LinuxNodeConfig: &container.NodePoolNodeConfigLinuxNodeConfigArgs{
						Sysctls: pulumi.StringMap{"vm.max_map_count": pulumi.String("262144")},
					},
				},
				NetworkConfig: &container.NodePoolNetworkConfigArgs{
					EnablePrivateNodes: pulumi.Bool(true),
					Subnetwork:         args.PrivateSubnetNames.ToStringArrayOutput().Index(pulumi.Int(0)),
				},
				Management: &container.NodePoolManagementArgs{AutoRepair: pulumi.Bool(true), AutoUpgrade: pulumi.Bool(true)},
				Autoscaling: &container.NodePoolAutoscalingArgs{
					MinNodeCount: pulumi.Int(args.StandardNodeMinCount),
					MaxNodeCount: pulumi.Int(args.StandardNodeMaxCount),
				},
			}, parent, nodeIdentityCreationOnly, pulumi.DependsOn([]pulumi.Resource{cluster}))
			if err != nil {
				return nil, fmt.Errorf("create GKE Standard node pool: %w", err)
			}
			clusterResources = append(clusterResources, nodePool)
		}

	}

	kubeconfig := pulumi.ToSecret(generateKubeconfig(ctx, args.Project, cluster.Name, cluster.Endpoint, cluster.MasterAuth)).(pulumi.StringOutput)
	providerArgs := &kubernetes.ProviderArgs{Kubeconfig: kubeconfig, ClusterIdentifier: cluster.ID().ToStringOutput()}
	providerName := name + "-k8s"
	if args.Namespace != "" {
		providerName += "-cluster"
	}
	clusterProvider, err := kubernetes.NewProvider(ctx, providerName, providerArgs, parent, pulumi.DependsOn(clusterResources))
	if err != nil {
		return nil, fmt.Errorf("create Kubernetes provider for GKE %s: %w", args.Runtime, err)
	}
	k8sProvider := clusterProvider
	workloadServiceAccount := ""
	if args.Namespace != "" {
		namespace, err := corev1.NewNamespace(ctx, name+"-namespace", &corev1.NamespaceArgs{Metadata: &metav1.ObjectMetaArgs{Name: pulumi.String(args.Namespace), Annotations: skipAwait}}, parent, pulumi.Provider(clusterProvider), pulumi.DependsOn(clusterResources))
		if err != nil {
			return nil, fmt.Errorf("create preview namespace: %w", err)
		}
		providerArgs.Namespace = pulumi.StringPtr(args.Namespace)
		k8sProvider, err = kubernetes.NewProvider(ctx, name+"-k8s", providerArgs, parent, pulumi.DependsOn([]pulumi.Resource{namespace}))
		if err != nil {
			return nil, fmt.Errorf("create preview Kubernetes provider: %w", err)
		}
		workloadServiceAccount = "magelift"
		accountID := "mlp-" + strings.TrimPrefix(args.Namespace, "ml-preview-")
		account, err := serviceaccount.NewAccount(ctx, name+"-workload-identity", &serviceaccount.AccountArgs{Project: pulumi.String(args.Project), AccountId: pulumi.String(accountID), DisplayName: pulumi.String("MageLift preview workload")}, parent)
		if err != nil {
			return nil, fmt.Errorf("create preview workload identity: %w", err)
		}
		member, err := serviceaccount.NewIAMMember(ctx, name+"-workload-identity-binding", &serviceaccount.IAMMemberArgs{
			ServiceAccountId: account.Name, Role: pulumi.String("roles/iam.workloadIdentityUser"),
			Member: pulumi.String("serviceAccount:" + args.Project + ".svc.id.goog[" + args.Namespace + "/" + workloadServiceAccount + "]"),
		}, parent)
		if err != nil {
			return nil, fmt.Errorf("bind preview workload identity: %w", err)
		}
		workloadAccount, err := corev1.NewServiceAccount(ctx, name+"-workload-service-account", &corev1.ServiceAccountArgs{
			Metadata: &metav1.ObjectMetaArgs{Name: pulumi.String(workloadServiceAccount), Annotations: pulumi.StringMap{"iam.gke.io/gcp-service-account": account.Email}},
		}, parent, pulumi.Provider(k8sProvider), pulumi.DependsOn([]pulumi.Resource{member}))
		if err != nil {
			return nil, fmt.Errorf("create preview service account: %w", err)
		}
		clusterResources = append(clusterResources, workloadAccount)
		internalPolicy, err := networkingv1.NewNetworkPolicy(ctx, name+"-preview-internal", &networkingv1.NetworkPolicyArgs{
			Spec: &networkingv1.NetworkPolicySpecArgs{
				PodSelector: &metav1.LabelSelectorArgs{},
				PolicyTypes: pulumi.StringArray{pulumi.String("Ingress"), pulumi.String("Egress")},
				Ingress:     networkingv1.NetworkPolicyIngressRuleArray{&networkingv1.NetworkPolicyIngressRuleArgs{From: networkingv1.NetworkPolicyPeerArray{&networkingv1.NetworkPolicyPeerArgs{PodSelector: &metav1.LabelSelectorArgs{}}}}},
				Egress:      networkingv1.NetworkPolicyEgressRuleArray{&networkingv1.NetworkPolicyEgressRuleArgs{}},
			},
		}, parent, pulumi.Provider(k8sProvider))
		if err != nil {
			return nil, fmt.Errorf("create preview network isolation policy: %w", err)
		}
		peers := networkingv1.NetworkPolicyPeerArray{}
		for _, cidr := range args.IngressCIDRs {
			peers = append(peers, &networkingv1.NetworkPolicyPeerArgs{IpBlock: &networkingv1.IPBlockArgs{Cidr: pulumi.String(cidr)}})
		}
		webPolicy, err := networkingv1.NewNetworkPolicy(ctx, name+"-preview-web", &networkingv1.NetworkPolicyArgs{
			Spec: &networkingv1.NetworkPolicySpecArgs{
				PodSelector: &metav1.LabelSelectorArgs{MatchLabels: pulumi.StringMap{"app": pulumi.String(name + "-web")}},
				PolicyTypes: pulumi.StringArray{pulumi.String("Ingress")},
				Ingress:     networkingv1.NetworkPolicyIngressRuleArray{&networkingv1.NetworkPolicyIngressRuleArgs{From: peers}},
			},
		}, parent, pulumi.Provider(k8sProvider))
		if err != nil {
			return nil, fmt.Errorf("create preview web ingress policy: %w", err)
		}
		clusterResources = append(clusterResources, internalPolicy, webPolicy)
	}
	k8sOpts := []pulumi.ResourceOption{parent, pulumi.Provider(k8sProvider), pulumi.DependsOn(clusterResources)}

	searchEndpoint := pulumi.String("").ToStringOutput()
	if args.ExternalSearchEndpoint != "" {
		searchEndpoint = pulumi.String(args.ExternalSearchEndpoint).ToStringOutput()
	} else if args.SearchMode == "opensearch" {
		searchComp, err := search.New(ctx, naming.Resource(args.MagentoProject, args.Environment, "search"), search.Args{
			Replicas: args.SearchReplicas, Image: args.SearchImage, Plugins: args.SearchPlugins, Namespace: args.Namespace, StorageGiB: args.SearchStorageGiB, K8sProvider: k8sProvider,
		}, k8sOpts...)
		if err != nil {
			return nil, fmt.Errorf("create OpenSearch: %w", err)
		}
		searchEndpoint = searchComp.Endpoint
	}

	queueHost := pulumi.String("").ToStringOutput()
	queueUser := ""
	var queuePassword pulumi.StringInput
	if args.QueueMode == "rabbitmq" && args.ExternalQueueHost != "" {
		if args.ExternalQueueUsername == "" || args.ExternalQueuePassword == nil {
			return nil, errors.New("external queue username and password are required")
		}
		queueHost = pulumi.String(args.ExternalQueueHost).ToStringOutput()
		queueUser = args.ExternalQueueUsername
		queuePassword = args.ExternalQueuePassword
	} else if args.QueueMode == "rabbitmq" {
		queueComp, err := queue.New(ctx, naming.Resource(args.MagentoProject, args.Environment, "rabbitmq"), queue.Args{
			Replicas: args.QueueReplicas, Image: args.QueueImage, K8sProvider: k8sProvider, Username: "magento", Namespace: args.Namespace, StorageGiB: args.QueueStorageGiB,
		}, k8sOpts...)
		if err != nil {
			return nil, fmt.Errorf("create RabbitMQ: %w", err)
		}
		queueHost = queueComp.Host
		queueUser = "magento"
		queuePassword = queueComp.Password
	}

	env := containerEnv(args, searchEndpoint, queueHost, queueUser)
	encryptionKeySecret, err := kube.NewMagentoEncryptionKeySecret(ctx, name, args.EncryptionKey, k8sOpts...)
	if err != nil {
		return nil, fmt.Errorf("create Magento encryption key Secret: %w", err)
	}
	encryptionKeySecretName := kube.EncryptionKeySecretName(name)
	env = kube.AppendEncryptionKeyEnv(env, encryptionKeySecretName)
	databaseSecret, err := kube.NewDatabaseCredentialsSecret(ctx, name, args.DatabaseUsername, args.DatabasePassword, k8sOpts...)
	if err != nil {
		return nil, fmt.Errorf("create database credentials Secret: %w", err)
	}
	databaseSecretName := kube.DatabaseCredentialsSecretName(name)
	env = kube.AppendDatabaseCredentialEnv(env, databaseSecretName)
	var databaseAdminSecret pulumi.Resource
	databaseAdminSecretName := ""
	if !args.ExternalDatabase || args.DatabaseService.Ownership == "self-hosted" {
		databaseAdminSecret, err = kube.NewDatabaseAdminCredentialsSecret(ctx, name, args.DatabaseAdminUsername, args.DatabaseAdminPassword, k8sOpts...)
		if err != nil {
			return nil, fmt.Errorf("create database admin credentials Secret: %w", err)
		}
		databaseAdminSecretName = kube.DatabaseAdminCredentialsSecretName(name)
	}

	smtpPasswordSecretName := ""
	var smtpPasswordSecret pulumi.Resource
	if args.SmtpPassword != nil {
		var err error
		smtpPasswordSecret, err = kube.NewSmtpPasswordSecret(ctx, name, args.SmtpPassword, k8sOpts...)
		if err != nil {
			return nil, fmt.Errorf("create SMTP password Secret: %w", err)
		}
		smtpPasswordSecretName = kube.SmtpPasswordSecretName(name)
		env = kube.AppendSmtpPasswordEnv(env, smtpPasswordSecretName)
	}
	cachePasswordSecretName := ""
	var cachePasswordSecret pulumi.Resource
	if args.ExternalCachePassword != nil {
		cachePasswordSecret, err = kube.NewCachePasswordSecret(ctx, name, args.ExternalCachePassword, k8sOpts...)
		if err != nil {
			return nil, fmt.Errorf("create cache password Secret: %w", err)
		}
		cachePasswordSecretName = name + "-cache-password"
	}
	sessionPasswordSecretName := ""
	var sessionPasswordSecret pulumi.Resource
	if args.SessionService.Family == "" {
		sessionPasswordSecretName = cachePasswordSecretName
	} else if args.ExternalSessionPassword != nil {
		sessionPasswordSecret, err = kube.NewCachePasswordSecret(ctx, name+"-session", args.ExternalSessionPassword, k8sOpts...)
		if err != nil {
			return nil, fmt.Errorf("create session password Secret: %w", err)
		}
		sessionPasswordSecretName = name + "-session-cache-password"
	}
	env = kube.AppendStorePasswordEnv(env, cachePasswordSecretName, sessionPasswordSecretName)
	mediaHmacSecretName := ""
	var mediaHmacSecret pulumi.Resource
	if args.MediaHmacSecret != nil {
		var err error
		mediaHmacSecret, err = kube.NewMediaHmacSecret(ctx, name, args.MediaHmacSecret, k8sOpts...)
		if err != nil {
			return nil, fmt.Errorf("create media HMAC Secret: %w", err)
		}
		mediaHmacSecretName = kube.MediaHmacSecretName(name)
		env = kube.AppendMediaHmacSecretEnv(env, mediaHmacSecretName)
	}
	queuePasswordSecretName := ""
	var queuePasswordSecret pulumi.Resource
	if args.QueueMode == "rabbitmq" {
		var err error
		queuePasswordSecret, err = kube.NewQueuePasswordSecret(ctx, name, queuePassword, k8sOpts...)
		if err != nil {
			return nil, fmt.Errorf("create queue password Secret: %w", err)
		}
		queuePasswordSecretName = kube.QueuePasswordSecretName(name)
		env = kube.AppendQueuePasswordEnv(env, queuePasswordSecretName)
	}
	workloadDependencies := []pulumi.Resource{databaseSecret, encryptionKeySecret}
	if args.DatabaseService.Ownership == "self-hosted" {
		dbname := naming.Resource(args.MagentoProject, args.Environment, "db")
		dbEnv := corev1.EnvVarArray{
			&corev1.EnvVarArgs{Name: pulumi.String("MARIADB_DATABASE"), Value: pulumi.String(args.DatabaseName)},
			&corev1.EnvVarArgs{Name: pulumi.String("MARIADB_USER"), Value: pulumi.String(args.DatabaseUsername)},
			secretEnv("MARIADB_PASSWORD", databaseSecretName, "password"),
			secretEnv("MARIADB_ROOT_PASSWORD", databaseAdminSecretName, "password"),
		}
		db, dbErr := newStatefulService(ctx, dbname, serviceImage(args.DatabaseService), 3306, args.DatabaseService.StorageGiB, 0, args.RetainServiceVolumes, sdk.DatabaseArguments(args.DatabaseService.Parameters), dbEnv, append(k8sOpts, pulumi.DependsOn([]pulumi.Resource{databaseSecret, databaseAdminSecret}))...)
		if dbErr != nil {
			return nil, fmt.Errorf("create MariaDB: %w", dbErr)
		}
		workloadDependencies = append(workloadDependencies, db)
	}
	for _, item := range []struct {
		role       string
		binding    sdk.ServiceBinding
		secret     string
		dependency pulumi.Resource
	}{
		{"cache", args.CacheService, cachePasswordSecretName, cachePasswordSecret},
		{"session", args.SessionService, sessionPasswordSecretName, sessionPasswordSecret},
	} {
		if item.binding.Ownership != "self-hosted" {
			continue
		}
		policy := item.binding.EvictionPolicy
		if policy == "" {
			policy = "noeviction"
		}
		cacheName := naming.Resource(args.MagentoProject, args.Environment, item.role)
		memoryMiB := item.binding.MemoryMiB
		if memoryMiB == 0 {
			memoryMiB = 512
		}
		executable := "valkey-server"
		if item.binding.Family == "redis" {
			executable = "redis-server"
		}
		command := []string{executable, "--requirepass", "$(VALKEY_PASSWORD)", "--maxmemory-policy", policy, "--maxmemory", fmt.Sprintf("%dmb", memoryMiB)}
		if item.role == "session" {
			command = append(command, "--appendonly", "yes")
		}
		store, storeErr := newStatefulService(ctx, cacheName, serviceImage(item.binding), 6379, item.binding.StorageGiB, memoryMiB, args.RetainServiceVolumes, command, corev1.EnvVarArray{secretEnv("VALKEY_PASSWORD", item.secret, "password")}, append(k8sOpts, pulumi.DependsOn([]pulumi.Resource{item.dependency}))...)
		if storeErr != nil {
			return nil, fmt.Errorf("create %s store: %w", item.role, storeErr)
		}
		workloadDependencies = append(workloadDependencies, store)
	}
	if !args.ExternalDatabase {
		grantStatement, err := databaseGrantStatement(args.DatabaseName, args.DatabaseUsername)
		if err != nil {
			return nil, fmt.Errorf("build database privilege grant: %w", err)
		}
		grantDependencies := []pulumi.Resource{databaseSecret, databaseAdminSecret}
		grantDependencies = append(grantDependencies, args.DatabaseUsersReady...)
		grantOpts := append(k8sOpts, pulumi.DependsOn(grantDependencies))
		grantOpts = append(grantOpts, pulumi.Timeouts(&pulumi.CustomTimeouts{Create: "15m", Delete: "15m"}))
		databaseGrant, err := newDatabaseGrantJob(ctx, name, args.DatabaseWriter, args.DatabaseName, args.DatabaseAdminUsername, databaseAdminSecretName, grantStatement, workloadServiceAccount, grantOpts...)
		if err != nil {
			return nil, fmt.Errorf("create database privilege grant Job: %w", err)
		}
		workloadDependencies = append(workloadDependencies, databaseGrant)
	}
	workloadOpts := append(k8sOpts, pulumi.DependsOn(workloadDependencies))
	if cachePasswordSecret != nil {
		workloadOpts = append(workloadOpts, pulumi.DependsOn([]pulumi.Resource{cachePasswordSecret}))
	}
	if sessionPasswordSecret != nil {
		workloadOpts = append(workloadOpts, pulumi.DependsOn([]pulumi.Resource{sessionPasswordSecret}))
	}
	if queuePasswordSecret != nil {
		workloadOpts = append(workloadOpts, pulumi.DependsOn([]pulumi.Resource{queuePasswordSecret}))
	}
	if smtpPasswordSecret != nil {
		workloadOpts = append(workloadOpts, pulumi.DependsOn([]pulumi.Resource{smtpPasswordSecret}))
	}
	if mediaHmacSecret != nil {
		workloadOpts = append(workloadOpts, pulumi.DependsOn([]pulumi.Resource{mediaHmacSecret}))
	}

	webContainers, err := kube.WebRuntimeContainers(args.WebRuntime, args.Image, env, args.CPURequest, args.MemoryRequest)
	if err != nil {
		return nil, err
	}
	web, err := appsv1.NewDeployment(ctx, name+"-web", &appsv1.DeploymentArgs{
		Metadata: &metav1.ObjectMetaArgs{Name: pulumi.String(name + "-web"), Annotations: skipAwait},
		Spec: &appsv1.DeploymentSpecArgs{
			Replicas: pulumi.Int(args.DesiredWebReplicas),
			Selector: &metav1.LabelSelectorArgs{MatchLabels: pulumi.StringMap{"app": pulumi.String(name + "-web")}},
			Template: &corev1.PodTemplateSpecArgs{
				Metadata: &metav1.ObjectMetaArgs{Labels: pulumi.StringMap{"app": pulumi.String(name + "-web")}},
				Spec: &corev1.PodSpecArgs{
					ServiceAccountName: pulumi.String(workloadServiceAccount),
					Containers:         webContainers,
				},
			},
		},
	}, workloadOpts...)
	if err != nil {
		return nil, fmt.Errorf("create web Deployment: %w", err)
	}

	serviceAnnotations := pulumi.StringMap{}
	for key, value := range skipAwait {
		serviceAnnotations[key] = value
	}
	serviceType := "LoadBalancer"
	if args.NativeEdge {
		serviceType = "ClusterIP"
		serviceAnnotations["cloud.google.com/backend-config"] = pulumi.Sprintf(`{"default":"%s"}`, args.NativeEdgeBackendConfigName)
		serviceAnnotations["cloud.google.com/neg"] = pulumi.String(`{"ingress": true}`)
	}
	service, err := corev1.NewService(ctx, name+"-web-svc", &corev1.ServiceArgs{
		Metadata: &metav1.ObjectMetaArgs{Name: pulumi.String(name + "-web"), Annotations: serviceAnnotations},
		Spec: &corev1.ServiceSpecArgs{
			Type:     pulumi.String(serviceType),
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
					ServiceAccountName: pulumi.String(workloadServiceAccount),
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
	}, workloadOpts...)
	if err != nil {
		return nil, fmt.Errorf("create cron Deployment: %w", err)
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
						ServiceAccountName: pulumi.String(workloadServiceAccount),
						Containers: corev1.ContainerArray{
							&corev1.ContainerArgs{
								Name:    pulumi.String("queue"),
								Image:   pulumi.String(args.Image),
								Command: kube.ToStringArray(platform.MagentoQueueArgsFor(args.Magento.ConsumerNames)),
								Env:     env,
								Resources: &corev1.ResourceRequirementsArgs{
									Requests: pulumi.StringMap{"cpu": pulumi.String(args.CPURequest), "memory": pulumi.String(args.MemoryRequest)},
								},
							},
						},
					},
				},
			},
		}, workloadOpts...)
		if err != nil {
			return nil, fmt.Errorf("create queue Deployment: %w", err)
		}
	}

	component.ClusterName = cluster.Name
	component.ServiceName = pulumi.String(name + "-web").ToStringOutput()
	component.ClusterEndpoint = cluster.Endpoint
	component.SearchEndpoint = searchEndpoint
	component.QueueHost = queueHost
	component.Namespace = pulumi.String(effectiveNamespace(args.Namespace)).ToStringOutput()
	component.WorkloadServiceAccount = pulumi.String(workloadServiceAccount).ToStringOutput()
	component.QueueMode = pulumi.String(args.QueueMode).ToStringOutput()
	component.QueueUsername = pulumi.String(queueUser).ToStringOutput()
	component.QueuePort = pulumi.String(args.ExternalQueuePort).ToStringOutput()
	component.QueueSSL = pulumi.String(args.ExternalQueueSSL).ToStringOutput()
	component.QueuePasswordSecretName = pulumi.String(queuePasswordSecretName).ToStringOutput()
	component.CachePasswordSecretName = pulumi.String(cachePasswordSecretName).ToStringOutput()
	component.SessionPasswordSecretName = pulumi.String(sessionPasswordSecretName).ToStringOutput()
	if args.SessionEndpoint != nil {
		component.SessionEndpoint = args.SessionEndpoint.ToStringOutput()
	} else {
		component.SessionEndpoint = args.CacheEndpoint.ToStringOutput()
	}
	component.DatabaseSecretName = pulumi.String(databaseSecretName).ToStringOutput()
	component.EncryptionKeySecretName = pulumi.String(encryptionKeySecretName).ToStringOutput()
	component.Kubeconfig = kubeconfig
	component.KubernetesProvider = k8sProvider
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
		"clusterName": component.ClusterName, "serviceName": component.ServiceName,
		"applicationURL": component.ApplicationURL, "searchEndpoint": component.SearchEndpoint,
		"queueMode": component.QueueMode, "queueHost": component.QueueHost,
		"queuePasswordSecretName": component.QueuePasswordSecretName,
		"databaseSecretName":      component.DatabaseSecretName,
		"encryptionKeySecretName": component.EncryptionKeySecretName,
		"kubeconfig":              component.Kubeconfig,
	}); err != nil {
		return nil, err
	}
	return component, nil
}

func containerEnv(args Args, searchEndpoint, queueHost pulumi.StringOutput, queueUser string) corev1.EnvVarArrayOutput {
	mediaBucket := args.MediaBucket
	if mediaBucket == nil {
		mediaBucket = pulumi.String("")
	}
	mediaHmacAccessID := args.MediaHmacAccessID
	if mediaHmacAccessID == nil {
		mediaHmacAccessID = pulumi.String("")
	}
	return pulumi.All(args.DatabaseWriter, args.CacheEndpoint, args.SessionEndpoint, searchEndpoint, queueHost, mediaBucket, mediaHmacAccessID).ApplyT(func(values []interface{}) []corev1.EnvVar {
		session := values[2].(string)
		if session == "" {
			session = values[1].(string)
		}
		bindings := appendSmtpBindings(platform.CoreEnvBindings(platform.CapabilityEndpoints{
			ApplicationMode:    args.ApplicationMode,
			ApplicationVersion: args.ApplicationVersion,
			WebRuntime:         args.WebRuntime,
			DatabaseWriter:     values[0].(string),
			DatabaseName:       args.DatabaseName,
			CacheEndpoint:      values[1].(string),
			SessionEndpoint:    session,
			SearchEndpoint:     values[3].(string),
			QueueMode:          args.QueueMode,
			QueueHost:          values[4].(string),
			QueueUsername:      queueUser,
			QueuePort:          args.ExternalQueuePort,
			QueueSSL:           args.ExternalQueueSSL,
			Magento:            args.Magento,
		}), args)
		bindings = appendMediaDCBindings(bindings, values[5].(string), values[6].(string), args.Region, args.MediaS3Prefix)
		return kube.EnvVars(bindings)
	}).(corev1.EnvVarArrayOutput)
}

// appendMediaDCBindings renders the env.php remote_storage section.
// Markers use MAGENTO_DC_* names (the application image build rejects
// MAGELIFT markers in env.php); the secret arrives separately via
// SecretKeyRef. Empty values append nothing: unset markers fall back
// to the template defaults (notably the empty root prefix).
func appendMediaDCBindings(bindings []platform.EnvBinding, bucket, keyID, region, prefix string) []platform.EnvBinding {
	if strings.TrimSpace(bucket) == "" {
		return bindings
	}
	add := func(name, value string) {
		if strings.TrimSpace(value) == "" {
			return
		}
		bindings = append(bindings, platform.EnvBinding{Name: name, Value: value})
	}
	add(platform.EnvMagentoMediaDriver, mediaDriver)
	add(platform.EnvMagentoMediaPrefix, prefix)
	add(platform.EnvMagentoMediaBucket, bucket)
	add(platform.EnvMagentoMediaRegion, region)
	add(platform.EnvMagentoMediaEndpoint, mediaS3Endpoint)
	add(platform.EnvMagentoMediaKey, keyID)
	// Delivery runs through the storefront: pin the media base URLs
	// app-relative. Env config overrides core_config_data, so even a
	// migrated shop database pointing at object storage cannot divert
	// emission (a remote URL would only 403 against the private bucket).
	bindings = append(bindings,
		platform.EnvBinding{Name: platform.EnvMagentoBaseMediaURL, Value: "{{unsecure_base_url}}media/"},
		platform.EnvBinding{Name: platform.EnvMagentoBaseMediaURLSecure, Value: "{{secure_base_url}}media/"},
	)
	return bindings
}

// appendSmtpBindings wires operator-relay SMTP into Magento's system/smtp
// config surface. Keys mirror the AWS managed-SES wiring; empty host means
// unmanaged and appends nothing. The password travels via SecretKeyRef,
// never as a plain binding.
func appendSmtpBindings(bindings []platform.EnvBinding, args Args) []platform.EnvBinding {
	if strings.TrimSpace(args.SmtpHost) == "" {
		return bindings
	}
	bindings = append(bindings,
		platform.EnvBinding{Name: "CONFIG__DEFAULT__SYSTEM__SMTP__TRANSPORT", Value: "smtp"},
		platform.EnvBinding{Name: "CONFIG__DEFAULT__SYSTEM__SMTP__HOST", Value: args.SmtpHost},
		platform.EnvBinding{Name: "CONFIG__DEFAULT__SYSTEM__SMTP__PORT", Value: strconv.Itoa(args.SmtpPort)},
		platform.EnvBinding{Name: "CONFIG__DEFAULT__SYSTEM__SMTP__USERNAME", Value: args.SmtpUsername},
		platform.EnvBinding{Name: "CONFIG__DEFAULT__SYSTEM__SMTP__AUTH", Value: "LOGIN"},
		platform.EnvBinding{Name: "CONFIG__DEFAULT__SYSTEM__SMTP__SSL", Value: "tls"},
		platform.EnvBinding{Name: "CONFIG__DEFAULT__SYSTEM__SMTP__DISABLE", Value: "0"},
	)
	if strings.TrimSpace(args.SmtpFrom) != "" {
		bindings = append(bindings,
			platform.EnvBinding{Name: "CONFIG__DEFAULT__TRANS_EMAIL__IDENT_GENERAL__EMAIL", Value: args.SmtpFrom},
		)
	}
	return bindings
}

func generateKubeconfig(ctx *pulumi.Context, project string, name, endpoint pulumi.StringOutput, auth container.ClusterMasterAuthOutput) pulumi.StringOutput {
	return pulumi.All(name, endpoint, auth.ClusterCaCertificate(), organizations.GetClientConfigOutput(ctx).AccessToken()).ApplyT(func(values []interface{}) (string, error) {
		contextName := fmt.Sprintf("%s_magelift_%s", project, asString(values[0]))
		server := "https://" + asString(values[1])
		return kube.BuildStaticTokenKubeconfig(contextName, server, asString(values[2]), asString(values[3])), nil
	}).(pulumi.StringOutput)
}

func asString(value interface{}) string {
	switch typed := value.(type) {
	case string:
		return typed
	case *string:
		if typed == nil {
			return ""
		}
		return *typed
	default:
		return fmt.Sprintf("%v", value)
	}
}

func effectiveNamespace(namespace string) string {
	if namespace == "" {
		return "default"
	}
	return namespace
}
