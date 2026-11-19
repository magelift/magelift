// Package stack constructs Azure resources for a Magento environment.
package stack

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/magelift/magelift/sdk"
	"github.com/pulumi/pulumi-azure-native-sdk/authorization/v3"
	"github.com/pulumi/pulumi-azure-native-sdk/containerservice/v3"
	"github.com/pulumi/pulumi-azure-native-sdk/dbformysql/v3"
	"github.com/pulumi/pulumi-azure-native-sdk/keyvault/v3"
	"github.com/pulumi/pulumi-azure-native-sdk/network/v3"
	"github.com/pulumi/pulumi-azure-native-sdk/privatedns/v3"
	"github.com/pulumi/pulumi-azure-native-sdk/storage/v3"
	azurenative "github.com/pulumi/pulumi-azure-native-sdk/v3"
	"github.com/pulumi/pulumi-random/sdk/v4/go/random"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"k8s.io/apimachinery/pkg/util/validation"
)

// Spec holds the resource inputs which must be resolved before constructing a
// stack. The resource group and any supplied network references remain owned
// by the caller. An omitted network is created as part of the stack.
type Spec struct {
	DestroyOnly        bool
	ObserveOnly        bool
	SubscriptionID     string
	TenantID           string
	ResourceGroup      string
	Region             string
	Project            string
	Environment        string
	ClusterName        string
	PreviewPoolCluster string
	Namespace          string
	NodeSubnetID       string
	MySQLServerName    string
	MySQLSubnetID      string
	MySQLDNSZoneID     string
	MySQLVersion       string
	MySQLUsername      string
	MySQLPassword      pulumi.StringInput `json:"-"`
	StorageAccount     string
	VaultName          string
	HighAvailability   bool
	ApplicationImage   string
	ApplicationMode    string
	MagentoVersion     string
	Magento            sdk.MagentoOverlays
	WebRuntime         string
	CacheImage         string
	CacheEndpoint      string
	SearchImage        string
	ImagePullSecret    string
	ACRResourceID      string
	IngressClass       string
	TLSSecret          string
	Domain             string
	MonitorWorkspaceID string
	EncryptionKey      pulumi.StringInput `json:"-"`
}

func (s Spec) Validate() error {
	if s.DestroyOnly || s.ObserveOnly {
		if strings.TrimSpace(s.SubscriptionID) == "" || strings.TrimSpace(s.TenantID) == "" || strings.TrimSpace(s.ResourceGroup) == "" || strings.TrimSpace(s.Project) == "" || strings.TrimSpace(s.Environment) == "" || strings.TrimSpace(s.Region) == "" {
			return errors.New("Azure destroy requires subscription, tenant, resource group, project, environment and region")
		}
		return nil
	}
	var missing []string
	for name, value := range map[string]string{
		"subscription ID": s.SubscriptionID, "tenant ID": s.TenantID,
		"resource group": s.ResourceGroup, "region": s.Region,
		"project": s.Project, "environment": s.Environment,
		"cluster name":      s.ClusterName,
		"MySQL server name": s.MySQLServerName, "MySQL version": s.MySQLVersion,
		"MySQL username": s.MySQLUsername, "storage account": s.StorageAccount,
		"vault name": s.VaultName,
	} {
		if strings.TrimSpace(value) == "" {
			missing = append(missing, name)
		}
	}
	if len(missing) != 0 {
		sort.Strings(missing)
		return fmt.Errorf("Azure stack requires %s", strings.Join(missing, ", "))
	}
	if s.PreviewPoolCluster != "" {
		if s.Namespace == "" || s.NodeSubnetID == "" || s.MySQLSubnetID == "" || s.MySQLDNSZoneID == "" {
			return errors.New("Azure preview pool requires a namespace and all existing network references")
		}
		if s.ACRResourceID != "" {
			return errors.New("Azure preview pool cannot modify the shared cluster registry identity")
		}
		if s.MonitorWorkspaceID != "" {
			return errors.New("Azure preview pool monitoring must be configured by the shared cluster operator")
		}
	}
	if s.Domain != "" && (s.IngressClass == "" || len(validation.IsDNS1123Subdomain(s.Domain)) != 0) {
		return errors.New("Azure ingress domain requires a valid hostname and ingress class")
	}
	if s.TLSSecret != "" && (s.Domain == "" || s.IngressClass == "") {
		return errors.New("Azure ingress TLS requires a hostname and ingress class")
	}
	refs := []string{s.NodeSubnetID, s.MySQLSubnetID, s.MySQLDNSZoneID}
	provided := 0
	for _, ref := range refs {
		if strings.TrimSpace(ref) != "" {
			provided++
		}
	}
	if provided != 0 && provided != len(refs) {
		return errors.New("provide node subnet, MySQL subnet and MySQL private DNS zone together, or omit all three")
	}
	if provided == len(refs) && !strings.Contains(strings.ToLower(s.NodeSubnetID), "/subnets/") {
		return errors.New("node subnet must be an Azure subnet resource ID")
	}
	if s.ApplicationImage != "" {
		if strings.TrimSpace(s.MagentoVersion) == "" || strings.TrimSpace(s.SearchImage) == "" {
			return errors.New("Magento version and search image are required for an application workload")
		}
		if strings.TrimSpace(s.CacheEndpoint) == "" && strings.TrimSpace(s.CacheImage) == "" {
			return errors.New("a cache image or existing cache endpoint is required for an application workload")
		}
		if s.HighAvailability && strings.TrimSpace(s.CacheEndpoint) == "" {
			return errors.New("Azure high-availability workloads require an operator-provided HA Redis or Valkey endpoint: set services.cache ownership to external and endpoint to its hostname; cache authentication is not yet supported")
		}
	}
	if s.ACRResourceID != "" {
		parts := strings.Split(strings.Trim(s.ACRResourceID, "/"), "/")
		if len(parts) != 8 || !strings.EqualFold(parts[0], "subscriptions") || !strings.EqualFold(parts[2], "resourceGroups") || !strings.EqualFold(parts[4], "providers") || !strings.EqualFold(parts[5], "Microsoft.ContainerRegistry") || !strings.EqualFold(parts[6], "registries") || parts[1] == "" || parts[3] == "" || parts[7] == "" {
			return errors.New("acrResourceId must be an Azure Container Registry resource ID")
		}
	}
	return nil
}

// Program creates infrastructure only. The provider lifecycle must wire its
// outputs to Magento workloads and deployment operations before exposing it.
func Program(spec Spec) pulumi.RunFunc {
	return func(ctx *pulumi.Context) error {
		if err := spec.Validate(); err != nil {
			return err
		}
		// Pulumi destroys resources from the selected stack snapshot. Do not
		// reconstruct a potentially outdated application graph during cleanup.
		if spec.DestroyOnly || spec.ObserveOnly {
			return nil
		}
		provider, err := azurenative.NewProvider(ctx, "azure", &azurenative.ProviderArgs{
			SubscriptionId: pulumi.String(spec.SubscriptionID),
			TenantId:       pulumi.String(spec.TenantID),
		})
		if err != nil {
			return fmt.Errorf("configure Azure provider: %w", err)
		}
		options := []pulumi.ResourceOption{pulumi.Provider(provider)}
		tags := pulumi.StringMap{
			"magelift-project":     pulumi.String(spec.Project),
			"magelift-environment": pulumi.String(spec.Environment),
			"magelift-managed-by":  pulumi.String("magelift"),
		}
		vault, err := keyvault.NewVault(ctx, "vault", &keyvault.VaultArgs{
			ResourceGroupName: pulumi.String(spec.ResourceGroup), VaultName: pulumi.String(spec.VaultName),
			Location: pulumi.String(spec.Region), Tags: tags,
			Properties: &keyvault.VaultPropertiesArgs{TenantId: pulumi.String(spec.TenantID),
				Sku:                     &keyvault.SkuArgs{Family: pulumi.String("A"), Name: keyvault.SkuNameStandard},
				EnableRbacAuthorization: pulumi.Bool(true), AccessPolicies: keyvault.AccessPolicyEntryArray{},
			},
		}, options...)
		if err != nil {
			return fmt.Errorf("create Azure Key Vault: %w", err)
		}
		var password pulumi.StringOutput
		if spec.MySQLPassword == nil {
			generated, err := random.NewRandomPassword(ctx, "mysql-password", &random.RandomPasswordArgs{
				Length: pulumi.Int(32), Special: pulumi.Bool(true), OverrideSpecial: pulumi.String("!#$%_+-"),
				MinUpper: pulumi.Int(1), MinLower: pulumi.Int(1), MinNumeric: pulumi.Int(1), MinSpecial: pulumi.Int(1),
			})
			if err != nil {
				return fmt.Errorf("generate Azure MySQL password: %w", err)
			}
			password = generated.Result
		} else {
			password = pulumi.ToSecret(spec.MySQLPassword).(pulumi.StringOutput)
		}
		_, err = keyvault.NewSecret(ctx, "mysql-password-secret", &keyvault.SecretArgs{
			ResourceGroupName: pulumi.String(spec.ResourceGroup), VaultName: vault.Name,
			SecretName: pulumi.String("mysql-password"), Properties: &keyvault.SecretPropertiesArgs{Value: password.ToStringPtrOutput()}, Tags: tags,
		}, options...)
		if err != nil {
			return fmt.Errorf("store Azure MySQL password: %w", err)
		}
		nodeSubnet := pulumi.String(spec.NodeSubnetID).ToStringOutput()
		dbSubnet := pulumi.String(spec.MySQLSubnetID).ToStringOutput()
		dbZone := pulumi.String(spec.MySQLDNSZoneID).ToStringOutput()
		networkID := pulumi.String("").ToStringOutput()
		if offset := strings.LastIndex(strings.ToLower(spec.NodeSubnetID), "/subnets/"); offset >= 0 {
			networkID = pulumi.String(spec.NodeSubnetID[:offset]).ToStringOutput()
		}
		var dnsLink *privatedns.VirtualNetworkLink
		if strings.TrimSpace(spec.NodeSubnetID) == "" {
			vnet, err := network.NewVirtualNetwork(ctx, "network", &network.VirtualNetworkArgs{
				ResourceGroupName: pulumi.String(spec.ResourceGroup), VirtualNetworkName: pulumi.String(spec.ClusterName + "-vnet"),
				Location: pulumi.String(spec.Region), Tags: tags,
				AddressSpace: &network.AddressSpaceArgs{AddressPrefixes: pulumi.StringArray{pulumi.String("10.42.0.0/16")}},
			}, options...)
			if err != nil {
				return fmt.Errorf("create Azure network: %w", err)
			}
			nodes, err := network.NewSubnet(ctx, "node-subnet", &network.SubnetArgs{
				ResourceGroupName: pulumi.String(spec.ResourceGroup), VirtualNetworkName: vnet.Name,
				SubnetName: pulumi.String("nodes"), AddressPrefix: pulumi.String("10.42.0.0/20"),
			}, options...)
			if err != nil {
				return fmt.Errorf("create Azure node subnet: %w", err)
			}
			databaseSubnet, err := network.NewSubnet(ctx, "database-subnet", &network.SubnetArgs{
				ResourceGroupName: pulumi.String(spec.ResourceGroup), VirtualNetworkName: vnet.Name,
				SubnetName: pulumi.String("database"), AddressPrefix: pulumi.String("10.42.16.0/24"),
				Delegations: network.DelegationArray{&network.DelegationArgs{Name: pulumi.String("mysql"), ServiceName: pulumi.String("Microsoft.DBforMySQL/flexibleServers")}},
			}, append(append([]pulumi.ResourceOption{}, options...), pulumi.DependsOn([]pulumi.Resource{nodes}))...)
			if err != nil {
				return fmt.Errorf("create Azure MySQL subnet: %w", err)
			}
			zone, err := privatedns.NewPrivateZone(ctx, "database-dns", &privatedns.PrivateZoneArgs{
				ResourceGroupName: pulumi.String(spec.ResourceGroup), PrivateZoneName: pulumi.String(spec.ClusterName + ".mysql.database.azure.com"),
				Location: pulumi.String("global"), Tags: tags,
			}, options...)
			if err != nil {
				return fmt.Errorf("create Azure MySQL private DNS zone: %w", err)
			}
			dnsLink, err = privatedns.NewVirtualNetworkLink(ctx, "database-dns-link", &privatedns.VirtualNetworkLinkArgs{
				ResourceGroupName: pulumi.String(spec.ResourceGroup), PrivateZoneName: zone.Name,
				VirtualNetworkLinkName: pulumi.String("magelift"), Location: pulumi.String("global"),
				VirtualNetwork:      &privatedns.SubResourceArgs{Id: vnet.ID().ToStringOutput()},
				RegistrationEnabled: pulumi.Bool(false), Tags: tags,
			}, options...)
			if err != nil {
				return fmt.Errorf("link Azure MySQL private DNS zone: %w", err)
			}
			nodeSubnet = nodes.ID().ToStringOutput()
			dbSubnet = databaseSubnet.ID().ToStringOutput()
			dbZone = zone.ID().ToStringOutput()
			networkID = vnet.ID().ToStringOutput()
		}
		nodes := 1
		zones := pulumi.StringArray{}
		haMode := "Disabled"
		storageSKU := "Standard_LRS"
		if spec.HighAvailability {
			nodes = 3
			zones = pulumi.StringArray{pulumi.String("1"), pulumi.String("2"), pulumi.String("3")}
			haMode = "ZoneRedundant"
			storageSKU = "Standard_ZRS"
		}
		var cluster *containerservice.ManagedCluster
		if spec.PreviewPoolCluster != "" {
			id := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.ContainerService/managedClusters/%s", spec.SubscriptionID, spec.ResourceGroup, spec.PreviewPoolCluster)
			cluster, err = containerservice.GetManagedCluster(ctx, "shared-aks", pulumi.ID(id), nil, options...)
		} else {
			addons := containerservice.ManagedClusterAddonProfileMap{}
			if spec.MonitorWorkspaceID != "" {
				addons["omsagent"] = &containerservice.ManagedClusterAddonProfileArgs{Enabled: pulumi.Bool(true), Config: pulumi.StringMap{"logAnalyticsWorkspaceResourceID": pulumi.String(spec.MonitorWorkspaceID)}}
			}
			cluster, err = containerservice.NewManagedCluster(ctx, "aks", &containerservice.ManagedClusterArgs{
				ResourceGroupName: pulumi.String(spec.ResourceGroup), ResourceName: pulumi.String(spec.ClusterName),
				Location: pulumi.String(spec.Region), DnsPrefix: pulumi.String(spec.ClusterName),
				Identity:   &containerservice.ManagedClusterIdentityArgs{Type: containerservice.ResourceIdentityTypeSystemAssigned},
				EnableRBAC: pulumi.Bool(true), Tags: tags,
				AddonProfiles:     addons,
				OidcIssuerProfile: &containerservice.ManagedClusterOIDCIssuerProfileArgs{Enabled: pulumi.Bool(true)},
				AgentPoolProfiles: containerservice.ManagedClusterAgentPoolProfileArray{
					&containerservice.ManagedClusterAgentPoolProfileArgs{
						Name: pulumi.String("system"), Mode: pulumi.String("System"),
						Count: pulumi.Int(nodes), VmSize: pulumi.String("Standard_D4s_v5"),
						Type:         pulumi.String("VirtualMachineScaleSets"),
						VnetSubnetID: nodeSubnet, AvailabilityZones: zones,
					},
				},
			}, options...)
		}
		if err != nil {
			return fmt.Errorf("create AKS cluster: %w", err)
		}
		if spec.ACRResourceID != "" {
			parts := strings.Split(strings.Trim(spec.ACRResourceID, "/"), "/")
			roleID := "/subscriptions/" + parts[1] + "/providers/Microsoft.Authorization/roleDefinitions/7f951dda-4ed3-4680-a7ca-43fe172d538d"
			id := sha256.Sum256([]byte(strings.ToLower(spec.ACRResourceID) + "/" + spec.SubscriptionID + "/" + spec.ResourceGroup + "/" + spec.ClusterName))
			assignment := fmt.Sprintf("%x-%x-%x-%x-%x", id[0:4], id[4:6], id[6:8], id[8:10], id[10:16])
			_, err = authorization.NewRoleAssignment(ctx, "acr-pull", &authorization.RoleAssignmentArgs{
				Scope: pulumi.String(spec.ACRResourceID), RoleAssignmentName: pulumi.String(assignment),
				PrincipalId:   cluster.IdentityProfile.MapIndex(pulumi.String("kubeletidentity")).ObjectId().Elem(),
				PrincipalType: pulumi.String("ServicePrincipal"), RoleDefinitionId: pulumi.String(roleID),
			}, options...)
			if err != nil {
				return fmt.Errorf("grant AKS image pull access: %w", err)
			}
		}
		databaseOptions := options
		if dnsLink != nil {
			databaseOptions = append(append([]pulumi.ResourceOption{}, options...), pulumi.DependsOn([]pulumi.Resource{dnsLink}))
		}
		database, err := dbformysql.NewServer(ctx, "mysql", &dbformysql.ServerArgs{
			ResourceGroupName: pulumi.String(spec.ResourceGroup), ServerName: pulumi.String(spec.MySQLServerName),
			Location: pulumi.String(spec.Region), Version: pulumi.String(spec.MySQLVersion),
			AdministratorLogin: pulumi.String(spec.MySQLUsername), AdministratorLoginPassword: password.ToStringPtrOutput(),
			Sku:              &dbformysql.MySQLServerSkuArgs{Name: pulumi.String("Standard_D2ds_v4"), Tier: pulumi.String("GeneralPurpose")},
			Network:          &dbformysql.NetworkArgs{DelegatedSubnetResourceId: dbSubnet, PrivateDnsZoneResourceId: dbZone, PublicNetworkAccess: pulumi.String("Disabled")},
			HighAvailability: &dbformysql.HighAvailabilityArgs{Mode: pulumi.String(haMode)}, Tags: tags,
		}, databaseOptions...)
		if err != nil {
			return fmt.Errorf("create Azure MySQL server: %w", err)
		}
		_, err = dbformysql.NewDatabase(ctx, "magento", &dbformysql.DatabaseArgs{
			ResourceGroupName: pulumi.String(spec.ResourceGroup), ServerName: database.Name,
			DatabaseName: pulumi.String("magento"), Charset: pulumi.String("utf8mb4"), Collation: pulumi.String("utf8mb4_general_ci"),
		}, options...)
		if err != nil {
			return fmt.Errorf("create Magento database: %w", err)
		}
		account, err := storage.NewStorageAccount(ctx, "media", &storage.StorageAccountArgs{
			ResourceGroupName: pulumi.String(spec.ResourceGroup), AccountName: pulumi.String(spec.StorageAccount),
			Location: pulumi.String(spec.Region), Kind: pulumi.String("StorageV2"),
			Sku: &storage.SkuArgs{Name: pulumi.String(storageSKU)}, AllowBlobPublicAccess: pulumi.Bool(false),
			EnableHttpsTrafficOnly: pulumi.Bool(true), MinimumTlsVersion: pulumi.String("TLS1_2"), Tags: tags,
		}, options...)
		if err != nil {
			return fmt.Errorf("create Azure media storage account: %w", err)
		}
		share, err := storage.NewFileShare(ctx, "media-share", &storage.FileShareArgs{
			ResourceGroupName: pulumi.String(spec.ResourceGroup), AccountName: account.Name,
			ShareName: pulumi.String("media"), ShareQuota: pulumi.Int(100),
		}, options...)
		if err != nil {
			return fmt.Errorf("create Azure media share: %w", err)
		}
		if spec.ApplicationImage != "" {
			if err := addMagentoWorkloads(ctx, spec, provider, cluster, database, account, share, vault, password); err != nil {
				return fmt.Errorf("create AKS Magento workloads: %w", err)
			}
		}
		ctx.Export("clusterID", cluster.ID())
		ctx.Export("databaseHost", database.FullyQualifiedDomainName)
		ctx.Export("databaseWriter", database.FullyQualifiedDomainName)
		ctx.Export("networkVpcId", networkID)
		ctx.Export("privateSubnetIds", pulumi.StringArray{nodeSubnet, dbSubnet})
		ctx.Export("mediaAccount", account.Name)
		ctx.Export("mediaShare", share.Name)
		ctx.Export("vaultURI", vault.Properties.VaultUri())
		if spec.MonitorWorkspaceID != "" {
			ctx.Export("logAnalyticsWorkspaceId", pulumi.String(spec.MonitorWorkspaceID))
		}
		return nil
	}
}
