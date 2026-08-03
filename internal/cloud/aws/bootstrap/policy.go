package bootstrap

import "encoding/json"

func githubTrustPolicy(providerARN, owner, repository, environment string) (string, error) {
	subject := "repo:" + owner + "/" + repository + ":environment:" + environment
	return policyDocument([]map[string]any{{
		"Effect": "Allow", "Action": "sts:AssumeRoleWithWebIdentity",
		"Principal": map[string]string{"Federated": providerARN},
		"Condition": map[string]any{
			"StringEquals": map[string]string{
				"token.actions.githubusercontent.com:aud": "sts.amazonaws.com",
				"token.actions.githubusercontent.com:sub": subject,
			},
		},
	}})
}

func statePermissionsPolicy(bucketARN, kmsARN, metadataARN string) (string, error) {
	return policyJSON([]map[string]any{
		{"Effect": "Allow", "Action": []string{"s3:GetBucketLocation", "s3:ListBucket", "s3:ListBucketVersions"}, "Resource": bucketARN},
		{"Effect": "Allow", "Action": []string{"s3:DeleteObject", "s3:GetObject", "s3:GetObjectVersion", "s3:PutObject"}, "Resource": bucketARN + "/*"},
		{"Effect": "Allow", "Action": []string{"kms:Decrypt", "kms:DescribeKey", "kms:Encrypt", "kms:GenerateDataKey"}, "Resource": kmsARN},
		{"Effect": "Allow", "Action": []string{"ssm:GetParameter", "ssm:PutParameter"}, "Resource": metadataARN},
	})
}

func buildPermissionsPolicy(partition, account, region string) (string, error) {
	secretResources := []string{
		"arn:" + partition + ":secretsmanager:" + region + ":" + account + ":secret:magelift-*",
		"arn:" + partition + ":secretsmanager:" + region + ":" + account + ":secret:magelift/*",
	}
	parameterResources := []string{
		"arn:" + partition + ":ssm:" + region + ":" + account + ":parameter/magelift/*",
	}
	return policyJSON([]map[string]any{
		{"Effect": "Allow", "Action": []string{"sts:GetCallerIdentity"}, "Resource": "*"},
		{"Effect": "Allow", "Action": []string{"secretsmanager:DescribeSecret", "secretsmanager:GetSecretValue", "secretsmanager:ListSecretVersionIds"}, "Resource": secretResources},
		{"Effect": "Allow", "Action": []string{"ssm:GetParameter", "ssm:GetParameters", "ssm:GetParametersByPath"}, "Resource": parameterResources},
		{"Effect": "Allow", "Action": []string{"kms:Decrypt"}, "Resource": "*", "Condition": map[string]any{
			"StringEquals": map[string]string{
				"kms:CallerAccount": account,
				"kms:ViaService":    "secretsmanager." + region + "." + awsServiceSuffix(partition),
			},
			"StringLike": map[string]string{
				"kms:EncryptionContext:SecretARN": "arn:" + partition + ":secretsmanager:" + region + ":" + account + ":secret:magelift*",
			},
		}},
	})
}

func awsServiceSuffix(partition string) string {
	if partition == "aws-cn" {
		return "amazonaws.com.cn"
	}
	return "amazonaws.com"
}

// ciPermissionsBoundaryPolicy is the IAM permissions-boundary document for the
// CI role. Managed policies are capped at 6144 bytes, so this boundary only
// names the service families MageLift may use. The detailed inline role policy
// from ciPermissionsPolicy still constrains effective permissions.
func ciPermissionsBoundaryPolicy() (string, error) {
	return policyJSON([]map[string]any{{
		"Effect": "Allow",
		"Action": []string{
			"sts:GetCallerIdentity",
			"acm:*", "aoss:*", "cloudfront:*", "cloudwatch:*", "ec2:*", "ecr:*", "ecs:*",
			"elasticache:*", "elasticloadbalancing:*", "es:*", "iam:*", "kms:*", "logs:*",
			"mq:*", "rds:*", "route53:*", "s3:*", "secretsmanager:*", "ssm:*", "synthetics:*", "wafv2:*",
		},
		"Resource": "*",
	}})
}

// ciPermissionsPolicy is deliberately explicit about service actions. AWS
// create and describe APIs generally require Resource "*"; IAM escalation is
// constrained separately to MageLift-owned roles and policies.
func ciPermissionsPolicy(partition, account, region, providerARN, stateBucket, kmsARN, metadataARN string) (string, error) {
	stateBucketARN := "arn:" + partition + ":s3:::" + stateBucket
	roleARN := "arn:" + partition + ":iam::" + account + ":role/magelift-*"
	policyARN := "arn:" + partition + ":iam::" + account + ":policy/magelift-*"
	mediaARN := "arn:" + partition + ":s3:::magelift-*"
	statements := []map[string]any{}
	state, err := statePermissionsPolicy(stateBucketARN, kmsARN, metadataARN)
	if err != nil {
		return "", err
	}
	var stateDocument struct {
		Statement []map[string]any `json:"Statement"`
	}
	if err := json.Unmarshal([]byte(state), &stateDocument); err != nil {
		return "", err
	}
	statements = append(statements, stateDocument.Statement...)
	// Collapse Resource:"*" allow statements into one Action list so the
	// inline document stays under 90% of the 10240-character PutRolePolicy
	// quota (QUALITY-02). Effective permissions are unchanged.
	statements = append(statements,
		map[string]any{"Effect": "Allow", "Action": []string{
			"sts:GetCallerIdentity", "ecr:GetAuthorizationToken", "ecr:BatchCheckLayerAvailability", "ecr:BatchGetImage", "ecr:DescribeImages", "ecr:GetDownloadUrlForLayer",
			"ec2:DescribeAvailabilityZones", "ec2:DescribeVpcs", "ec2:DescribeSubnets", "ec2:DescribeRouteTables", "ec2:DescribeNatGateways", "ec2:DescribeVpcEndpoints", "ec2:DescribeSecurityGroups", "ec2:DescribeAddresses", "ec2:DescribeNetworkInterfaces",
			"ec2:CreateVpc", "ec2:DeleteVpc", "ec2:ModifyVpcAttribute", "ec2:CreateSubnet", "ec2:DeleteSubnet", "ec2:ModifySubnetAttribute", "ec2:CreateRouteTable", "ec2:DeleteRouteTable", "ec2:AssociateRouteTable", "ec2:DisassociateRouteTable", "ec2:CreateRoute", "ec2:ReplaceRoute", "ec2:DeleteRoute", "ec2:CreateNatGateway", "ec2:DeleteNatGateway", "ec2:AllocateAddress", "ec2:ReleaseAddress", "ec2:CreateInternetGateway", "ec2:DeleteInternetGateway", "ec2:AttachInternetGateway", "ec2:DetachInternetGateway", "ec2:CreateVpcEndpoint", "ec2:DeleteVpcEndpoint", "ec2:CreateSecurityGroup", "ec2:DeleteSecurityGroup", "ec2:AuthorizeSecurityGroupIngress", "ec2:RevokeSecurityGroupIngress", "ec2:AuthorizeSecurityGroupEgress", "ec2:RevokeSecurityGroupEgress", "ec2:CreateTags", "ec2:DeleteTags",
			"elasticloadbalancing:DescribeLoadBalancers", "elasticloadbalancing:DescribeListeners", "elasticloadbalancing:DescribeTargetGroups", "elasticloadbalancing:DescribeTags", "elasticloadbalancing:CreateLoadBalancer", "elasticloadbalancing:DeleteLoadBalancer", "elasticloadbalancing:CreateListener", "elasticloadbalancing:DeleteListener", "elasticloadbalancing:ModifyListener", "elasticloadbalancing:CreateTargetGroup", "elasticloadbalancing:DeleteTargetGroup", "elasticloadbalancing:ModifyTargetGroup", "elasticloadbalancing:RegisterTargets", "elasticloadbalancing:DeregisterTargets", "elasticloadbalancing:AddTags", "elasticloadbalancing:RemoveTags",
			"ecs:DescribeClusters", "ecs:DescribeServices", "ecs:DescribeTaskDefinition", "ecs:DescribeTasks", "ecs:ListTagsForResource", "ecs:CreateCluster", "ecs:DeleteCluster", "ecs:RegisterTaskDefinition", "ecs:DeregisterTaskDefinition", "ecs:CreateService", "ecs:UpdateService", "ecs:DeleteService", "ecs:TagResource", "ecs:UntagResource", "ecs:UpdateClusterSettings", "ecs:ExecuteCommand", "ecs:RunTask", "ecs:StopTask",
			"rds:DescribeDBClusters", "rds:DescribeDBInstances", "rds:DescribeDBSubnetGroups", "rds:DescribeDBClusterParameters", "rds:CreateDBCluster", "rds:ModifyDBCluster", "rds:DeleteDBCluster", "rds:CreateDBInstance", "rds:ModifyDBInstance", "rds:DeleteDBInstance", "rds:CreateDBSubnetGroup", "rds:ModifyDBSubnetGroup", "rds:DeleteDBSubnetGroup", "rds:AddTagsToResource", "rds:RemoveTagsFromResource", "rds:ListTagsForResource",
			"elasticache:DescribeReplicationGroups", "elasticache:DescribeSubnetGroups", "elasticache:DescribeCacheClusters", "elasticache:CreateReplicationGroup", "elasticache:ModifyReplicationGroup", "elasticache:DeleteReplicationGroup", "elasticache:CreateCacheSubnetGroup", "elasticache:ModifyCacheSubnetGroup", "elasticache:DeleteCacheSubnetGroup", "elasticache:AddTagsToResource", "elasticache:RemoveTagsFromResource", "elasticache:ListTagsForResource",
			"es:DescribeDomain", "es:DescribeDomains", "es:ListDomainNames", "es:CreateDomain", "es:UpdateDomainConfig", "es:DeleteDomain", "es:CreateVpcEndpoint", "es:DeleteVpcEndpoint", "es:CreateCollection", "es:UpdateCollection", "es:DeleteCollection", "es:CreateSecurityPolicy", "es:UpdateSecurityPolicy", "es:DeleteSecurityPolicy", "es:CreateAccessPolicy", "es:UpdateAccessPolicy", "es:DeleteAccessPolicy", "es:TagResource", "es:UntagResource", "aoss:CreateCollection", "aoss:UpdateCollection", "aoss:DeleteCollection", "aoss:CreateSecurityPolicy", "aoss:UpdateSecurityPolicy", "aoss:DeleteSecurityPolicy", "aoss:CreateAccessPolicy", "aoss:UpdateAccessPolicy", "aoss:DeleteAccessPolicy", "aoss:CreateVpcEndpoint", "aoss:DeleteVpcEndpoint", "aoss:TagResource", "aoss:UntagResource",
			"mq:DescribeBroker", "mq:ListBrokers", "mq:CreateBroker", "mq:UpdateBroker", "mq:DeleteBroker", "mq:RebootBroker", "mq:ListTags", "mq:CreateTags", "mq:DeleteTags",
			"cloudfront:CreateDistribution", "cloudfront:GetDistribution", "cloudfront:UpdateDistribution", "cloudfront:DeleteDistribution", "cloudfront:CreateCachePolicy", "cloudfront:GetCachePolicy", "cloudfront:DeleteCachePolicy", "cloudfront:CreateOriginRequestPolicy", "cloudfront:GetOriginRequestPolicy", "cloudfront:DeleteOriginRequestPolicy", "cloudfront:ListTagsForResource", "cloudfront:TagResource", "cloudfront:UntagResource", "cloudfront:CreateInvalidation",
			"route53:ListHostedZonesByName", "route53:GetHostedZone", "route53:ListResourceRecordSets", "route53:ChangeResourceRecordSets", "route53:ListTagsForResource",
			"wafv2:GetWebACL", "wafv2:CreateWebACL", "wafv2:UpdateWebACL", "wafv2:DeleteWebACL", "wafv2:ListTagsForResource", "wafv2:TagResource", "wafv2:UntagResource",
			"acm:DescribeCertificate", "acm:ListCertificates", "acm:ListTagsForCertificate",
			"logs:CreateLogGroup", "logs:DeleteLogGroup", "logs:DescribeLogGroups", "logs:PutRetentionPolicy", "logs:DeleteRetentionPolicy", "logs:PutResourcePolicy", "logs:DeleteResourcePolicy", "logs:TagResource", "logs:UntagResource", "cloudwatch:PutMetricAlarm", "cloudwatch:DeleteAlarms", "cloudwatch:DescribeAlarms", "cloudwatch:PutDashboard", "cloudwatch:DeleteDashboards", "cloudwatch:GetDashboard", "cloudwatch:PutMetricData",
			"synthetics:CreateCanary", "synthetics:UpdateCanary", "synthetics:DeleteCanary", "synthetics:GetCanary", "synthetics:StartCanary", "synthetics:StopCanary", "synthetics:TagResource", "synthetics:UntagResource",
			"iam:ListRoles", "iam:ListPolicies", "iam:ListOpenIDConnectProviders",
		}, "Resource": "*"},
		map[string]any{"Effect": "Allow", "Action": []string{
			"s3:CreateBucket", "s3:DeleteBucket", "s3:GetBucketLocation", "s3:ListBucket", "s3:ListBucketVersions", "s3:GetBucketVersioning", "s3:PutBucketVersioning", "s3:GetBucketEncryption", "s3:PutEncryptionConfiguration", "s3:GetBucketPolicy", "s3:PutBucketPolicy", "s3:DeleteBucketPolicy", "s3:GetBucketTagging", "s3:PutBucketTagging", "s3:GetLifecycleConfiguration", "s3:PutLifecycleConfiguration", "s3:DeleteLifecycleConfiguration", "s3:GetObject", "s3:GetObjectVersion", "s3:PutObject", "s3:DeleteObject", "s3:AbortMultipartUpload",
		}, "Resource": []string{mediaARN, mediaARN + "/*"}},
		map[string]any{"Effect": "Allow", "Action": []string{"secretsmanager:CreateSecret", "secretsmanager:DescribeSecret", "secretsmanager:GetSecretValue", "secretsmanager:PutSecretValue", "secretsmanager:UpdateSecret", "secretsmanager:DeleteSecret", "secretsmanager:TagResource", "secretsmanager:ListSecretVersionIds"}, "Resource": []string{"arn:" + partition + ":secretsmanager:" + region + ":" + account + ":secret:magelift-*", "arn:" + partition + ":secretsmanager:" + region + ":" + account + ":secret:magelift/*"}},
		map[string]any{"Effect": "Allow", "Action": []string{"iam:CreateRole", "iam:DeleteRole", "iam:GetRole", "iam:ListRolePolicies", "iam:ListAttachedRolePolicies", "iam:PutRolePolicy", "iam:DeleteRolePolicy", "iam:UpdateAssumeRolePolicy", "iam:PutRolePermissionsBoundary", "iam:TagRole", "iam:UntagRole"}, "Resource": roleARN},
		map[string]any{"Effect": "Allow", "Action": []string{"iam:PassRole"}, "Resource": roleARN, "Condition": map[string]any{"StringEquals": map[string][]string{"iam:PassedToService": {"ecs-tasks.amazonaws.com", "synthetics.amazonaws.com"}}}},
		map[string]any{"Effect": "Allow", "Action": []string{"iam:CreatePolicy", "iam:DeletePolicy", "iam:GetPolicy", "iam:GetPolicyVersion", "iam:ListPolicyVersions", "iam:CreatePolicyVersion", "iam:DeletePolicyVersion", "iam:SetDefaultPolicyVersion", "iam:TagPolicy", "iam:UntagPolicy"}, "Resource": policyARN},
		map[string]any{"Effect": "Allow", "Action": []string{"iam:CreateServiceLinkedRole"}, "Resource": "*", "Condition": map[string]any{"StringEquals": map[string][]string{"iam:AWSServiceName": {"aoss.amazonaws.com", "cloudfront.amazonaws.com", "ecs.amazonaws.com", "ecs.application-autoscaling.amazonaws.com", "elasticache.amazonaws.com", "elasticloadbalancing.amazonaws.com", "es.amazonaws.com", "mq.amazonaws.com", "rds.amazonaws.com", "synthetics.amazonaws.com", "wafv2.amazonaws.com"}}}},
		map[string]any{"Effect": "Allow", "Action": []string{"kms:Decrypt", "kms:DescribeKey", "kms:Encrypt", "kms:GenerateDataKey"}, "Resource": kmsARN},
		map[string]any{"Effect": "Allow", "Action": []string{"ssm:GetParameter", "ssm:PutParameter"}, "Resource": []string{metadataARN, "arn:" + partition + ":ssm:" + region + ":" + account + ":parameter/magelift/*"}},
		map[string]any{"Effect": "Allow", "Action": []string{"iam:ListOpenIDConnectProviders", "iam:GetOpenIDConnectProvider"}, "Resource": providerARN},
	)
	return policyJSON(statements)
}

func policyJSON(statements []map[string]any) (string, error) {
	return policyDocument(statements)
}

func policyDocument(statements []map[string]any) (string, error) {
	return canonicalJSON(map[string]any{"Version": "2012-10-17", "Statement": statements})
}
