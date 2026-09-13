package eks

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/pulumi/pulumi-aws/sdk/v7/go/aws/autoscaling"
	"github.com/pulumi/pulumi-aws/sdk/v7/go/aws/ec2"
	"github.com/pulumi/pulumi-aws/sdk/v7/go/aws/eks"
	"github.com/pulumi/pulumi-aws/sdk/v7/go/aws/iam"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

func newManagedNodeGroup(ctx *pulumi.Context, name string, cluster *eks.Cluster, nodeRole *iam.Role, dependencies []pulumi.Resource, args Args, parent pulumi.Resource) (*eks.NodeGroup, error) {
	if nodeRole == nil {
		return nil, fmt.Errorf("managed node groups require a node IAM role")
	}
	dependsOn := append([]pulumi.Resource{cluster}, dependencies...)
	return eks.NewNodeGroup(ctx, name+"-managed-nodes", &eks.NodeGroupArgs{
		ClusterName:   cluster.Name,
		NodeGroupName: pulumi.String(name + "-managed-nodes"),
		NodeRoleArn:   nodeRole.Arn,
		InstanceTypes: pulumi.StringArray{pulumi.String(args.NodeInstanceType)},
		CapacityType:  pulumi.String("ON_DEMAND"),
		ScalingConfig: &eks.NodeGroupScalingConfigArgs{
			MinSize:     pulumi.Int(args.NodeMinSize),
			DesiredSize: pulumi.Int(args.NodeDesiredSize),
			MaxSize:     pulumi.Int(args.NodeMaxSize),
		},
		SubnetIds: args.PrivateSubnetIDs,
		Tags:      pulumi.ToStringMap(args.Tags),
	}, pulumi.Parent(parent), pulumi.DependsOn(dependsOn))
}

func newSelfManagedNodes(ctx *pulumi.Context, name string, cluster *eks.Cluster, nodeRole *iam.Role, dependencies []pulumi.Resource, args Args, parent pulumi.Resource) ([]pulumi.Resource, error) {
	if nodeRole == nil {
		return nil, fmt.Errorf("self-managed nodes require a node IAM role")
	}
	dependsOn := append([]pulumi.Resource{cluster}, dependencies...)
	profile, err := iam.NewInstanceProfile(ctx, name+"-self-managed-node-profile", &iam.InstanceProfileArgs{
		Name: pulumi.String(name + "-self-managed-node-profile"), Role: nodeRole.Name, Tags: pulumi.ToStringMap(args.Tags),
	}, pulumi.Parent(parent), pulumi.DependsOn(dependsOn))
	if err != nil {
		return nil, fmt.Errorf("create self-managed node instance profile: %w", err)
	}

	launchTemplate, err := ec2.NewLaunchTemplate(ctx, name+"-self-managed-node-template", &ec2.LaunchTemplateArgs{
		ImageId: pulumi.String(args.NodeAMI), InstanceType: pulumi.String(args.NodeInstanceType),
		IamInstanceProfile:  &ec2.LaunchTemplateIamInstanceProfileArgs{Name: profile.Name},
		VpcSecurityGroupIds: pulumi.StringArray{stringPtrOutput(cluster.VpcConfig.ClusterSecurityGroupId())},
		UserData: eksNodeUserData(
			cluster.Name,
			cluster.Endpoint,
			stringPtrOutput(cluster.CertificateAuthority.Data()),
			stringPtrOutput(cluster.KubernetesNetworkConfig.ServiceIpv4Cidr()),
		),
		MetadataOptions: &ec2.LaunchTemplateMetadataOptionsArgs{
			HttpEndpoint: pulumi.String("enabled"), HttpTokens: pulumi.String("required"),
		},
		TagSpecifications: ec2.LaunchTemplateTagSpecificationArray{
			&ec2.LaunchTemplateTagSpecificationArgs{
				ResourceType: pulumi.String("instance"), Tags: pulumi.ToStringMap(map[string]string{
					"kubernetes.io/cluster/" + name: "owned",
				}),
			},
		},
	}, pulumi.Parent(parent), pulumi.DependsOn(append(dependsOn, profile)))
	if err != nil {
		return nil, fmt.Errorf("create self-managed node launch template: %w", err)
	}

	group, err := autoscaling.NewGroup(ctx, name+"-self-managed-nodes", &autoscaling.GroupArgs{
		Name:               pulumi.String(name + "-self-managed-nodes"),
		VpcZoneIdentifiers: args.PrivateSubnetIDs,
		MinSize:            pulumi.Int(args.NodeMinSize),
		DesiredCapacity:    pulumi.Int(args.NodeDesiredSize),
		MaxSize:            pulumi.Int(args.NodeMaxSize),
		HealthCheckType:    pulumi.String("EC2"),
		CapacityRebalance:  pulumi.Bool(true),
		LaunchTemplate: &autoscaling.GroupLaunchTemplateArgs{
			Id: launchTemplate.ID(), Version: pulumi.String("$Latest"),
		},
		Tags: autoscaling.GroupTagArray{
			&autoscaling.GroupTagArgs{
				Key: pulumi.String("kubernetes.io/cluster/" + name), Value: pulumi.String("owned"), PropagateAtLaunch: pulumi.Bool(true),
			},
		},
	}, pulumi.Parent(parent), pulumi.DependsOn([]pulumi.Resource{launchTemplate}))
	if err != nil {
		return nil, fmt.Errorf("create self-managed node auto scaling group: %w", err)
	}

	accessEntry, err := eks.NewAccessEntry(ctx, name+"-self-managed-node-access", &eks.AccessEntryArgs{
		ClusterName: cluster.Name, PrincipalArn: nodeRole.Arn, Type: pulumi.String("EC2_LINUX"), Tags: pulumi.ToStringMap(args.Tags),
	}, pulumi.Parent(parent), pulumi.DependsOn([]pulumi.Resource{cluster, nodeRole}))
	if err != nil {
		return nil, fmt.Errorf("create self-managed node access entry: %w", err)
	}
	return []pulumi.Resource{profile, launchTemplate, group, accessEntry}, nil
}

func newFargateProfile(ctx *pulumi.Context, name string, cluster *eks.Cluster, args Args, parent pulumi.Resource) ([]pulumi.Resource, error) {
	role, err := iam.NewRole(ctx, name+"-fargate-pod-role", &iam.RoleArgs{
		Name:             pulumi.String(name + "-fargate-pod-role"),
		AssumeRolePolicy: pulumi.String(fargateTrustPolicy(args.Region, args.AccountID, name)),
		Tags:             pulumi.ToStringMap(args.Tags),
	}, pulumi.Parent(parent), pulumi.DependsOn([]pulumi.Resource{cluster}))
	if err != nil {
		return nil, fmt.Errorf("create Fargate pod execution role: %w", err)
	}
	policy, err := iam.NewRolePolicyAttachment(ctx, name+"-fargate-pod-policy", &iam.RolePolicyAttachmentArgs{
		Role: role.Name, PolicyArn: pulumi.String("arn:aws:iam::aws:policy/AmazonEKSFargatePodExecutionRolePolicy"),
	}, pulumi.Parent(parent), pulumi.DependsOn([]pulumi.Resource{role}))
	if err != nil {
		return nil, fmt.Errorf("attach Fargate pod execution policy: %w", err)
	}
	namespaces, err := fargateNamespaces(args.FargateNamespaces)
	if err != nil {
		return nil, err
	}
	selectors := make(eks.FargateProfileSelectorArray, 0, len(namespaces))
	for _, namespace := range namespaces {
		selectors = append(selectors, &eks.FargateProfileSelectorArgs{Namespace: pulumi.String(namespace)})
	}
	profile, err := eks.NewFargateProfile(ctx, name+"-fargate", &eks.FargateProfileArgs{
		ClusterName:         cluster.Name,
		FargateProfileName:  pulumi.String(name + "-fargate"),
		PodExecutionRoleArn: role.Arn,
		Selectors:           selectors,
		SubnetIds:           args.PrivateSubnetIDs,
		Tags:                pulumi.ToStringMap(args.Tags),
	}, pulumi.Parent(parent), pulumi.DependsOn([]pulumi.Resource{cluster, policy}))
	if err != nil {
		return nil, fmt.Errorf("create Fargate profile: %w", err)
	}
	return []pulumi.Resource{role, policy, profile}, nil
}

const (
	ebsCSIControllerServiceAccount = "ebs-csi-controller-sa"
	ebsCSIDriverPolicyARN          = "arn:aws:iam::aws:policy/service-role/AmazonEBSCSIDriverPolicy"
)

func newEBSAddon(ctx *pulumi.Context, name string, cluster *eks.Cluster, nodeDependencies, computeDependencies []pulumi.Resource, args Args, parent pulumi.Resource) ([]pulumi.Resource, error) {
	dependencies := append([]pulumi.Resource{cluster}, nodeDependencies...)
	dependencies = append(dependencies, computeDependencies...)
	oidcIssuer := stringPtrOutput(cluster.Identities.Index(pulumi.Int(0)).Oidcs().Index(pulumi.Int(0)).Issuer())
	oidcProvider, err := iam.NewOpenIdConnectProvider(ctx, name+"-oidc-provider", &iam.OpenIdConnectProviderArgs{
		Url: oidcIssuer,
		ClientIdLists: pulumi.StringArray{
			pulumi.String("sts.amazonaws.com"),
		},
		Tags: pulumi.ToStringMap(args.Tags),
	}, pulumi.Parent(parent), pulumi.DependsOn([]pulumi.Resource{cluster}))
	if err != nil {
		return nil, fmt.Errorf("create EKS IAM OIDC provider: %w", err)
	}

	// IAM may return the provider URL without its https:// scheme even though
	// the EKS cluster issuer always includes it. Build the condition keys from
	// the cluster issuer, while retaining the provider ARN as the federated
	// principal, so the trust-policy validator never depends on IAM's URL
	// normalization.
	trustPolicy := pulumi.All(oidcIssuer, oidcProvider.Arn).ApplyT(func(values []interface{}) (string, error) {
		issuer, _ := values[0].(string)
		providerARN, _ := values[1].(string)
		return ebsCSITrustPolicy(issuer, providerARN)
	}).(pulumi.StringOutput)
	role, err := iam.NewRole(ctx, name+"-ebs-csi-role", &iam.RoleArgs{
		AssumeRolePolicy: trustPolicy,
		Tags:             pulumi.ToStringMap(args.Tags),
	}, pulumi.Parent(parent), pulumi.DependsOn([]pulumi.Resource{oidcProvider}))
	if err != nil {
		return nil, fmt.Errorf("create EKS EBS CSI IAM role: %w", err)
	}
	policyAttachment, err := iam.NewRolePolicyAttachment(ctx, name+"-ebs-csi-policy", &iam.RolePolicyAttachmentArgs{
		Role: role.Name, PolicyArn: pulumi.String(ebsCSIDriverPolicyARN),
	}, pulumi.Parent(parent), pulumi.DependsOn([]pulumi.Resource{role}))
	if err != nil {
		return nil, fmt.Errorf("attach EKS EBS CSI IAM policy: %w", err)
	}
	dependencies = append(dependencies, oidcProvider, role, policyAttachment)
	addon, err := eks.NewAddon(ctx, name+"-ebs-csi", &eks.AddonArgs{
		AddonName:                pulumi.String("aws-ebs-csi-driver"),
		ClusterName:              cluster.Name,
		ResolveConflictsOnCreate: pulumi.String("OVERWRITE"),
		ResolveConflictsOnUpdate: pulumi.String("OVERWRITE"),
		ServiceAccountRoleArn:    role.Arn,
		Tags:                     pulumi.ToStringMap(args.Tags),
	}, pulumi.Parent(parent), pulumi.DependsOn(dependencies))
	if err != nil {
		return nil, fmt.Errorf("create aws-ebs-csi-driver add-on: %w", err)
	}
	return []pulumi.Resource{oidcProvider, role, policyAttachment, addon}, nil
}

func ebsCSITrustPolicy(issuer, providerARN string) (string, error) {
	issuer = strings.TrimSpace(issuer)
	if !strings.HasPrefix(issuer, "https://") {
		return "", fmt.Errorf("EKS OIDC issuer must use https")
	}
	issuer = strings.TrimSuffix(strings.TrimPrefix(issuer, "https://"), "/")
	if issuer == "" {
		return "", fmt.Errorf("EKS OIDC issuer is empty")
	}
	if strings.TrimSpace(providerARN) == "" {
		return "", fmt.Errorf("EKS OIDC provider ARN is empty")
	}

	policy := map[string]any{
		"Version": "2012-10-17",
		"Statement": []map[string]any{{
			"Effect": "Allow",
			"Principal": map[string]string{
				"Federated": providerARN,
			},
			"Action": "sts:AssumeRoleWithWebIdentity",
			"Condition": map[string]map[string]string{
				"StringEquals": {
					issuer + ":aud": "sts.amazonaws.com",
					issuer + ":sub": "system:serviceaccount:kube-system:" + ebsCSIControllerServiceAccount,
				},
			},
		}},
	}
	encoded, err := json.Marshal(policy)
	if err != nil {
		return "", fmt.Errorf("marshal EKS EBS CSI trust policy: %w", err)
	}
	return string(encoded), nil
}

// newCloudWatchObservabilityAddon installs the AWS-managed EKS observability
// agent for node-backed modes. AWS documents the add-on as supported for EC2
// and EKS Auto Mode; Fargate uses a separate ADOT/log-router composition and
// is intentionally handled as an explicit capability boundary by the caller.
// The first-party path uses the node role because this component does not yet
// own an OIDC provider lifecycle. The permissions are limited to the two AWS
// managed policies required by the add-on and never placed on the cluster role.
func newCloudWatchObservabilityAddon(ctx *pulumi.Context, name string, cluster *eks.Cluster, nodeRole *iam.Role, dependencies []pulumi.Resource, args Args, parent pulumi.Resource) ([]pulumi.Resource, error) {
	if nodeRole == nil {
		return nil, fmt.Errorf("CloudWatch observability requires a node IAM role")
	}
	dependsOn := append([]pulumi.Resource{cluster, nodeRole}, dependencies...)
	policies := []struct {
		suffix string
		arn    string
	}{
		{suffix: "cloudwatch", arn: "arn:aws:iam::aws:policy/CloudWatchAgentServerPolicy"},
		{suffix: "xray", arn: "arn:aws:iam::aws:policy/AWSXrayWriteOnlyAccess"},
	}
	resources := make([]pulumi.Resource, 0, len(policies)+1)
	for _, policy := range policies {
		attachment, err := iam.NewRolePolicyAttachment(ctx, name+"-node-"+policy.suffix, &iam.RolePolicyAttachmentArgs{
			Role: nodeRole.Name, PolicyArn: pulumi.String(policy.arn),
		}, pulumi.Parent(parent), pulumi.DependsOn(dependsOn))
		if err != nil {
			return nil, fmt.Errorf("attach CloudWatch %s policy: %w", policy.suffix, err)
		}
		resources = append(resources, attachment)
	}
	addon, err := eks.NewAddon(ctx, name+"-cloudwatch-observability", &eks.AddonArgs{
		AddonName:                pulumi.String("amazon-cloudwatch-observability"),
		ClusterName:              cluster.Name,
		ResolveConflictsOnCreate: pulumi.String("OVERWRITE"),
		ResolveConflictsOnUpdate: pulumi.String("OVERWRITE"),
		Tags:                     pulumi.ToStringMap(args.Tags),
	}, pulumi.Parent(parent), pulumi.DependsOn(append(dependsOn, resources...)))
	if err != nil {
		return nil, fmt.Errorf("create amazon-cloudwatch-observability add-on: %w", err)
	}
	resources = append(resources, addon)
	return resources, nil
}

func fargateNamespaces(values []string) ([]string, error) {
	seen := map[string]struct{}{}
	result := make([]string, 0, len(values)+2)
	for _, namespace := range append([]string{"default", "kube-system"}, values...) {
		namespace = strings.TrimSpace(namespace)
		if !namespacePattern.MatchString(namespace) {
			return nil, fmt.Errorf("invalid EKS Fargate namespace %q", namespace)
		}
		if _, exists := seen[namespace]; exists {
			continue
		}
		seen[namespace] = struct{}{}
		result = append(result, namespace)
	}
	return result, nil
}

func fargateTrustPolicy(region, accountID, clusterName string) string {
	return fmt.Sprintf(`{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"eks-fargate-pods.amazonaws.com"},"Action":"sts:AssumeRole","Condition":{"ArnLike":{"aws:SourceArn":"arn:aws:eks:%s:%s:fargateprofile/%s/*"}}}]}`, region, accountID, clusterName)
}

func eksNodeUserData(clusterName, endpoint, certificateAuthority, serviceCIDR pulumi.StringInput) pulumi.StringOutput {
	return pulumi.All(clusterName, endpoint, certificateAuthority, serviceCIDR).ApplyT(func(values []interface{}) string {
		name := values[0].(string)
		apiServerEndpoint := kubeServerURL(values[1].(string))
		certificateAuthorityData := values[2].(string)
		cidr := values[3].(string)
		content := fmt.Sprintf(`MIME-Version: 1.0
Content-Type: multipart/mixed; boundary="//"

--//
Content-Type: application/node.eks.aws

apiVersion: node.eks.aws/v1alpha1
kind: NodeConfig
spec:
  cluster:
    name: %s
    apiServerEndpoint: %s
    certificateAuthority: %s
    cidr: %s

--//
Content-Type: text/x-shellscript

#!/bin/bash
set -eu

# AL2023 consumes the application/node.eks.aws part above through nodeadm.
# Older EKS AMIs still need the legacy bootstrap script.
if command -v nodeadm >/dev/null 2>&1 || [ -d /etc/eks/nodeadm.d ]; then
  exit 0
fi
/etc/eks/bootstrap.sh %q --apiserver-endpoint %q --b64-cluster-ca %q
--//--
`, name, apiServerEndpoint, certificateAuthorityData, cidr, name, apiServerEndpoint, certificateAuthorityData)
		return base64.StdEncoding.EncodeToString([]byte(content))
	}).(pulumi.StringOutput)
}

func stringPtrOutput(input pulumi.StringPtrOutput) pulumi.StringOutput {
	return input.ApplyT(func(value *string) string {
		if value == nil {
			return ""
		}
		return *value
	}).(pulumi.StringOutput)
}
