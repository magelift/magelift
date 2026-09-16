// Package eks provisions the provider-specific runtime for the supported EKS
// compute boundaries. The portable workload contract remains in the core;
// IAM, bootstrap, storage, and observability add-ons stay in this adapter.
package eks

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"

	kubequeue "github.com/magelift/magelift/internal/cloud/kube/queue"
	kubesearch "github.com/magelift/magelift/internal/cloud/kube/search"
	"github.com/magelift/magelift/internal/cloud/kube"
	"github.com/magelift/magelift/internal/platform"
	"github.com/pulumi/pulumi-aws/sdk/v7/go/aws/eks"
	"github.com/pulumi/pulumi-aws/sdk/v7/go/aws/iam"
	"github.com/pulumi/pulumi-aws/sdk/v7/go/aws/vpc"
	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	appsv1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/apps/v1"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	storagev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/storage/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

const (
	TypeToken                = "magelift:aws:EKSRuntime"
	ApplicationPort          = 8080
	DefaultKubernetesVersion = "1.36"
	ComputeModeAuto          = "auto-mode"
	ComputeModeManagedNodes  = "managed-node-groups"
	ComputeModeSelfManaged   = "self-managed"
	ComputeModeFargate       = "fargate"
	openSearchTypeToken      = "magelift:aws:EKSOpenSearch"
	rabbitMQTypeToken        = "magelift:aws:EKSRabbitMQ"
)

var (
	accountIDPattern = regexp.MustCompile(`^[0-9]{12}$`)
	nodeAMIPattern   = regexp.MustCompile(`^ami-[A-Za-z0-9]+$`)
	namespacePattern = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`)
)

type nodePolicy struct {
	suffix string
	arn    string
}

// eksNodePolicies returns the IAM policies required by the EC2 node role for
// the selected non-Fargate compute mode. Auto Mode supplies its own managed
// networking and storage permissions; managed and self-managed nodes run the
// aws-node CNI daemonset and need the corresponding EC2 API policy. The EBS
// CSI controller receives its permissions through its dedicated service
// account role instead of broadening the instance role.
func eksNodePolicies(computeMode string) []nodePolicy {
	workerPolicy := "arn:aws:iam::aws:policy/AmazonEKSWorkerNodePolicy"
	if computeMode == ComputeModeAuto {
		workerPolicy = "arn:aws:iam::aws:policy/AmazonEKSWorkerNodeMinimalPolicy"
	}

	policies := []nodePolicy{
		{suffix: "worker", arn: workerPolicy},
		{suffix: "ecr", arn: "arn:aws:iam::aws:policy/AmazonEC2ContainerRegistryPullOnly"},
	}
	if computeMode != ComputeModeAuto {
		policies = append(policies, nodePolicy{suffix: "cni", arn: "arn:aws:iam::aws:policy/AmazonEKS_CNI_Policy"})
	}
	return policies
}

// eksWebServiceAnnotations keeps Pulumi from awaiting the Service, then (for
// Auto Mode) asks EKS to provision an internet-facing NLB. Auto Mode defaults
// to an internal NLB, which Magento HTTP health cannot reach from outside the VPC.
func eksWebServiceAnnotations(computeMode string) pulumi.StringMap {
	annotations := kube.SkipAwaitAnnotations()
	if computeMode == ComputeModeAuto {
		annotations["service.beta.kubernetes.io/aws-load-balancer-scheme"] = pulumi.String("internet-facing")
	}
	return annotations
}

func storefrontURLFromLoadBalancer(hostname, ip string) string {
	host := strings.TrimSpace(hostname)
	if host == "" {
		host = strings.TrimSpace(ip)
	}
	if host == "" {
		return ""
	}
	return "http://" + host
}

type Args struct {
	Region                  string
	AccountID               string
	PrivateSubnetIDs        pulumi.StringArrayInput
	KubernetesVersion       string
	ComputeMode             string
	NodeInstanceType        string
	NodeAMI                 string
	NodeMinSize             int
	NodeDesiredSize         int
	NodeMaxSize             int
	FargateNamespaces       []string
	Image                   string
	ApplicationMode         string
	ApplicationVersion      string
	WebRuntime              string
	EncryptionKey           pulumi.StringInput
	DatabaseWriter          pulumi.StringInput
	DatabaseName            string
	DatabaseUsername        string
	DatabasePassword        pulumi.StringInput
	DatabaseAdminUsername   string
	DatabaseAdminPassword   pulumi.StringInput
	DatabaseReady           []pulumi.Resource
	CacheEndpoint           pulumi.StringInput
	SessionEndpoint         pulumi.StringInput
	DatabaseSecurityGroupID pulumi.StringInput
	CacheSecurityGroupID    pulumi.StringInput
	SearchMode              string
	SearchReplicas          int
	SearchImage             string
	QueueMode               string
	QueueReplicas           int
	QueueImage              string
	CPURequest              string
	MemoryRequest           string
	DesiredWebReplicas      int
	QueueConsumerCount      int
	Tags                    map[string]string
	Magento                 platform.MagentoOverlays
}

type Component struct {
	pulumi.ResourceState
	ClusterName             pulumi.StringOutput
	ServiceName             pulumi.StringOutput
	ApplicationURL          pulumi.StringOutput
	ClusterARN              pulumi.StringOutput
	SearchEndpoint          pulumi.StringOutput
	QueueHost               pulumi.StringOutput
	QueueMode               pulumi.StringOutput
	QueuePasswordSecretName pulumi.StringOutput
	DatabaseSecretName      pulumi.StringOutput
	EncryptionKeySecretName pulumi.StringOutput
	Kubeconfig              pulumi.StringOutput
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
	if args.EncryptionKey == nil {
		return nil, errors.New("Magento encryption key is required")
	}
	if strings.TrimSpace(args.DatabaseUsername) == "" {
		return nil, errors.New("database username is required")
	}
	if args.DatabasePassword == nil {
		return nil, errors.New("database password is required")
	}
	if strings.TrimSpace(args.DatabaseAdminUsername) == "" {
		return nil, errors.New("database admin username is required")
	}
	if args.DatabaseAdminPassword == nil {
		return nil, errors.New("database admin password is required")
	}
	computeMode := strings.TrimSpace(args.ComputeMode)
	if computeMode == "" {
		computeMode = ComputeModeAuto
	}
	if computeMode != ComputeModeAuto && computeMode != ComputeModeManagedNodes && computeMode != ComputeModeSelfManaged && computeMode != ComputeModeFargate {
		return nil, fmt.Errorf("unsupported EKS compute mode %q", computeMode)
	}
	args.ComputeMode = computeMode
	if computeMode == ComputeModeSelfManaged && !nodeAMIPattern.MatchString(strings.TrimSpace(args.NodeAMI)) {
		return nil, errors.New("self-managed EKS nodes require a pinned ami-* node AMI")
	}
	if computeMode != ComputeModeSelfManaged && strings.TrimSpace(args.NodeAMI) != "" {
		return nil, errors.New("nodeAMI is only supported for self-managed EKS nodes")
	}
	if (computeMode == ComputeModeAuto || computeMode == ComputeModeFargate) && (strings.TrimSpace(args.NodeInstanceType) != "" || args.NodeMinSize > 0 || args.NodeDesiredSize > 0 || args.NodeMaxSize > 0) {
		return nil, errors.New("EKS node capacity settings require managed-node-groups or self-managed mode")
	}
	if computeMode != ComputeModeFargate && len(args.FargateNamespaces) > 0 {
		return nil, errors.New("Fargate namespaces require fargate compute mode")
	}
	if computeMode == ComputeModeFargate {
		if !accountIDPattern.MatchString(strings.TrimSpace(args.AccountID)) {
			return nil, errors.New("EKS Fargate requires a 12-digit account ID for the pod execution trust policy")
		}
		if args.SearchMode == "opensearch" || args.QueueMode == "rabbitmq" {
			return nil, errors.New("EKS Fargate cannot host persistent OpenSearch or RabbitMQ workloads")
		}
	}
	if strings.TrimSpace(args.KubernetesVersion) == "" {
		args.KubernetesVersion = DefaultKubernetesVersion
	}
	if !strings.HasPrefix(args.KubernetesVersion, "1.") {
		return nil, fmt.Errorf("unsupported EKS Kubernetes version %q", args.KubernetesVersion)
	}
	if args.SearchMode == "" {
		args.SearchMode = "disabled"
	}
	if args.SearchMode != "opensearch" && args.SearchMode != "disabled" {
		return nil, fmt.Errorf("unsupported EKS search mode %q", args.SearchMode)
	}
	if args.SearchMode == "opensearch" {
		if args.SearchReplicas < 1 {
			args.SearchReplicas = 1
		}
	} else {
		args.SearchReplicas = 0
	}
	if args.QueueMode == "" {
		args.QueueMode = "database"
	}
	if args.QueueMode != "database" && args.QueueMode != "rabbitmq" {
		return nil, fmt.Errorf("unsupported EKS queue mode %q", args.QueueMode)
	}
	if args.QueueMode == "rabbitmq" && args.QueueReplicas < 1 {
		args.QueueReplicas = 1
	}
	if args.QueueMode == "database" {
		args.QueueReplicas = 0
	}
	if args.DesiredWebReplicas < 1 {
		args.DesiredWebReplicas = 1
	}
	if computeMode != ComputeModeAuto && computeMode != ComputeModeFargate {
		if strings.TrimSpace(args.NodeInstanceType) == "" {
			args.NodeInstanceType = "m6i.large"
		}
		if args.NodeMinSize < 1 {
			args.NodeMinSize = 1
		}
		if args.NodeDesiredSize < args.NodeMinSize {
			args.NodeDesiredSize = args.DesiredWebReplicas
			if args.NodeDesiredSize < args.NodeMinSize {
				args.NodeDesiredSize = args.NodeMinSize
			}
		}
		if args.NodeMaxSize < args.NodeDesiredSize {
			args.NodeMaxSize = args.NodeDesiredSize
		}
	}
	if args.CPURequest == "" {
		args.CPURequest = "500m"
	}
	if args.MemoryRequest == "" {
		args.MemoryRequest = kube.DefaultApplicationMemoryRequest
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
	}
	if computeMode == ComputeModeAuto {
		clusterPolicies = append(clusterPolicies,
			struct{ suffix, arn string }{"compute", "arn:aws:iam::aws:policy/AmazonEKSComputePolicy"},
			struct{ suffix, arn string }{"block", "arn:aws:iam::aws:policy/AmazonEKSBlockStoragePolicy"},
			struct{ suffix, arn string }{"lb", "arn:aws:iam::aws:policy/AmazonEKSLoadBalancingPolicy"},
			struct{ suffix, arn string }{"net", "arn:aws:iam::aws:policy/AmazonEKSNetworkingPolicy"},
		)
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

	var nodeRole *iam.Role
	var nodePolicyAttachments []pulumi.Resource
	if computeMode != ComputeModeFargate {
		nodeRole, err = iam.NewRole(ctx, name+"-node-role", &iam.RoleArgs{
			AssumeRolePolicy: pulumi.String(`{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"ec2.amazonaws.com"},"Action":"sts:AssumeRole"}]}`),
			Tags:             pulumi.ToStringMap(args.Tags),
		}, parent)
		if err != nil {
			return nil, fmt.Errorf("create EKS node role: %w", err)
		}
		nodePolicies := eksNodePolicies(computeMode)
		for _, policy := range nodePolicies {
			attachment, err := iam.NewRolePolicyAttachment(ctx, name+"-node-"+policy.suffix, &iam.RolePolicyAttachmentArgs{
				Role: nodeRole.Name, PolicyArn: pulumi.String(policy.arn),
			}, parent)
			if err != nil {
				return nil, fmt.Errorf("attach node %s policy: %w", policy.suffix, err)
			}
			nodePolicyAttachments = append(nodePolicyAttachments, attachment)
		}
	}

	clusterDeps := append([]pulumi.Resource{}, clusterPolicyAttachments...)
	clusterArgs := &eks.ClusterArgs{
		Name:                       pulumi.String(name),
		RoleArn:                    clusterRole.Arn,
		Version:                    pulumi.String(args.KubernetesVersion),
		EnabledClusterLogTypes:     pulumi.ToStringArray([]string{"api", "audit", "authenticator", "controllerManager", "scheduler"}),
		BootstrapSelfManagedAddons: pulumi.Bool(true),
		AccessConfig: &eks.ClusterAccessConfigArgs{
			AuthenticationMode:                      pulumi.String("API"),
			BootstrapClusterCreatorAdminPermissions: pulumi.Bool(true),
		},
		VpcConfig: &eks.ClusterVpcConfigArgs{
			EndpointPrivateAccess: pulumi.Bool(true),
			EndpointPublicAccess:  pulumi.Bool(true),
			SubnetIds:             args.PrivateSubnetIDs,
		},
		Tags: pulumi.ToStringMap(args.Tags),
	}
	if computeMode == ComputeModeAuto {
		clusterArgs.BootstrapSelfManagedAddons = pulumi.Bool(false)
		clusterArgs.ComputeConfig = &eks.ClusterComputeConfigArgs{
			Enabled:     pulumi.Bool(true),
			NodePools:   pulumi.StringArray{pulumi.String("general-purpose"), pulumi.String("system")},
			NodeRoleArn: nodeRole.Arn,
		}
		clusterArgs.KubernetesNetworkConfig = &eks.ClusterKubernetesNetworkConfigArgs{
			ElasticLoadBalancing: &eks.ClusterKubernetesNetworkConfigElasticLoadBalancingArgs{Enabled: pulumi.Bool(true)},
		}
		clusterArgs.StorageConfig = &eks.ClusterStorageConfigArgs{
			BlockStorage: &eks.ClusterStorageConfigBlockStorageArgs{Enabled: pulumi.Bool(true)},
		}
	}
	cluster, err := eks.NewCluster(ctx, name+"-cluster", clusterArgs, append([]pulumi.ResourceOption{parent}, pulumi.DependsOn(clusterDeps))...)
	if err != nil {
		return nil, fmt.Errorf("create EKS cluster: %w", err)
	}
	if err := createExternalServiceIngressRules(ctx, name, cluster.VpcConfig.ClusterSecurityGroupId(), args, component); err != nil {
		return nil, fmt.Errorf("allow EKS workloads to reach external services: %w", err)
	}

	var computeResources []pulumi.Resource
	switch computeMode {
	case ComputeModeManagedNodes:
		nodeGroup, err := newManagedNodeGroup(ctx, name, cluster, nodeRole, nodePolicyAttachments, args, component)
		if err != nil {
			return nil, fmt.Errorf("create EKS managed node group: %w", err)
		}
		computeResources = append(computeResources, nodeGroup)
	case ComputeModeSelfManaged:
		resources, err := newSelfManagedNodes(ctx, name, cluster, nodeRole, nodePolicyAttachments, args, component)
		if err != nil {
			return nil, fmt.Errorf("create EKS self-managed nodes: %w", err)
		}
		computeResources = append(computeResources, resources...)
	case ComputeModeFargate:
		resources, err := newFargateProfile(ctx, name, cluster, args, component)
		if err != nil {
			return nil, fmt.Errorf("create EKS Fargate profile: %w", err)
		}
		computeResources = append(computeResources, resources...)
	}
	if computeMode != ComputeModeFargate {
		observabilityResources, err := newCloudWatchObservabilityAddon(ctx, name, cluster, nodeRole, computeResources, args, component)
		if err != nil {
			return nil, fmt.Errorf("create EKS CloudWatch observability add-on: %w", err)
		}
		computeResources = append(computeResources, observabilityResources...)
	}
	if computeMode == ComputeModeManagedNodes || computeMode == ComputeModeSelfManaged {
		ebsResources, err := newEBSAddon(ctx, name, cluster, nodePolicyAttachments, computeResources, args, component)
		if err != nil {
			return nil, fmt.Errorf("create EKS EBS CSI add-on: %w", err)
		}
		computeResources = append(computeResources, ebsResources...)
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
	if computeMode == ComputeModeFargate {
		if _, err := appsv1.NewDeploymentPatch(ctx, name+"-coredns-fargate", &appsv1.DeploymentPatchArgs{
			ApiVersion: pulumi.String("apps/v1"), Kind: pulumi.String("Deployment"),
			Metadata: &metav1.ObjectMetaPatchArgs{Name: pulumi.String("coredns"), Namespace: pulumi.String("kube-system")},
			Spec: &appsv1.DeploymentSpecPatchArgs{Template: &corev1.PodTemplateSpecPatchArgs{
				Spec: &corev1.PodSpecPatchArgs{NodeSelector: pulumi.StringMap{"eks.amazonaws.com/compute-type": pulumi.String("fargate")}},
			}},
		}, append(k8sOpts, pulumi.DependsOn(computeResources))...); err != nil {
			return nil, fmt.Errorf("schedule EKS CoreDNS on Fargate: %w", err)
		}
	}

	storageClassName := ""
	var storageClass pulumi.Resource
	if computeMode == ComputeModeAuto || computeMode == ComputeModeManagedNodes || computeMode == ComputeModeSelfManaged {
		storageClassName = name + "-ebs"
		provisioner := "ebs.csi.aws.com"
		if computeMode == ComputeModeAuto {
			storageClassName = name + "-auto-ebs"
			provisioner = "ebs.csi.eks.amazonaws.com"
		}
		storageClassArgs := &storagev1.StorageClassArgs{
			Metadata: &metav1.ObjectMetaArgs{
				Name: pulumi.String(storageClassName),
				Annotations: pulumi.StringMap{
					"storageclass.kubernetes.io/is-default-class": pulumi.String("true"),
				},
			},
			Provisioner:       pulumi.String(provisioner),
			VolumeBindingMode: pulumi.StringPtr("WaitForFirstConsumer"),
			Parameters: pulumi.StringMap{
				"type":      pulumi.String("gp3"),
				"encrypted": pulumi.String("true"),
			},
		}
		if computeMode == ComputeModeAuto {
			storageClassArgs.AllowedTopologies = corev1.TopologySelectorTermArray{
				&corev1.TopologySelectorTermArgs{
					MatchLabelExpressions: corev1.TopologySelectorLabelRequirementArray{
						&corev1.TopologySelectorLabelRequirementArgs{
							Key: pulumi.String("eks.amazonaws.com/compute-type"), Values: pulumi.StringArray{pulumi.String("auto")},
						},
					},
				},
			}
		}
		storageClassResource, err := storagev1.NewStorageClass(ctx, storageClassName, storageClassArgs, append(k8sOpts, pulumi.DependsOn(computeResources))...)
		if err != nil {
			return nil, fmt.Errorf("create EKS %s StorageClass: %w", computeMode, err)
		}
		storageClass = storageClassResource
	}

	searchEndpoint := pulumi.String("").ToStringOutput()
	var searchResource pulumi.Resource
	if args.SearchMode == "opensearch" {
		searchComponent, err := kubesearch.New(ctx, name+"-search", kubesearch.Args{
			Replicas: args.SearchReplicas, Image: args.SearchImage, K8sProvider: k8sProvider,
			StorageClassName: storageClassName, Dependencies: []pulumi.Resource{storageClass},
			TypeToken: openSearchTypeToken,
		}, k8sOpts...)
		if err != nil {
			return nil, fmt.Errorf("create OpenSearch: %w", err)
		}
		searchEndpoint = searchComponent.Endpoint
		searchResource = searchComponent
	}

	queueHost := pulumi.String("").ToStringOutput()
	queueUser := ""
	var queuePassword pulumi.StringInput
	var queueResource pulumi.Resource
	if args.QueueMode == "rabbitmq" {
		queueComponent, err := kubequeue.New(ctx, name+"-rabbitmq", kubequeue.Args{
			Replicas: args.QueueReplicas, Image: args.QueueImage, K8sProvider: k8sProvider,
			StorageClassName: storageClassName, Dependencies: []pulumi.Resource{storageClass},
			Username: "magento", TypeToken: rabbitMQTypeToken,
		}, k8sOpts...)
		if err != nil {
			return nil, fmt.Errorf("create RabbitMQ: %w", err)
		}
		queueHost = queueComponent.Host
		queueUser = "magento"
		queuePassword = queueComponent.Password
		queueResource = queueComponent
	}

	env := containerEnv(args, searchEndpoint, queueHost, queueUser)
	encryptionKeySecret, err := kube.NewMagentoEncryptionKeySecret(ctx, name, args.EncryptionKey, k8sOpts...)
	if err != nil {
		return nil, fmt.Errorf("create Magento encryption key Secret: %w", err)
	}
	databaseSecret, err := kube.NewDatabaseCredentialsSecret(ctx, name, args.DatabaseUsername, args.DatabasePassword, k8sOpts...)
	if err != nil {
		return nil, fmt.Errorf("create database credentials Secret: %w", err)
	}
	databaseSecretName := kube.DatabaseCredentialsSecretName(name)
	encryptionKeySecretName := kube.EncryptionKeySecretName(name)
	env = kube.AppendDatabaseCredentialEnv(env, databaseSecretName)
	env = kube.AppendEncryptionKeyEnv(env, encryptionKeySecretName)
	databaseAdminSecret, err := kube.NewDatabaseAdminCredentialsSecret(ctx, name, args.DatabaseAdminUsername, args.DatabaseAdminPassword, k8sOpts...)
	if err != nil {
		return nil, fmt.Errorf("create database admin credentials Secret: %w", err)
	}
	databaseAdminSecretName := kube.DatabaseAdminCredentialsSecretName(name)
	grantStatement, err := databaseGrantStatement(args.DatabaseName, args.DatabaseUsername)
	if err != nil {
		return nil, fmt.Errorf("build database privilege grant: %w", err)
	}
	grantDependencies := []pulumi.Resource{databaseSecret, databaseAdminSecret}
	grantDependencies = append(grantDependencies, args.DatabaseReady...)
	grantOpts := append(k8sOpts, pulumi.DependsOn(grantDependencies))
	grantOpts = append(grantOpts, pulumi.Timeouts(&pulumi.CustomTimeouts{Create: "15m", Delete: "15m"}))
	databaseGrant, err := newDatabaseGrantJob(ctx, name, args.DatabaseWriter, args.DatabaseName, args.DatabaseAdminUsername, databaseAdminSecretName, grantStatement, grantOpts...)
	if err != nil {
		return nil, fmt.Errorf("create database privilege grant Job: %w", err)
	}
	queuePasswordSecretName := ""
	var queuePasswordSecret pulumi.Resource
	if args.QueueMode == "rabbitmq" {
		queuePasswordSecret, err = kube.NewQueuePasswordSecret(ctx, name, queuePassword, k8sOpts...)
		if err != nil {
			return nil, fmt.Errorf("create queue password Secret: %w", err)
		}
		queuePasswordSecretName = kube.QueuePasswordSecretName(name)
		env = kube.AppendQueuePasswordEnv(env, queuePasswordSecretName)
	}
	workloadDependencies := []pulumi.Resource{databaseSecret, databaseGrant, encryptionKeySecret}
	if searchResource != nil {
		workloadDependencies = append(workloadDependencies, searchResource)
	}
	if queueResource != nil {
		workloadDependencies = append(workloadDependencies, queueResource)
	}
	if queuePasswordSecret != nil {
		workloadDependencies = append(workloadDependencies, queuePasswordSecret)
	}
	workloadOpts := append(k8sOpts, pulumi.DependsOn(workloadDependencies))
	skipAwait := kube.SkipAwaitAnnotations()
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
					Containers: webContainers,
				},
			},
		},
	}, workloadOpts...)
	if err != nil {
		return nil, fmt.Errorf("create web Deployment: %w", err)
	}

	service, err := corev1.NewService(ctx, name+"-web-svc", &corev1.ServiceArgs{
		Metadata: &metav1.ObjectMetaArgs{Name: pulumi.String(name + "-web"), Annotations: eksWebServiceAnnotations(args.ComputeMode)},
		Spec: &corev1.ServiceSpecArgs{
			Type:     pulumi.String("LoadBalancer"),
			Selector: pulumi.StringMap{"app": pulumi.String(name + "-web")},
			Ports: corev1.ServicePortArray{
				&corev1.ServicePortArgs{Port: pulumi.Int(80), TargetPort: pulumi.Int(ApplicationPort)},
			},
		},
	}, append(workloadOpts, pulumi.DependsOn([]pulumi.Resource{web}))...)
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
	component.ClusterARN = cluster.Arn
	component.ServiceName = pulumi.String(name + "-web").ToStringOutput()
	component.SearchEndpoint = searchEndpoint
	component.QueueHost = queueHost
	component.QueueMode = pulumi.String(args.QueueMode).ToStringOutput()
	component.QueuePasswordSecretName = pulumi.String(queuePasswordSecretName).ToStringOutput()
	component.DatabaseSecretName = pulumi.String(databaseSecretName).ToStringOutput()
	component.EncryptionKeySecretName = pulumi.String(encryptionKeySecretName).ToStringOutput()
	component.Kubeconfig = kubeconfig
	component.ApplicationURL = service.Status.ApplyT(func(status *corev1.ServiceStatus) string {
		if status == nil || len(status.LoadBalancer.Ingress) == 0 {
			return ""
		}
		ingress := status.LoadBalancer.Ingress[0]
		hostname, ip := "", ""
		if ingress.Hostname != nil {
			hostname = *ingress.Hostname
		}
		if ingress.Ip != nil {
			ip = *ingress.Ip
		}
		return storefrontURLFromLoadBalancer(hostname, ip)
	}).(pulumi.StringOutput)

	if err := ctx.RegisterResourceOutputs(component, pulumi.Map{
		"clusterName": component.ClusterName, "serviceName": component.ServiceName,
		"applicationURL": component.ApplicationURL, "clusterArn": component.ClusterARN,
		"searchEndpoint": component.SearchEndpoint, "queueMode": component.QueueMode,
		"queueHost": component.QueueHost, "queuePasswordSecretName": component.QueuePasswordSecretName,
		"databaseSecretName":      component.DatabaseSecretName,
		"encryptionKeySecretName": component.EncryptionKeySecretName,
		"kubeconfig":              component.Kubeconfig,
	}); err != nil {
		return nil, err
	}
	return component, nil
}

func createExternalServiceIngressRules(ctx *pulumi.Context, name string, clusterSecurityGroupID pulumi.StringPtrOutput, args Args, parent pulumi.Resource) error {
	rules := []struct {
		suffix      string
		description string
		port        int
		destination pulumi.StringInput
	}{
		{suffix: "data-mysql", description: "EKS workloads to Aurora MySQL", port: 3306, destination: args.DatabaseSecurityGroupID},
		{suffix: "cache-valkey", description: "EKS workloads to Valkey", port: 6379, destination: args.CacheSecurityGroupID},
	}
	for _, spec := range rules {
		if spec.destination == nil {
			continue
		}
		if _, err := vpc.NewSecurityGroupIngressRule(ctx, name+"-ingress-eks-"+spec.suffix, &vpc.SecurityGroupIngressRuleArgs{
			SecurityGroupId:           spec.destination,
			ReferencedSecurityGroupId: clusterSecurityGroupID,
			Description:               pulumi.String(spec.description),
			IpProtocol:                pulumi.String("tcp"),
			FromPort:                  pulumi.Int(spec.port),
			ToPort:                    pulumi.Int(spec.port),
			Region:                    pulumi.String(args.Region),
			Tags:                      eksRuleTags(args.Tags, name, spec.suffix),
		}, pulumi.Parent(parent)); err != nil {
			return err
		}
	}
	return nil
}

func eksRuleTags(input map[string]string, component, role string) pulumi.StringMap {
	result := pulumi.StringMap{}
	for key, value := range input {
		result[key] = pulumi.String(value)
	}
	result["Name"] = pulumi.String(component + "-ingress-eks-" + role)
	result["magelift:component"] = pulumi.String(component)
	result["magelift:role"] = pulumi.String("eks-" + role)
	return result
}

func containerEnv(args Args, searchEndpoint, queueHost pulumi.StringOutput, queueUser string) corev1.EnvVarArrayOutput {
	return pulumi.All(args.DatabaseWriter, args.CacheEndpoint, args.SessionEndpoint, searchEndpoint, queueHost).ApplyT(func(values []interface{}) []corev1.EnvVar {
		session := values[2].(string)
		if session == "" {
			session = values[1].(string)
		}
		bindings := platform.CoreEnvBindings(platform.CapabilityEndpoints{
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
			Magento:            args.Magento,
		})
		return kube.EnvVars(bindings)
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
		server := kubeServerURL(clusterEndpoint)
		execArgs, err := json.Marshal([]string{"eks", "get-token", "--cluster-name", clusterName, "--region", region})
		if err != nil {
			return "", err
		}
		return fmt.Sprintf(`apiVersion: v1
clusters:
- cluster:
    certificate-authority-data: %s
    server: %s
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
`, caValue, server, clusterName, clusterName, clusterName, clusterName, clusterName, clusterName, string(execArgs)), nil
	}).(pulumi.StringOutput)
}

func kubeServerURL(endpoint string) string {
	endpoint = strings.TrimSpace(endpoint)
	if strings.HasPrefix(endpoint, "https://") || strings.HasPrefix(endpoint, "http://") {
		return endpoint
	}
	return "https://" + endpoint
}
